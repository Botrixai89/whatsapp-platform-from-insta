package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Botrixai89/botrixai/internal/billing"
	"github.com/Botrixai89/botrixai/internal/config"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/Botrixai89/botrixai/test/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func paymentsTestApp(t *testing.T) (*App, *models.Organization, *models.User) {
	t.Helper()
	app := newProcessorTestApp(t)
	app.Config = &config.Config{
		Billing:  config.BillingConfig{Enabled: true, Currency: "INR"},
		Payments: config.PaymentsConfig{RazorpayKeyID: "rzp_test_key", RazorpayKeySecret: "key_secret", RazorpayWebhookSecret: "hook_secret"},
		JWT:      config.JWTConfig{Secret: testutil.TestJWTSecret},
	}
	app.Billing = billing.New(app.DB, app.Config.Billing)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))
	return app, org, user
}

func walletBalance(t *testing.T, app *App, org *models.Organization) float64 {
	var w models.Wallet
	require.NoError(t, app.DB.Where("organization_id = ?", org.ID).First(&w).Error)
	return w.Balance
}

func TestRechargeFlow(t *testing.T) {
	app, org, user := paymentsTestApp(t)
	orderID := "order_" + uuid.New().String()[:12] // unique per run; order IDs are globally unique
	var gotAmount float64
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		assert.Equal(t, "rzp_test_key", u)
		assert.Equal(t, "key_secret", p)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotAmount = body["amount"].(float64)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": orderID})
	}))
	defer fake.Close()
	old := razorpayAPI
	razorpayAPI = fake.URL
	defer func() { razorpayAPI = old }()

	// Too small
	small := testutil.NewJSONRequest(t, map[string]any{"amount": 10})
	testutil.SetAuthContext(small, org.ID, user.ID)
	require.NoError(t, app.CreateRechargeOrder(small))
	assert.Equal(t, fasthttp.StatusBadRequest, testutil.GetResponseStatusCode(small))

	req := testutil.NewJSONRequest(t, map[string]any{"amount": 500})
	testutil.SetAuthContext(req, org.ID, user.ID)
	require.NoError(t, app.CreateRechargeOrder(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req), string(testutil.GetResponseBody(req)))
	assert.Equal(t, float64(50000), gotAmount, "amount is sent in paise")

	// A forged signature is refused
	bad := testutil.NewJSONRequest(t, map[string]any{"razorpay_order_id": orderID, "razorpay_payment_id": "pay_1", "razorpay_signature": "nope"})
	testutil.SetAuthContext(bad, org.ID, user.ID)
	require.NoError(t, app.VerifyRecharge(bad))
	assert.Equal(t, fasthttp.StatusBadRequest, testutil.GetResponseStatusCode(bad))

	verify := func() {
		v := testutil.NewJSONRequest(t, map[string]any{"razorpay_order_id": orderID, "razorpay_payment_id": "pay_1",
			"razorpay_signature": hmacHex("key_secret", orderID+"|pay_1")})
		testutil.SetAuthContext(v, org.ID, user.ID)
		require.NoError(t, app.VerifyRecharge(v))
		require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(v))
	}
	verify()
	assert.Equal(t, 500.0, walletBalance(t, app, org))

	// Browser retry and webhook for the same payment: still credited once
	verify()
	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_1","order_id":"` + orderID + `","status":"captured"}}}}`)
	hook := testutil.NewRequest(t)
	hook.RequestCtx.Request.SetBody(body)
	hook.RequestCtx.Request.Header.Set("X-Razorpay-Signature", hmacHex("hook_secret", string(body)))
	require.NoError(t, app.RazorpayWebhook(hook))
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(hook))
	assert.Equal(t, 500.0, walletBalance(t, app, org))

	var txCount int64
	app.DB.Model(&models.WalletTransaction{}).Where("organization_id = ? AND source = ?", org.ID, models.WalletSourceRecharge).Count(&txCount)
	assert.Equal(t, int64(1), txCount)
}

func TestRazorpayWebhook_CreditsWhenBrowserNeverReturned(t *testing.T) {
	app, org, user := paymentsTestApp(t)
	orderID := "order_" + uuid.New().String()[:12]
	require.NoError(t, app.DB.Create(&models.PaymentOrder{OrganizationID: org.ID, UserID: &user.ID, GatewayOrderID: orderID,
		Amount: 250, Currency: "INR", Status: models.PaymentCreated}).Error)

	body := []byte(`{"event":"order.paid","payload":{"payment":{"entity":{"id":"pay_W1","order_id":"` + orderID + `"}}}}`)
	forged := testutil.NewRequest(t)
	forged.RequestCtx.Request.SetBody(body)
	forged.RequestCtx.Request.Header.Set("X-Razorpay-Signature", "deadbeef")
	require.NoError(t, app.RazorpayWebhook(forged))
	assert.Equal(t, fasthttp.StatusUnauthorized, testutil.GetResponseStatusCode(forged))

	// Concurrent duplicate deliveries still credit once
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := testutil.NewRequest(t)
			h.RequestCtx.Request.SetBody(body)
			h.RequestCtx.Request.Header.Set("X-Razorpay-Signature", hmacHex("hook_secret", string(body)))
			_ = app.RazorpayWebhook(h)
		}()
	}
	wg.Wait()
	assert.Equal(t, 250.0, walletBalance(t, app, org))
}
