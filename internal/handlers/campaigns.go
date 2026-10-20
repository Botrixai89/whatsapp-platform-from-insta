package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/Botrixai89/botrixai/internal/queue"
	"github.com/Botrixai89/botrixai/internal/utils"
	"github.com/Botrixai89/botrixai/internal/websocket"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CampaignRequest represents campaign create/update request
type CampaignRequest struct {
	Name            string     `json:"name" validate:"required"`
	WhatsAppAccount string     `json:"whatsapp_account" validate:"required"`
	TemplateID      string     `json:"template_id" validate:"required"`
	HeaderMediaID   string     `json:"header_media_id"`
	ScheduledAt     *time.Time `json:"scheduled_at"`
	TrackLinks      bool       `json:"track_links"`
}

// CampaignResponse represents campaign in API responses
type CampaignResponse struct {
	ID                  uuid.UUID             `json:"id"`
	Name                string                `json:"name"`
	WhatsAppAccount     string                `json:"whatsapp_account"`
	TemplateID          uuid.UUID             `json:"template_id"`
	TemplateName        string                `json:"template_name,omitempty"`
	HeaderMediaID       string                `json:"header_media_id,omitempty"`
	HeaderMediaFilename string                `json:"header_media_filename,omitempty"`
	HeaderMediaMimeType string                `json:"header_media_mime_type,omitempty"`
	Status              models.CampaignStatus `json:"status"`
	TotalRecipients     int                   `json:"total_recipients"`
	SentCount           int                   `json:"sent_count"`
	DeliveredCount      int                   `json:"delivered_count"`
	ReadCount           int                   `json:"read_count"`
	FailedCount         int                   `json:"failed_count"`
	TrackLinks          bool                  `json:"track_links"`
	ClickedCount        int                   `json:"clicked_count"`
	PendingCount        int                   `json:"pending_count"`
	RepliedCount        int                   `json:"replied_count"`
	ScheduledAt         *time.Time            `json:"scheduled_at,omitempty"`
	StartedAt           *time.Time            `json:"started_at,omitempty"`
	CompletedAt         *time.Time            `json:"completed_at,omitempty"`
	CreatedByName       string                `json:"created_by_name,omitempty"`
	UpdatedByName       string                `json:"updated_by_name,omitempty"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
}

// RecipientRequest represents recipient import request
type RecipientRequest struct {
	PhoneNumber    string         `json:"phone_number" validate:"required"`
	RecipientName  string         `json:"recipient_name"`
	TemplateParams map[string]any `json:"template_params"`
	// HeaderParams carries the value for a TEXT-header variable (max 1 per
	// Meta), keyed by the variable's name. Kept separate from TemplateParams
	// so a positional header {{1}} doesn't collide with body {{1}}.
	HeaderParams map[string]any `json:"header_params"`
}

// ListCampaigns implements campaign listing
func (a *App) ListCampaigns(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	pg := parsePagination(r)

	// Get query params
	status := string(r.RequestCtx.QueryArgs().Peek("status"))
	whatsappAccount := string(r.RequestCtx.QueryArgs().Peek("whatsapp_account"))
	search := string(r.RequestCtx.QueryArgs().Peek("search"))

	baseQuery := a.DB.Where("organization_id = ?", orgID)

	if search != "" {
		baseQuery = baseQuery.Where("name ILIKE ?", "%"+search+"%")
	}

	if status != "" {
		baseQuery = baseQuery.Where("status = ?", status)
	}
	if whatsappAccount != "" {
		baseQuery = baseQuery.Where("whats_app_account = ?", whatsappAccount)
	}
	if from, ok := parseDateParam(r, "from"); ok {
		baseQuery = baseQuery.Where("created_at >= ?", from)
	}
	if to, ok := parseDateParam(r, "to"); ok {
		baseQuery = baseQuery.Where("created_at <= ?", endOfDay(to))
	}

	// Get total count
	var total int64
	baseQuery.Model(&models.BulkMessageCampaign{}).Count(&total)

	// Scheduled broadcasts read best in the order they will go out
	order := "created_at DESC"
	if status == string(models.CampaignStatusScheduled) {
		order = "scheduled_at ASC NULLS LAST"
	}
	var campaigns []models.BulkMessageCampaign
	if err := pg.Apply(baseQuery.
		Preload("Template").
		Order(order)).
		Find(&campaigns).Error; err != nil {
		a.Log.Error("Failed to list campaigns", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list campaigns", nil, "")
	}

	// Convert to response format
	response := make([]CampaignResponse, len(campaigns))
	for i, c := range campaigns {
		response[i] = CampaignResponse{
			ID:                  c.ID,
			Name:                c.Name,
			WhatsAppAccount:     c.WhatsAppAccount,
			TemplateID:          c.TemplateID,
			HeaderMediaID:       c.HeaderMediaID,
			HeaderMediaFilename: c.HeaderMediaFilename,
			HeaderMediaMimeType: c.HeaderMediaMimeType,
			Status:              c.Status,
			TotalRecipients:     c.TotalRecipients,
			SentCount:           c.SentCount,
			DeliveredCount:      c.DeliveredCount,
			ReadCount:           c.ReadCount,
			FailedCount:         c.FailedCount,
			TrackLinks:          c.TrackLinks,
			ClickedCount:        c.ClickedCount,
			ScheduledAt:         c.ScheduledAt,
			StartedAt:           c.StartedAt,
			CompletedAt:         c.CompletedAt,
			CreatedAt:           c.CreatedAt,
			UpdatedAt:           c.UpdatedAt,
		}
		if c.Template != nil {
			response[i].TemplateName = c.Template.Name
		}
	}
	a.fillCampaignListStats(orgID, response)

	return r.SendEnvelope(listEnvelope("campaigns", response, total, pg))
}

// CreateCampaign implements campaign creation
func (a *App) CreateCampaign(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	var req CampaignRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	// Validate template exists
	templateID, err := uuid.Parse(req.TemplateID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid template ID", nil, "")
	}

	template, err := findByIDAndOrg[models.Template](a.DB, r, templateID, orgID, "Template")
	if err != nil {
		return nil
	}

	// Validate WhatsApp account exists
	if _, err := a.resolveWhatsAppAccount(orgID, req.WhatsAppAccount); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "WhatsApp account not found", nil, "")
	}

	campaign := models.BulkMessageCampaign{
		OrganizationID:  orgID,
		WhatsAppAccount: req.WhatsAppAccount,
		Name:            req.Name,
		TemplateID:      templateID,
		HeaderMediaID:   req.HeaderMediaID,
		Status:          models.CampaignStatusDraft,
		ScheduledAt:     req.ScheduledAt,
		TrackLinks:      req.TrackLinks,
		CreatedBy:       userID,
		UpdatedByID:     &userID,
	}

	if err := a.DB.Create(&campaign).Error; err != nil {
		a.Log.Error("Failed to create campaign", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create campaign", nil, "")
	}

	a.logAudit(orgID, userID,
		"campaign", campaign.ID, models.AuditActionCreated, nil, &campaign)

	a.Log.Info("Campaign created", "campaign_id", campaign.ID, "name", campaign.Name)

	return r.SendEnvelope(CampaignResponse{
		ID:                  campaign.ID,
		Name:                campaign.Name,
		WhatsAppAccount:     campaign.WhatsAppAccount,
		TemplateID:          campaign.TemplateID,
		TemplateName:        template.Name,
		HeaderMediaID:       campaign.HeaderMediaID,
		HeaderMediaFilename: campaign.HeaderMediaFilename,
		HeaderMediaMimeType: campaign.HeaderMediaMimeType,
		Status:              campaign.Status,
		TotalRecipients:     campaign.TotalRecipients,
		SentCount:           campaign.SentCount,
		DeliveredCount:      campaign.DeliveredCount,
		ReadCount:           campaign.ReadCount,
		FailedCount:         campaign.FailedCount,
		TrackLinks:          campaign.TrackLinks,
		ClickedCount:        campaign.ClickedCount,
		ScheduledAt:         campaign.ScheduledAt,
		CreatedAt:           campaign.CreatedAt,
		UpdatedAt:           campaign.UpdatedAt,
	})
}

// GetCampaign implements getting a single campaign
func (a *App) GetCampaign(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	var campaign models.BulkMessageCampaign
	if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).
		Preload("Template").
		Preload("Creator").
		Preload("UpdatedBy").
		First(&campaign).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Campaign not found", nil, "")
	}

	response := CampaignResponse{
		ID:                  campaign.ID,
		Name:                campaign.Name,
		WhatsAppAccount:     campaign.WhatsAppAccount,
		TemplateID:          campaign.TemplateID,
		HeaderMediaID:       campaign.HeaderMediaID,
		HeaderMediaFilename: campaign.HeaderMediaFilename,
		HeaderMediaMimeType: campaign.HeaderMediaMimeType,
		Status:              campaign.Status,
		TotalRecipients:     campaign.TotalRecipients,
		SentCount:           campaign.SentCount,
		DeliveredCount:      campaign.DeliveredCount,
		ReadCount:           campaign.ReadCount,
		FailedCount:         campaign.FailedCount,
		TrackLinks:          campaign.TrackLinks,
		ClickedCount:        campaign.ClickedCount,
		ScheduledAt:         campaign.ScheduledAt,
		StartedAt:           campaign.StartedAt,
		CompletedAt:         campaign.CompletedAt,
		CreatedAt:           campaign.CreatedAt,
		UpdatedAt:           campaign.UpdatedAt,
	}
	if campaign.Template != nil {
		response.TemplateName = campaign.Template.Name
	}
	if campaign.Creator != nil {
		response.CreatedByName = campaign.Creator.FullName
	}
	if campaign.UpdatedBy != nil {
		response.UpdatedByName = campaign.UpdatedBy.FullName
	}

	return r.SendEnvelope(response)
}

// UpdateCampaign implements campaign update
func (a *App) UpdateCampaign(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	campaign, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, id, orgID, "Campaign")
	if err != nil {
		return nil
	}

	// Only allow updates to draft campaigns
	if campaign.Status != models.CampaignStatusDraft {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Can only update draft campaigns", nil, "")
	}

	oldCampaign := *campaign

	var req CampaignRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	// Update fields
	updates := map[string]any{
		"name":          req.Name,
		"scheduled_at":  req.ScheduledAt,
		"track_links":   req.TrackLinks,
		"updated_by_id": userID,
	}

	if req.TemplateID != "" {
		templateID, err := uuid.Parse(req.TemplateID)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid template ID", nil, "")
		}
		updates["template_id"] = templateID
	}

	if req.WhatsAppAccount != "" {
		updates["whats_app_account"] = req.WhatsAppAccount
	}

	if err := a.DB.Model(campaign).Updates(updates).Error; err != nil {
		a.Log.Error("Failed to update campaign", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update campaign", nil, "")
	}

	// Reload campaign
	a.DB.Where("id = ?", id).Preload("Template").Preload("Creator").Preload("UpdatedBy").First(campaign)

	a.logAudit(orgID, userID,
		"campaign", campaign.ID, models.AuditActionUpdated, &oldCampaign, campaign)

	response := CampaignResponse{
		ID:                  campaign.ID,
		Name:                campaign.Name,
		WhatsAppAccount:     campaign.WhatsAppAccount,
		TemplateID:          campaign.TemplateID,
		HeaderMediaID:       campaign.HeaderMediaID,
		HeaderMediaFilename: campaign.HeaderMediaFilename,
		HeaderMediaMimeType: campaign.HeaderMediaMimeType,
		Status:              campaign.Status,
		TotalRecipients:     campaign.TotalRecipients,
		SentCount:           campaign.SentCount,
		DeliveredCount:      campaign.DeliveredCount,
		ReadCount:           campaign.ReadCount,
		FailedCount:         campaign.FailedCount,
		TrackLinks:          campaign.TrackLinks,
		ClickedCount:        campaign.ClickedCount,
		ScheduledAt:         campaign.ScheduledAt,
		CreatedAt:           campaign.CreatedAt,
		UpdatedAt:           campaign.UpdatedAt,
	}
	if campaign.Template != nil {
		response.TemplateName = campaign.Template.Name
	}
	if campaign.Creator != nil {
		response.CreatedByName = campaign.Creator.FullName
	}
	if campaign.UpdatedBy != nil {
		response.UpdatedByName = campaign.UpdatedBy.FullName
	}

	return r.SendEnvelope(response)
}

// DeleteCampaign implements campaign deletion
func (a *App) DeleteCampaign(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	campaign, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, id, orgID, "Campaign")
	if err != nil {
		return nil
	}

	// Don't allow deletion of running campaigns
	if campaign.Status == models.CampaignStatusProcessing || campaign.Status == models.CampaignStatusQueued {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Cannot delete running campaign", nil, "")
	}

	// Delete recipients first
	if err := a.DB.Where("campaign_id = ?", id).Delete(&models.BulkMessageRecipient{}).Error; err != nil {
		a.Log.Error("Failed to delete campaign recipients", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete campaign", nil, "")
	}

	// Delete campaign
	if err := a.DB.Delete(campaign).Error; err != nil {
		a.Log.Error("Failed to delete campaign", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete campaign", nil, "")
	}

	a.logAudit(orgID, userID,
		"campaign", id, models.AuditActionDeleted, campaign, nil)

	a.Log.Info("Campaign deleted", "campaign_id", id)

	return r.SendEnvelope(map[string]any{
		"message": "Campaign deleted successfully",
	})
}

// StartCampaign implements starting a campaign
func (a *App) StartCampaign(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	campaign, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, id, orgID, "Campaign")
	if err != nil {
		return nil
	}

	// Check if campaign can be started
	if campaign.Status != models.CampaignStatusDraft && campaign.Status != models.CampaignStatusScheduled && campaign.Status != models.CampaignStatusPaused {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Campaign cannot be started in current state", nil, "")
	}

	if err := a.launchCampaign(r.RequestCtx, campaign); err != nil {
		if sendBillingError(r, err) {
			return nil
		}
		var le *campaignLaunchError
		if errors.As(err, &le) {
			return r.SendErrorEnvelope(le.status, le.msg, nil, "")
		}
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to start campaign", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"message": "Campaign started",
		"status":  models.CampaignStatusProcessing,
	})
}

// campaignLaunchError is a user-facing reason a campaign could not start.
type campaignLaunchError struct {
	status int
	msg    string
}

func (e *campaignLaunchError) Error() string { return e.msg }

// launchCampaign queues every pending recipient after template and wallet checks.
// Used by the Start button and by the scheduler for scheduled campaigns.
func (a *App) launchCampaign(ctx context.Context, campaign *models.BulkMessageCampaign) error {
	id, orgID := campaign.ID, campaign.OrganizationID
	// Get all pending recipients
	var recipients []models.BulkMessageRecipient
	if err := a.DB.Where("campaign_id = ? AND status = ?", id, models.MessageStatusPending).Find(&recipients).Error; err != nil {
		a.Log.Error("Failed to load recipients", "error", err)
		return &campaignLaunchError{fasthttp.StatusInternalServerError, "Failed to load recipients"}
	}

	if len(recipients) == 0 {
		return &campaignLaunchError{fasthttp.StatusBadRequest, "Campaign has no pending recipients"}
	}

	// Validate template still exists
	var template models.Template
	if campaign.TemplateID != uuid.Nil {
		if err := a.DB.Where("id = ? AND organization_id = ?", campaign.TemplateID, orgID).First(&template).Error; err != nil {
			return &campaignLaunchError{fasthttp.StatusBadRequest, "Campaign template no longer exists"}
		}
	}

	// Wallet / plan checks: the balance must cover every pending recipient
	if err := a.Billing.CheckCanSend(orgID, "", ""); err != nil {
		return err
	}
	phones := make([]string, len(recipients))
	for i, rcpt := range recipients {
		phones[i] = rcpt.PhoneNumber
	}
	if err := a.Billing.CheckBalanceFor(orgID, a.Billing.EstimateCost(orgID, phones, template.Category)); err != nil {
		return err
	}

	// Update status to processing
	now := time.Now()
	updates := map[string]any{
		"status":     models.CampaignStatusProcessing,
		"started_at": now,
	}

	if err := a.DB.Model(campaign).Updates(updates).Error; err != nil {
		a.Log.Error("Failed to start campaign", "error", err)
		return &campaignLaunchError{fasthttp.StatusInternalServerError, "Failed to start campaign"}
	}

	a.Log.Info("Campaign started", "campaign_id", id, "recipients", len(recipients))

	// Enqueue all recipients as individual jobs for parallel processing
	jobs := make([]*queue.RecipientJob, len(recipients))
	for i, recipient := range recipients {
		jobs[i] = &queue.RecipientJob{
			CampaignID:     id,
			RecipientID:    recipient.ID,
			OrganizationID: orgID,
			PhoneNumber:    recipient.PhoneNumber,
			RecipientName:  recipient.RecipientName,
			TemplateParams: recipient.TemplateParams,
			HeaderParams:   recipient.HeaderParams,
		}
	}

	if err := a.Queue.EnqueueRecipients(ctx, jobs); err != nil {
		a.Log.Error("Failed to enqueue recipients", "error", err)
		// Revert status on failure
		a.DB.Model(campaign).Update("status", models.CampaignStatusDraft)
		return &campaignLaunchError{fasthttp.StatusInternalServerError, "Failed to queue recipients"}
	}

	a.Log.Info("Recipients enqueued for processing", "campaign_id", id, "count", len(jobs))

	return nil
}

// PauseCampaign implements pausing a campaign
func (a *App) PauseCampaign(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	campaign, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, id, orgID, "Campaign")
	if err != nil {
		return nil
	}

	if campaign.Status != models.CampaignStatusProcessing && campaign.Status != models.CampaignStatusQueued {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Campaign is not running", nil, "")
	}

	if err := a.DB.Model(campaign).Update("status", models.CampaignStatusPaused).Error; err != nil {
		a.Log.Error("Failed to pause campaign", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to pause campaign", nil, "")
	}

	a.Log.Info("Campaign paused", "campaign_id", id)

	return r.SendEnvelope(map[string]any{
		"message": "Campaign paused",
		"status":  models.CampaignStatusPaused,
	})
}

// CancelCampaign implements cancelling a campaign
func (a *App) CancelCampaign(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	campaign, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, id, orgID, "Campaign")
	if err != nil {
		return nil
	}

	if campaign.Status == models.CampaignStatusCompleted || campaign.Status == models.CampaignStatusCancelled {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Campaign already finished", nil, "")
	}

	if err := a.DB.Model(campaign).Update("status", models.CampaignStatusCancelled).Error; err != nil {
		a.Log.Error("Failed to cancel campaign", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to cancel campaign", nil, "")
	}

	a.Log.Info("Campaign cancelled", "campaign_id", id)

	return r.SendEnvelope(map[string]any{
		"message": "Campaign cancelled",
		"status":  models.CampaignStatusCancelled,
	})
}

// RetryFailed retries sending to all failed recipients
func (a *App) RetryFailed(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	campaign, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, id, orgID, "Campaign")
	if err != nil {
		return nil
	}

	// Only allow retry on completed or paused campaigns
	if campaign.Status != models.CampaignStatusCompleted && campaign.Status != models.CampaignStatusPaused && campaign.Status != models.CampaignStatusFailed {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Can only retry failed messages on completed, paused, or failed campaigns", nil, "")
	}

	// Get failed recipients
	var failedRecipients []models.BulkMessageRecipient
	if err := a.DB.Where("campaign_id = ? AND status = ?", id, models.MessageStatusFailed).Find(&failedRecipients).Error; err != nil {
		a.Log.Error("Failed to load failed recipients", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load failed recipients", nil, "")
	}

	if len(failedRecipients) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "No failed messages to retry", nil, "")
	}

	// Reset failed recipients to pending
	if err := a.DB.Model(&models.BulkMessageRecipient{}).
		Where("campaign_id = ? AND status = ?", id, models.MessageStatusFailed).
		Updates(map[string]any{
			"status":        models.MessageStatusPending,
			"error_message": "",
		}).Error; err != nil {
		a.Log.Error("Failed to reset failed recipients", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to reset failed recipients", nil, "")
	}

	// Reset failed messages in messages table to pending
	if err := a.DB.Model(&models.Message{}).
		Where("metadata->>'campaign_id' = ? AND status = ?", id.String(), models.MessageStatusFailed).
		Updates(map[string]any{
			"status":        models.MessageStatusPending,
			"error_message": "",
		}).Error; err != nil {
		a.Log.Error("Failed to reset failed messages", "error", err)
	}

	// Recalculate campaign stats from messages table
	a.recalculateCampaignStats(id)

	// Update campaign status to processing
	if err := a.DB.Model(campaign).Update("status", models.CampaignStatusProcessing).Error; err != nil {
		a.Log.Error("Failed to update campaign status", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update campaign", nil, "")
	}

	a.Log.Info("Retrying failed messages", "campaign_id", id, "failed_count", len(failedRecipients))

	// Enqueue failed recipients as individual jobs for parallel processing
	jobs := make([]*queue.RecipientJob, len(failedRecipients))
	for i, recipient := range failedRecipients {
		jobs[i] = &queue.RecipientJob{
			CampaignID:     id,
			RecipientID:    recipient.ID,
			OrganizationID: orgID,
			PhoneNumber:    recipient.PhoneNumber,
			RecipientName:  recipient.RecipientName,
			TemplateParams: recipient.TemplateParams,
			HeaderParams:   recipient.HeaderParams,
		}
	}

	if err := a.Queue.EnqueueRecipients(r.RequestCtx, jobs); err != nil {
		a.Log.Error("Failed to enqueue recipients for retry", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to queue recipients", nil, "")
	}

	a.Log.Info("Failed recipients enqueued for retry", "campaign_id", id, "count", len(jobs))

	return r.SendEnvelope(map[string]any{
		"message":     "Retrying failed messages",
		"retry_count": len(failedRecipients),
		"status":      models.CampaignStatusProcessing,
	})
}

// ImportRecipients implements adding recipients to a campaign
func (a *App) ImportRecipients(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	campaign, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, id, orgID, "Campaign")
	if err != nil {
		return nil
	}

	if campaign.Status != models.CampaignStatusDraft {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Can only add recipients to draft campaigns", nil, "")
	}

	var req struct {
		Recipients []RecipientRequest `json:"recipients" validate:"required"`
	}
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	// Create recipients
	recipients := make([]models.BulkMessageRecipient, len(req.Recipients))
	for i, rec := range req.Recipients {
		recipients[i] = models.BulkMessageRecipient{
			CampaignID:     id,
			PhoneNumber:    rec.PhoneNumber,
			RecipientName:  rec.RecipientName,
			TemplateParams: models.JSONB(rec.TemplateParams),
			HeaderParams:   models.JSONB(rec.HeaderParams),
			Status:         models.MessageStatusPending,
		}
	}

	if err := a.DB.Create(&recipients).Error; err != nil {
		a.Log.Error("Failed to add recipients", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to add recipients", nil, "")
	}

	// Update total recipients count
	var totalCount int64
	a.DB.Model(&models.BulkMessageRecipient{}).Where("campaign_id = ?", id).Count(&totalCount)
	a.DB.Model(campaign).Update("total_recipients", totalCount)

	a.Log.Info("Recipients added to campaign", "campaign_id", id, "count", len(req.Recipients))

	// Log recipient addition as audit
	phoneNumbers := make([]string, len(req.Recipients))
	for i, rec := range req.Recipients {
		phoneNumbers[i] = rec.PhoneNumber
	}
	a.logAudit(orgID, userID,
		"campaign", id, models.AuditActionUpdated, nil, nil,
		map[string]any{
			"field":     "recipients_added",
			"old_value": nil,
			"new_value": fmt.Sprintf("%d recipients added", len(req.Recipients)),
		})

	return r.SendEnvelope(map[string]any{
		"message":          "Recipients added successfully",
		"added_count":      len(req.Recipients),
		"total_recipients": totalCount,
	})
}

// GetCampaignRecipients implements listing campaign recipients
func (a *App) GetCampaignRecipients(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	// Verify campaign belongs to org
	_, err = findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, id, orgID, "Campaign")
	if err != nil {
		return nil
	}

	var recipients []models.BulkMessageRecipient
	if err := a.DB.Where("campaign_id = ?", id).Order("created_at ASC").Find(&recipients).Error; err != nil {
		a.Log.Error("Failed to list recipients", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list recipients", nil, "")
	}

	if a.ShouldMaskPhoneNumbers(orgID) {
		for i := range recipients {
			recipients[i].PhoneNumber = utils.MaskPhoneNumber(recipients[i].PhoneNumber)
			recipients[i].RecipientName = utils.MaskIfPhoneNumber(recipients[i].RecipientName)
		}
	}

	return r.SendEnvelope(map[string]any{
		"recipients": recipients,
		"total":      len(recipients),
	})
}

// DeleteCampaignRecipient deletes a single recipient from a campaign
func (a *App) DeleteCampaignRecipient(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	campaignUUID, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	recipientUUID, err := parsePathUUID(r, "recipientId", "recipient")
	if err != nil {
		return nil
	}

	// Verify campaign belongs to org and is in draft status
	campaign, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, campaignUUID, orgID, "Campaign")
	if err != nil {
		return nil
	}

	if campaign.Status != models.CampaignStatusDraft {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Can only delete recipients from draft campaigns", nil, "")
	}

	// Load recipient for audit before deleting
	var recipient models.BulkMessageRecipient
	a.DB.Where("id = ? AND campaign_id = ?", recipientUUID, campaignUUID).First(&recipient)

	// Delete recipient
	result := a.DB.Where("id = ? AND campaign_id = ?", recipientUUID, campaignUUID).Delete(&models.BulkMessageRecipient{})
	if result.Error != nil {
		a.Log.Error("Failed to delete recipient", "error", result.Error)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete recipient", nil, "")
	}

	if result.RowsAffected == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Recipient not found", nil, "")
	}

	// Update campaign recipient count
	a.DB.Model(campaign).Update("total_recipients", gorm.Expr("total_recipients - 1"))

	a.logAudit(orgID, userID,
		"campaign", campaignUUID, models.AuditActionUpdated, nil, nil,
		map[string]any{
			"field":     "recipient_removed",
			"old_value": recipient.PhoneNumber,
			"new_value": nil,
		})

	return r.SendEnvelope(map[string]any{
		"message": "Recipient deleted successfully",
	})
}

// UploadCampaignMedia uploads media for a campaign's template header
func (a *App) UploadCampaignMedia(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	campaignUUID, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	// Get campaign with template
	var campaign models.BulkMessageCampaign
	if err := a.DB.Where("id = ? AND organization_id = ?", campaignUUID, orgID).
		Preload("Template").
		First(&campaign).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Campaign not found", nil, "")
	}

	// Only allow media upload for draft campaigns
	if campaign.Status != models.CampaignStatusDraft {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Can only upload media for draft campaigns", nil, "")
	}

	// Verify template has media header
	if campaign.Template == nil || campaign.Template.HeaderType == "" || campaign.Template.HeaderType == "TEXT" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Template does not have a media header", nil, "")
	}

	// Get WhatsApp account
	account, err := a.resolveWhatsAppAccount(orgID, campaign.WhatsAppAccount)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "WhatsApp account not found", nil, "")
	}

	// Parse multipart form
	form, err := r.RequestCtx.MultipartForm()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid multipart form", nil, "")
	}

	files := form.File["file"]
	if len(files) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "No file provided", nil, "")
	}

	fileHeader := files[0]
	file, err := fileHeader.Open()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Failed to open file", nil, "")
	}
	defer func() { _ = file.Close() }()

	// Read file content (limit to 16MB)
	const maxMediaSize = 16 << 20 // 16MB
	data, err := io.ReadAll(io.LimitReader(file, maxMediaSize+1))
	if err != nil {
		a.Log.Error("Failed to read file", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to read file", nil, "")
	}
	if len(data) > maxMediaSize {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "File too large. Maximum size is 16MB", nil, "")
	}

	// Determine and validate MIME type
	mimeType := fileHeader.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	allowedMIME := map[string]bool{
		"image/jpeg": true, "image/png": true, "image/webp": true,
		"video/mp4": true, "video/3gpp": true,
		"audio/aac": true, "audio/mp4": true, "audio/mpeg": true, "audio/ogg": true,
		"application/pdf": true, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	}
	if !allowedMIME[mimeType] {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Unsupported file type: "+mimeType, nil, "")
	}

	// Upload to WhatsApp
	waAccount := a.toWhatsAppAccount(account)

	ctx := r.RequestCtx
	mediaID, err := a.WhatsApp.UploadMedia(ctx, waAccount, data, mimeType, fileHeader.Filename)
	if err != nil {
		a.Log.Error("Failed to upload media to WhatsApp", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to upload media to WhatsApp", nil, "")
	}

	// Save file locally for preview
	localPath, err := a.saveCampaignMedia(campaignUUID.String(), data, mimeType)
	if err != nil {
		a.Log.Error("Failed to save media locally", "error", err)
		// Don't fail the request, just log the error - preview won't work
	}

	// Update campaign with media ID, filename, mime type, and local path
	updates := map[string]any{
		"header_media_id":         mediaID,
		"header_media_filename":   sanitizeFilename(fileHeader.Filename),
		"header_media_mime_type":  mimeType,
		"header_media_local_path": localPath,
	}
	if err := a.DB.Model(&campaign).Updates(updates).Error; err != nil {
		a.Log.Error("Failed to update campaign with media info", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save media info", nil, "")
	}

	a.Log.Info("Campaign media uploaded", "campaign_id", campaignUUID, "media_id", mediaID, "filename", fileHeader.Filename, "local_path", localPath)

	return r.SendEnvelope(map[string]any{
		"media_id":   mediaID,
		"filename":   fileHeader.Filename,
		"mime_type":  mimeType,
		"local_path": localPath,
		"message":    "Media uploaded successfully",
	})
}

// saveCampaignMedia saves uploaded media locally for preview
func (a *App) saveCampaignMedia(campaignID string, data []byte, mimeType string) (string, error) {
	// Determine file extension
	ext := getExtensionFromMimeType(mimeType)
	if ext == "" {
		ext = ".bin"
	}

	// Create campaigns media directory
	subdir := "campaigns"
	if err := a.ensureMediaDir(subdir); err != nil {
		return "", fmt.Errorf("failed to create media directory: %w", err)
	}

	// Generate filename using campaign ID
	filename := campaignID + ext
	filePath := filepath.Join(a.getMediaStoragePath(), subdir, filename)

	// Save file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to save media file: %w", err)
	}

	// Return relative path for storage
	relativePath := filepath.Join(subdir, filename)
	a.Log.Info("Campaign media saved locally", "path", relativePath, "size", len(data))

	return relativePath, nil
}

// ServeCampaignMedia serves campaign media files for preview
func (a *App) ServeCampaignMedia(r *fastglue.Request) error {
	// Get auth context
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	// Get campaign ID from URL
	campaignUUID, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	// Find campaign and verify access
	campaign, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, campaignUUID, orgID, "Campaign")
	if err != nil {
		return nil
	}

	// Check if campaign has media
	if campaign.HeaderMediaLocalPath == "" {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "No media found", nil, "")
	}

	// Security: prevent directory traversal and symlink attacks
	filePath := filepath.Clean(campaign.HeaderMediaLocalPath)
	baseDir, err := filepath.Abs(a.getMediaStoragePath())
	if err != nil {
		a.Log.Error("Storage configuration error", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Storage configuration error", nil, "")
	}
	fullPath, err := filepath.Abs(filepath.Join(baseDir, filePath))
	if err != nil || !strings.HasPrefix(fullPath, baseDir+string(os.PathSeparator)) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid file path", nil, "")
	}

	// Reject symlinks
	info, err := os.Lstat(fullPath)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "File not found", nil, "")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid file path", nil, "")
	}

	// Read file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		a.Log.Error("Failed to read media file", "path", fullPath, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to read file", nil, "")
	}

	// Use stored mime type or determine from extension
	contentType := campaign.HeaderMediaMimeType
	if contentType == "" {
		ext := strings.ToLower(filepath.Ext(filePath))
		contentType = getMimeTypeFromExtension(ext)
	}

	r.RequestCtx.Response.Header.Set("Content-Type", contentType)
	r.RequestCtx.Response.Header.Set("Cache-Control", "private, max-age=3600")
	r.RequestCtx.SetBody(data)

	return nil
}

// getMimeTypeFromExtension returns MIME type from file extension
func getMimeTypeFromExtension(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".3gp":
		return "video/3gpp"
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}

// incrementCampaignStat increments the appropriate campaign counter based on status
func (a *App) incrementCampaignStat(campaignID string, status string) {
	campaignUUID, err := uuid.Parse(campaignID)
	if err != nil {
		a.Log.Error("Invalid campaign ID for stats update", "campaign_id", campaignID)
		return
	}

	var column string
	switch models.MessageStatus(status) {
	case models.MessageStatusDelivered:
		column = "delivered_count"
	case models.MessageStatusRead:
		column = "read_count"
	case models.MessageStatusFailed:
		column = "failed_count"
	default:
		// sent is already counted during processCampaign
		return
	}

	var campaign models.BulkMessageCampaign
	campaign.ID = campaignUUID

	// atomic update and return updated record
	result := a.DB.Model(&campaign).
		Clauses(clause.Returning{}).
		Update(column, gorm.Expr(column+" + 1"))

	if result.Error != nil {
		a.Log.Error("Failed to increment campaign stat", "error", result.Error, "campaign_id", campaignID, "column", column)
		return
	}

	// Broadcast stats update via WebSocket
	if a.WSHub != nil && result.RowsAffected > 0 {
		a.WSHub.BroadcastToOrg(campaign.OrganizationID, websocket.WSMessage{
			Type: websocket.TypeCampaignStatsUpdate,
			Payload: map[string]any{
				"campaign_id":     campaignID,
				"status":          campaign.Status,
				"sent_count":      campaign.SentCount,
				"delivered_count": campaign.DeliveredCount,
				"read_count":      campaign.ReadCount,
				"failed_count":    campaign.FailedCount,
			},
		})
	}
}

// recalculateCampaignStats recalculates all campaign stats from messages table
func (a *App) recalculateCampaignStats(campaignID uuid.UUID) {
	var stats struct {
		Sent      int64
		Delivered int64
		Read      int64
		Failed    int64
	}

	if err := a.DB.Model(&models.Message{}).
		Where("metadata->>'campaign_id' = ?", campaignID.String()).
		Select(`
			COUNT(CASE WHEN status IN ('sent','delivered','read') THEN 1 END) as sent,
			COUNT(CASE WHEN status IN ('delivered','read') THEN 1 END) as delivered,
			COUNT(CASE WHEN status = 'read' THEN 1 END) as read,
			COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed
		`).Scan(&stats).Error; err != nil {
		a.Log.Error("Failed to scan campaign message stats", "error", err, "campaign_id", campaignID)
		return
	}

	if err := a.DB.Model(&models.BulkMessageCampaign{}).Where("id = ?", campaignID).
		Updates(map[string]any{
			"sent_count":      stats.Sent,
			"delivered_count": stats.Delivered,
			"read_count":      stats.Read,
			"failed_count":    stats.Failed,
		}).Error; err != nil {
		a.Log.Error("Failed to recalculate campaign stats", "error", err, "campaign_id", campaignID)
	}
}

// sanitizeFilename removes path separators, dangerous characters, and truncates length.
var safeFilenameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func sanitizeFilename(name string) string {
	// Strip any path component
	name = filepath.Base(name)
	// Replace unsafe characters
	name = safeFilenameRe.ReplaceAllString(name, "_")
	// Truncate to 255 chars
	if len(name) > 255 {
		name = name[:255]
	}
	if name == "" || name == "." || name == ".." {
		name = "unnamed"
	}
	return name
}

// campaignRepliedSQL matches recipients who wrote back after the broadcast reached them.
const campaignRepliedSQL = `EXISTS (SELECT 1 FROM messages m JOIN contacts c ON c.id = m.contact_id
	WHERE c.organization_id = ? AND c.phone_number = bulk_message_recipients.phone_number
	AND m.direction = 'incoming' AND m.created_at > bulk_message_recipients.sent_at)`

// fillCampaignListStats adds the still-processing and replied counts for one page of campaigns.
func (a *App) fillCampaignListStats(orgID uuid.UUID, list []CampaignResponse) {
	if len(list) == 0 {
		return
	}
	ids := make([]uuid.UUID, len(list))
	for i, c := range list {
		ids[i] = c.ID
	}
	var rows []struct {
		CampaignID uuid.UUID
		Pending    int
		Replied    int
	}
	if err := a.DB.Model(&models.BulkMessageRecipient{}).
		Select(`campaign_id,
			COUNT(*) FILTER (WHERE status = ?) AS pending,
			COUNT(*) FILTER (WHERE sent_at IS NOT NULL AND `+campaignRepliedSQL+`) AS replied`,
			models.MessageStatusPending, orgID).
		Where("campaign_id IN ?", ids).
		Group("campaign_id").Scan(&rows).Error; err != nil {
		a.Log.Error("Failed to count campaign replies", "error", err)
		return
	}
	byID := make(map[uuid.UUID]int, len(rows))
	for i, row := range rows {
		byID[row.CampaignID] = i
	}
	for i := range list {
		if j, ok := byID[list[i].ID]; ok {
			list[i].PendingCount = rows[j].Pending
			list[i].RepliedCount = rows[j].Replied
		}
	}
}

// CampaignLogEntry is one recipient of any broadcast, for the delivery log.
type CampaignLogEntry struct {
	ID            uuid.UUID            `json:"id"`
	CampaignID    uuid.UUID            `json:"campaign_id"`
	CampaignName  string               `json:"campaign_name"`
	PhoneNumber   string               `json:"phone_number"`
	RecipientName string               `json:"recipient_name"`
	Status        models.MessageStatus `json:"status"`
	ErrorMessage  string               `json:"error_message,omitempty"`
	SentAt        *time.Time           `json:"sent_at,omitempty"`
	DeliveredAt   *time.Time           `json:"delivered_at,omitempty"`
	ReadAt        *time.Time           `json:"read_at,omitempty"`
	CreatedAt     time.Time            `json:"created_at"`
}

// ListCampaignLogs lists every broadcast message across campaigns, newest first.
func (a *App) ListCampaignLogs(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionRead)
	if err != nil {
		return nil
	}
	pg := parsePagination(r)
	args := r.RequestCtx.QueryArgs()

	q := a.DB.Table("bulk_message_recipients r").
		Joins("JOIN bulk_message_campaigns c ON c.id = r.campaign_id").
		Where("c.organization_id = ? AND c.deleted_at IS NULL AND r.deleted_at IS NULL", orgID)
	if status := string(args.Peek("status")); status != "" {
		q = q.Where("r.status = ?", status)
	}
	if id := string(args.Peek("campaign_id")); id != "" {
		if cid, err := uuid.Parse(id); err == nil {
			q = q.Where("r.campaign_id = ?", cid)
		}
	}
	if search := strings.TrimSpace(string(args.Peek("search"))); search != "" {
		like := "%" + search + "%"
		q = q.Where("r.phone_number ILIKE ? OR r.recipient_name ILIKE ? OR c.name ILIKE ?", like, like, like)
	}
	if from, ok := parseDateParam(r, "from"); ok {
		q = q.Where("COALESCE(r.sent_at, r.created_at) >= ?", from)
	}
	if to, ok := parseDateParam(r, "to"); ok {
		q = q.Where("COALESCE(r.sent_at, r.created_at) <= ?", endOfDay(to))
	}

	if string(args.Peek("format")) == "csv" {
		return a.exportCampaignLogs(r, orgID, q)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		a.Log.Error("Failed to count campaign logs", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load logs", nil, "")
	}
	var logs []CampaignLogEntry
	if err := pg.Apply(selectCampaignLogs(q)).Scan(&logs).Error; err != nil {
		a.Log.Error("Failed to list campaign logs", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load logs", nil, "")
	}
	if logs == nil {
		logs = []CampaignLogEntry{}
	}
	if a.ShouldMaskPhoneNumbers(orgID) {
		for i := range logs {
			logs[i].PhoneNumber = utils.MaskPhoneNumber(logs[i].PhoneNumber)
			logs[i].RecipientName = utils.MaskIfPhoneNumber(logs[i].RecipientName)
		}
	}
	return r.SendEnvelope(listEnvelope("logs", logs, total, pg))
}

func selectCampaignLogs(q *gorm.DB) *gorm.DB {
	return q.Select(`r.id, r.campaign_id, c.name AS campaign_name, r.phone_number, r.recipient_name,
		r.status, r.error_message, r.sent_at, r.delivered_at, r.read_at, r.created_at`).
		Order("COALESCE(r.sent_at, r.created_at) DESC")
}

// maxLogExportRows keeps a single export to a size spreadsheets open comfortably.
const maxLogExportRows = 50000

// exportCampaignLogs downloads the filtered delivery log as a CSV file.
func (a *App) exportCampaignLogs(r *fastglue.Request, orgID uuid.UUID, q *gorm.DB) error {
	var logs []CampaignLogEntry
	if err := selectCampaignLogs(q).Limit(maxLogExportRows).Scan(&logs).Error; err != nil {
		a.Log.Error("Failed to export campaign logs", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to export logs", nil, "")
	}
	mask := a.ShouldMaskPhoneNumbers(orgID)
	stamp := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format(time.RFC3339)
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Campaign", "Phone", "Name", "Status", "Sent at", "Delivered at", "Read at", "Error"})
	for _, l := range logs {
		phone, name := l.PhoneNumber, l.RecipientName
		if mask {
			phone, name = utils.MaskPhoneNumber(phone), utils.MaskIfPhoneNumber(name)
		}
		_ = w.Write([]string{csvSafe(l.CampaignName), phone, csvSafe(name), string(l.Status),
			stamp(l.SentAt), stamp(l.DeliveredAt), stamp(l.ReadAt), csvSafe(l.ErrorMessage)})
	}
	w.Flush()
	r.RequestCtx.Response.Header.Set("Content-Type", "text/csv; charset=utf-8")
	r.RequestCtx.Response.Header.Set("Content-Disposition", `attachment; filename="broadcast-logs-`+time.Now().Format("2006-01-02")+`.csv"`)
	r.RequestCtx.SetBody(buf.Bytes())
	return nil
}

// csvSafe stops spreadsheet apps from running free-text cells as formulas.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}
