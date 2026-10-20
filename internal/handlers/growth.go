package handlers

import (
	"crypto/rand"
	"errors"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Click-to-WhatsApp ad attribution
// ---------------------------------------------------------------------------

// IncomingReferral is the "referral" object Meta adds to the first message a
// customer sends after tapping a Click-to-WhatsApp ad or post.
type IncomingReferral struct {
	SourceURL  string `json:"source_url"`
	SourceID   string `json:"source_id"`
	SourceType string `json:"source_type"`
	Headline   string `json:"headline"`
	Body       string `json:"body"`
	MediaType  string `json:"media_type"`
	ImageURL   string `json:"image_url"`
	VideoURL   string `json:"video_url"`
	CtwaClid   string `json:"ctwa_clid"`
}

func (a *App) recordAdReferral(account *models.WhatsAppAccount, contact *models.Contact, wamid string, ref *IncomingReferral) {
	if ref == nil || (ref.SourceID == "" && ref.SourceURL == "" && ref.Headline == "") {
		return
	}
	now := time.Now()
	a.DB.Create(&models.AdReferral{
		OrganizationID:  account.OrganizationID,
		ContactID:       contact.ID,
		WhatsAppAccount: account.Name,
		SourceID:        truncateRunes(ref.SourceID, 100),
		SourceType:      truncateRunes(ref.SourceType, 20),
		SourceURL:       ref.SourceURL,
		Headline:        truncateRunes(ref.Headline, 500),
		Body:            truncateRunes(ref.Body, 2000),
		CtwaClid:        truncateRunes(ref.CtwaClid, 255),
		MessageWAMID:    wamid,
	})
	a.DB.Model(contact).Updates(map[string]any{
		"ad_headline":   truncateRunes(firstNonEmpty(ref.Headline, ref.SourceURL, ref.SourceID), 500),
		"ad_source_url": ref.SourceURL,
		"ad_clicked_at": now,
	})
	a.Log.Info("Lead from Click-to-WhatsApp", "contact_id", contact.ID, "source_id", ref.SourceID)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// GetAdAnalytics reports leads per Click-to-WhatsApp ad for a period.
func (a *App) GetAdAnalytics(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceAnalytics, models.ActionRead)
	if err != nil {
		return nil
	}
	from, to, ok := a.periodFromQuery(r)
	if !ok {
		return nil
	}
	type adRow struct {
		SourceID   string    `json:"source_id"`
		SourceType string    `json:"source_type"`
		Headline   string    `json:"headline"`
		SourceURL  string    `json:"source_url"`
		Leads      int64     `json:"leads"`
		Clicks     int64     `json:"clicks"`
		FirstAt    time.Time `json:"first_at"`
		LastAt     time.Time `json:"last_at"`
	}
	var rows []adRow
	a.DB.Model(&models.AdReferral{}).
		Select(`source_id, MAX(source_type) AS source_type, MAX(headline) AS headline, MAX(source_url) AS source_url,
			COUNT(DISTINCT contact_id) AS leads, COUNT(*) AS clicks, MIN(created_at) AS first_at, MAX(created_at) AS last_at`).
		Where("organization_id = ? AND created_at BETWEEN ? AND ?", orgID, from, to).
		Group("source_id").
		Order("leads DESC").
		Scan(&rows)
	if rows == nil {
		rows = []adRow{}
	}
	var totalLeads int64
	a.DB.Model(&models.AdReferral{}).
		Where("organization_id = ? AND created_at BETWEEN ? AND ?", orgID, from, to).
		Distinct("contact_id").Count(&totalLeads)
	return r.SendEnvelope(map[string]any{"ads": rows, "total_leads": totalLeads})
}

// periodFromQuery reads ?from=&to= (YYYY-MM-DD), defaulting to the last 30 days.
// It writes the error response itself and returns ok=false on bad input.
func (a *App) periodFromQuery(r *fastglue.Request) (time.Time, time.Time, bool) {
	now := time.Now()
	from, to := dayStart(now).AddDate(0, 0, -29), endOfDay(dayStart(now))
	args := r.RequestCtx.QueryArgs()
	if fs, ts := string(args.Peek("from")), string(args.Peek("to")); fs != "" || ts != "" {
		f, t, msg := parseDateRange(fs, ts)
		if msg != "" {
			_ = r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
			return from, to, false
		}
		from, to = f, t
	}
	return from, to, true
}

// ---------------------------------------------------------------------------
// Tracked links
// ---------------------------------------------------------------------------

const linkCodeAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newLinkCode() string {
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(linkCodeAlphabet))))
		b[i] = linkCodeAlphabet[n.Int64()]
	}
	return string(b)
}

// validTargetURL only allows absolute http(s) links, so short links can't carry javascript: or data: URLs.
func validTargetURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("enter a full http:// or https:// link")
	}
	return raw, nil
}

// publicBaseURL is where customers reach this server (for short links).
func (a *App) publicBaseURL(r *fastglue.Request) string {
	if base := strings.TrimRight(a.Config.Server.PublicURL, "/"); base != "" {
		return base
	}
	if r == nil {
		return ""
	}
	scheme := "http"
	if r.RequestCtx.IsTLS() || string(r.RequestCtx.Request.Header.Peek("X-Forwarded-Proto")) == "https" {
		scheme = "https"
	}
	return scheme + "://" + string(r.RequestCtx.Host())
}

// CreateTrackedLinkRequest creates a standalone short link.
type CreateTrackedLinkRequest struct {
	TargetURL string `json:"target_url"`
	Title     string `json:"title"`
}

type trackedLinkResponse struct {
	models.TrackedLink
	ShortURL string `json:"short_url"`
}

// CreateTrackedLink makes a short link that counts clicks.
func (a *App) CreateTrackedLink(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionWrite)
	if err != nil {
		return nil
	}
	var req CreateTrackedLinkRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	target, err := validTargetURL(req.TargetURL)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	link := models.TrackedLink{
		OrganizationID: orgID,
		Code:           newLinkCode(),
		TargetURL:      target,
		Title:          truncateRunes(strings.TrimSpace(req.Title), 255),
	}
	if err := a.DB.Create(&link).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create link", nil, "")
	}
	return r.SendEnvelope(trackedLinkResponse{link, a.publicBaseURL(r) + "/l/" + link.Code})
}

// ListTrackedLinks returns standalone short links (campaign per-recipient links are summarised on the campaign).
func (a *App) ListTrackedLinks(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionRead)
	if err != nil {
		return nil
	}
	var links []models.TrackedLink
	a.DB.Where("organization_id = ? AND campaign_id IS NULL", orgID).Order("created_at DESC").Limit(200).Find(&links)
	base := a.publicBaseURL(r)
	out := make([]trackedLinkResponse, len(links))
	for i, l := range links {
		out[i] = trackedLinkResponse{l, base + "/l/" + l.Code}
	}
	return r.SendEnvelope(map[string]any{"links": out})
}

// DeleteTrackedLink removes a standalone short link.
func (a *App) DeleteTrackedLink(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionWrite)
	if err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "link")
	if err != nil {
		return nil
	}
	res := a.DB.Where("id = ? AND organization_id = ?", id, orgID).Delete(&models.TrackedLink{})
	if res.RowsAffected == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Link not found", nil, "")
	}
	return r.SendEnvelope(map[string]any{"deleted": true})
}

// RedirectTrackedLink is the public /l/{code} endpoint: count the click, then redirect.
func (a *App) RedirectTrackedLink(r *fastglue.Request) error {
	code, _ := r.RequestCtx.UserValue("code").(string)
	var link models.TrackedLink
	if code == "" || a.DB.Where("code = ?", code).First(&link).Error != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		r.RequestCtx.SetBodyString("Link not found")
		return nil
	}
	// Link-preview bots (WhatsApp, Facebook, Slack…) fetch the URL when a message is
	// shown; counting them would make every delivered message look like a click.
	if !isPreviewBot(string(r.RequestCtx.UserAgent())) {
		a.countLinkClick(&link)
	}
	r.RequestCtx.Redirect(link.TargetURL, fasthttp.StatusFound)
	return nil
}

func isPreviewBot(ua string) bool {
	ua = strings.ToLower(ua)
	for _, bot := range []string{"whatsapp", "facebookexternalhit", "facebot", "slackbot", "twitterbot", "telegrambot", "linkedinbot", "discordbot", "googlebot", "bingbot", "skypeuripreview"} {
		if strings.Contains(ua, bot) {
			return true
		}
	}
	return false
}

func (a *App) countLinkClick(link *models.TrackedLink) {
	now := time.Now()
	first := link.FirstClickedAt == nil
	updates := map[string]any{"clicks": gorm.Expr("clicks + 1"), "last_clicked_at": now}
	if first {
		updates["first_clicked_at"] = now
	}
	a.DB.Model(link).Updates(updates)
	// A recipient's first click counts once toward the campaign's "clicked" total.
	if first && link.CampaignID != nil {
		a.DB.Model(&models.BulkMessageCampaign{}).Where("id = ?", *link.CampaignID).
			UpdateColumn("clicked_count", gorm.Expr("clicked_count + 1"))
	}
}

// ---------------------------------------------------------------------------
// Campaign retargeting
// ---------------------------------------------------------------------------

// RetargetRequest creates a follow-up campaign for part of an earlier campaign's audience.
type RetargetRequest struct {
	Audience   string `json:"audience"` // not_read, read, failed, clicked, not_clicked, replied, not_replied
	Name       string `json:"name"`
	TemplateID string `json:"template_id"` // optional; defaults to the original template
}

// RetargetCampaign copies matching recipients into a new draft campaign.
func (a *App) RetargetCampaign(r *fastglue.Request) error {
	orgID, userID, err := a.requireAuth(r, models.ResourceCampaigns, models.ActionWrite)
	if err != nil {
		return nil
	}
	campaignID, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}
	var req RetargetRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	source, err := findByIDAndOrg[models.BulkMessageCampaign](a.DB, r, campaignID, orgID, "Campaign")
	if err != nil {
		return nil
	}

	q := a.DB.Model(&models.BulkMessageRecipient{}).Where("bulk_message_recipients.campaign_id = ?", source.ID)
	sentStatuses := []models.MessageStatus{models.MessageStatusSent, models.MessageStatusDelivered, models.MessageStatusRead}
	repliedSQL := `EXISTS (SELECT 1 FROM messages m JOIN contacts c ON c.id = m.contact_id
		WHERE c.organization_id = ? AND c.phone_number = bulk_message_recipients.phone_number
		AND m.direction = 'incoming' AND m.created_at > bulk_message_recipients.sent_at)`
	clickedSQL := `EXISTS (SELECT 1 FROM tracked_links l WHERE l.recipient_id = bulk_message_recipients.id AND l.clicks > 0)`
	switch req.Audience {
	case "not_read":
		q = q.Where("status IN ?", []models.MessageStatus{models.MessageStatusSent, models.MessageStatusDelivered})
	case "read":
		q = q.Where("status = ?", models.MessageStatusRead)
	case "failed":
		q = q.Where("status = ?", models.MessageStatusFailed)
	case "clicked":
		q = q.Where(clickedSQL)
	case "not_clicked":
		q = q.Where("status IN ?", sentStatuses).Where("NOT " + clickedSQL)
	case "replied":
		q = q.Where("sent_at IS NOT NULL").Where(repliedSQL, orgID)
	case "not_replied":
		q = q.Where("status IN ? AND sent_at IS NOT NULL", sentStatuses).Where("NOT "+repliedSQL, orgID)
	default:
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Choose who to retarget", nil, "")
	}
	var recipients []models.BulkMessageRecipient
	if err := q.Find(&recipients).Error; err != nil {
		a.Log.Error("Retarget query failed", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to build audience", nil, "")
	}
	if len(recipients) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "No recipients match this audience", nil, "")
	}

	templateID := source.TemplateID
	if req.TemplateID != "" {
		id, err := uuid.Parse(req.TemplateID)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid template ID", nil, "")
		}
		if _, err := findByIDAndOrg[models.Template](a.DB, r, id, orgID, "Template"); err != nil {
			return nil
		}
		templateID = id
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = truncateRunes(source.Name+" – "+strings.ReplaceAll(req.Audience, "_", " "), 255)
	}

	campaign := models.BulkMessageCampaign{
		OrganizationID:  orgID,
		WhatsAppAccount: source.WhatsAppAccount,
		Name:            name,
		TemplateID:      templateID,
		Status:          models.CampaignStatusDraft,
		TrackLinks:      source.TrackLinks,
		CreatedBy:       userID,
		UpdatedByID:     &userID,
	}
	// Keep the original header media only when the template is unchanged.
	if templateID == source.TemplateID {
		campaign.HeaderMediaID = source.HeaderMediaID
		campaign.HeaderMediaFilename = source.HeaderMediaFilename
		campaign.HeaderMediaMimeType = source.HeaderMediaMimeType
		campaign.HeaderMediaLocalPath = source.HeaderMediaLocalPath
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&campaign).Error; err != nil {
			return err
		}
		copies := make([]models.BulkMessageRecipient, len(recipients))
		for i, rc := range recipients {
			copies[i] = models.BulkMessageRecipient{
				CampaignID:     campaign.ID,
				PhoneNumber:    rc.PhoneNumber,
				RecipientName:  rc.RecipientName,
				TemplateParams: rc.TemplateParams,
				HeaderParams:   rc.HeaderParams,
				Status:         models.MessageStatusPending,
			}
		}
		if err := tx.CreateInBatches(copies, 500).Error; err != nil {
			return err
		}
		return tx.Model(&campaign).Update("total_recipients", len(copies)).Error
	})
	if err != nil {
		a.Log.Error("Failed to create retarget campaign", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create campaign", nil, "")
	}
	a.logAudit(orgID, userID, "campaign", campaign.ID, models.AuditActionCreated, nil, &campaign)
	return r.SendEnvelope(map[string]any{"id": campaign.ID, "name": campaign.Name, "total_recipients": len(recipients)})
}
