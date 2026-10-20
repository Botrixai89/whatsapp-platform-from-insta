package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// ErrContactBlocked is returned when sending to a blocked contact.
var ErrContactBlocked = errors.New("this contact is blocked; unblock them to send messages")

// OptOutSettings lets contacts unsubscribe from marketing by replying with a keyword.
type OptOutSettings struct {
	Enabled        bool     `json:"enabled"`
	OptOutKeywords []string `json:"opt_out_keywords"`
	OptInKeywords  []string `json:"opt_in_keywords"`
	OptOutReply    string   `json:"opt_out_reply"`
	OptInReply     string   `json:"opt_in_reply"`
}

// CSATSettings asks customers to rate the conversation when an agent closes it.
type CSATSettings struct {
	Enabled       bool   `json:"enabled"`
	Question      string `json:"question"`
	ThanksMessage string `json:"thanks_message"`
}

// EngagementSettings is stored under organizations.settings["engagement"].
type EngagementSettings struct {
	OptOut OptOutSettings `json:"opt_out"`
	CSAT   CSATSettings   `json:"csat"`
}

func defaultEngagementSettings() EngagementSettings {
	return EngagementSettings{
		OptOut: OptOutSettings{
			Enabled:        true,
			OptOutKeywords: []string{"STOP", "UNSUBSCRIBE", "STOP PROMOTIONS"},
			OptInKeywords:  []string{"START", "SUBSCRIBE"},
			OptOutReply:    "You have been unsubscribed from promotional messages. Reply START anytime to subscribe again.",
			OptInReply:     "You are subscribed again. Reply STOP anytime to unsubscribe.",
		},
		CSAT: CSATSettings{
			Enabled:       false,
			Question:      "Thanks for chatting with us! How would you rate your experience?",
			ThanksMessage: "Thank you for your feedback!",
		},
	}
}

// CSAT reply buttons; the ID carries the 1-5 score.
var csatButtons = []map[string]any{
	{"id": "csat_5", "title": "😊 Great"},
	{"id": "csat_3", "title": "😐 Okay"},
	{"id": "csat_1", "title": "😞 Poor"},
}

func (a *App) loadEngagementSettings(orgID uuid.UUID) EngagementSettings {
	s := defaultEngagementSettings()
	var org models.Organization
	if err := a.DB.Select("settings").Where("id = ?", orgID).First(&org).Error; err != nil || org.Settings == nil {
		return s
	}
	if raw, ok := org.Settings["engagement"]; ok {
		if b, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(b, &s)
		}
	}
	return s
}

// GetEngagementSettings returns opt-out keyword and CSAT settings.
func (a *App) GetEngagementSettings(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceSettingsGeneral, models.ActionRead)
	if err != nil {
		return nil
	}
	return r.SendEnvelope(a.loadEngagementSettings(orgID))
}

// UpdateEngagementSettings replaces opt-out keyword and CSAT settings.
func (a *App) UpdateEngagementSettings(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceSettingsGeneral, models.ActionWrite)
	if err != nil {
		return nil
	}
	var req EngagementSettings
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	req.OptOut.OptOutKeywords = cleanKeywords(req.OptOut.OptOutKeywords)
	req.OptOut.OptInKeywords = cleanKeywords(req.OptOut.OptInKeywords)
	if req.OptOut.Enabled && len(req.OptOut.OptOutKeywords) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Add at least one opt-out keyword", nil, "")
	}
	for _, k := range req.OptOut.OptOutKeywords {
		for _, in := range req.OptOut.OptInKeywords {
			if k == in {
				return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "A keyword cannot be both opt-out and opt-in: "+k, nil, "")
			}
		}
	}
	if req.CSAT.Enabled && strings.TrimSpace(req.CSAT.Question) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Enter the CSAT question", nil, "")
	}
	req.OptOut.OptOutReply = truncateRunes(strings.TrimSpace(req.OptOut.OptOutReply), 1000)
	req.OptOut.OptInReply = truncateRunes(strings.TrimSpace(req.OptOut.OptInReply), 1000)
	req.CSAT.Question = truncateRunes(strings.TrimSpace(req.CSAT.Question), 1000)
	req.CSAT.ThanksMessage = truncateRunes(strings.TrimSpace(req.CSAT.ThanksMessage), 1000)

	var org models.Organization
	if err := a.DB.Where("id = ?", orgID).First(&org).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Organization not found", nil, "")
	}
	if org.Settings == nil {
		org.Settings = models.JSONB{}
	}
	org.Settings["engagement"] = req
	if err := a.DB.Model(&org).Update("settings", org.Settings).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save settings", nil, "")
	}
	return r.SendEnvelope(req)
}

// cleanKeywords upper-cases, trims and de-duplicates keywords (max 20, 40 chars each).
func cleanKeywords(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, k := range in {
		k = strings.ToUpper(strings.Join(strings.Fields(k), " "))
		if k == "" || seen[k] || len([]rune(k)) > 40 {
			continue
		}
		seen[k] = true
		out = append(out, k)
		if len(out) == 20 {
			break
		}
	}
	return out
}

// matchesKeyword compares a whole message (not a substring) with the keywords,
// so "stop" opts out but "please don't stop the order" does not.
func matchesKeyword(text string, keywords []string) bool {
	normalized := strings.ToUpper(strings.Join(strings.Fields(strings.Trim(text, " .!\n\t")), " "))
	if normalized == "" {
		return false
	}
	for _, k := range keywords {
		if normalized == k {
			return true
		}
	}
	return false
}

// handleEngagementReply processes opt-out/opt-in keywords and CSAT answers.
// It returns true when the message was fully handled and the chatbot must not run.
func (a *App) handleEngagementReply(account *models.WhatsAppAccount, contact *models.Contact, text, buttonID string) bool {
	if strings.HasPrefix(buttonID, "csat_") {
		return a.recordCSATAnswer(account, contact, buttonID)
	}
	settings := a.loadEngagementSettings(account.OrganizationID)
	if !settings.OptOut.Enabled || buttonID != "" {
		return false
	}
	switch {
	case !contact.MarketingOptOut && matchesKeyword(text, settings.OptOut.OptOutKeywords):
		a.DB.Model(contact).Updates(map[string]any{"marketing_opt_out": true, "opt_out_source": "keyword"})
		contact.MarketingOptOut = true
		a.Log.Info("Contact opted out by keyword", "contact_id", contact.ID)
		if settings.OptOut.OptOutReply != "" {
			_ = a.sendAndSaveTextMessage(account, contact, settings.OptOut.OptOutReply)
		}
		return true
	case contact.MarketingOptOut && matchesKeyword(text, settings.OptOut.OptInKeywords):
		a.DB.Model(contact).Updates(map[string]any{"marketing_opt_out": false, "opt_out_source": ""})
		contact.MarketingOptOut = false
		a.Log.Info("Contact opted in by keyword", "contact_id", contact.ID)
		if settings.OptOut.OptInReply != "" {
			_ = a.sendAndSaveTextMessage(account, contact, settings.OptOut.OptInReply)
		}
		return true
	}
	return false
}

// sendCSATSurvey asks for a rating after an agent closes a chat. It only runs
// inside the 24-hour window, where a free-form interactive message is allowed.
func (a *App) sendCSATSurvey(contact *models.Contact) {
	settings := a.loadEngagementSettings(contact.OrganizationID)
	if !settings.CSAT.Enabled || contact.IsBlocked {
		return
	}
	if contact.LastInboundAt == nil || time.Since(*contact.LastInboundAt) >= 24*time.Hour {
		return
	}
	var account models.WhatsAppAccount
	q := a.DB.Where("organization_id = ?", contact.OrganizationID)
	if contact.WhatsAppAccount != "" {
		q = q.Where("name = ?", contact.WhatsAppAccount)
	}
	if err := q.First(&account).Error; err != nil {
		return
	}
	a.decryptAccountSecrets(&account)
	if err := a.sendAndSaveInteractiveButtons(&account, contact, settings.CSAT.Question, csatButtons); err != nil {
		a.Log.Warn("Failed to send CSAT survey", "contact_id", contact.ID, "error", err)
		return
	}
	a.DB.Create(&models.CSATResponse{
		OrganizationID:  contact.OrganizationID,
		ContactID:       contact.ID,
		AgentID:         contact.AssignedUserID,
		WhatsAppAccount: account.Name,
		SentAt:          time.Now(),
	})
}

func (a *App) recordCSATAnswer(account *models.WhatsAppAccount, contact *models.Contact, buttonID string) bool {
	rating := map[string]int{"csat_5": 5, "csat_4": 4, "csat_3": 3, "csat_2": 2, "csat_1": 1}[buttonID]
	if rating == 0 {
		return false
	}
	var survey models.CSATResponse
	err := a.DB.Where("contact_id = ? AND responded_at IS NULL AND sent_at > ?", contact.ID, time.Now().Add(-7*24*time.Hour)).
		Order("sent_at DESC").First(&survey).Error
	if err != nil {
		return false // an old or unknown survey button; treat as a normal message
	}
	now := time.Now()
	a.DB.Model(&survey).Updates(map[string]any{"rating": rating, "responded_at": now})
	// Answering the survey shouldn't reopen the chat the agent just closed.
	a.DB.Model(contact).Update("conversation_status", "closed")
	if thanks := a.loadEngagementSettings(account.OrganizationID).CSAT.ThanksMessage; thanks != "" {
		_ = a.sendAndSaveTextMessage(account, contact, thanks)
		a.DB.Model(contact).Update("conversation_status", "closed")
	}
	return true
}

// GetCSATAnalytics returns satisfaction scores for a period (?from=&to=, default 30 days).
func (a *App) GetCSATAnalytics(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceAnalytics, models.ActionRead)
	if err != nil {
		return nil
	}
	now := time.Now()
	from, to := dayStart(now).AddDate(0, 0, -29), endOfDay(dayStart(now))
	args := r.RequestCtx.QueryArgs()
	if fs, ts := string(args.Peek("from")), string(args.Peek("to")); fs != "" || ts != "" {
		f, t, msg := parseDateRange(fs, ts)
		if msg != "" {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
		}
		from, to = f, t
	}

	var totals struct {
		Sent      int64
		Responded int64
		Average   float64
		Positive  int64
		Neutral   int64
		Negative  int64
	}
	a.DB.Model(&models.CSATResponse{}).
		Select(`COUNT(*) AS sent,
			COUNT(*) FILTER (WHERE responded_at IS NOT NULL) AS responded,
			COALESCE(AVG(rating) FILTER (WHERE responded_at IS NOT NULL), 0) AS average,
			COUNT(*) FILTER (WHERE rating >= 4) AS positive,
			COUNT(*) FILTER (WHERE rating = 3) AS neutral,
			COUNT(*) FILTER (WHERE rating BETWEEN 1 AND 2) AS negative`).
		Where("organization_id = ? AND sent_at BETWEEN ? AND ?", orgID, from, to).
		Scan(&totals)

	type agentRow struct {
		AgentID   *uuid.UUID `json:"agent_id"`
		AgentName string     `json:"agent_name"`
		Responses int64      `json:"responses"`
		Average   float64    `json:"average"`
	}
	var agents []agentRow
	a.DB.Table("csat_responses c").
		Select("c.agent_id, COALESCE(u.full_name, '') AS agent_name, COUNT(*) AS responses, AVG(c.rating) AS average").
		Joins("LEFT JOIN users u ON u.id = c.agent_id").
		Where("c.organization_id = ? AND c.sent_at BETWEEN ? AND ? AND c.responded_at IS NOT NULL AND c.deleted_at IS NULL", orgID, from, to).
		Group("c.agent_id, u.full_name").
		Order("average DESC").
		Scan(&agents)
	if agents == nil {
		agents = []agentRow{}
	}
	responseRate := 0.0
	if totals.Sent > 0 {
		responseRate = float64(totals.Responded) / float64(totals.Sent)
	}
	return r.SendEnvelope(map[string]any{
		"sent":          totals.Sent,
		"responded":     totals.Responded,
		"response_rate": responseRate,
		"average":       totals.Average,
		"positive":      totals.Positive,
		"neutral":       totals.Neutral,
		"negative":      totals.Negative,
		"agents":        agents,
	})
}

// BlockContactRequest blocks or unblocks a contact.
type BlockContactRequest struct {
	Blocked bool `json:"blocked"`
}

// BlockContact stops a contact's messages from reaching the inbox and refuses sends to them.
func (a *App) BlockContact(r *fastglue.Request) error {
	orgID, userID, err := a.requireAuth(r, models.ResourceContacts, models.ActionWrite)
	if err != nil {
		return nil
	}
	contactID, err := parsePathUUID(r, "id", "contact")
	if err != nil {
		return nil
	}
	var req BlockContactRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	contact, err := findByIDAndOrg[models.Contact](a.DB, r, contactID, orgID, "Contact")
	if err != nil {
		return nil
	}
	updates := map[string]any{"is_blocked": req.Blocked}
	if req.Blocked {
		updates["bot_paused"] = true
		updates["conversation_status"] = "closed"
	}
	if err := a.DB.Model(contact).Updates(updates).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update contact", nil, "")
	}
	a.Log.Info("Contact block changed", "contact_id", contactID, "blocked", req.Blocked, "by", userID)
	return r.SendEnvelope(map[string]any{"is_blocked": req.Blocked})
}

// SendTypingIndicator shows "typing…" to the customer while an agent writes a reply.
func (a *App) SendTypingIndicator(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	contactID, err := parsePathUUID(r, "id", "contact")
	if err != nil {
		return nil
	}
	query := a.scopeAssignedContact(a.ScopeToOrg(a.DB, userID, orgID), userID, orgID)
	var contact models.Contact
	if err := query.Where("id = ?", contactID).First(&contact).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Contact not found", nil, "")
	}
	// Meta shows the bubble against the customer's latest message, inside the 24h window.
	var last models.Message
	if err := a.DB.Where("contact_id = ? AND direction = ? AND whats_app_message_id <> '' AND created_at > ?",
		contact.ID, models.DirectionIncoming, time.Now().Add(-24*time.Hour)).
		Order("created_at DESC").First(&last).Error; err != nil {
		return r.SendEnvelope(map[string]any{"sent": false})
	}
	var account models.WhatsAppAccount
	if err := a.DB.Where("organization_id = ? AND name = ?", orgID, last.WhatsAppAccount).First(&account).Error; err != nil {
		return r.SendEnvelope(map[string]any{"sent": false})
	}
	a.decryptAccountSecrets(&account)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.WhatsApp.SendTypingIndicator(ctx, account.ToWAAccount(), last.WhatsAppMessageID); err != nil {
		a.Log.Debug("Typing indicator not sent", "error", err)
		return r.SendEnvelope(map[string]any{"sent": false})
	}
	return r.SendEnvelope(map[string]any{"sent": true})
}
