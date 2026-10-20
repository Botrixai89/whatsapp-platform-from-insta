package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/Botrixai89/botrixai/internal/config"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/Botrixai89/botrixai/test/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSequenceFixture(t *testing.T, category string) (*App, *models.Sequence, *models.Contact) {
	t.Helper()
	app := newProcessorTestApp(t)
	app.Config = &config.Config{}
	org, account := createProcessorTestOrg(t, app)
	contact := engagementContact(t, app, org, account)
	tpl := &models.Template{OrganizationID: org.ID, WhatsAppAccount: account.Name, Name: "drip_" + uuid.New().String()[:6],
		Language: "en", Category: category, Status: "APPROVED", BodyContent: "Hi {{1}}, still interested?"}
	require.NoError(t, app.DB.Create(tpl).Error)
	seq := &models.Sequence{OrganizationID: org.ID, Name: "Welcome", WhatsAppAccount: account.Name, IsActive: true, StopOnReply: true, TriggerTag: "lead"}
	require.NoError(t, app.DB.Create(seq).Error)
	require.NoError(t, app.DB.Create(&[]models.SequenceStep{
		{SequenceID: seq.ID, Position: 0, DelayMinutes: 0, TemplateID: tpl.ID, TemplateParams: models.JSONB{"1": "{name}"}},
		{SequenceID: seq.ID, Position: 1, DelayMinutes: 60, TemplateID: tpl.ID, TemplateParams: models.JSONB{"1": "{name}"}},
	}).Error)
	return app, seq, contact
}

func enrollment(t *testing.T, app *App, seqID, contactID uuid.UUID) models.SequenceEnrollment {
	t.Helper()
	var e models.SequenceEnrollment
	require.NoError(t, app.DB.Where("sequence_id = ? AND contact_id = ?", seqID, contactID).First(&e).Error)
	return e
}

func TestSequence_RunsStepsInOrderThenCompletes(t *testing.T) {
	app, seq, contact := newSequenceFixture(t, "UTILITY")
	require.Equal(t, 1, app.enrollContacts(seq, []uuid.UUID{contact.ID}))
	assert.Equal(t, 0, app.enrollContacts(seq, []uuid.UUID{contact.ID}), "no double enrolment")

	require.Equal(t, 1, app.RunDueSequenceSteps(context.Background(), 10))
	e := enrollment(t, app, seq.ID, contact.ID)
	assert.Equal(t, 1, e.NextStep)
	assert.Equal(t, 1, e.StepsSent)
	require.NotNil(t, e.NextRunAt)
	assert.WithinDuration(t, time.Now().Add(time.Hour), *e.NextRunAt, 2*time.Minute)

	assert.Equal(t, 0, app.RunDueSequenceSteps(context.Background(), 10), "step 2 is not due yet")

	past := time.Now().Add(-time.Minute)
	require.NoError(t, app.DB.Model(&e).Update("next_run_at", past).Error)
	require.Equal(t, 1, app.RunDueSequenceSteps(context.Background(), 10))
	e = enrollment(t, app, seq.ID, contact.ID)
	assert.Equal(t, models.EnrollmentCompleted, e.Status)

	var sent []models.Message
	app.DB.Where("contact_id = ? AND direction = ?", contact.ID, models.DirectionOutgoing).Find(&sent)
	require.Len(t, sent, 2)
	assert.Contains(t, sent[0].Content, "Hi ", "contact name filled in")
}

func TestSequence_StopsOnReplyAndRespectsOptOut(t *testing.T) {
	app, seq, contact := newSequenceFixture(t, "MARKETING")
	app.enrollContacts(seq, []uuid.UUID{contact.ID})
	app.stopSequencesOnReply(contact.ID)
	e := enrollment(t, app, seq.ID, contact.ID)
	assert.Equal(t, models.EnrollmentStopped, e.Status)
	assert.Equal(t, "Customer replied", e.StoppedReason)

	// Opted-out contacts don't get marketing steps
	require.NoError(t, app.DB.Model(&e).Updates(map[string]any{"status": models.EnrollmentActive, "next_run_at": time.Now().Add(-time.Minute)}).Error)
	require.NoError(t, app.DB.Model(contact).Update("marketing_opt_out", true).Error)
	assert.Equal(t, 0, app.RunDueSequenceSteps(context.Background(), 10))
	e = enrollment(t, app, seq.ID, contact.ID)
	assert.Equal(t, models.EnrollmentStopped, e.Status)
	assert.Contains(t, e.StoppedReason, "opted out")
}

func TestSequence_TagTrigger(t *testing.T) {
	app, seq, contact := newSequenceFixture(t, "UTILITY")
	app.enrollByTags(seq.OrganizationID, contact.ID, []string{"vip"})
	var n int64
	app.DB.Model(&models.SequenceEnrollment{}).Where("contact_id = ?", contact.ID).Count(&n)
	assert.Zero(t, n)
	app.enrollByTags(seq.OrganizationID, contact.ID, []string{"lead"})
	app.DB.Model(&models.SequenceEnrollment{}).Where("contact_id = ?", contact.ID).Count(&n)
	assert.Equal(t, int64(1), n)
}

func TestRunScheduledCampaigns_PausesWhenItCannotStart(t *testing.T) {
	app, seq, _ := newSequenceFixture(t, "UTILITY")
	var tpl models.SequenceStep
	require.NoError(t, app.DB.Where("sequence_id = ?", seq.ID).First(&tpl).Error)
	creator := testutil.CreateTestUser(t, app.DB, seq.OrganizationID)
	due := time.Now().Add(-time.Minute)
	later := time.Now().Add(time.Hour)
	c1 := &models.BulkMessageCampaign{OrganizationID: seq.OrganizationID, WhatsAppAccount: seq.WhatsAppAccount, Name: "due",
		TemplateID: tpl.TemplateID, Status: models.CampaignStatusScheduled, ScheduledAt: &due, CreatedBy: creator.ID}
	c2 := &models.BulkMessageCampaign{OrganizationID: seq.OrganizationID, WhatsAppAccount: seq.WhatsAppAccount, Name: "later",
		TemplateID: tpl.TemplateID, Status: models.CampaignStatusScheduled, ScheduledAt: &later, CreatedBy: creator.ID}
	require.NoError(t, app.DB.Create(c1).Error)
	require.NoError(t, app.DB.Create(c2).Error)

	app.RunScheduledCampaigns(context.Background())
	require.NoError(t, app.DB.First(c1, c1.ID).Error)
	require.NoError(t, app.DB.First(c2, c2.ID).Error)
	assert.Equal(t, models.CampaignStatusPaused, c1.Status, "due but no recipients: paused for the owner to fix")
	assert.Equal(t, models.CampaignStatusScheduled, c2.Status, "not due yet: untouched")
}
