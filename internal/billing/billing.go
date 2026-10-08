// Package billing implements the prepaid wallet: per-message pricing from the
// owner's rate card, pre-send balance/plan checks, and real-time deductions
// that are reconciled against the pricing Meta reports in status webhooks.
//
// Charging model:
//   - When a template message is accepted by Meta, the expected price is
//     debited immediately (ChargeOnSend) so the balance is always current.
//   - Status webhooks carry Meta's final verdict (ApplyStatus): a failed or
//     non-billable message is refunded, and a category mismatch is adjusted.
//   - Every message is tracked in message_charges keyed by its WhatsApp
//     message ID, so each one is settled exactly once no matter how many
//     status webhooks arrive or in which order.
package billing

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Botrixai89/botrixai/internal/config"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Error codes returned by CheckCanSend and CheckLimit
const (
	CodeInsufficientBalance = "insufficient_balance"
	CodeSuspended           = "organization_suspended"
	CodePlanExpired         = "plan_expired"
	CodeMessageLimit        = "monthly_message_limit"
	CodeUserLimit           = "user_limit"
	CodeAccountLimit        = "account_limit"
)

// Error is a billing rule violation that should be shown to the client.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// AsError returns the billing error wrapped in err, if any.
func AsError(err error) (*Error, bool) {
	var be *Error
	if errors.As(err, &be) {
		return be, true
	}
	return nil, false
}

// Change describes a wallet balance movement, for real-time notifications.
type Change struct {
	OrganizationID uuid.UUID
	Balance        float64
	Currency       string
	LowBalance     bool
}

const cacheTTL = 30 * time.Second

type orgState struct {
	status        string
	planExpiresAt *time.Time
	plan          *models.Plan
	loadedAt      time.Time
}

// Service is safe for concurrent use. Both the API server and the campaign
// workers hold their own instance; caches are short-lived so admin changes
// propagate across processes within cacheTTL.
type Service struct {
	DB  *gorm.DB
	Cfg config.BillingConfig

	mu          sync.RWMutex
	rates       []models.MessageRate
	ratesLoaded time.Time
	orgs        map[uuid.UUID]*orgState
}

// New creates a billing service.
func New(db *gorm.DB, cfg config.BillingConfig) *Service {
	return &Service{DB: db, Cfg: cfg, orgs: make(map[uuid.UUID]*orgState)}
}

// Enabled reports whether wallet checks and deductions are active.
func (s *Service) Enabled() bool { return s != nil && s.Cfg.Enabled }

// InvalidateRates drops the cached rate card.
func (s *Service) InvalidateRates() {
	s.mu.Lock()
	s.ratesLoaded = time.Time{}
	s.mu.Unlock()
}

// InvalidateOrg drops the cached status/plan of an organization.
func (s *Service) InvalidateOrg(orgID uuid.UUID) {
	s.mu.Lock()
	delete(s.orgs, orgID)
	s.mu.Unlock()
}

// round4 keeps amounts at the precision of the numeric(18,4) columns.
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// NormalizeCategory maps template and webhook categories onto rate-card
// categories (e.g. "marketing_lite" -> MARKETING).
func NormalizeCategory(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	switch {
	case c == "":
		return ""
	case strings.HasPrefix(c, models.PricingCategoryMarketing):
		return models.PricingCategoryMarketing
	case strings.HasPrefix(c, models.PricingCategoryUtility):
		return models.PricingCategoryUtility
	case strings.HasPrefix(c, models.PricingCategoryAuthentication):
		return models.PricingCategoryAuthentication
	case strings.HasPrefix(c, models.PricingCategoryService):
		return models.PricingCategoryService
	}
	return c
}

// categoryLabel renders MARKETING as "Marketing" for ledger descriptions.
func categoryLabel(c string) string {
	if c == "" {
		return "Billable"
	}
	return c[:1] + strings.ToLower(c[1:])
}

func digitsOnly(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Service) loadRates() []models.MessageRate {
	s.mu.RLock()
	if time.Since(s.ratesLoaded) < cacheTTL {
		r := s.rates
		s.mu.RUnlock()
		return r
	}
	s.mu.RUnlock()

	var rates []models.MessageRate
	if err := s.DB.Find(&rates).Error; err != nil {
		return nil
	}
	s.mu.Lock()
	s.rates = rates
	s.ratesLoaded = time.Now()
	s.mu.Unlock()
	return rates
}

// PriceFor returns the price of one message of the given category sent to
// phone. Org-specific rates beat global ones, and a longer country prefix
// beats a shorter one or "*". Returns 0 when no rate matches.
func (s *Service) PriceFor(orgID uuid.UUID, phone, category string) float64 {
	category = NormalizeCategory(category)
	if s == nil || category == "" {
		return 0
	}
	digits := digitsOnly(phone)

	best := -1
	price := 0.0
	for _, r := range s.loadRates() {
		if NormalizeCategory(r.Category) != category {
			continue
		}
		score := 0
		if r.OrganizationID != nil {
			if *r.OrganizationID != orgID {
				continue
			}
			score += 1000
		}
		if r.CountryCode == "*" {
			// matches every destination with the lowest specificity
		} else if strings.HasPrefix(digits, r.CountryCode) {
			score += 1 + len(r.CountryCode)
		} else {
			continue
		}
		if score > best {
			best = score
			price = r.Price
		}
	}
	return round4(price)
}

func (s *Service) orgState(orgID uuid.UUID) (*orgState, error) {
	s.mu.RLock()
	st, ok := s.orgs[orgID]
	s.mu.RUnlock()
	if ok && time.Since(st.loadedAt) < cacheTTL {
		return st, nil
	}

	var org models.Organization
	if err := s.DB.Select("id", "status", "plan_id", "plan_expires_at").Where("id = ?", orgID).First(&org).Error; err != nil {
		return nil, err
	}
	st = &orgState{status: org.Status, planExpiresAt: org.PlanExpiresAt, loadedAt: time.Now()}
	if org.PlanID != nil {
		var plan models.Plan
		if err := s.DB.Where("id = ?", *org.PlanID).First(&plan).Error; err == nil {
			st.plan = &plan
		}
	}
	s.mu.Lock()
	s.orgs[orgID] = st
	s.mu.Unlock()
	return st, nil
}

// IsSuspended reports whether the organization has been suspended by the owner.
func (s *Service) IsSuspended(orgID uuid.UUID) bool {
	if s == nil {
		return false
	}
	st, err := s.orgState(orgID)
	return err == nil && st.status == models.OrgStatusSuspended
}

// EnsureWallet returns the organization's wallet, creating it if missing.
func (s *Service) EnsureWallet(db *gorm.DB, orgID uuid.UUID) (*models.Wallet, error) {
	if db == nil {
		db = s.DB
	}
	w := models.Wallet{
		OrganizationID:      orgID,
		Currency:            s.Cfg.Currency,
		LowBalanceThreshold: s.Cfg.LowBalanceThreshold,
	}
	if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}}, DoNothing: true}).Create(&w).Error; err != nil {
		return nil, err
	}
	var wallet models.Wallet
	if err := db.Where("organization_id = ?", orgID).First(&wallet).Error; err != nil {
		return nil, err
	}
	return &wallet, nil
}

// CheckCanSend verifies the organization may send a message of the given
// category to phone: not suspended, plan not expired, under the monthly
// message limit, and enough balance (plus credit limit) to cover the price.
func (s *Service) CheckCanSend(orgID uuid.UUID, phone, category string) error {
	if !s.Enabled() {
		return nil
	}
	st, err := s.orgState(orgID)
	if err != nil {
		return nil // never block sends on a lookup failure
	}
	if st.status == models.OrgStatusSuspended {
		return &Error{Code: CodeSuspended, Message: "Your account has been suspended. Please contact support."}
	}
	if st.planExpiresAt != nil && time.Now().After(*st.planExpiresAt) {
		return &Error{Code: CodePlanExpired, Message: "Your plan has expired. Please renew to continue sending messages."}
	}
	if st.plan != nil && st.plan.MaxMonthlyMessages > 0 {
		now := time.Now()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		var sent int64
		s.DB.Model(&models.Message{}).
			Where("organization_id = ? AND direction = ? AND created_at >= ?", orgID, models.DirectionOutgoing, monthStart).
			Count(&sent)
		if sent >= int64(st.plan.MaxMonthlyMessages) {
			return &Error{Code: CodeMessageLimit, Message: fmt.Sprintf("Monthly message limit of %d reached for your plan.", st.plan.MaxMonthlyMessages)}
		}
	}

	price := s.PriceFor(orgID, phone, category)
	if price <= 0 {
		return nil
	}
	wallet, err := s.EnsureWallet(nil, orgID)
	if err != nil {
		return nil
	}
	if wallet.Balance+wallet.CreditLimit < price {
		return &Error{Code: CodeInsufficientBalance, Message: fmt.Sprintf("Insufficient wallet balance (%.2f %s). Please recharge to send messages.", wallet.Balance, wallet.Currency)}
	}
	return nil
}

// EstimateCost returns the total price of sending a category to the given phones.
func (s *Service) EstimateCost(orgID uuid.UUID, phones []string, category string) float64 {
	total := 0.0
	for _, p := range phones {
		total += s.PriceFor(orgID, p, category)
	}
	return round4(total)
}

// CheckBalanceFor verifies the wallet can cover amount.
func (s *Service) CheckBalanceFor(orgID uuid.UUID, amount float64) error {
	if !s.Enabled() || amount <= 0 {
		return nil
	}
	wallet, err := s.EnsureWallet(nil, orgID)
	if err != nil {
		return nil
	}
	if wallet.Balance+wallet.CreditLimit < amount {
		return &Error{Code: CodeInsufficientBalance, Message: fmt.Sprintf("Insufficient wallet balance: %.2f %s needed, %.2f %s available.", amount, wallet.Currency, wallet.Balance, wallet.Currency)}
	}
	return nil
}

// CheckLimit enforces plan limits on users ("users") and WhatsApp numbers
// ("accounts") before one more is added.
func (s *Service) CheckLimit(orgID uuid.UUID, resource string) error {
	if s == nil {
		return nil
	}
	st, err := s.orgState(orgID)
	if err != nil || st.plan == nil {
		return nil
	}
	var count int64
	switch resource {
	case "users":
		if st.plan.MaxUsers <= 0 {
			return nil
		}
		s.DB.Model(&models.UserOrganization{}).Where("organization_id = ?", orgID).Count(&count)
		if count >= int64(st.plan.MaxUsers) {
			return &Error{Code: CodeUserLimit, Message: fmt.Sprintf("Your plan allows up to %d users. Please upgrade your plan.", st.plan.MaxUsers)}
		}
	case "accounts":
		if st.plan.MaxAccounts <= 0 {
			return nil
		}
		s.DB.Model(&models.WhatsAppAccount{}).Where("organization_id = ?", orgID).Count(&count)
		if count >= int64(st.plan.MaxAccounts) {
			return &Error{Code: CodeAccountLimit, Message: fmt.Sprintf("Your plan allows up to %d WhatsApp numbers. Please upgrade your plan.", st.plan.MaxAccounts)}
		}
	}
	return nil
}

// move applies a signed delta to the wallet inside tx and writes a ledger
// entry. Positive delta credits, negative debits.
func (s *Service) move(tx *gorm.DB, orgID uuid.UUID, delta float64, source, category, reference, description string, actorID *uuid.UUID) (*models.WalletTransaction, *Change, error) {
	delta = round4(delta)
	if delta == 0 {
		return nil, nil, nil
	}
	if _, err := s.EnsureWallet(tx, orgID); err != nil {
		return nil, nil, err
	}

	credited, spent := 0.0, 0.0
	switch source {
	case models.WalletSourceMessage, models.WalletSourceRefund, models.WalletSourceAdjustment:
		spent = -delta // refunds reduce total spent
	default:
		credited = delta
	}

	var w models.Wallet
	if err := tx.Raw(`UPDATE wallets SET balance = balance + ?, total_credited = total_credited + ?, total_spent = total_spent + ?, updated_at = NOW()
		WHERE organization_id = ? RETURNING *`, delta, credited, spent, orgID).Scan(&w).Error; err != nil {
		return nil, nil, err
	}

	txType := models.WalletTxCredit
	if delta < 0 {
		txType = models.WalletTxDebit
	}
	entry := &models.WalletTransaction{
		OrganizationID: orgID,
		Type:           txType,
		Amount:         math.Abs(delta),
		BalanceAfter:   w.Balance,
		Source:         source,
		Category:       category,
		Reference:      reference,
		Description:    description,
		CreatedByID:    actorID,
	}
	if err := tx.Create(entry).Error; err != nil {
		return nil, nil, err
	}
	return entry, &Change{
		OrganizationID: orgID,
		Balance:        w.Balance,
		Currency:       w.Currency,
		LowBalance:     w.LowBalanceThreshold > 0 && w.Balance < w.LowBalanceThreshold,
	}, nil
}

// Adjust credits (amount > 0) or debits (amount < 0) a wallet manually,
// e.g. a recharge or an owner correction.
func (s *Service) Adjust(orgID uuid.UUID, amount float64, source, description string, actorID *uuid.UUID) (*models.WalletTransaction, *Change, error) {
	var entry *models.WalletTransaction
	var change *Change
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		entry, change, err = s.move(tx, orgID, amount, source, "", "", description, actorID)
		return err
	})
	return entry, change, err
}

// ChargeOnSend debits the expected price of a message that Meta just
// accepted. It is a no-op if the message was already settled by a webhook.
func (s *Service) ChargeOnSend(orgID uuid.UUID, wamid, phone, category string) (*Change, error) {
	if !s.Enabled() || wamid == "" {
		return nil, nil
	}
	category = NormalizeCategory(category)
	price := s.PriceFor(orgID, phone, category)
	if price <= 0 {
		return nil, nil
	}

	var change *Change
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		charge := models.MessageCharge{
			OrganizationID: orgID,
			WAMessageID:    wamid,
			Phone:          phone,
			Category:       category,
			Amount:         price,
			State:          models.ChargeStateCharged,
		}
		res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "wa_message_id"}}, DoNothing: true}).Create(&charge)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // already settled by a status webhook
		}
		var err error
		_, change, err = s.move(tx, orgID, -price, models.WalletSourceMessage, category, wamid,
			fmt.Sprintf("%s message to +%s", categoryLabel(category), digitsOnly(phone)), nil)
		return err
	})
	return change, err
}

// StatusUpdate is the billing-relevant part of a Meta status webhook.
type StatusUpdate struct {
	WAMessageID string
	Phone       string
	Status      string // sent, delivered, read, failed
	HasPricing  bool
	Billable    bool
	Category    string
}

// ApplyStatus reconciles a message charge with a status webhook: refunds
// failed or non-billable messages, charges billable messages that were not
// charged at send time, and adjusts the amount if Meta's category differs.
func (s *Service) ApplyStatus(orgID uuid.UUID, u StatusUpdate) (*Change, error) {
	if !s.Enabled() || u.WAMessageID == "" {
		return nil, nil
	}
	failed := u.Status == "failed"
	if !failed && !u.HasPricing {
		return nil, nil
	}

	var change *Change
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// Make sure a row exists, then lock it so concurrent webhooks and the
		// send path settle this message one at a time.
		seed := models.MessageCharge{
			OrganizationID: orgID,
			WAMessageID:    u.WAMessageID,
			Phone:          u.Phone,
			Category:       NormalizeCategory(u.Category),
			State:          models.ChargeStateFree,
		}
		created := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "wa_message_id"}}, DoNothing: true}).Create(&seed)
		if created.Error != nil {
			return created.Error
		}
		isNew := created.RowsAffected == 1

		var c models.MessageCharge
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("wa_message_id = ?", u.WAMessageID).First(&c).Error; err != nil {
			return err
		}

		refund := func(reason string) error {
			if c.State == models.ChargeStateCharged && c.Amount > 0 {
				var err error
				_, change, err = s.move(tx, orgID, c.Amount, models.WalletSourceRefund, c.Category, c.WAMessageID, reason, nil)
				if err != nil {
					return err
				}
				c.State = models.ChargeStateRefunded
				c.Amount = 0
			} else if c.State != models.ChargeStateRefunded {
				c.State = models.ChargeStateFree
			}
			c.Reconciled = true
			return tx.Save(&c).Error
		}

		if failed {
			if c.State == models.ChargeStateRefunded {
				return nil
			}
			return refund("Refund: message failed")
		}
		if c.Reconciled {
			return nil
		}
		if !u.Billable {
			return refund("Refund: message not billable by Meta")
		}

		category := NormalizeCategory(u.Category)
		if category == "" {
			category = c.Category
		}
		phone := c.Phone
		if phone == "" {
			phone = u.Phone
		}
		price := s.PriceFor(orgID, phone, category)

		switch {
		case isNew || c.State == models.ChargeStateFree:
			// Not charged at send time (e.g. non-template or external send)
			if price > 0 {
				var err error
				_, change, err = s.move(tx, orgID, -price, models.WalletSourceMessage, category, c.WAMessageID,
					fmt.Sprintf("%s message to +%s", categoryLabel(category), digitsOnly(u.Phone)), nil)
				if err != nil {
					return err
				}
				c.State = models.ChargeStateCharged
				c.Amount = price
			}
		case c.State == models.ChargeStateCharged:
			if delta := round4(price - c.Amount); delta != 0 {
				var err error
				_, change, err = s.move(tx, orgID, -delta, models.WalletSourceAdjustment, category, c.WAMessageID,
					fmt.Sprintf("Price adjusted: billed as %s by Meta", strings.ToLower(category)), nil)
				if err != nil {
					return err
				}
				c.Amount = price
			}
		}
		c.Category = category
		c.Reconciled = true
		return tx.Save(&c).Error
	})
	return change, err
}
