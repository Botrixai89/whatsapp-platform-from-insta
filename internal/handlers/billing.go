package handlers

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/billing"
	"github.com/shridarpatil/whatomate/internal/middleware"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/websocket"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// TypeWalletUpdate is broadcast to an organization whenever its balance changes.
const TypeWalletUpdate = "wallet_update"

// sendBillingError writes a 402 envelope if err is a billing rule violation.
// Returns true when a response was sent.
func sendBillingError(r *fastglue.Request, err error) bool {
	be, ok := billing.AsError(err)
	if !ok {
		return false
	}
	status := fasthttp.StatusPaymentRequired
	if be.Code == billing.CodeSuspended {
		status = fasthttp.StatusForbidden
	}
	_ = r.SendErrorEnvelope(status, be.Message, map[string]string{"code": be.Code}, "billing_error")
	return true
}

// broadcastWalletChange pushes the new balance to every connected client of the org.
func (a *App) broadcastWalletChange(change *billing.Change) {
	if change == nil || a.WSHub == nil {
		return
	}
	a.WSHub.BroadcastToOrg(change.OrganizationID, websocket.WSMessage{
		Type: TypeWalletUpdate,
		Payload: map[string]any{
			"balance":     change.Balance,
			"currency":    change.Currency,
			"low_balance": change.LowBalance,
		},
	})
}

// templateCategory returns the pricing category of an outgoing message, or
// "" for free-form messages which Meta only bills outside the service window.
func templateCategory(req OutgoingMessageRequest) string {
	if req.Type == models.MessageTypeTemplate && req.Template != nil {
		return req.Template.Category
	}
	return ""
}

// chargeAfterSend debits the wallet for a message Meta has accepted.
func (a *App) chargeAfterSend(req OutgoingMessageRequest, wamid string) {
	if !a.Billing.Enabled() || req.Account == nil || req.Contact == nil {
		return
	}
	category := templateCategory(req)
	if category == "" {
		return
	}
	change, err := a.Billing.ChargeOnSend(req.Account.OrganizationID, wamid, req.Contact.PhoneNumber, category)
	if err != nil {
		a.Log.Error("Failed to charge wallet for message", "error", err, "wamid", wamid)
		return
	}
	a.broadcastWalletChange(change)
}

// applyBillingStatus reconciles a message charge with Meta's status webhook.
func (a *App) applyBillingStatus(phoneNumberID string, status WebhookStatus) {
	if !a.Billing.Enabled() {
		return
	}
	account, err := a.getWhatsAppAccountCached(phoneNumberID)
	if err != nil || account == nil {
		return
	}
	u := billing.StatusUpdate{
		WAMessageID: status.ID,
		Phone:       status.RecipientID,
		Status:      status.Status,
	}
	if status.Pricing != nil {
		u.HasPricing = true
		u.Billable = status.Pricing.Billable
		u.Category = status.Pricing.Category
	}
	change, err := a.Billing.ApplyStatus(account.OrganizationID, u)
	if err != nil {
		a.Log.Error("Failed to reconcile message charge", "error", err, "wamid", status.ID)
		return
	}
	a.broadcastWalletChange(change)
}

// OrgStatusGuard blocks API access for suspended organizations. Super admins,
// auth/profile endpoints and the wallet page stay reachable so the client can
// see why and contact the owner.
func (a *App) OrgStatusGuard(r *fastglue.Request) *fastglue.Request {
	path := string(r.RequestCtx.Path())
	if !strings.HasPrefix(path, "/api/") ||
		strings.HasPrefix(path, "/api/auth/") ||
		strings.HasPrefix(path, "/api/me") ||
		strings.HasPrefix(path, "/api/wallet") ||
		strings.HasPrefix(path, "/api/admin/") ||
		path == "/api/webhook" || path == "/api/embedded-signup/config" {
		return r
	}
	if r.RequestCtx.UserValue(middleware.ContextKeyUserID) == nil {
		return r // unauthenticated (public route); auth middleware handles the rest
	}
	if isSuper, _ := r.RequestCtx.UserValue(middleware.ContextKeyIsSuperAdmin).(bool); isSuper {
		return r
	}
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r
	}
	if a.Billing.IsSuspended(orgID) {
		_ = r.SendErrorEnvelope(fasthttp.StatusForbidden, "Your account has been suspended. Please contact support.",
			map[string]string{"code": billing.CodeSuspended}, "billing_error")
		return nil
	}
	return r
}

// ============================================================================
// Client wallet endpoints
// ============================================================================

// WalletPlanUsage summarises plan limits against current usage.
type WalletPlanUsage struct {
	Plan              *models.Plan `json:"plan"`
	PlanExpiresAt     *time.Time   `json:"plan_expires_at"`
	Users             int64        `json:"users"`
	Accounts          int64        `json:"accounts"`
	MessagesThisMonth int64        `json:"messages_this_month"`
}

func (a *App) planUsage(orgID uuid.UUID, org *models.Organization) WalletPlanUsage {
	u := WalletPlanUsage{PlanExpiresAt: org.PlanExpiresAt}
	if org.PlanID != nil {
		var plan models.Plan
		if err := a.DB.Where("id = ?", *org.PlanID).First(&plan).Error; err == nil {
			u.Plan = &plan
		}
	}
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	a.DB.Model(&models.UserOrganization{}).Where("organization_id = ?", orgID).Count(&u.Users)
	a.DB.Model(&models.WhatsAppAccount{}).Where("organization_id = ?", orgID).Count(&u.Accounts)
	a.DB.Model(&models.Message{}).
		Where("organization_id = ? AND direction = ? AND created_at >= ?", orgID, models.DirectionOutgoing, monthStart).
		Count(&u.MessagesThisMonth)
	return u
}

// GetWallet returns the current organization's wallet, plan usage and month spend.
func (a *App) GetWallet(r *fastglue.Request) error {
	orgID, _, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	wallet, err := a.Billing.EnsureWallet(nil, orgID)
	if err != nil {
		a.Log.Error("Failed to load wallet", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load wallet", nil, "")
	}
	var org models.Organization
	if err := a.DB.Where("id = ?", orgID).First(&org).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Organization not found", nil, "")
	}

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	type catSpend struct {
		Category string  `json:"category"`
		Count    int64   `json:"count"`
		Amount   float64 `json:"amount"`
	}
	var byCategory []catSpend
	a.DB.Model(&models.MessageCharge{}).
		Select("category, COUNT(*) AS count, COALESCE(SUM(amount),0) AS amount").
		Where("organization_id = ? AND state = ? AND created_at >= ?", orgID, models.ChargeStateCharged, monthStart).
		Group("category").Order("amount DESC").Scan(&byCategory)
	spentMonth := 0.0
	for _, c := range byCategory {
		spentMonth += c.Amount
	}

	return r.SendEnvelope(map[string]any{
		"enabled":           a.Billing.Enabled(),
		"wallet":            wallet,
		"low_balance":       wallet.LowBalanceThreshold > 0 && wallet.Balance < wallet.LowBalanceThreshold,
		"status":            org.Status,
		"suspended_reason":  org.SuspendedReason,
		"usage":             a.planUsage(orgID, &org),
		"spent_this_month":  spentMonth,
		"spend_by_category": byCategory,
	})
}

// ListWalletTransactions returns the current organization's ledger.
func (a *App) ListWalletTransactions(r *fastglue.Request) error {
	orgID, _, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	return a.sendTransactions(r, &orgID)
}

// sendTransactions lists ledger entries, optionally for one organization.
// Supports ?type=credit|debit and ?source= filters.
func (a *App) sendTransactions(r *fastglue.Request, orgID *uuid.UUID) error {
	pg := parsePagination(r)
	q := a.DB.Table("wallet_transactions AS t").
		Joins("LEFT JOIN organizations o ON o.id = t.organization_id")
	if orgID != nil {
		q = q.Where("t.organization_id = ?", *orgID)
	}
	if v := string(r.RequestCtx.QueryArgs().Peek("type")); v != "" {
		q = q.Where("t.type = ?", v)
	}
	if v := string(r.RequestCtx.QueryArgs().Peek("source")); v != "" {
		q = q.Where("t.source = ?", v)
	}

	var total int64
	q.Count(&total)

	type row struct {
		models.WalletTransaction
		OrganizationName string `json:"organization_name"`
	}
	var rows []row
	if err := pg.Apply(q.Select("t.*, o.name AS organization_name").Order("t.created_at DESC")).Scan(&rows).Error; err != nil {
		a.Log.Error("Failed to list wallet transactions", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list transactions", nil, "")
	}
	return r.SendEnvelope(listEnvelope("transactions", rows, total, pg))
}

// ListWalletRates returns the rates that apply to the current organization.
func (a *App) ListWalletRates(r *fastglue.Request) error {
	orgID, _, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	var rates []models.MessageRate
	a.DB.Where("organization_id IS NULL OR organization_id = ?", orgID).
		Order("country_code, category").Find(&rates)

	// Org-specific entries replace global ones for the same country/category
	effective := map[string]models.MessageRate{}
	for _, rt := range rates {
		key := rt.CountryCode + "|" + billing.NormalizeCategory(rt.Category)
		if cur, ok := effective[key]; ok && cur.OrganizationID != nil {
			continue
		}
		effective[key] = rt
	}
	out := make([]models.MessageRate, 0, len(effective))
	for _, rt := range rates {
		key := rt.CountryCode + "|" + billing.NormalizeCategory(rt.Category)
		if e, ok := effective[key]; ok && e.ID == rt.ID {
			out = append(out, rt)
		}
	}
	return r.SendEnvelope(map[string]any{"rates": out})
}
