package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Botrixai89/botrixai/internal/contactutil"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---------------------------------------------------------------------------
// Background runner: scheduled campaigns + drip sequences
// ---------------------------------------------------------------------------

// AutomationProcessor starts scheduled campaigns and sends due sequence steps.
type AutomationProcessor struct {
	app      *App
	interval time.Duration
	stopCh   chan struct{}
}

// NewAutomationProcessor creates the runner; interval is how often it looks for due work.
func NewAutomationProcessor(app *App, interval time.Duration) *AutomationProcessor {
	return &AutomationProcessor{app: app, interval: interval, stopCh: make(chan struct{})}
}

// Start runs until ctx is cancelled or Stop is called.
func (p *AutomationProcessor) Start(ctx context.Context) {
	p.app.Log.Info("Automation processor started", "interval", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.app.RunScheduledCampaigns(ctx)
			p.app.RunDueSequenceSteps(ctx, 200)
		}
	}
}

// Stop ends the loop.
func (p *AutomationProcessor) Stop() {
	select {
	case <-p.stopCh:
	default:
		close(p.stopCh)
	}
}

// RunScheduledCampaigns launches campaigns whose scheduled time has passed.
func (a *App) RunScheduledCampaigns(ctx context.Context) {
	var due []models.BulkMessageCampaign
	a.DB.Where("status = ? AND scheduled_at <= ?", models.CampaignStatusScheduled, time.Now()).Limit(20).Find(&due)
	for i := range due {
		c := &due[i]
		// Claim it so two servers never launch the same campaign
		res := a.DB.Model(&models.BulkMessageCampaign{}).
			Where("id = ? AND status = ?", c.ID, models.CampaignStatusScheduled).
			Update("status", models.CampaignStatusQueued)
		if res.RowsAffected != 1 {
			continue
		}
		if err := a.launchCampaign(ctx, c); err != nil {
			// Couldn't start (e.g. low balance): pause so the owner can fix it and press Start.
			a.Log.Warn("Scheduled campaign could not start", "campaign_id", c.ID, "error", err)
			a.DB.Model(c).Update("status", models.CampaignStatusPaused)
			continue
		}
		a.Log.Info("Scheduled campaign started", "campaign_id", c.ID)
	}
}

// ScheduleCampaignRequest sets or clears a campaign's start time.
type ScheduleCampaignRequest struct {
	ScheduledAt *time.Time `json:"scheduled_at"` // null to unschedule (back to draft)
}

// ScheduleCampaign puts a draft campaign on the schedule, or takes it off.
func (a *App) ScheduleCampaign(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionWrite)
	if err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}
	var req ScheduleCampaignRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	campaign, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, id, orgID, "Campaign")
	if err != nil {
		return nil
	}
	if campaign.Status != models.CampaignStatusDraft && campaign.Status != models.CampaignStatusScheduled {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Only draft campaigns can be scheduled", nil, "")
	}
	if req.ScheduledAt == nil {
		a.DB.Model(campaign).Updates(map[string]any{"status": models.CampaignStatusDraft, "scheduled_at": nil})
		return r.SendEnvelope(map[string]any{"status": models.CampaignStatusDraft})
	}
	if req.ScheduledAt.Before(time.Now().Add(time.Minute)) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Pick a time at least a minute from now", nil, "")
	}
	var pending int64
	a.DB.Model(&models.BulkMessageRecipient{}).Where("campaign_id = ? AND status = ?", id, models.MessageStatusPending).Count(&pending)
	if pending == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Add recipients before scheduling", nil, "")
	}
	a.DB.Model(campaign).Updates(map[string]any{"status": models.CampaignStatusScheduled, "scheduled_at": req.ScheduledAt})
	return r.SendEnvelope(map[string]any{"status": models.CampaignStatusScheduled, "scheduled_at": req.ScheduledAt})
}

// ---------------------------------------------------------------------------
// Drip sequences
// ---------------------------------------------------------------------------

// RunDueSequenceSteps sends the next step for every enrolment that is due.
func (a *App) RunDueSequenceSteps(ctx context.Context, limit int) int {
	sent := 0
	for i := 0; i < limit; i++ {
		var enrollment models.SequenceEnrollment
		// Lock one due row at a time; SKIP LOCKED lets several servers share the work.
		err := a.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where("status = ? AND next_run_at <= ?", models.EnrollmentActive, time.Now()).
				Order("next_run_at").First(&enrollment).Error; err != nil {
				return err
			}
			// Push it out while we work, so a crash doesn't send it twice in a tight loop.
			return tx.Model(&enrollment).Update("next_run_at", time.Now().Add(10*time.Minute)).Error
		})
		if err != nil {
			return sent
		}
		if a.sendSequenceStep(ctx, &enrollment) {
			sent++
		}
	}
	return sent
}

func (a *App) stopEnrollment(e *models.SequenceEnrollment, reason string) {
	a.DB.Model(e).Updates(map[string]any{"status": models.EnrollmentStopped, "stopped_reason": reason, "next_run_at": nil})
}

// sendSequenceStep sends one step and schedules the next. Returns true if a message went out.
func (a *App) sendSequenceStep(ctx context.Context, e *models.SequenceEnrollment) bool {
	var seq models.Sequence
	if err := a.DB.Preload("Steps", func(db *gorm.DB) *gorm.DB { return db.Order("position") }).
		Preload("Steps.Template").First(&seq, "id = ?", e.SequenceID).Error; err != nil {
		a.stopEnrollment(e, "Sequence was deleted")
		return false
	}
	if !seq.IsActive {
		// Paused sequence: check again later without losing the contact's place.
		a.DB.Model(e).Update("next_run_at", time.Now().Add(time.Hour))
		return false
	}
	if e.NextStep >= len(seq.Steps) {
		a.DB.Model(e).Updates(map[string]any{"status": models.EnrollmentCompleted, "next_run_at": nil})
		return false
	}
	step := seq.Steps[e.NextStep]
	var contact models.Contact
	if err := a.DB.First(&contact, "id = ?", e.ContactID).Error; err != nil {
		a.stopEnrollment(e, "Contact was deleted")
		return false
	}
	if step.Template == nil {
		a.stopEnrollment(e, "Template was deleted")
		return false
	}
	if contact.IsBlocked {
		a.stopEnrollment(e, "Contact is blocked")
		return false
	}
	if contact.MarketingOptOut && strings.EqualFold(step.Template.Category, "MARKETING") {
		a.stopEnrollment(e, "Contact opted out of marketing")
		return false
	}
	account, err := a.resolveWhatsAppAccount(seq.OrganizationID, seq.WhatsAppAccount)
	if err != nil {
		a.stopEnrollment(e, "WhatsApp account not found")
		return false
	}

	params := map[string]string{}
	for k, v := range step.TemplateParams {
		s := fmt.Sprintf("%v", v)
		s = strings.ReplaceAll(s, "{name}", firstNonEmpty(contact.ProfileName, "there"))
		s = strings.ReplaceAll(s, "{phone}", contact.PhoneNumber)
		params[k] = s
	}
	_, err = a.SendOutgoingMessage(ctx, OutgoingMessageRequest{
		Account:    account,
		Contact:    &contact,
		Type:       models.MessageTypeTemplate,
		Template:   step.Template,
		BodyParams: params,
	}, MessageSendOptions{BroadcastWebSocket: true, DispatchWebhook: true})
	if err != nil {
		// Wallet empty or Meta refused: retry in an hour, keep the error visible.
		a.DB.Model(e).Updates(map[string]any{"last_error": truncateRunes(err.Error(), 500), "next_run_at": time.Now().Add(time.Hour)})
		return false
	}

	updates := map[string]any{"steps_sent": e.StepsSent + 1, "next_step": e.NextStep + 1, "last_error": ""}
	if e.NextStep+1 < len(seq.Steps) {
		updates["next_run_at"] = time.Now().Add(time.Duration(seq.Steps[e.NextStep+1].DelayMinutes) * time.Minute)
	} else {
		updates["status"] = models.EnrollmentCompleted
		updates["next_run_at"] = nil
	}
	a.DB.Model(e).Updates(updates)
	return true
}

// enrollContacts adds contacts to a sequence, skipping anyone already active in it.
func (a *App) enrollContacts(seq *models.Sequence, contactIDs []uuid.UUID) int {
	var first models.SequenceStep
	if err := a.DB.Where("sequence_id = ?", seq.ID).Order("position").First(&first).Error; err != nil {
		return 0
	}
	runAt := time.Now().Add(time.Duration(first.DelayMinutes) * time.Minute)
	added := 0
	for _, cid := range contactIDs {
		var existing int64
		a.DB.Model(&models.SequenceEnrollment{}).
			Where("sequence_id = ? AND contact_id = ? AND status = ?", seq.ID, cid, models.EnrollmentActive).Count(&existing)
		if existing > 0 {
			continue
		}
		if a.DB.Create(&models.SequenceEnrollment{
			OrganizationID: seq.OrganizationID, SequenceID: seq.ID, ContactID: cid,
			Status: models.EnrollmentActive, NextRunAt: &runAt,
		}).Error == nil {
			added++
		}
	}
	return added
}

// stopSequencesOnReply ends "stop on reply" sequences when the customer writes back.
func (a *App) stopSequencesOnReply(contactID uuid.UUID) {
	a.DB.Model(&models.SequenceEnrollment{}).
		Where("contact_id = ? AND status = ? AND sequence_id IN (?)", contactID, models.EnrollmentActive,
			a.DB.Model(&models.Sequence{}).Select("id").Where("stop_on_reply = ?", true)).
		Updates(map[string]any{"status": models.EnrollmentStopped, "stopped_reason": "Customer replied", "next_run_at": nil})
}

// enrollByTags starts tag-triggered sequences for tags newly added to a contact.
func (a *App) enrollByTags(orgID, contactID uuid.UUID, addedTags []string) {
	if len(addedTags) == 0 {
		return
	}
	var seqs []models.Sequence
	a.DB.Where("organization_id = ? AND is_active = ? AND trigger_tag IN ?", orgID, true, addedTags).Find(&seqs)
	for i := range seqs {
		a.enrollContacts(&seqs[i], []uuid.UUID{contactID})
	}
}

// SequenceRequest creates or replaces a sequence and its steps.
type SequenceRequest struct {
	Name            string `json:"name"`
	WhatsAppAccount string `json:"whatsapp_account"`
	IsActive        bool   `json:"is_active"`
	StopOnReply     bool   `json:"stop_on_reply"`
	TriggerTag      string `json:"trigger_tag"`
	Steps           []struct {
		DelayMinutes   int            `json:"delay_minutes"`
		TemplateID     string         `json:"template_id"`
		TemplateParams map[string]any `json:"template_params"`
	} `json:"steps"`
}

func (a *App) validateSequence(orgID uuid.UUID, req *SequenceRequest) ([]models.SequenceStep, string) {
	req.Name = strings.TrimSpace(req.Name)
	req.TriggerTag = strings.TrimSpace(req.TriggerTag)
	if req.Name == "" {
		return nil, "Name is required"
	}
	if _, err := a.resolveWhatsAppAccount(orgID, req.WhatsAppAccount); err != nil {
		return nil, "Choose a WhatsApp number"
	}
	if len(req.Steps) == 0 || len(req.Steps) > 20 {
		return nil, "Add between 1 and 20 steps"
	}
	steps := make([]models.SequenceStep, 0, len(req.Steps))
	for i, s := range req.Steps {
		if s.DelayMinutes < 0 || s.DelayMinutes > 60*24*90 {
			return nil, fmt.Sprintf("Step %d: delay must be between 0 minutes and 90 days", i+1)
		}
		tid, err := uuid.Parse(s.TemplateID)
		if err != nil {
			return nil, fmt.Sprintf("Step %d: choose a template", i+1)
		}
		var tpl models.Template
		if err := a.DB.Where("id = ? AND organization_id = ?", tid, orgID).First(&tpl).Error; err != nil {
			return nil, fmt.Sprintf("Step %d: template not found", i+1)
		}
		if !strings.EqualFold(tpl.Status, "APPROVED") {
			return nil, fmt.Sprintf("Step %d: template %q is not approved yet", i+1, tpl.Name)
		}
		params := models.JSONB{}
		for k, v := range s.TemplateParams {
			params[k] = v
		}
		steps = append(steps, models.SequenceStep{Position: i, DelayMinutes: s.DelayMinutes, TemplateID: tid, TemplateParams: params})
	}
	return steps, ""
}

// ListSequences returns sequences with their step count and enrolment stats.
func (a *App) ListSequences(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionRead)
	if err != nil {
		return nil
	}
	var seqs []models.Sequence
	a.DB.Where("organization_id = ?", orgID).Preload("Steps", func(db *gorm.DB) *gorm.DB { return db.Order("position") }).
		Order("created_at DESC").Find(&seqs)
	type stat struct {
		SequenceID uuid.UUID
		Status     string
		N          int64
	}
	var stats []stat
	a.DB.Model(&models.SequenceEnrollment{}).Select("sequence_id, status, COUNT(*) AS n").
		Where("organization_id = ?", orgID).Group("sequence_id, status").Scan(&stats)
	counts := map[uuid.UUID]map[string]int64{}
	for _, s := range stats {
		if counts[s.SequenceID] == nil {
			counts[s.SequenceID] = map[string]int64{}
		}
		counts[s.SequenceID][s.Status] = s.N
	}
	out := make([]map[string]any, len(seqs))
	for i, s := range seqs {
		c := counts[s.ID]
		out[i] = map[string]any{
			"id": s.ID, "name": s.Name, "whatsapp_account": s.WhatsAppAccount, "is_active": s.IsActive,
			"stop_on_reply": s.StopOnReply, "trigger_tag": s.TriggerTag, "steps": s.Steps,
			"active": c[models.EnrollmentActive], "completed": c[models.EnrollmentCompleted], "stopped": c[models.EnrollmentStopped],
			"created_at": s.CreatedAt,
		}
	}
	return r.SendEnvelope(map[string]any{"sequences": out})
}

// CreateSequence creates a drip sequence.
func (a *App) CreateSequence(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionWrite)
	if err != nil {
		return nil
	}
	var req SequenceRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	steps, msg := a.validateSequence(orgID, &req)
	if msg != "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
	}
	seq := models.Sequence{OrganizationID: orgID, Name: req.Name, WhatsAppAccount: req.WhatsAppAccount,
		IsActive: req.IsActive, StopOnReply: req.StopOnReply, TriggerTag: req.TriggerTag}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&seq).Error; err != nil {
			return err
		}
		for i := range steps {
			steps[i].SequenceID = seq.ID
		}
		return tx.Create(&steps).Error
	})
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create sequence", nil, "")
	}
	seq.Steps = steps
	return r.SendEnvelope(seq)
}

// UpdateSequence replaces a sequence's settings and steps. Contacts already
// enrolled continue from the same step number.
func (a *App) UpdateSequence(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionWrite)
	if err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "sequence")
	if err != nil {
		return nil
	}
	seq, err := findByIDAndOrg[models.Sequence](a.DB, r, id, orgID, "Sequence")
	if err != nil {
		return nil
	}
	var req SequenceRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	steps, msg := a.validateSequence(orgID, &req)
	if msg != "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(seq).Updates(map[string]any{"name": req.Name, "whats_app_account": req.WhatsAppAccount,
			"is_active": req.IsActive, "stop_on_reply": req.StopOnReply, "trigger_tag": req.TriggerTag}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("sequence_id = ?", seq.ID).Delete(&models.SequenceStep{}).Error; err != nil {
			return err
		}
		for i := range steps {
			steps[i].SequenceID = seq.ID
		}
		return tx.Create(&steps).Error
	})
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update sequence", nil, "")
	}
	seq.Steps = steps
	return r.SendEnvelope(seq)
}

// DeleteSequence deletes a sequence and stops everyone in it.
func (a *App) DeleteSequence(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionDelete)
	if err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "sequence")
	if err != nil {
		return nil
	}
	seq, err := findByIDAndOrg[models.Sequence](a.DB, r, id, orgID, "Sequence")
	if err != nil {
		return nil
	}
	a.DB.Model(&models.SequenceEnrollment{}).Where("sequence_id = ? AND status = ?", seq.ID, models.EnrollmentActive).
		Updates(map[string]any{"status": models.EnrollmentStopped, "stopped_reason": "Sequence deleted", "next_run_at": nil})
	a.DB.Delete(seq)
	return r.SendEnvelope(map[string]any{"deleted": true})
}

// EnrollRequest enrols contacts by ID, by phone number, or everyone with a tag.
type EnrollRequest struct {
	ContactIDs []string `json:"contact_ids"`
	Phones     []string `json:"phones"`
	Tag        string   `json:"tag"`
}

// EnrollInSequence adds contacts to a sequence.
func (a *App) EnrollInSequence(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionWrite)
	if err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "sequence")
	if err != nil {
		return nil
	}
	seq, err := findByIDAndOrg[models.Sequence](a.DB, r, id, orgID, "Sequence")
	if err != nil {
		return nil
	}
	var req EnrollRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	ids := map[uuid.UUID]bool{}
	for _, s := range req.ContactIDs {
		if cid, err := uuid.Parse(s); err == nil {
			var n int64
			a.DB.Model(&models.Contact{}).Where("id = ? AND organization_id = ?", cid, orgID).Count(&n)
			if n > 0 {
				ids[cid] = true
			}
		}
	}
	for _, p := range req.Phones {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if c, _, err := contactutil.GetOrCreateContact(a.DB, orgID, p, ""); err == nil && c != nil {
			ids[c.ID] = true
		}
	}
	if tag := strings.TrimSpace(req.Tag); tag != "" {
		var tagged []models.Contact
		a.DB.Select("id").Where("organization_id = ? AND tags @> ?::jsonb", orgID, fmt.Sprintf("[%q]", tag)).Limit(10000).Find(&tagged)
		for _, c := range tagged {
			ids[c.ID] = true
		}
	}
	list := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	if len(list) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "No matching contacts", nil, "")
	}
	added := a.enrollContacts(seq, list)
	return r.SendEnvelope(map[string]any{"enrolled": added, "skipped": len(list) - added})
}

// ListSequenceEnrollments shows who is in a sequence and where they are.
func (a *App) ListSequenceEnrollments(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionRead)
	if err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "sequence")
	if err != nil {
		return nil
	}
	if _, err := findByIDAndOrg[models.Sequence](a.DB, r, id, orgID, "Sequence"); err != nil {
		return nil
	}
	var rows []models.SequenceEnrollment
	a.DB.Where("sequence_id = ?", id).Preload("Contact").Order("created_at DESC").Limit(500).Find(&rows)
	return r.SendEnvelope(map[string]any{"enrollments": rows})
}

// StopSequenceEnrollment removes one contact from a sequence.
func (a *App) StopSequenceEnrollment(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionWrite)
	if err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "enrollmentId", "enrollment")
	if err != nil {
		return nil
	}
	var e models.SequenceEnrollment
	if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).First(&e).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Enrollment not found", nil, "")
	}
	a.stopEnrollment(&e, "Stopped by an agent")
	return r.SendEnvelope(map[string]any{"stopped": true})
}
