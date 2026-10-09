package handlers_test

import (
	"encoding/json"
	"testing"

	"github.com/Botrixai89/botrixai/internal/handlers"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/Botrixai89/botrixai/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

func TestApp_UpdateConversation(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
	contact := testutil.CreateTestContact(t, app.DB, org.ID)

	var last *fastglue.Request
	update := func(body map[string]any) int {
		req := testutil.NewJSONRequest(t, body)
		testutil.SetAuthContext(req, org.ID, user.ID)
		testutil.SetPathParam(req, "id", contact.ID.String())
		require.NoError(t, app.UpdateConversation(req))
		last = req
		return testutil.GetResponseStatusCode(req)
	}

	assert.Equal(t, fasthttp.StatusOK, update(map[string]any{"status": "closed", "bot_paused": true}))
	var resp struct {
		Data struct {
			ConversationStatus string `json:"conversation_status"`
			BotPaused          bool   `json:"bot_paused"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(last), &resp))
	assert.Equal(t, "closed", resp.Data.ConversationStatus)
	assert.True(t, resp.Data.BotPaused)
	var got models.Contact
	require.NoError(t, app.DB.First(&got, contact.ID).Error)
	assert.Equal(t, "closed", got.ConversationStatus)
	assert.True(t, got.BotPaused)

	// Partial update leaves the other field alone
	assert.Equal(t, fasthttp.StatusOK, update(map[string]any{"bot_paused": false}))
	require.NoError(t, app.DB.First(&got, contact.ID).Error)
	assert.Equal(t, "closed", got.ConversationStatus)
	assert.False(t, got.BotPaused)

	assert.Equal(t, fasthttp.StatusBadRequest, update(map[string]any{"status": "archived"}))
	assert.Equal(t, fasthttp.StatusBadRequest, update(map[string]any{}))
}

func TestApp_ListContacts_InboxFilters(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))

	open := testutil.CreateTestContact(t, app.DB, org.ID)
	closed := testutil.CreateTestContact(t, app.DB, org.ID)
	mine := testutil.CreateTestContact(t, app.DB, org.ID)
	require.NoError(t, app.DB.Model(&closed).Updates(map[string]any{"conversation_status": "closed", "bot_paused": true}).Error)
	require.NoError(t, app.DB.Model(&mine).Updates(map[string]any{"assigned_user_id": user.ID, "is_read": false}).Error)

	list := func(params map[string]string) []handlers.ContactResponse {
		req := testutil.NewGETRequest(t)
		testutil.SetAuthContext(req, org.ID, user.ID)
		for k, v := range params {
			testutil.SetQueryParam(req, k, v)
		}
		require.NoError(t, app.ListContacts(req))
		var resp struct {
			Data struct {
				Contacts []handlers.ContactResponse `json:"contacts"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
		return resp.Data.Contacts
	}
	ids := func(cs []handlers.ContactResponse) []string {
		out := make([]string, 0, len(cs))
		for _, c := range cs {
			out = append(out, c.ID.String())
		}
		return out
	}

	assert.ElementsMatch(t, []string{open.ID.String(), mine.ID.String()}, ids(list(map[string]string{"conversation_status": "open"})))
	assert.ElementsMatch(t, []string{closed.ID.String()}, ids(list(map[string]string{"conversation_status": "closed"})))
	assert.ElementsMatch(t, []string{closed.ID.String()}, ids(list(map[string]string{"bot": "off"})))
	assert.ElementsMatch(t, []string{mine.ID.String()}, ids(list(map[string]string{"assignment": "mine"})))
	assert.ElementsMatch(t, []string{open.ID.String(), closed.ID.String()}, ids(list(map[string]string{"assignment": "unassigned"})))
	assert.ElementsMatch(t, []string{mine.ID.String()}, ids(list(map[string]string{"unread": "true"})))

	all := list(nil)
	for _, c := range all {
		if c.ID == closed.ID {
			assert.Equal(t, "closed", c.ConversationStatus)
			assert.True(t, c.BotPaused)
		}
	}
}
