package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/Botrixai89/botrixai/pkg/whatsapp"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// providerWebhookHeader carries each provider account's shared secret on webhook
// deliveries (the provider signs nothing, so a static secret is the only proof).
const providerWebhookHeader = "X-Botrix-Webhook-Token"

// ConnectProviderRequest connects a number that is managed by a Heltar-compatible API.
type ConnectProviderRequest struct {
	Name      string `json:"name"`
	APIURL    string `json:"api_url"`
	APIKey    string `json:"api_key"`
	PublicURL string `json:"public_url"` // optional override of this server's public address
}

// ConnectProviderResponse returns the new account and how its webhook was set up.
type ConnectProviderResponse struct {
	Account           AccountResponse `json:"account"`
	WebhookURL        string          `json:"webhook_url"`
	WebhookHeader     string          `json:"webhook_header"`
	WebhookSecret     string          `json:"webhook_secret"`
	WebhookRegistered bool            `json:"webhook_registered"`
	WebhookError      string          `json:"webhook_error,omitempty"`
}

// ConnectProviderAccount validates a BotrixAI API (Heltar) key, creates the
// account from the number it belongs to, and registers our webhook with the provider.
func (a *App) ConnectProviderAccount(r *fastglue.Request) error {
	orgID, userID, err := a.requireAuth(r, models.ResourceAccounts, models.ActionWrite)
	if err != nil {
		return nil
	}
	var req ConnectProviderRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	req.Name = strings.TrimSpace(req.Name)
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.APIURL = strings.TrimRight(strings.TrimSpace(req.APIURL), "/")
	if req.APIURL == "" {
		req.APIURL = whatsapp.DefaultHeltarURL
	}
	if req.Name == "" || req.APIKey == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Name and API key are required", nil, "")
	}
	if u, err := url.Parse(req.APIURL); err != nil || u.Scheme != "https" || u.Host == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "API URL must be an https:// address", nil, "")
	}
	if err := a.Billing.CheckLimit(orgID, "accounts"); sendBillingError(r, err) {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	biz, err := a.WhatsApp.GetHeltarBusiness(ctx, req.APIURL, req.APIKey)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Could not verify the API key: "+err.Error(), nil, "")
	}

	// One number can only live in one place, or incoming messages would be ambiguous.
	var existing models.WhatsAppAccount
	if a.DB.Where("phone_id = ?", biz.PhoneNumberID).First(&existing).Error == nil {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, "This WhatsApp number is already connected", nil, "")
	}

	secret := generateVerifyToken()
	account := models.WhatsAppAccount{
		OrganizationID:     orgID,
		Name:               req.Name,
		PhoneID:            biz.PhoneNumberID,
		BusinessID:         biz.BusinessAccountID,
		AccessToken:        req.APIKey,
		WebhookVerifyToken: secret,
		APIVersion:         a.defaultAPIVersion(),
		Provider:           whatsapp.ProviderHeltar,
		ProviderURL:        req.APIURL,
		Status:             "active",
		CreatedByID:        &userID,
		UpdatedByID:        &userID,
	}
	var defaults int64
	a.DB.Model(&models.WhatsAppAccount{}).Where("organization_id = ?", orgID).Count(&defaults)
	if defaults == 0 {
		account.IsDefaultIncoming, account.IsDefaultOutgoing = true, true
	}
	if err := a.encryptAccountSecrets(&account); err != nil {
		a.Log.Error("Failed to encrypt account secrets", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to connect account", nil, "")
	}
	if err := a.DB.Create(&account).Error; err != nil {
		a.Log.Error("Failed to create provider account", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to connect account", nil, "")
	}
	a.InvalidateWhatsAppAccountCache(account.PhoneID)
	a.logAudit(orgID, userID, "account", account.ID, models.AuditActionCreated, nil, &account)

	resp := ConnectProviderResponse{
		WebhookURL:    a.providerWebhookURL(r, req.PublicURL, account.ID),
		WebhookHeader: providerWebhookHeader,
		WebhookSecret: secret,
	}
	if strings.HasPrefix(resp.WebhookURL, "https://") {
		waAccount := &whatsapp.Account{AccessToken: req.APIKey, Provider: whatsapp.ProviderHeltar, ProviderURL: req.APIURL}
		err := a.WhatsApp.RegisterHeltarWebhook(ctx, waAccount, whatsapp.HeltarWebhook{
			URL: resp.WebhookURL, IsEnabled: true, Fields: []string{"metaWebhooks"},
			Headers: map[string]string{providerWebhookHeader: secret},
		})
		if err != nil {
			resp.WebhookError = err.Error()
		} else {
			resp.WebhookRegistered = true
		}
	} else {
		resp.WebhookError = "This server has no public https:// address, so the provider cannot deliver incoming messages here. Set server.public_url (or open the app through its public domain) and register the webhook below."
	}

	a.DB.Preload("CreatedBy").Preload("UpdatedBy").First(&account, "id = ?", account.ID)
	resp.Account = accountToResponse(account)
	return r.SendEnvelope(resp)
}

// providerWebhookURL builds the address the provider should deliver to.
func (a *App) providerWebhookURL(r *fastglue.Request, override string, accountID uuid.UUID) string {
	base := strings.TrimRight(strings.TrimSpace(override), "/")
	if base == "" {
		base = strings.TrimRight(a.Config.Server.PublicURL, "/")
	}
	if base == "" {
		scheme := "http"
		if r.RequestCtx.IsTLS() || string(r.RequestCtx.Request.Header.Peek("X-Forwarded-Proto")) == "https" {
			scheme = "https"
		}
		base = scheme + "://" + string(r.RequestCtx.Host())
	}
	return base + "/api/webhook/provider/" + accountID.String()
}

// ProviderWebhook receives Meta-format webhooks forwarded by a provider for one account.
func (a *App) ProviderWebhook(r *fastglue.Request) error {
	accountID, err := uuid.Parse(r.RequestCtx.UserValue("id").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Not found", nil, "")
	}
	var account models.WhatsAppAccount
	if err := a.DB.Where("id = ? AND provider = ?", accountID, whatsapp.ProviderHeltar).First(&account).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Not found", nil, "")
	}
	got := r.RequestCtx.Request.Header.Peek(providerWebhookHeader)
	if len(got) == 0 || subtle.ConstantTimeCompare(got, []byte(account.WebhookVerifyToken)) != 1 {
		a.Log.Warn("Provider webhook with bad secret", "account_id", accountID)
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Forbidden", nil, "")
	}

	var payload WebhookPayload
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &payload); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid payload", nil, "")
	}
	// The secret proves which account sent this; it must only speak for its own number.
	if err := payloadBelongsTo(payload, account.PhoneID, account.BusinessID); err != nil {
		a.Log.Warn("Provider webhook for another number rejected", "account_id", accountID, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Forbidden", nil, "")
	}

	a.processWebhookPayload(payload)
	return r.SendEnvelope(map[string]string{"status": "ok"})
}

func payloadBelongsTo(payload WebhookPayload, phoneID, wabaID string) error {
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			pid := change.Value.Metadata.PhoneNumberID
			if pid != "" && pid != phoneID {
				return errors.New("phone number mismatch")
			}
			if pid == "" && entry.ID != "" && wabaID != "" && entry.ID != wabaID {
				return errors.New("business account mismatch")
			}
		}
	}
	return nil
}

// testProviderAccount checks a provider account's key and number.
func (a *App) testProviderAccount(r *fastglue.Request, account *models.WhatsAppAccount) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	biz, err := a.WhatsApp.GetHeltarBusiness(ctx, account.ProviderURL, account.AccessToken)
	if err != nil {
		return r.SendEnvelope(map[string]any{"success": false, "error": err.Error()})
	}
	if biz.PhoneNumberID != account.PhoneID {
		return r.SendEnvelope(map[string]any{"success": false, "error": "This API key now belongs to a different WhatsApp number"})
	}
	return r.SendEnvelope(map[string]any{
		"success":              true,
		"display_phone_number": biz.BizWhatsappNumber,
		"verified_name":        biz.Name,
		"provider":             whatsapp.ProviderHeltar,
	})
}
