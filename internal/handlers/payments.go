package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Botrixai89/botrixai/internal/billing"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// razorpayAPI is a variable so tests can point it at a fake server.
var razorpayAPI = "https://api.razorpay.com/v1"

func (a *App) paymentsEnabled() bool {
	p := a.Config.Payments
	return p.RazorpayKeyID != "" && p.RazorpayKeySecret != ""
}

func (a *App) rechargeLimits() (float64, float64) {
	minAmt, maxAmt := a.Config.Payments.MinRecharge, a.Config.Payments.MaxRecharge
	if minAmt <= 0 {
		minAmt = 100
	}
	if maxAmt <= 0 {
		maxAmt = 200000
	}
	return minAmt, maxAmt
}

// GetPaymentConfig tells the wallet page whether online recharge is available.
func (a *App) GetPaymentConfig(r *fastglue.Request) error {
	if _, _, err := a.requireAuth(r, models.ResourceSettingsGeneral, models.ActionRead); err != nil {
		return nil
	}
	minAmt, maxAmt := a.rechargeLimits()
	return r.SendEnvelope(map[string]any{
		"enabled":      a.paymentsEnabled() && a.Billing.Enabled(),
		"gateway":      "razorpay",
		"key_id":       a.Config.Payments.RazorpayKeyID,
		"min_recharge": minAmt,
		"max_recharge": maxAmt,
		"currency":     a.Config.Billing.Currency,
	})
}

// CreateRechargeRequest starts a wallet top-up.
type CreateRechargeRequest struct {
	Amount float64 `json:"amount"`
}

// CreateRechargeOrder creates a Razorpay order the browser checkout will pay.
func (a *App) CreateRechargeOrder(r *fastglue.Request) error {
	orgID, userID, err := a.requireAuth(r, models.ResourceSettingsGeneral, models.ActionRead)
	if err != nil {
		return nil
	}
	if !a.paymentsEnabled() || !a.Billing.Enabled() {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "Online recharge is not set up. Contact your account manager to add funds.", nil, "")
	}
	var req CreateRechargeRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	minAmt, maxAmt := a.rechargeLimits()
	amount := math.Round(req.Amount*100) / 100
	if amount < minAmt || amount > maxAmt {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, fmt.Sprintf("Enter an amount between %.0f and %.0f", minAmt, maxAmt), nil, "")
	}
	currency := strings.ToUpper(firstNonEmpty(a.Config.Billing.Currency, "INR"))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	orderID, err := a.razorpayCreateOrder(ctx, int64(math.Round(amount*100)), currency, orgID)
	if err != nil {
		a.Log.Error("Razorpay order failed", "error", err, "org_id", orgID)
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, "The payment gateway is not responding. Please try again.", nil, "")
	}
	order := models.PaymentOrder{OrganizationID: orgID, UserID: &userID, Gateway: "razorpay",
		GatewayOrderID: orderID, Amount: amount, Currency: currency, Status: models.PaymentCreated}
	if err := a.DB.Create(&order).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to start the payment", nil, "")
	}

	var user models.User
	a.DB.Select("full_name, email").First(&user, "id = ?", userID)
	var org models.Organization
	a.DB.Select("name, contact_phone").First(&org, "id = ?", orgID)
	return r.SendEnvelope(map[string]any{
		"order_id":    orderID,
		"key_id":      a.Config.Payments.RazorpayKeyID,
		"amount":      int64(math.Round(amount * 100)), // smallest currency unit, as Checkout expects
		"currency":    currency,
		"name":        firstNonEmpty(a.Config.App.Name, "BotrixAI"),
		"business":    org.Name,
		"prefill":     map[string]string{"name": user.FullName, "email": user.Email, "contact": org.ContactPhone},
		"description": "Wallet recharge",
	})
}

func (a *App) razorpayCreateOrder(ctx context.Context, paise int64, currency string, orgID uuid.UUID) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"amount":   paise,
		"currency": currency,
		"receipt":  "wallet_" + strings.ReplaceAll(orgID.String(), "-", "")[:20] + "_" + fmt.Sprint(time.Now().Unix()),
		"notes":    map[string]string{"organization_id": orgID.String(), "purpose": "wallet_recharge"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, razorpayAPI+"/orders", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(a.Config.Payments.RazorpayKeyID, a.Config.Payments.RazorpayKeySecret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("razorpay %d: %s", resp.StatusCode, truncateRunes(string(raw), 300))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ID == "" {
		return "", errors.New("razorpay returned no order id")
	}
	return out.ID, nil
}

// VerifyRechargeRequest is what Razorpay Checkout hands the browser on success.
type VerifyRechargeRequest struct {
	OrderID   string `json:"razorpay_order_id"`
	PaymentID string `json:"razorpay_payment_id"`
	Signature string `json:"razorpay_signature"`
}

// VerifyRecharge checks Checkout's signature and credits the wallet.
func (a *App) VerifyRecharge(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceSettingsGeneral, models.ActionRead)
	if err != nil {
		return nil
	}
	var req VerifyRechargeRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if !a.paymentsEnabled() {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "Online recharge is not set up", nil, "")
	}
	expected := hmacHex(a.Config.Payments.RazorpayKeySecret, req.OrderID+"|"+req.PaymentID)
	if req.OrderID == "" || req.PaymentID == "" || !hmac.Equal([]byte(expected), []byte(req.Signature)) {
		a.Log.Warn("Recharge signature mismatch", "org_id", orgID, "order_id", req.OrderID)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Payment could not be verified", nil, "")
	}
	balance, err := a.settlePayment(req.OrderID, req.PaymentID, &orgID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	return r.SendEnvelope(map[string]any{"status": models.PaymentPaid, "balance": balance})
}

// settlePayment marks an order paid and credits the wallet exactly once, no matter
// whether the browser or the webhook reports the payment first (or both do).
// expectOrg, when set, must own the order (browser path).
func (a *App) settlePayment(gatewayOrderID, paymentID string, expectOrg *uuid.UUID) (float64, error) {
	var balance float64
	var change *billing.Change
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		var order models.PaymentOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("gateway_order_id = ?", gatewayOrderID).First(&order).Error; err != nil {
			return errors.New("Unknown payment order")
		}
		if expectOrg != nil && order.OrganizationID != *expectOrg {
			return errors.New("Unknown payment order")
		}
		if order.Status == models.PaymentPaid {
			var w models.Wallet
			tx.Where("organization_id = ?", order.OrganizationID).First(&w)
			balance = w.Balance
			return nil // already credited
		}
		now := time.Now()
		if err := tx.Model(&order).Updates(map[string]any{"status": models.PaymentPaid, "payment_id": paymentID, "paid_at": now}).Error; err != nil {
			return err
		}
		_, ch, err := a.Billing.CreditInTx(tx, order.OrganizationID, order.Amount, models.WalletSourceRecharge,
			paymentID, "Online recharge (Razorpay "+paymentID+")")
		if err != nil {
			return err
		}
		if ch != nil {
			balance = ch.Balance
			change = ch
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	a.broadcastWalletChange(change) // nil-safe
	return balance, nil
}

// RazorpayWebhook credits wallets for payments whose browser never came back.
func (a *App) RazorpayWebhook(r *fastglue.Request) error {
	secret := a.Config.Payments.RazorpayWebhookSecret
	body := r.RequestCtx.PostBody()
	sig := string(r.RequestCtx.Request.Header.Peek("X-Razorpay-Signature"))
	if secret == "" || sig == "" || !hmac.Equal([]byte(hmacHex(secret, string(body))), []byte(sig)) {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Invalid signature", nil, "")
	}
	var evt struct {
		Event   string `json:"event"`
		Payload struct {
			Payment struct {
				Entity struct {
					ID      string `json:"id"`
					OrderID string `json:"order_id"`
					Status  string `json:"status"`
				} `json:"entity"`
			} `json:"payment"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &evt); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid payload", nil, "")
	}
	p := evt.Payload.Payment.Entity
	switch evt.Event {
	case "payment.captured", "order.paid":
		if p.OrderID != "" {
			if _, err := a.settlePayment(p.OrderID, p.ID, nil); err != nil {
				a.Log.Warn("Razorpay webhook for unknown order", "order_id", p.OrderID, "error", err)
			}
		}
	case "payment.failed":
		a.DB.Model(&models.PaymentOrder{}).Where("gateway_order_id = ? AND status = ?", p.OrderID, models.PaymentCreated).
			Update("status", models.PaymentFailed)
	}
	return r.SendEnvelope(map[string]string{"status": "ok"})
}

func hmacHex(secret, msg string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}
