package whatsapp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Botrixai89/botrixai/pkg/whatsapp"
	"github.com/Botrixai89/botrixai/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// heltarServer is a fake Heltar API; handle receives each request's decoded JSON body.
func heltarServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, body map[string]any)) (*whatsapp.Client, *whatsapp.Account) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/s3/put" {
			// The pre-signed upload must not carry our API key
			assert.Empty(t, r.Header.Get("Authorization"))
		} else {
			assert.Equal(t, "Bearer hk_live_test", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
		handle(w, r, body)
	}))
	t.Cleanup(srv.Close)
	client := whatsapp.NewWithTimeout(testutil.NopLogger(), 5*time.Second)
	return client, &whatsapp.Account{
		PhoneID: "106540352242922", BusinessID: "102290129340398", AccessToken: "hk_live_test",
		Provider: whatsapp.ProviderHeltar, ProviderURL: srv.URL,
	}
}

func sendOK(w http.ResponseWriter, wamid string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": "ok",
		"data": map[string]any{
			"success": map[string]any{"0": map[string]any{"message": map[string]any{"wamid": wamid, "status": "waiting"}}},
			"fail":    map[string]any{},
		},
	})
}

func firstMessage(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	msgs, ok := body["messages"].([]any)
	require.True(t, ok)
	require.Len(t, msgs, 1)
	return msgs[0].(map[string]any)
}

func TestHeltar_SendText(t *testing.T) {
	t.Parallel()
	client, acct := heltarServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		assert.Equal(t, "/v1/messages/send", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("priority"))
		m := firstMessage(t, body)
		assert.Equal(t, "919876543210", m["clientWaNumber"])
		assert.Equal(t, "text", m["messageType"])
		assert.Equal(t, "Hello", m["message"])
		assert.Equal(t, "wamid.prev", m["contextId"])
		sendOK(w, "wamid.ABC")
	})
	id, err := client.SendTextMessage(context.Background(), acct, whatsapp.Recipient{Phone: "919876543210"}, "Hello", "wamid.prev")
	require.NoError(t, err)
	assert.Equal(t, "wamid.ABC", id)
}

func TestHeltar_SendTemplateMapsComponentsToVariables(t *testing.T) {
	t.Parallel()
	client, acct := heltarServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		m := firstMessage(t, body)
		assert.Equal(t, "template", m["messageType"])
		assert.Equal(t, "order_update", m["templateName"])
		assert.Equal(t, "en", m["languageCode"])
		vars := m["variables"].([]any)
		require.Len(t, vars, 1)
		assert.Equal(t, "body", vars[0].(map[string]any)["type"])
		assert.NotContains(t, m, "contextId")
		sendOK(w, "wamid.TPL")
	})
	comps := []map[string]any{{"type": "body", "parameters": []map[string]any{{"type": "text", "text": "Rohit"}}}}
	id, err := client.SendTemplateMessage(context.Background(), acct, whatsapp.Recipient{Phone: "919876543210"}, "order_update", "en", comps)
	require.NoError(t, err)
	assert.Equal(t, "wamid.TPL", id)
}

func TestHeltar_SendFailureKeepsMetaError(t *testing.T) {
	t.Parallel()
	client, acct := heltarServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errorMessage": "Failed to send all messages",
			"errorRaw": map[string]any{
				"success": map[string]any{},
				"fail": map[string]any{"0": map[string]any{
					"errorCode": 400,
					"errorData": map[string]any{"message": "Re-engagement message", "code": 131047},
				}},
			},
		})
	})
	_, err := client.SendTextMessage(context.Background(), acct, whatsapp.Recipient{Phone: "919876543210"}, "Hi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Re-engagement")
}

func TestHeltar_UploadThenSendMedia(t *testing.T) {
	t.Parallel()
	var uploaded atomic.Bool
	var cdnURL string
	client, acct := heltarServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		switch {
		case r.URL.Path == "/v1/messages/presigned-url":
			assert.Equal(t, "invoice.pdf", r.URL.Query().Get("file_name"))
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"signedRequest": "http://" + r.Host + "/s3/put", "url": cdnURL,
			}})
		case r.URL.Path == "/s3/put":
			uploaded.Store(true)
		case r.URL.Path == "/v1/messages/send":
			m := firstMessage(t, body)
			assert.Equal(t, "media", m["messageType"])
			assert.Equal(t, "document", m["mediaType"])
			assert.Equal(t, cdnURL, m["url"])
			assert.Equal(t, "invoice.pdf", m["name"])
			assert.Equal(t, "application/pdf", m["mimeType"])
			sendOK(w, "wamid.DOC")
		}
	})
	cdnURL = "https://cdn.example.com/abc-invoice.pdf"
	mediaID, err := client.UploadMedia(context.Background(), acct, []byte("%PDF"), "application/pdf", "invoice.pdf")
	require.NoError(t, err)
	assert.Equal(t, cdnURL, mediaID)
	assert.True(t, uploaded.Load())

	id, err := client.SendDocumentMessage(context.Background(), acct, whatsapp.Recipient{Phone: "919876543210"}, mediaID, "invoice.pdf", "")
	require.NoError(t, err)
	assert.Equal(t, "wamid.DOC", id)
}

func TestHeltar_FetchTemplates(t *testing.T) {
	t.Parallel()
	client, acct := heltarServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		assert.Equal(t, "/v1/templates", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "1", "name": "order_update", "language": "en", "status": "APPROVED", "category": "UTILITY",
				"components": []map[string]any{{"type": "BODY", "text": "Hi {{1}}"}}},
		}})
	})
	list, err := client.FetchTemplates(context.Background(), acct)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "order_update", list[0].Name)
	assert.Equal(t, "APPROVED", list[0].Status)
}

func TestHeltar_MarkReadIsLocalAndGraphCallsAreRefused(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client, acct := heltarServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) { calls.Add(1) })
	require.NoError(t, client.MarkMessageRead(context.Background(), acct, "wamid.X"))
	assert.Zero(t, calls.Load())

	_, err := client.GetBusinessProfile(context.Background(), acct)
	assert.ErrorIs(t, err, whatsapp.ErrNotSupportedByProvider)
}

func TestHeltar_IncomingMediaURL(t *testing.T) {
	t.Parallel()
	client, acct := heltarServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		assert.Equal(t, "wamid.IN", r.URL.Query().Get("wamid"))
		_ = json.NewEncoder(w).Encode(map[string]any{"wamid": "wamid.IN", "awsLink": "https://cdn.example.com/photo.jpg"})
	})
	link, err := client.GetMediaURL(context.Background(), "wamid.IN", acct)
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example.com/photo.jpg", link)
}

func TestHeltar_GetBusiness(t *testing.T) {
	t.Parallel()
	client, acct := heltarServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		assert.Equal(t, "/v1/business", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"id": 12345, "name": "Acme", "businessAccountId": "1022", "bizWhatsappNumber": "919876543210", "phoneNumberId": "1065",
		}})
	})
	biz, err := client.GetHeltarBusiness(context.Background(), acct.ProviderURL, "hk_live_test")
	require.NoError(t, err)
	assert.Equal(t, "1065", biz.PhoneNumberID)
	assert.Equal(t, "1022", biz.BusinessAccountID)
}
