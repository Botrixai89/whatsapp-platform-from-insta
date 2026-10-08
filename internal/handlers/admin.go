package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/Botrixai89/botrixai/internal/billing"
	"github.com/Botrixai89/botrixai/internal/database"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Owner Panel: platform-wide management of client organizations, plans,
// the message rate card and client wallets. Super admins only.

// requireSuperAdmin returns the caller's user ID, or sends 403.
func (a *App) requireSuperAdmin(r *fastglue.Request) (uuid.UUID, error) {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok || !a.IsSuperAdmin(userID) {
		_ = r.SendErrorEnvelope(fasthttp.StatusForbidden, "Super admin access required", nil, "")
		return uuid.Nil, errEnvelopeSent
	}
	return userID, nil
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// ============================================================================
// Dashboard
// ============================================================================

// AdminDailyPoint is one day of platform activity.
type AdminDailyPoint struct {
	Date     string  `json:"date"`
	Outgoing int64   `json:"outgoing"`
	Incoming int64   `json:"incoming"`
	Spend    float64 `json:"spend"`
	Signups  int64   `json:"signups"`
}

// AdminGetStats returns platform-wide KPIs and a daily activity series.
func (a *App) AdminGetStats(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}

	now := time.Now()
	today := dayStart(now)
	month := monthStart(now)
	days := 30
	since := today.AddDate(0, 0, -(days - 1))

	stats := map[string]any{}
	count := func(q *gorm.DB) int64 {
		var n int64
		q.Count(&n)
		return n
	}

	orgs := func() *gorm.DB { return a.DB.Model(&models.Organization{}) }
	stats["total_clients"] = count(orgs())
	stats["active_clients"] = count(orgs().Where("status = ? OR status IS NULL OR status = ''", models.OrgStatusActive))
	stats["suspended_clients"] = count(orgs().Where("status = ?", models.OrgStatusSuspended))
	stats["new_clients_this_month"] = count(orgs().Where("created_at >= ?", month))
	stats["total_users"] = count(a.DB.Model(&models.User{}).Where("is_super_admin = ?", false))
	stats["total_numbers"] = count(a.DB.Model(&models.WhatsAppAccount{}))
	stats["total_contacts"] = count(a.DB.Model(&models.Contact{}))

	msgs := func() *gorm.DB { return a.DB.Model(&models.Message{}) }
	stats["messages_today"] = count(msgs().Where("created_at >= ?", today))
	stats["messages_sent_this_month"] = count(msgs().Where("created_at >= ? AND direction = ?", month, models.DirectionOutgoing))
	stats["messages_received_this_month"] = count(msgs().Where("created_at >= ? AND direction = ?", month, models.DirectionIncoming))

	var sums struct {
		Balance float64
	}
	a.DB.Model(&models.Wallet{}).Select("COALESCE(SUM(balance),0) AS balance").Scan(&sums)
	stats["total_wallet_balance"] = sums.Balance

	var spendMonth, spendToday, rechargeMonth float64
	a.DB.Model(&models.MessageCharge{}).Select("COALESCE(SUM(amount),0)").
		Where("state = ? AND created_at >= ?", models.ChargeStateCharged, month).Scan(&spendMonth)
	a.DB.Model(&models.MessageCharge{}).Select("COALESCE(SUM(amount),0)").
		Where("state = ? AND created_at >= ?", models.ChargeStateCharged, today).Scan(&spendToday)
	a.DB.Model(&models.WalletTransaction{}).Select("COALESCE(SUM(amount),0)").
		Where("type = ? AND source IN ? AND created_at >= ?", models.WalletTxCredit,
			[]string{models.WalletSourceRecharge, models.WalletSourceManual}, month).Scan(&rechargeMonth)
	stats["spend_this_month"] = spendMonth
	stats["spend_today"] = spendToday
	stats["recharges_this_month"] = rechargeMonth
	stats["currency"] = a.Config.Billing.Currency
	stats["billing_enabled"] = a.Billing.Enabled()

	// Daily series
	series := make([]AdminDailyPoint, days)
	index := map[string]int{}
	for i := 0; i < days; i++ {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		series[i] = AdminDailyPoint{Date: d}
		index[d] = i
	}
	type dirRow struct {
		Day       time.Time
		Direction string
		N         int64
	}
	var dirRows []dirRow
	a.DB.Model(&models.Message{}).
		Select("date_trunc('day', created_at) AS day, direction, COUNT(*) AS n").
		Where("created_at >= ?", since).Group("1, 2").Scan(&dirRows)
	for _, row := range dirRows {
		if i, ok := index[row.Day.Format("2006-01-02")]; ok {
			if row.Direction == string(models.DirectionOutgoing) {
				series[i].Outgoing += row.N
			} else {
				series[i].Incoming += row.N
			}
		}
	}
	type amtRow struct {
		Day    time.Time
		Amount float64
	}
	var spendRows []amtRow
	a.DB.Model(&models.MessageCharge{}).
		Select("date_trunc('day', created_at) AS day, COALESCE(SUM(amount),0) AS amount").
		Where("state = ? AND created_at >= ?", models.ChargeStateCharged, since).Group("1").Scan(&spendRows)
	for _, row := range spendRows {
		if i, ok := index[row.Day.Format("2006-01-02")]; ok {
			series[i].Spend = row.Amount
		}
	}
	type cntRow struct {
		Day time.Time
		N   int64
	}
	var signupRows []cntRow
	a.DB.Model(&models.Organization{}).
		Select("date_trunc('day', created_at) AS day, COUNT(*) AS n").
		Where("created_at >= ?", since).Group("1").Scan(&signupRows)
	for _, row := range signupRows {
		if i, ok := index[row.Day.Format("2006-01-02")]; ok {
			series[i].Signups = row.N
		}
	}
	stats["daily"] = series

	// Top clients by spend this month
	type topRow struct {
		ID       uuid.UUID `json:"id"`
		Name     string    `json:"name"`
		Spend    float64   `json:"spend"`
		Messages int64     `json:"messages"`
	}
	var top []topRow
	a.DB.Table("message_charges AS c").
		Select("o.id, o.name, COALESCE(SUM(c.amount),0) AS spend, COUNT(*) AS messages").
		Joins("JOIN organizations o ON o.id = c.organization_id AND o.deleted_at IS NULL").
		Where("c.state = ? AND c.created_at >= ?", models.ChargeStateCharged, month).
		Group("o.id, o.name").Order("spend DESC").Limit(5).Scan(&top)
	stats["top_clients"] = top

	// Clients running low on balance
	type lowRow struct {
		ID                  uuid.UUID `json:"id"`
		Name                string    `json:"name"`
		Balance             float64   `json:"balance"`
		LowBalanceThreshold float64   `json:"low_balance_threshold"`
	}
	var low []lowRow
	a.DB.Table("wallets AS w").
		Select("o.id, o.name, w.balance, w.low_balance_threshold").
		Joins("JOIN organizations o ON o.id = w.organization_id AND o.deleted_at IS NULL").
		Where("w.low_balance_threshold > 0 AND w.balance < w.low_balance_threshold").
		Order("w.balance ASC").Limit(10).Scan(&low)
	stats["low_balance_clients"] = low

	return r.SendEnvelope(stats)
}

// ============================================================================
// Clients
// ============================================================================

// AdminClientRow is one organization in the Owner Panel client list.
type AdminClientRow struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Status         string     `json:"status"`
	ContactPhone   string     `json:"contact_phone"`
	CreatedAt      time.Time  `json:"created_at"`
	PlanID         *uuid.UUID `json:"plan_id"`
	PlanName       string     `json:"plan_name"`
	PlanExpiresAt  *time.Time `json:"plan_expires_at"`
	OwnerName      string     `json:"owner_name"`
	OwnerEmail     string     `json:"owner_email"`
	Balance        float64    `json:"balance"`
	Currency       string     `json:"currency"`
	UsersCount     int64      `json:"users_count"`
	NumbersCount   int64      `json:"numbers_count"`
	ContactsCount  int64      `json:"contacts_count"`
	MessagesMonth  int64      `json:"messages_month"`
	SpendMonth     float64    `json:"spend_month"`
	LastActivityAt *time.Time `json:"last_activity_at"`
}

const adminClientSelect = `o.id, o.name, o.slug, COALESCE(NULLIF(o.status, ''), 'active') AS status, o.contact_phone, o.created_at,
	o.plan_id, p.name AS plan_name, o.plan_expires_at,
	ow.full_name AS owner_name, ow.email AS owner_email,
	COALESCE(w.balance, 0) AS balance, COALESCE(w.currency, ?) AS currency,
	(SELECT COUNT(*) FROM user_organizations uo WHERE uo.organization_id = o.id AND uo.deleted_at IS NULL) AS users_count,
	(SELECT COUNT(*) FROM whatsapp_accounts wa WHERE wa.organization_id = o.id AND wa.deleted_at IS NULL) AS numbers_count,
	(SELECT COUNT(*) FROM contacts c WHERE c.organization_id = o.id AND c.deleted_at IS NULL) AS contacts_count,
	(SELECT COUNT(*) FROM messages m WHERE m.organization_id = o.id AND m.created_at >= ? AND m.deleted_at IS NULL) AS messages_month,
	(SELECT COALESCE(SUM(mc.amount), 0) FROM message_charges mc WHERE mc.organization_id = o.id AND mc.state = 'charged' AND mc.created_at >= ?) AS spend_month,
	(SELECT MAX(m.created_at) FROM messages m WHERE m.organization_id = o.id) AS last_activity_at`

// adminClientsQuery builds the client list query. The owner is the earliest
// non-super-admin member, preferring the org's admin role.
func (a *App) adminClientsQuery() *gorm.DB {
	month := monthStart(time.Now())
	return a.DB.Table("organizations AS o").
		Select(adminClientSelect, a.Config.Billing.Currency, month, month).
		Joins("LEFT JOIN plans p ON p.id = o.plan_id").
		Joins("LEFT JOIN wallets w ON w.organization_id = o.id").
		Joins(`LEFT JOIN LATERAL (
			SELECT u.full_name, u.email FROM user_organizations uo
			JOIN users u ON u.id = uo.user_id AND u.deleted_at IS NULL
			LEFT JOIN custom_roles cr ON cr.id = uo.role_id
			WHERE uo.organization_id = o.id AND uo.deleted_at IS NULL AND u.is_super_admin = false
			ORDER BY (cr.name = 'admin') DESC NULLS LAST, uo.created_at ASC LIMIT 1
		) ow ON true`).
		Where("o.deleted_at IS NULL")
}

// AdminListClients lists client organizations with usage and wallet info.
// Query: search, status, plan_id, sort (name|created_at|balance|messages_month|spend_month|last_activity_at), order, page, limit.
func (a *App) AdminListClients(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	args := r.RequestCtx.QueryArgs()
	pg := parsePagination(r)

	filter := func(q *gorm.DB) *gorm.DB {
		if s := strings.TrimSpace(string(args.Peek("search"))); s != "" {
			like := "%" + s + "%"
			q = q.Where(`o.name ILIKE ? OR o.contact_phone ILIKE ? OR EXISTS (
				SELECT 1 FROM user_organizations uo JOIN users u ON u.id = uo.user_id
				WHERE uo.organization_id = o.id AND uo.deleted_at IS NULL AND (u.email ILIKE ? OR u.full_name ILIKE ?))`,
				like, like, like, like)
		}
		if s := string(args.Peek("status")); s != "" {
			if s == models.OrgStatusActive {
				q = q.Where("COALESCE(NULLIF(o.status, ''), 'active') = ?", s)
			} else {
				q = q.Where("o.status = ?", s)
			}
		}
		if s := string(args.Peek("plan_id")); s != "" {
			if s == "none" {
				q = q.Where("o.plan_id IS NULL")
			} else if id, err := uuid.Parse(s); err == nil {
				q = q.Where("o.plan_id = ?", id)
			}
		}
		return q
	}

	var total int64
	filter(a.DB.Table("organizations AS o").Where("o.deleted_at IS NULL")).Count(&total)

	sortCols := map[string]string{
		"name": "o.name", "created_at": "o.created_at", "balance": "balance",
		"messages_month": "messages_month", "spend_month": "spend_month", "last_activity_at": "last_activity_at",
	}
	sortCol, ok := sortCols[string(args.Peek("sort"))]
	if !ok {
		sortCol = "o.created_at"
	}
	order := "DESC"
	if string(args.Peek("order")) == "asc" {
		order = "ASC"
	}

	var rows []AdminClientRow
	if err := pg.Apply(filter(a.adminClientsQuery()).Order(sortCol + " " + order + " NULLS LAST")).Scan(&rows).Error; err != nil {
		a.Log.Error("Failed to list clients", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list clients", nil, "")
	}
	return r.SendEnvelope(listEnvelope("clients", rows, total, pg))
}

// AdminGetClient returns everything about one client organization.
func (a *App) AdminGetClient(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "client")
	if err != nil {
		return nil
	}

	var client AdminClientRow
	if err := a.adminClientsQuery().Where("o.id = ?", id).Scan(&client).Error; err != nil || client.ID == uuid.Nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Client not found", nil, "")
	}
	var org models.Organization
	a.DB.Where("id = ?", id).First(&org)
	wallet, _ := a.Billing.EnsureWallet(nil, id)

	// Members
	var members []MemberResponse
	a.DB.Table("user_organizations").
		Joins("JOIN users ON users.id = user_organizations.user_id AND users.deleted_at IS NULL").
		Joins("LEFT JOIN custom_roles ON custom_roles.id = user_organizations.role_id").
		Where("user_organizations.organization_id = ? AND user_organizations.deleted_at IS NULL", id).
		Select(`user_organizations.id, user_organizations.user_id, user_organizations.organization_id,
			user_organizations.role_id, user_organizations.is_default, user_organizations.created_at,
			users.email, users.full_name, users.is_active, custom_roles.name AS role_name`).
		Order("user_organizations.created_at ASC").Scan(&members)

	// WhatsApp numbers
	type numberRow struct {
		ID         uuid.UUID `json:"id"`
		Name       string    `json:"name"`
		PhoneID    string    `json:"phone_id"`
		BusinessID string    `json:"business_id"`
		Status     string    `json:"status"`
		CreatedAt  time.Time `json:"created_at"`
	}
	var numbers []numberRow
	a.DB.Model(&models.WhatsAppAccount{}).Where("organization_id = ?", id).
		Select("id, name, phone_id, business_id, status, created_at").Order("created_at ASC").Scan(&numbers)

	// Daily usage, last 30 days
	days := 30
	since := dayStart(time.Now()).AddDate(0, 0, -(days - 1))
	series := make([]AdminDailyPoint, days)
	index := map[string]int{}
	for i := 0; i < days; i++ {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		series[i] = AdminDailyPoint{Date: d}
		index[d] = i
	}
	type dirRow struct {
		Day       time.Time
		Direction string
		N         int64
	}
	var dirRows []dirRow
	a.DB.Model(&models.Message{}).
		Select("date_trunc('day', created_at) AS day, direction, COUNT(*) AS n").
		Where("organization_id = ? AND created_at >= ?", id, since).Group("1, 2").Scan(&dirRows)
	for _, row := range dirRows {
		if i, ok := index[row.Day.Format("2006-01-02")]; ok {
			if row.Direction == string(models.DirectionOutgoing) {
				series[i].Outgoing += row.N
			} else {
				series[i].Incoming += row.N
			}
		}
	}
	type amtRow struct {
		Day    time.Time
		Amount float64
	}
	var spendRows []amtRow
	a.DB.Model(&models.MessageCharge{}).
		Select("date_trunc('day', created_at) AS day, COALESCE(SUM(amount),0) AS amount").
		Where("organization_id = ? AND state = ? AND created_at >= ?", id, models.ChargeStateCharged, since).
		Group("1").Scan(&spendRows)
	for _, row := range spendRows {
		if i, ok := index[row.Day.Format("2006-01-02")]; ok {
			series[i].Spend = row.Amount
		}
	}

	type catSpend struct {
		Category string  `json:"category"`
		Count    int64   `json:"count"`
		Amount   float64 `json:"amount"`
	}
	var byCategory []catSpend
	a.DB.Model(&models.MessageCharge{}).
		Select("category, COUNT(*) AS count, COALESCE(SUM(amount),0) AS amount").
		Where("organization_id = ? AND state = ? AND created_at >= ?", id, models.ChargeStateCharged, monthStart(time.Now())).
		Group("category").Order("amount DESC").Scan(&byCategory)

	var recent []models.WalletTransaction
	a.DB.Where("organization_id = ?", id).Order("created_at DESC").Limit(10).Find(&recent)

	return r.SendEnvelope(map[string]any{
		"client":              client,
		"organization":        org,
		"wallet":              wallet,
		"usage":               a.planUsage(id, &org),
		"members":             members,
		"numbers":             numbers,
		"daily":               series,
		"spend_by_category":   byCategory,
		"recent_transactions": recent,
	})
}

// AdminCreateClientRequest creates a client organization with its owner login.
type AdminCreateClientRequest struct {
	Name           string  `json:"name"`
	ContactPhone   string  `json:"contact_phone"`
	OwnerName      string  `json:"owner_name"`
	OwnerEmail     string  `json:"owner_email"`
	OwnerPassword  string  `json:"owner_password"`
	PlanID         string  `json:"plan_id"`
	PlanExpiresAt  string  `json:"plan_expires_at"` // YYYY-MM-DD
	InitialBalance float64 `json:"initial_balance"`
}

// parsePlanExpiry parses a YYYY-MM-DD date as the end of that day.
func parsePlanExpiry(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return nil, fmt.Errorf("plan_expires_at must be YYYY-MM-DD")
	}
	t = endOfDay(t)
	return &t, nil
}

// AdminCreateClient creates an organization, seeds its roles and defaults,
// creates the owner as the org admin, and credits an opening balance.
func (a *App) AdminCreateClient(r *fastglue.Request) error {
	actorID, err := a.requireSuperAdmin(r)
	if err != nil {
		return nil
	}
	var req AdminCreateClientRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	req.Name = strings.TrimSpace(req.Name)
	req.OwnerEmail = strings.ToLower(strings.TrimSpace(req.OwnerEmail))
	if req.Name == "" || req.OwnerEmail == "" || req.OwnerName == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Business name, owner name and owner email are required", nil, "")
	}
	if len(req.OwnerPassword) < 8 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Owner password must be at least 8 characters", nil, "")
	}
	if req.InitialBalance < 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Initial balance cannot be negative", nil, "")
	}
	var planID *uuid.UUID
	if req.PlanID != "" {
		id, err := uuid.Parse(req.PlanID)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid plan", nil, "")
		}
		planID = &id
	} else {
		var def models.Plan
		if err := a.DB.Where("is_default = ? AND is_active = ?", true, true).First(&def).Error; err == nil {
			planID = &def.ID
		}
	}
	expiresAt, err := parsePlanExpiry(req.PlanExpiresAt)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	var existing int64
	a.DB.Model(&models.User{}).Where("email = ?", req.OwnerEmail).Count(&existing)
	if existing > 0 {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, "A user with this email already exists", nil, "")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.OwnerPassword), bcrypt.DefaultCost)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create client", nil, "")
	}

	org := models.Organization{
		Name:          req.Name,
		Slug:          generateSlug(req.Name),
		Settings:      models.JSONB{},
		Status:        models.OrgStatusActive,
		PlanID:        planID,
		PlanExpiresAt: expiresAt,
		ContactPhone:  strings.TrimSpace(req.ContactPhone),
	}
	var owner models.User
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&org).Error; err != nil {
			return err
		}
		if err := database.SeedSystemRolesForOrg(tx, org.ID); err != nil {
			return err
		}
		if err := tx.Create(&models.ChatbotSettings{OrganizationID: org.ID, SessionTimeoutMins: 30}).Error; err != nil {
			return err
		}
		var adminRole models.CustomRole
		if err := tx.Where("organization_id = ? AND name = ? AND is_system = ?", org.ID, "admin", true).First(&adminRole).Error; err != nil {
			return err
		}
		owner = models.User{
			OrganizationID: org.ID,
			Email:          req.OwnerEmail,
			PasswordHash:   string(hash),
			FullName:       strings.TrimSpace(req.OwnerName),
			RoleID:         &adminRole.ID,
			IsActive:       true,
		}
		if err := tx.Create(&owner).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.UserOrganization{UserID: owner.ID, OrganizationID: org.ID, RoleID: &adminRole.ID, IsDefault: true}).Error; err != nil {
			return err
		}
		if err := database.SeedDefaultWidgetsForOrg(tx, org.ID, owner.ID); err != nil {
			return err
		}
		_, err := a.Billing.EnsureWallet(tx, org.ID)
		return err
	})
	if err != nil {
		a.Log.Error("Failed to create client", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create client", nil, "")
	}

	if req.InitialBalance > 0 {
		if _, _, err := a.Billing.Adjust(org.ID, req.InitialBalance, models.WalletSourceBonus, "Opening balance", &actorID); err != nil {
			a.Log.Error("Failed to credit opening balance", "error", err, "org_id", org.ID)
		}
	}

	a.logAudit(org.ID, actorID, "organization", org.ID, models.AuditActionCreated, nil,
		map[string]any{"name": org.Name, "owner_email": owner.Email})
	a.Log.Info("Owner panel: client created", "org_id", org.ID, "owner", owner.Email)

	return r.SendEnvelope(map[string]any{"id": org.ID, "name": org.Name, "owner_id": owner.ID})
}

// AdminUpdateClientRequest updates client settings. Nil fields are left unchanged;
// an empty plan_id / plan_expires_at clears the value.
type AdminUpdateClientRequest struct {
	Name                *string  `json:"name"`
	Status              *string  `json:"status"`
	SuspendedReason     *string  `json:"suspended_reason"`
	PlanID              *string  `json:"plan_id"`
	PlanExpiresAt       *string  `json:"plan_expires_at"`
	ContactPhone        *string  `json:"contact_phone"`
	Notes               *string  `json:"notes"`
	CreditLimit         *float64 `json:"credit_limit"`
	LowBalanceThreshold *float64 `json:"low_balance_threshold"`
}

// AdminUpdateClient changes a client's status, plan, contact info and wallet settings.
func (a *App) AdminUpdateClient(r *fastglue.Request) error {
	actorID, err := a.requireSuperAdmin(r)
	if err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "client")
	if err != nil {
		return nil
	}
	var org models.Organization
	if err := a.DB.Where("id = ?", id).First(&org).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Client not found", nil, "")
	}
	var req AdminUpdateClientRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	updates := map[string]any{}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Name cannot be empty", nil, "")
		}
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Status != nil {
		if *req.Status != models.OrgStatusActive && *req.Status != models.OrgStatusSuspended {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Status must be active or suspended", nil, "")
		}
		updates["status"] = *req.Status
		if *req.Status == models.OrgStatusActive {
			updates["suspended_reason"] = ""
		}
	}
	if req.SuspendedReason != nil {
		updates["suspended_reason"] = *req.SuspendedReason
	}
	if req.PlanID != nil {
		if *req.PlanID == "" {
			updates["plan_id"] = nil
		} else {
			pid, err := uuid.Parse(*req.PlanID)
			if err != nil {
				return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid plan", nil, "")
			}
			var n int64
			a.DB.Model(&models.Plan{}).Where("id = ?", pid).Count(&n)
			if n == 0 {
				return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Plan not found", nil, "")
			}
			updates["plan_id"] = pid
		}
	}
	if req.PlanExpiresAt != nil {
		t, err := parsePlanExpiry(*req.PlanExpiresAt)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
		}
		updates["plan_expires_at"] = t
	}
	if req.ContactPhone != nil {
		updates["contact_phone"] = strings.TrimSpace(*req.ContactPhone)
	}
	if req.Notes != nil {
		updates["notes"] = *req.Notes
	}

	if len(updates) > 0 {
		if err := a.DB.Model(&org).Updates(updates).Error; err != nil {
			a.Log.Error("Failed to update client", "error", err)
			return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update client", nil, "")
		}
	}

	walletUpdates := map[string]any{}
	if req.CreditLimit != nil {
		if *req.CreditLimit < 0 {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Credit limit cannot be negative", nil, "")
		}
		walletUpdates["credit_limit"] = *req.CreditLimit
	}
	if req.LowBalanceThreshold != nil {
		walletUpdates["low_balance_threshold"] = *req.LowBalanceThreshold
	}
	if len(walletUpdates) > 0 {
		if _, err := a.Billing.EnsureWallet(nil, id); err == nil {
			a.DB.Model(&models.Wallet{}).Where("organization_id = ?", id).Updates(walletUpdates)
		}
	}

	a.Billing.InvalidateOrg(id)
	a.logAudit(id, actorID, "organization", id, models.AuditActionUpdated, nil, updates)
	return r.SendEnvelope(map[string]any{"message": "Client updated"})
}

// AdminWalletAdjustRequest credits or debits a client's wallet.
type AdminWalletAdjustRequest struct {
	Type        string  `json:"type"`   // credit, debit
	Amount      float64 `json:"amount"` // always positive
	Source      string  `json:"source"` // recharge, bonus, manual
	Description string  `json:"description"`
}

// AdminAdjustWallet records a recharge, bonus or manual correction.
func (a *App) AdminAdjustWallet(r *fastglue.Request) error {
	actorID, err := a.requireSuperAdmin(r)
	if err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "client")
	if err != nil {
		return nil
	}
	var n int64
	a.DB.Model(&models.Organization{}).Where("id = ?", id).Count(&n)
	if n == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Client not found", nil, "")
	}
	var req AdminWalletAdjustRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if req.Amount <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Amount must be greater than zero", nil, "")
	}
	amount := req.Amount
	switch req.Type {
	case models.WalletTxCredit:
		if req.Source == "" {
			req.Source = models.WalletSourceRecharge
		}
	case models.WalletTxDebit:
		amount = -amount
		req.Source = models.WalletSourceManual
	default:
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Type must be credit or debit", nil, "")
	}
	if req.Source != models.WalletSourceRecharge && req.Source != models.WalletSourceBonus && req.Source != models.WalletSourceManual {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Source must be recharge, bonus or manual", nil, "")
	}
	if req.Description == "" {
		switch req.Source {
		case models.WalletSourceRecharge:
			req.Description = "Wallet recharge"
		case models.WalletSourceBonus:
			req.Description = "Bonus credit"
		default:
			req.Description = "Manual adjustment"
		}
	}

	entry, change, err := a.Billing.Adjust(id, amount, req.Source, req.Description, &actorID)
	if err != nil {
		a.Log.Error("Failed to adjust wallet", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update wallet", nil, "")
	}
	a.broadcastWalletChange(change)
	a.logAudit(id, actorID, "wallet", id, models.AuditActionUpdated, nil,
		map[string]any{"type": req.Type, "amount": req.Amount, "source": req.Source, "description": req.Description})
	return r.SendEnvelope(map[string]any{"transaction": entry, "balance": change.Balance})
}

// AdminListClientTransactions returns one client's wallet ledger.
func (a *App) AdminListClientTransactions(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "client")
	if err != nil {
		return nil
	}
	return a.sendTransactions(r, &id)
}

// AdminListTransactions returns the ledger across all clients (?organization_id= to filter).
func (a *App) AdminListTransactions(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	if s := string(r.RequestCtx.QueryArgs().Peek("organization_id")); s != "" {
		if id, err := uuid.Parse(s); err == nil {
			return a.sendTransactions(r, &id)
		}
	}
	return a.sendTransactions(r, nil)
}

// ============================================================================
// Plans
// ============================================================================

// AdminListPlans lists all plans with the number of clients on each.
func (a *App) AdminListPlans(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	type planRow struct {
		models.Plan
		ClientsCount int64 `json:"clients_count"`
	}
	var plans []planRow
	if err := a.DB.Table("plans AS p").
		Select("p.*, (SELECT COUNT(*) FROM organizations o WHERE o.plan_id = p.id AND o.deleted_at IS NULL) AS clients_count").
		Where("p.deleted_at IS NULL").Order("p.price ASC, p.name ASC").Scan(&plans).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list plans", nil, "")
	}
	return r.SendEnvelope(map[string]any{"plans": plans})
}

func validatePlan(p *models.Plan) string {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return "Plan name is required"
	}
	if p.Price < 0 || p.MaxUsers < 0 || p.MaxAccounts < 0 || p.MaxMonthlyMessages < 0 {
		return "Price and limits cannot be negative"
	}
	if p.BillingCycle == "" {
		p.BillingCycle = "monthly"
	}
	if p.BillingCycle != "monthly" && p.BillingCycle != "yearly" {
		return "Billing cycle must be monthly or yearly"
	}
	return ""
}

// savePlan writes a plan, keeping at most one default plan.
func (a *App) savePlan(p *models.Plan, isNew bool) error {
	return a.DB.Transaction(func(tx *gorm.DB) error {
		if isNew {
			if err := tx.Create(p).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&models.Plan{}).Where("id = ?", p.ID).Updates(map[string]any{
			"name": p.Name, "description": p.Description, "price": p.Price, "billing_cycle": p.BillingCycle,
			"max_users": p.MaxUsers, "max_accounts": p.MaxAccounts, "max_monthly_messages": p.MaxMonthlyMessages,
			"is_default": p.IsDefault, "is_active": p.IsActive,
		}).Error; err != nil {
			return err
		}
		if p.IsDefault {
			return tx.Model(&models.Plan{}).Where("id <> ?", p.ID).Update("is_default", false).Error
		}
		return nil
	})
}

// AdminCreatePlan creates a plan.
func (a *App) AdminCreatePlan(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	var p models.Plan
	if err := a.decodeRequest(r, &p); err != nil {
		return nil
	}
	if msg := validatePlan(&p); msg != "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
	}
	p.ID = uuid.Nil
	if err := a.savePlan(&p, true); err != nil {
		a.Log.Error("Failed to create plan", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create plan", nil, "")
	}
	return r.SendEnvelope(p)
}

// AdminUpdatePlan updates a plan; changes apply to every client on it.
func (a *App) AdminUpdatePlan(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "plan")
	if err != nil {
		return nil
	}
	var existing models.Plan
	if err := a.DB.Where("id = ?", id).First(&existing).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Plan not found", nil, "")
	}
	var p models.Plan
	if err := a.decodeRequest(r, &p); err != nil {
		return nil
	}
	if msg := validatePlan(&p); msg != "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
	}
	p.ID = id
	if err := a.savePlan(&p, false); err != nil {
		a.Log.Error("Failed to update plan", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update plan", nil, "")
	}
	a.DB.Where("id = ?", id).First(&p)
	// Clients on this plan pick up the new limits
	var orgIDs []uuid.UUID
	a.DB.Model(&models.Organization{}).Where("plan_id = ?", id).Pluck("id", &orgIDs)
	for _, oid := range orgIDs {
		a.Billing.InvalidateOrg(oid)
	}
	return r.SendEnvelope(p)
}

// AdminDeletePlan deletes a plan that no client is on.
func (a *App) AdminDeletePlan(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "plan")
	if err != nil {
		return nil
	}
	var n int64
	a.DB.Model(&models.Organization{}).Where("plan_id = ?", id).Count(&n)
	if n > 0 {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, fmt.Sprintf("%d client(s) are on this plan. Move them to another plan first.", n), nil, "")
	}
	if err := a.DB.Where("id = ?", id).Delete(&models.Plan{}).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete plan", nil, "")
	}
	return r.SendEnvelope(map[string]any{"message": "Plan deleted"})
}

// ============================================================================
// Rate card
// ============================================================================

// AdminRateRequest creates or updates a rate-card entry.
type AdminRateRequest struct {
	OrganizationID string  `json:"organization_id"` // empty = global
	CountryCode    string  `json:"country_code"`
	CountryName    string  `json:"country_name"`
	Category       string  `json:"category"`
	Price          float64 `json:"price"`
}

func (req *AdminRateRequest) toModel() (*models.MessageRate, string) {
	rate := &models.MessageRate{
		CountryCode: strings.TrimPrefix(strings.TrimSpace(req.CountryCode), "+"),
		CountryName: strings.TrimSpace(req.CountryName),
		Category:    billing.NormalizeCategory(req.Category),
		Price:       req.Price,
	}
	if req.OrganizationID != "" {
		id, err := uuid.Parse(req.OrganizationID)
		if err != nil {
			return nil, "Invalid client"
		}
		rate.OrganizationID = &id
	}
	if rate.CountryCode == "" {
		return nil, "Country code is required (use * for all countries)"
	}
	if rate.CountryCode != "*" {
		for _, c := range rate.CountryCode {
			if c < '0' || c > '9' {
				return nil, "Country code must be digits (e.g. 91) or *"
			}
		}
	}
	switch rate.Category {
	case models.PricingCategoryMarketing, models.PricingCategoryUtility,
		models.PricingCategoryAuthentication, models.PricingCategoryService:
	default:
		return nil, "Category must be MARKETING, UTILITY, AUTHENTICATION or SERVICE"
	}
	if rate.Price < 0 {
		return nil, "Price cannot be negative"
	}
	if rate.CountryName == "" && rate.CountryCode == "*" {
		rate.CountryName = "All countries"
	}
	return rate, ""
}

// AdminListRates lists rate-card entries. ?organization_id=global|<uuid> filters.
func (a *App) AdminListRates(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	q := a.DB.Table("message_rates AS mr").
		Select("mr.*, o.name AS organization_name").
		Joins("LEFT JOIN organizations o ON o.id = mr.organization_id").
		Where("mr.deleted_at IS NULL")
	switch s := string(r.RequestCtx.QueryArgs().Peek("organization_id")); s {
	case "":
	case "global":
		q = q.Where("mr.organization_id IS NULL")
	default:
		if id, err := uuid.Parse(s); err == nil {
			q = q.Where("mr.organization_id = ?", id)
		}
	}
	type rateRow struct {
		models.MessageRate
		OrganizationName string `json:"organization_name"`
	}
	var rates []rateRow
	if err := q.Order("mr.organization_id NULLS FIRST, mr.country_code, mr.category").Scan(&rates).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list rates", nil, "")
	}
	return r.SendEnvelope(map[string]any{"rates": rates, "currency": a.Config.Billing.Currency})
}

func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505"))
}

// AdminCreateRate adds a rate-card entry.
func (a *App) AdminCreateRate(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	var req AdminRateRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	rate, msg := req.toModel()
	if msg != "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
	}
	if err := a.DB.Create(rate).Error; err != nil {
		if isUniqueViolation(err) {
			return r.SendErrorEnvelope(fasthttp.StatusConflict, "A rate for this country and category already exists", nil, "")
		}
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create rate", nil, "")
	}
	a.Billing.InvalidateRates()
	return r.SendEnvelope(rate)
}

// AdminUpdateRate updates a rate-card entry.
func (a *App) AdminUpdateRate(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "rate")
	if err != nil {
		return nil
	}
	var existing models.MessageRate
	if err := a.DB.Where("id = ?", id).First(&existing).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Rate not found", nil, "")
	}
	var req AdminRateRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	rate, msg := req.toModel()
	if msg != "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
	}
	if err := a.DB.Model(&existing).Updates(map[string]any{
		"organization_id": rate.OrganizationID,
		"country_code":    rate.CountryCode,
		"country_name":    rate.CountryName,
		"category":        rate.Category,
		"price":           rate.Price,
	}).Error; err != nil {
		if isUniqueViolation(err) {
			return r.SendErrorEnvelope(fasthttp.StatusConflict, "A rate for this country and category already exists", nil, "")
		}
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update rate", nil, "")
	}
	a.Billing.InvalidateRates()
	a.DB.Where("id = ?", id).First(&existing)
	return r.SendEnvelope(existing)
}

// AdminDeleteRate removes a rate-card entry.
func (a *App) AdminDeleteRate(r *fastglue.Request) error {
	if _, err := a.requireSuperAdmin(r); err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "rate")
	if err != nil {
		return nil
	}
	// Hard delete so the (org, country, category) slot can be reused
	if err := a.DB.Unscoped().Where("id = ?", id).Delete(&models.MessageRate{}).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete rate", nil, "")
	}
	a.Billing.InvalidateRates()
	return r.SendEnvelope(map[string]any{"message": "Rate deleted"})
}
