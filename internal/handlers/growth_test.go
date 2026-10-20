package handlers_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Botrixai89/botrixai/internal/config"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/Botrixai89/botrixai/test/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestApp_TrackedLinks(t *testing.T) {
	app := newTestApp(t)
	app.Config.Server = config.ServerConfig{PublicURL: "https://app.example.com"}
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))

	bad := testutil.NewJSONRequest(t, map[string]any{"target_url": "javascript:alert(1)"})
	testutil.SetAuthContext(bad, org.ID, user.ID)
	require.NoError(t, app.CreateTrackedLink(bad))
	assert.Equal(t, fasthttp.StatusBadRequest, testutil.GetResponseStatusCode(bad))

	req := testutil.NewJSONRequest(t, map[string]any{"target_url": "https://shop.example.com/diwali", "title": "Diwali"})
	testutil.SetAuthContext(req, org.ID, user.ID)
	require.NoError(t, app.CreateTrackedLink(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))
	var resp struct {
		Data struct {
			Code     string `json:"code"`
			ShortURL string `json:"short_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	assert.Equal(t, "https://app.example.com/l/"+resp.Data.Code, resp.Data.ShortURL)

	click := func(ua string) string {
		r := testutil.NewGETRequest(t)
		r.RequestCtx.Request.Header.SetUserAgent(ua)
		testutil.SetPathParam(r, "code", resp.Data.Code)
		require.NoError(t, app.RedirectTrackedLink(r))
		assert.Equal(t, fasthttp.StatusFound, testutil.GetResponseStatusCode(r))
		return string(r.RequestCtx.Response.Header.Peek("Location"))
	}
	assert.Equal(t, "https://shop.example.com/diwali", click("WhatsApp/2.24.1 A"))           // link preview: not counted
	assert.Equal(t, "https://shop.example.com/diwali", click("Mozilla/5.0 (iPhone) Safari")) // a person
	click("Mozilla/5.0 (Android) Chrome")

	var link models.TrackedLink
	require.NoError(t, app.DB.Where("code = ?", resp.Data.Code).First(&link).Error)
	assert.Equal(t, 2, link.Clicks)
	assert.NotNil(t, link.FirstClickedAt)
}

func TestApp_RetargetCampaign(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
	account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)
	tpl := &models.Template{OrganizationID: org.ID, WhatsAppAccount: account.Name, Name: "promo_" + uuid.New().String()[:6],
		Language: "en", Category: "MARKETING", Status: "APPROVED", BodyContent: "Hi {{1}}"}
	require.NoError(t, app.DB.Create(tpl).Error)
	campaign := &models.BulkMessageCampaign{OrganizationID: org.ID, WhatsAppAccount: account.Name, Name: "Diwali",
		TemplateID: tpl.ID, Status: models.CampaignStatusCompleted, CreatedBy: user.ID}
	require.NoError(t, app.DB.Create(campaign).Error)
	sent := time.Now().Add(-time.Hour)
	for i, st := range []models.MessageStatus{models.MessageStatusRead, models.MessageStatusDelivered, models.MessageStatusSent, models.MessageStatusFailed} {
		require.NoError(t, app.DB.Create(&models.BulkMessageRecipient{CampaignID: campaign.ID,
			PhoneNumber: "91980000000" + string(rune('1'+i)), Status: st, SentAt: &sent,
			TemplateParams: models.JSONB{"1": "Customer"}}).Error)
	}

	run := func(audience string) (int, int) {
		req := testutil.NewJSONRequest(t, map[string]any{"audience": audience})
		testutil.SetAuthContext(req, org.ID, user.ID)
		testutil.SetPathParam(req, "id", campaign.ID.String())
		require.NoError(t, app.RetargetCampaign(req))
		var resp struct {
			Data struct {
				Total int `json:"total_recipients"`
			} `json:"data"`
		}
		_ = json.Unmarshal(testutil.GetResponseBody(req), &resp)
		return testutil.GetResponseStatusCode(req), resp.Data.Total
	}
	code, n := run("not_read")
	require.Equal(t, fasthttp.StatusOK, code)
	assert.Equal(t, 2, n)
	_, n = run("failed")
	assert.Equal(t, 1, n)
	code, _ = run("clicked") // nobody clicked
	assert.Equal(t, fasthttp.StatusBadRequest, code)
	code, _ = run("bogus")
	assert.Equal(t, fasthttp.StatusBadRequest, code)

	var drafts []models.BulkMessageCampaign
	app.DB.Where("organization_id = ? AND status = ?", org.ID, models.CampaignStatusDraft).Find(&drafts)
	require.Len(t, drafts, 2)
	var copied models.BulkMessageRecipient
	require.NoError(t, app.DB.Where("campaign_id = ?", drafts[0].ID).First(&copied).Error)
	assert.Equal(t, models.MessageStatusPending, copied.Status)
	assert.Equal(t, "Customer", copied.TemplateParams["1"], "personalisation is kept")
}

func TestApp_CampaignListStatsAndLogs(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
	account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)
	tpl := &models.Template{OrganizationID: org.ID, WhatsAppAccount: account.Name, Name: "promo_" + uuid.New().String()[:6],
		Language: "en", Category: "MARKETING", Status: "APPROVED", BodyContent: "Hi"}
	require.NoError(t, app.DB.Create(tpl).Error)
	campaign := &models.BulkMessageCampaign{OrganizationID: org.ID, WhatsAppAccount: account.Name, Name: "Navratri sale",
		TemplateID: tpl.ID, Status: models.CampaignStatusProcessing, CreatedBy: user.ID}
	require.NoError(t, app.DB.Create(campaign).Error)

	sent := time.Now().Add(-time.Hour)
	add := func(phone string, st models.MessageStatus, sentAt *time.Time, errMsg string) {
		require.NoError(t, app.DB.Create(&models.BulkMessageRecipient{CampaignID: campaign.ID, PhoneNumber: phone,
			Status: st, SentAt: sentAt, ErrorMessage: errMsg}).Error)
	}
	add("919800000001", models.MessageStatusRead, &sent, "")
	add("919800000002", models.MessageStatusDelivered, &sent, "")
	add("919800000003", models.MessageStatusPending, nil, "")
	add("919800000004", models.MessageStatusFailed, nil, "Not a WhatsApp number")

	// The first recipient wrote back after the broadcast; the second only wrote before it
	replier := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithPhoneNumber("919800000001"))
	quiet := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithPhoneNumber("919800000002"))
	for _, m := range []struct {
		c  *models.Contact
		at time.Time
	}{{replier, time.Now()}, {quiet, sent.Add(-time.Hour)}} {
		msg := &models.Message{OrganizationID: org.ID, WhatsAppAccount: account.Name, ContactID: m.c.ID,
			Direction: models.DirectionIncoming, MessageType: models.MessageTypeText, Content: "interested"}
		require.NoError(t, app.DB.Create(msg).Error)
		require.NoError(t, app.DB.Model(msg).UpdateColumn("created_at", m.at).Error)
	}

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	require.NoError(t, app.ListCampaigns(req))
	var list struct {
		Data struct {
			Campaigns []struct {
				ID      string `json:"id"`
				Pending int    `json:"pending_count"`
				Replied int    `json:"replied_count"`
			} `json:"campaigns"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &list))
	require.Len(t, list.Data.Campaigns, 1)
	assert.Equal(t, 1, list.Data.Campaigns[0].Pending)
	assert.Equal(t, 1, list.Data.Campaigns[0].Replied)

	logs := func(params map[string]string) (int, []map[string]any) {
		req := testutil.NewGETRequest(t)
		testutil.SetAuthContext(req, org.ID, user.ID)
		for k, v := range params {
			testutil.SetQueryParam(req, k, v)
		}
		require.NoError(t, app.ListCampaignLogs(req))
		require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))
		var resp struct {
			Data struct {
				Logs  []map[string]any `json:"logs"`
				Total int              `json:"total"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
		return resp.Data.Total, resp.Data.Logs
	}
	total, _ := logs(nil)
	assert.Equal(t, 4, total)
	total, rows := logs(map[string]string{"status": "failed"})
	require.Equal(t, 1, total)
	assert.Equal(t, "Navratri sale", rows[0]["campaign_name"])
	assert.Equal(t, "Not a WhatsApp number", rows[0]["error_message"])
	total, _ = logs(map[string]string{"search": "navratri"})
	assert.Equal(t, 4, total, "search matches the campaign name")

	// CSV export follows the same filters
	req = testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetQueryParam(req, "format", "csv")
	testutil.SetQueryParam(req, "status", "failed")
	require.NoError(t, app.ListCampaignLogs(req))
	csvBody := string(testutil.GetResponseBody(req))
	assert.Contains(t, string(req.RequestCtx.Response.Header.ContentType()), "text/csv")
	assert.Contains(t, csvBody, "Campaign,Phone,Name,Status")
	assert.Contains(t, csvBody, "Navratri sale,919800000004,,failed")
	assert.NotContains(t, csvBody, "919800000001")

	// Another organization sees none of it
	other := testutil.CreateTestOrganization(t, app.DB)
	otherRole := testutil.CreateAdminRole(t, app.DB, other.ID)
	otherUser := testutil.CreateTestUser(t, app.DB, other.ID, testutil.WithRoleID(&otherRole.ID))
	req = testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, other.ID, otherUser.ID)
	require.NoError(t, app.ListCampaignLogs(req))
	assert.Contains(t, string(testutil.GetResponseBody(req)), `"total":0`)
}
