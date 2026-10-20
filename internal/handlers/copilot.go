package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// CopilotRequest asks the AI to help an agent in a chat.
type CopilotRequest struct {
	Action   string `json:"action"`   // suggest, improve, translate, summarize
	Draft    string `json:"draft"`    // the agent's text for improve/translate
	Language string `json:"language"` // target language for translate
}

const copilotSystemPrompt = `You help customer-support agents who chat with customers on WhatsApp for a business.
Write like a friendly, professional human agent: short, clear, no corporate filler, no markdown headings, at most one emoji.
Reply in the language and script the customer uses (English, Hindi or Hinglish), unless told otherwise.
Never invent prices, policies, order details or promises that are not in the conversation or the business information.
Return only the text that was asked for — no preamble, no quotes.`

// Copilot drafts a reply, polishes or translates the agent's draft, or summarises the chat.
func (a *App) Copilot(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	contactID, err := parsePathUUID(r, "id", "contact")
	if err != nil {
		return nil
	}
	var req CopilotRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	query := a.scopeAssignedContact(a.ScopeToOrg(a.DB, userID, orgID), userID, orgID)
	var contact models.Contact
	if err := query.Where("id = ?", contactID).First(&contact).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Contact not found", nil, "")
	}
	req.Draft = truncateRunes(strings.TrimSpace(req.Draft), 2000)
	if (req.Action == "improve" || req.Action == "translate") && req.Draft == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Type a message first", nil, "")
	}

	provider, apiKey, model, err := a.resolveTemplateAI(orgID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, err.Error(), nil, "ai_not_configured")
	}

	// Recent conversation, oldest first
	var recent []models.Message
	a.DB.Where("contact_id = ?", contact.ID).Order("created_at DESC").Limit(20).Find(&recent)
	var transcript strings.Builder
	lastCustomer := ""
	for i := len(recent) - 1; i >= 0; i-- {
		m := recent[i]
		body := strings.TrimSpace(m.Content)
		if body == "" {
			body = "[" + string(m.MessageType) + "]"
		}
		who := "Agent"
		if m.Direction == models.DirectionIncoming {
			who = "Customer"
			lastCustomer = body
		}
		fmt.Fprintf(&transcript, "%s: %s\n", who, truncateRunes(body, 600))
	}
	if transcript.Len() == 0 && req.Action != "improve" && req.Action != "translate" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "There are no messages in this chat yet", nil, "")
	}

	var task string
	switch req.Action {
	case "suggest":
		task = "Write the agent's next reply to the customer."
		if info := a.buildAIContext(orgID, nil, lastCustomer); info != "" {
			task += "\n\nBusiness information you may use:\n" + truncateRunes(info, 8000)
		}
	case "improve":
		task = "Rewrite the agent's draft below so it is clear, polite and professional. Keep its meaning and language.\n\nDraft:\n" + req.Draft
	case "translate":
		lang := strings.TrimSpace(req.Language)
		if lang == "" {
			lang = "the customer's language"
		}
		task = "Translate the agent's draft below into " + truncateRunes(lang, 40) + ". Keep the tone natural for WhatsApp.\n\nDraft:\n" + req.Draft
	case "summarize":
		task = "Summarise this conversation for another agent in 3-5 short bullet points (use \"- \"): what the customer wants, what was already done, and what is pending. Write it in English."
	default:
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Unknown copilot action", nil, "")
	}

	prompt := "CONVERSATION (customer: " + firstNonEmpty(contact.ProfileName, "unknown name") + "):\n" + transcript.String() + "\nTASK:\n" + task
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	out, err := a.callTemplateLLM(ctx, provider, apiKey, model, copilotSystemPrompt, prompt)
	if err != nil {
		a.Log.Error("Copilot request failed", "error", err, "action", req.Action)
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, "The AI could not answer right now. Please try again.", nil, "")
	}
	out = strings.Trim(strings.TrimSpace(out), "\"")
	return r.SendEnvelope(map[string]any{"action": req.Action, "text": out})
}
