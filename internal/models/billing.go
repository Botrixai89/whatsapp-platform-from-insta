package models

import (
	"time"

	"github.com/google/uuid"
)

// Organization status values (managed from the Owner Panel)
const (
	OrgStatusActive    = "active"
	OrgStatusSuspended = "suspended"
)

// Message pricing categories used by the rate card. They mirror the
// categories Meta reports in the status webhook's pricing object.
const (
	PricingCategoryMarketing      = "MARKETING"
	PricingCategoryUtility        = "UTILITY"
	PricingCategoryAuthentication = "AUTHENTICATION"
	PricingCategoryService        = "SERVICE"
)

// Wallet transaction types and sources
const (
	WalletTxCredit = "credit"
	WalletTxDebit  = "debit"

	WalletSourceRecharge   = "recharge"   // client top-up
	WalletSourceManual     = "manual"     // owner adjustment
	WalletSourceBonus      = "bonus"      // promotional credit
	WalletSourceMessage    = "message"    // per-message deduction
	WalletSourceRefund     = "refund"     // message failed / not billable
	WalletSourceAdjustment = "adjustment" // price reconciled with Meta's category
)

// Message charge states
const (
	ChargeStateCharged  = "charged"
	ChargeStateRefunded = "refunded"
	ChargeStateFree     = "free"
)

// Plan is a subscription plan the owner assigns to client organizations.
// Zero limits mean unlimited.
type Plan struct {
	BaseModel
	Name               string  `gorm:"size:100;not null" json:"name"`
	Description        string  `gorm:"type:text" json:"description"`
	Price              float64 `gorm:"type:numeric(18,4);default:0" json:"price"`
	BillingCycle       string  `gorm:"size:20;default:'monthly'" json:"billing_cycle"` // monthly, yearly
	MaxUsers           int     `gorm:"default:0" json:"max_users"`
	MaxAccounts        int     `gorm:"default:0" json:"max_accounts"`
	MaxMonthlyMessages int     `gorm:"default:0" json:"max_monthly_messages"`
	IsDefault          bool    `gorm:"default:false" json:"is_default"`
	IsActive           bool    `gorm:"default:true" json:"is_active"`
}

func (Plan) TableName() string {
	return "plans"
}

// Wallet holds the prepaid balance of an organization.
type Wallet struct {
	BaseModel
	OrganizationID      uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"organization_id"`
	Balance             float64   `gorm:"type:numeric(18,4);default:0;not null" json:"balance"`
	Currency            string    `gorm:"size:10;default:'INR'" json:"currency"`
	CreditLimit         float64   `gorm:"type:numeric(18,4);default:0" json:"credit_limit"` // allowed overdraft
	LowBalanceThreshold float64   `gorm:"type:numeric(18,4);default:0" json:"low_balance_threshold"`
	TotalCredited       float64   `gorm:"type:numeric(18,4);default:0" json:"total_credited"`
	TotalSpent          float64   `gorm:"type:numeric(18,4);default:0" json:"total_spent"`
}

func (Wallet) TableName() string {
	return "wallets"
}

// WalletTransaction is an immutable ledger entry for every balance movement.
type WalletTransaction struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index;not null" json:"organization_id"`
	Type           string     `gorm:"size:10;not null" json:"type"` // credit, debit
	Amount         float64    `gorm:"type:numeric(18,4);not null" json:"amount"`
	BalanceAfter   float64    `gorm:"type:numeric(18,4);not null" json:"balance_after"`
	Source         string     `gorm:"size:20;index;not null" json:"source"`
	Category       string     `gorm:"size:30" json:"category,omitempty"`
	Reference      string     `gorm:"size:255;index" json:"reference,omitempty"` // WhatsApp message ID for message charges
	Description    string     `gorm:"type:text" json:"description"`
	CreatedByID    *uuid.UUID `gorm:"type:uuid" json:"created_by_id,omitempty"`
}

func (WalletTransaction) TableName() string {
	return "wallet_transactions"
}

// MessageCharge tracks what was charged for a single WhatsApp message so
// that send-time charges can be reconciled with Meta's status webhooks
// exactly once.
type MessageCharge struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index;not null" json:"organization_id"`
	WAMessageID    string    `gorm:"column:wa_message_id;size:255;uniqueIndex;not null" json:"wa_message_id"`
	Phone          string    `gorm:"size:30" json:"phone"`
	Category       string    `gorm:"size:30" json:"category"`
	Amount         float64   `gorm:"type:numeric(18,4);default:0" json:"amount"`
	State          string    `gorm:"size:20;not null" json:"state"`
	Reconciled     bool      `gorm:"default:false" json:"reconciled"` // Meta pricing applied
}

func (MessageCharge) TableName() string {
	return "message_charges"
}

// MessageRate is one rate-card entry: the price charged per message of a
// category to a destination country. OrganizationID nil means the global
// rate; an organization-specific rate overrides it. CountryCode is the
// dialing prefix ("91", "1", ...) or "*" for all countries.
type MessageRate struct {
	BaseModel
	OrganizationID *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	CountryCode    string     `gorm:"size:10;not null" json:"country_code"`
	CountryName    string     `gorm:"size:100" json:"country_name"`
	Category       string     `gorm:"size:30;not null" json:"category"`
	Price          float64    `gorm:"type:numeric(18,4);not null" json:"price"`
}

func (MessageRate) TableName() string {
	return "message_rates"
}
