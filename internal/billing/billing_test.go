package billing_test

import (
	"testing"
	"time"

	"github.com/Botrixai89/botrixai/internal/billing"
	"github.com/Botrixai89/botrixai/internal/config"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/Botrixai89/botrixai/test/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setup creates an org with org-specific rates (so tests sharing the DB do
// not see each other's prices) and a wallet holding balance.
func setup(t *testing.T, balance float64) (*billing.Service, *gorm.DB, uuid.UUID) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)
	svc := billing.New(db, config.BillingConfig{Enabled: true, Currency: "INR"})

	rates := []models.MessageRate{
		{OrganizationID: &org.ID, CountryCode: "*", Category: models.PricingCategoryMarketing, Price: 1.00},
		{OrganizationID: &org.ID, CountryCode: "91", Category: models.PricingCategoryMarketing, Price: 0.80},
		{OrganizationID: &org.ID, CountryCode: "91", Category: models.PricingCategoryUtility, Price: 0.15},
	}
	require.NoError(t, db.Create(&rates).Error)

	if balance != 0 {
		_, _, err := svc.Adjust(org.ID, balance, models.WalletSourceRecharge, "test", nil)
		require.NoError(t, err)
	}
	return svc, db, org.ID
}

func balanceOf(t *testing.T, svc *billing.Service, orgID uuid.UUID) float64 {
	t.Helper()
	w, err := svc.EnsureWallet(nil, orgID)
	require.NoError(t, err)
	return w.Balance
}

func wamid() string { return "wamid." + uuid.New().String() }

func TestPriceFor_PrefersLongestPrefixAndOrgRates(t *testing.T) {
	svc, _, orgID := setup(t, 0)

	assert.Equal(t, 0.80, svc.PriceFor(orgID, "+91 98765 43210", "marketing"))
	assert.Equal(t, 1.00, svc.PriceFor(orgID, "14155550100", "MARKETING"), "falls back to *")
	assert.Equal(t, 0.15, svc.PriceFor(orgID, "919876543210", "utility"))
	assert.Equal(t, 0.0, svc.PriceFor(orgID, "14155550100", "UTILITY"), "no matching rate")
	assert.Equal(t, 0.80, svc.PriceFor(orgID, "919876543210", "marketing_lite"), "normalized category")
}

func TestChargeOnSend_DebitsOnceAndWritesLedger(t *testing.T) {
	svc, db, orgID := setup(t, 10)
	id := wamid()

	change, err := svc.ChargeOnSend(orgID, id, "919876543210", "MARKETING")
	require.NoError(t, err)
	require.NotNil(t, change)
	assert.InDelta(t, 9.20, change.Balance, 0.0001)

	// A second call for the same message is a no-op
	change, err = svc.ChargeOnSend(orgID, id, "919876543210", "MARKETING")
	require.NoError(t, err)
	assert.Nil(t, change)
	assert.InDelta(t, 9.20, balanceOf(t, svc, orgID), 0.0001)

	var entries []models.WalletTransaction
	db.Where("organization_id = ? AND source = ?", orgID, models.WalletSourceMessage).Find(&entries)
	require.Len(t, entries, 1)
	assert.Equal(t, models.WalletTxDebit, entries[0].Type)
	assert.Equal(t, id, entries[0].Reference)
}

func TestApplyStatus_FailedMessageIsRefunded(t *testing.T) {
	svc, _, orgID := setup(t, 10)
	id := wamid()
	_, err := svc.ChargeOnSend(orgID, id, "919876543210", "MARKETING")
	require.NoError(t, err)

	change, err := svc.ApplyStatus(orgID, billing.StatusUpdate{WAMessageID: id, Phone: "919876543210", Status: "failed"})
	require.NoError(t, err)
	require.NotNil(t, change)
	assert.InDelta(t, 10.0, change.Balance, 0.0001)

	// Duplicate failure webhook does not refund twice
	_, err = svc.ApplyStatus(orgID, billing.StatusUpdate{WAMessageID: id, Status: "failed"})
	require.NoError(t, err)
	assert.InDelta(t, 10.0, balanceOf(t, svc, orgID), 0.0001)
}

func TestApplyStatus_NonBillableIsRefunded(t *testing.T) {
	svc, _, orgID := setup(t, 10)
	id := wamid()
	_, err := svc.ChargeOnSend(orgID, id, "919876543210", "UTILITY")
	require.NoError(t, err)
	assert.InDelta(t, 9.85, balanceOf(t, svc, orgID), 0.0001)

	// Utility template inside the customer service window: free per Meta
	_, err = svc.ApplyStatus(orgID, billing.StatusUpdate{
		WAMessageID: id, Phone: "919876543210", Status: "sent",
		HasPricing: true, Billable: false, Category: "utility",
	})
	require.NoError(t, err)
	assert.InDelta(t, 10.0, balanceOf(t, svc, orgID), 0.0001)
}

func TestApplyStatus_AdjustsWhenMetaCategoryDiffers(t *testing.T) {
	svc, _, orgID := setup(t, 10)
	id := wamid()
	_, err := svc.ChargeOnSend(orgID, id, "919876543210", "UTILITY") // 0.15
	require.NoError(t, err)

	// Meta billed it as marketing (0.80): charge the 0.65 difference once
	for _, st := range []string{"sent", "delivered", "read"} {
		_, err = svc.ApplyStatus(orgID, billing.StatusUpdate{
			WAMessageID: id, Phone: "919876543210", Status: st,
			HasPricing: true, Billable: true, Category: "marketing",
		})
		require.NoError(t, err)
	}
	assert.InDelta(t, 9.20, balanceOf(t, svc, orgID), 0.0001)
}

func TestApplyStatus_ChargesBillableMessageNotChargedAtSend(t *testing.T) {
	svc, _, orgID := setup(t, 10)
	id := wamid()

	// Webhook arrives before (or without) a send-time charge
	_, err := svc.ApplyStatus(orgID, billing.StatusUpdate{
		WAMessageID: id, Phone: "919876543210", Status: "sent",
		HasPricing: true, Billable: true, Category: "marketing",
	})
	require.NoError(t, err)
	assert.InDelta(t, 9.20, balanceOf(t, svc, orgID), 0.0001)

	// The late send-time charge must not double charge
	_, err = svc.ChargeOnSend(orgID, id, "919876543210", "MARKETING")
	require.NoError(t, err)
	assert.InDelta(t, 9.20, balanceOf(t, svc, orgID), 0.0001)
}

func TestCheckCanSend_Rules(t *testing.T) {
	svc, db, orgID := setup(t, 0.50)

	// 0.50 balance cannot cover a 0.80 marketing message...
	err := svc.CheckCanSend(orgID, "919876543210", "MARKETING")
	be, ok := billing.AsError(err)
	require.True(t, ok)
	assert.Equal(t, billing.CodeInsufficientBalance, be.Code)

	// ...but free-form messages are still allowed
	assert.NoError(t, svc.CheckCanSend(orgID, "919876543210", ""))

	// A credit limit allows overdraft
	require.NoError(t, db.Model(&models.Wallet{}).Where("organization_id = ?", orgID).Update("credit_limit", 1).Error)
	assert.NoError(t, svc.CheckCanSend(orgID, "919876543210", "MARKETING"))

	// Suspension blocks everything
	require.NoError(t, db.Model(&models.Organization{}).Where("id = ?", orgID).Update("status", models.OrgStatusSuspended).Error)
	svc.InvalidateOrg(orgID)
	be, _ = billing.AsError(svc.CheckCanSend(orgID, "919876543210", ""))
	require.NotNil(t, be)
	assert.Equal(t, billing.CodeSuspended, be.Code)

	// Expired plan blocks sending
	past := time.Now().Add(-time.Hour)
	require.NoError(t, db.Model(&models.Organization{}).Where("id = ?", orgID).
		Updates(map[string]any{"status": models.OrgStatusActive, "plan_expires_at": past}).Error)
	svc.InvalidateOrg(orgID)
	be, _ = billing.AsError(svc.CheckCanSend(orgID, "919876543210", ""))
	require.NotNil(t, be)
	assert.Equal(t, billing.CodePlanExpired, be.Code)
}

func TestCheckLimit_Users(t *testing.T) {
	svc, db, orgID := setup(t, 0)
	plan := models.Plan{Name: "Starter", MaxUsers: 1, IsActive: true}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Model(&models.Organization{}).Where("id = ?", orgID).Update("plan_id", plan.ID).Error)
	svc.InvalidateOrg(orgID)

	assert.NoError(t, svc.CheckLimit(orgID, "users"))
	testutil.CreateTestUser(t, db, orgID) // also creates the org membership

	be, ok := billing.AsError(svc.CheckLimit(orgID, "users"))
	require.True(t, ok)
	assert.Equal(t, billing.CodeUserLimit, be.Code)
}

func TestDisabledBillingIsNoop(t *testing.T) {
	_, db, orgID := setup(t, 0)
	svc := billing.New(db, config.BillingConfig{Enabled: false})
	assert.NoError(t, svc.CheckCanSend(orgID, "919876543210", "MARKETING"))
	change, err := svc.ChargeOnSend(orgID, wamid(), "919876543210", "MARKETING")
	assert.NoError(t, err)
	assert.Nil(t, change)
}
