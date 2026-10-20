package handlers

import (
	"testing"
	"time"

	"github.com/Botrixai89/botrixai/internal/config"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/Botrixai89/botrixai/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchesKeyword(t *testing.T) {
	kw := cleanKeywords([]string{"stop", " Stop  Promotions ", "STOP", ""})
	assert.Equal(t, []string{"STOP", "STOP PROMOTIONS"}, kw)
	assert.True(t, matchesKeyword("stop", kw))
	assert.True(t, matchesKeyword("  Stop!  ", kw))
	assert.True(t, matchesKeyword("stop   promotions", kw))
	assert.False(t, matchesKeyword("please don't stop my order", kw))
	assert.False(t, matchesKeyword("", kw))
}

func engagementContact(t *testing.T, app *App, org *models.Organization, account *models.WhatsAppAccount) *models.Contact {
	t.Helper()
	now := time.Now()
	c := testutil.CreateTestContact(t, app.DB, org.ID)
	require.NoError(t, app.DB.Model(c).Updates(map[string]any{
		"whats_app_account": account.Name, "last_inbound_at": now,
	}).Error)
	require.NoError(t, app.DB.First(c, c.ID).Error)
	return c
}

func TestHandleEngagementReply_OptOutAndBackIn(t *testing.T) {
	app := newProcessorTestApp(t)
	org, account := createProcessorTestOrg(t, app)
	contact := engagementContact(t, app, org, account)

	assert.False(t, app.handleEngagementReply(account, contact, "hello", ""), "ordinary text goes to the bot")

	require.True(t, app.handleEngagementReply(account, contact, "Stop", ""))
	var got models.Contact
	require.NoError(t, app.DB.First(&got, contact.ID).Error)
	assert.True(t, got.MarketingOptOut)
	assert.Equal(t, "keyword", got.OptOutSource)

	// Saying STOP again while already opted out is just a message
	assert.False(t, app.handleEngagementReply(account, &got, "stop", ""))

	require.True(t, app.handleEngagementReply(account, &got, "START", ""))
	require.NoError(t, app.DB.First(&got, contact.ID).Error)
	assert.False(t, got.MarketingOptOut)

	// Confirmation replies were sent and stored
	var replies int64
	app.DB.Model(&models.Message{}).Where("contact_id = ? AND direction = ?", contact.ID, models.DirectionOutgoing).Count(&replies)
	assert.Equal(t, int64(2), replies)
}

func TestCSATSurveyAndAnswer(t *testing.T) {
	app := newProcessorTestApp(t)
	app.Config = &config.Config{} // account loading decrypts secrets (no key in tests)
	org, account := createProcessorTestOrg(t, app)
	contact := engagementContact(t, app, org, account)

	// Disabled by default: closing a chat sends nothing
	app.sendCSATSurvey(contact)
	var count int64
	app.DB.Model(&models.CSATResponse{}).Where("contact_id = ?", contact.ID).Count(&count)
	assert.Zero(t, count)

	settings := defaultEngagementSettings()
	settings.CSAT.Enabled = true
	require.NoError(t, app.DB.Model(org).Update("settings", models.JSONB{"engagement": settings}).Error)

	app.sendCSATSurvey(contact)
	app.DB.Model(&models.CSATResponse{}).Where("contact_id = ?", contact.ID).Count(&count)
	require.Equal(t, int64(1), count)

	require.NoError(t, app.DB.Model(contact).Update("conversation_status", "open").Error) // the reply reopened it
	require.True(t, app.handleEngagementReply(account, contact, "😊 Great", "csat_5"))

	var survey models.CSATResponse
	require.NoError(t, app.DB.Where("contact_id = ?", contact.ID).First(&survey).Error)
	assert.Equal(t, 5, survey.Rating)
	assert.NotNil(t, survey.RespondedAt)
	var got models.Contact
	require.NoError(t, app.DB.First(&got, contact.ID).Error)
	assert.Equal(t, "closed", got.ConversationStatus, "answering the survey keeps the chat closed")

	// Outside the 24h window no survey can be sent
	old := time.Now().Add(-25 * time.Hour)
	require.NoError(t, app.DB.Model(contact).Update("last_inbound_at", old).Error)
	contact.LastInboundAt = &old
	app.sendCSATSurvey(contact)
	app.DB.Model(&models.CSATResponse{}).Where("contact_id = ?", contact.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestBlockedContactCannotBeMessaged(t *testing.T) {
	app := newProcessorTestApp(t)
	org, account := createProcessorTestOrg(t, app)
	contact := engagementContact(t, app, org, account)
	contact.IsBlocked = true
	err := app.sendAndSaveTextMessage(account, contact, "hi")
	assert.ErrorIs(t, err, ErrContactBlocked)
}

func TestRecordAdReferral(t *testing.T) {
	app := newProcessorTestApp(t)
	org, account := createProcessorTestOrg(t, app)
	contact := engagementContact(t, app, org, account)

	app.recordAdReferral(account, contact, "wamid.1", nil) // ordinary message: nothing recorded
	app.recordAdReferral(account, contact, "wamid.2", &IncomingReferral{
		SourceID: "120200001", SourceType: "ad", Headline: "Diwali Sale 50% off", SourceURL: "https://fb.me/ad", CtwaClid: "clid-1",
	})
	var refs []models.AdReferral
	app.DB.Where("contact_id = ?", contact.ID).Find(&refs)
	require.Len(t, refs, 1)
	assert.Equal(t, "120200001", refs[0].SourceID)
	var got models.Contact
	require.NoError(t, app.DB.First(&got, contact.ID).Error)
	assert.Equal(t, "Diwali Sale 50% off", got.AdHeadline)
	assert.NotNil(t, got.AdClickedAt)
}
