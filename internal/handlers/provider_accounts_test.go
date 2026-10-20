package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Botrixai89/botrixai/internal/handlers"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/Botrixai89/botrixai/pkg/whatsapp"
	"github.com/Botrixai89/botrixai/test/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

func providerAccount(t *testing.T, app *handlers.App, phoneID string) *models.WhatsAppAccount {
	t.Helper()
	org := testutil.CreateTestOrganization(t, app.DB)
	acc := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		Name:               "prov-" + uuid.New().String()[:8],
		PhoneID:            phoneID,
		BusinessID:         "WABA-123",
		AccessToken:        "hk_live_x",
		WebhookVerifyToken: "shared-secret",
		APIVersion:         "v18.0",
		Provider:           whatsapp.ProviderHeltar,
		ProviderURL:        "https://newapi.example.com",
		Status:             "active",
	}
	require.NoError(t, app.DB.Create(acc).Error)
	return acc
}

func providerWebhookRequest(t *testing.T, accountID uuid.UUID, secret string, body []byte) *fastglue.Request {
	t.Helper()
	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.Header.SetContentType("application/json")
	if secret != "" {
		req.RequestCtx.Request.Header.Set("X-Botrix-Webhook-Token", secret)
	}
	req.RequestCtx.Request.SetBody(body)
	testutil.SetPathParam(req, "id", accountID.String())
	return req
}

func TestApp_ProviderWebhook(t *testing.T) {
	app := newAppForWebhook(t, "")
	phone := "prov-phone-" + uuid.New().String()[:6]
	acc := providerAccount(t, app, phone)

	t.Run("right secret and own number is accepted", func(t *testing.T) {
		r := providerWebhookRequest(t, acc.ID, "shared-secret", makeMessagesPayload(phone))
		require.NoError(t, app.ProviderWebhook(r))
		assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(r))
	})
	t.Run("wrong or missing secret is rejected", func(t *testing.T) {
		for _, secret := range []string{"", "guess"} {
			r := providerWebhookRequest(t, acc.ID, secret, makeMessagesPayload(phone))
			require.NoError(t, app.ProviderWebhook(r))
			assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(r))
		}
	})
	t.Run("a valid secret cannot speak for another number", func(t *testing.T) {
		r := providerWebhookRequest(t, acc.ID, "shared-secret", makeMessagesPayload("someone-else"))
		require.NoError(t, app.ProviderWebhook(r))
		assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(r))
	})
	t.Run("the Meta endpoint refuses payloads for provider numbers", func(t *testing.T) {
		req := testutil.NewRequest(t)
		req.RequestCtx.Request.Header.SetMethod("POST")
		req.RequestCtx.Request.SetBody(makeMessagesPayload(phone))
		require.NoError(t, app.WebhookHandler(req))
		assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(req))
	})
}

func TestApp_WebhookHandler_MissingSignatureRejectedWhenAppSecretSet(t *testing.T) {
	app := newAppForWebhook(t, "")
	org := testutil.CreateTestOrganization(t, app.DB)
	phone := "signed-" + uuid.New().String()[:6]
	require.NoError(t, app.DB.Create(&models.WhatsAppAccount{
		BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, Name: "sig-" + phone,
		PhoneID: phone, BusinessID: "biz", AccessToken: "tok", AppSecret: "real-secret",
		WebhookVerifyToken: "vt", APIVersion: "v18.0", Status: "active",
	}).Error)

	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.SetBody(makeMessagesPayload(phone)) // no X-Hub-Signature-256
	require.NoError(t, app.WebhookHandler(req))
	assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(req))
}

func TestApp_ConnectProviderAccount(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
	phone := "conn-" + uuid.New().String()[:8]

	var registered atomic.Value
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer hk_live_good", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v1/business":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"id": 1, "name": "Acme", "businessAccountId": "WABA-9", "bizWhatsappNumber": "919800000000", "phoneNumberId": phone,
			}})
		case "/v1/business/webhook-url":
			raw, _ := io.ReadAll(r.Body)
			registered.Store(string(raw))
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	app.WhatsApp = whatsapp.NewWithTimeout(testutil.NopLogger(), 5*time.Second)
	app.WhatsApp.HTTPClient = srv.Client()

	req := testutil.NewJSONRequest(t, map[string]any{
		"name": "Acme on BotrixAI API", "api_url": srv.URL, "api_key": "hk_live_good", "public_url": "https://app.example.com",
	})
	testutil.SetAuthContext(req, org.ID, user.ID)
	require.NoError(t, app.ConnectProviderAccount(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req), string(testutil.GetResponseBody(req)))

	var resp struct {
		Data handlers.ConnectProviderResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	assert.True(t, resp.Data.WebhookRegistered, resp.Data.WebhookError)
	assert.Equal(t, "heltar", resp.Data.Account.Provider)
	assert.Equal(t, "https://app.example.com/api/webhook/provider/"+resp.Data.Account.ID.String(), resp.Data.WebhookURL)
	assert.Contains(t, registered.Load().(string), resp.Data.WebhookSecret)
	assert.Contains(t, registered.Load().(string), "metaWebhooks")

	var saved models.WhatsAppAccount
	require.NoError(t, app.DB.First(&saved, "id = ?", resp.Data.Account.ID).Error)
	assert.Equal(t, phone, saved.PhoneID)
	assert.Equal(t, "WABA-9", saved.BusinessID)
	assert.NotEqual(t, "hk_live_good", saved.AccessToken, "API key must be encrypted at rest")

	// Same number again is refused
	again := testutil.NewJSONRequest(t, map[string]any{"name": "dup", "api_url": srv.URL, "api_key": "hk_live_good"})
	testutil.SetAuthContext(again, org.ID, user.ID)
	require.NoError(t, app.ConnectProviderAccount(again))
	assert.Equal(t, fasthttp.StatusConflict, testutil.GetResponseStatusCode(again))
}
