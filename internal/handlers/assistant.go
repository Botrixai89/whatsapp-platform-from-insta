package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// AssistantDestination is a page the signed-in user may open. The client sends
// only pages that pass its permission checks; the router enforces them again.
type AssistantDestination struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// AssistantTurn is one line of the help conversation.
type AssistantTurn struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// AssistantChatRequest asks the in-app help assistant a question.
type AssistantChatRequest struct {
	Messages     []AssistantTurn        `json:"messages"`
	CurrentPage  string                 `json:"current_page"`
	Destinations []AssistantDestination `json:"destinations"`
}

// AssistantChatResponse is the assistant's answer plus pages worth opening.
type AssistantChatResponse struct {
	Reply string   `json:"reply"`
	Links []string `json:"links"` // destination IDs, always a subset of the request's
}

const (
	assistantMaxTurns        = 8
	assistantMaxMessageRunes = 1000
	assistantMaxDestinations = 80
	assistantMaxLinks        = 3
)

const assistantSystemPrompt = `You are the in-app help assistant of BotrixAI, a WhatsApp Business messaging platform built on Meta's official WhatsApp Cloud API.
You help the signed-in user use the platform: answer how-to questions and point them to the right page.

What the platform does:
- Chat: shared WhatsApp inbox. Filter chips (All, Unread, Open, Closed, Mine, Unassigned, Assigned, Bot off). Agents can close a chat (it reopens when the customer writes again), turn the bot off for one chat, assign chats, add internal notes and use canned responses.
- WhatsApp's 24-hour rule: free-form replies are only possible within 24 hours of the customer's last message. After that only approved template messages can be sent.
- Templates: create manually with a live preview, or generate with AI from a prompt. Meta reviews every template (approved, pending, rejected). Categories: Marketing, Utility, Authentication; marketing costs more. Template Analytics shows sent, delivered, read, failed and spend per template.
- Campaigns: bulk-send an approved template to a list of contacts (CSV upload), track delivery and read rates. Campaigns pause automatically if the wallet runs out.
- Chatbot: keyword rules, visual conversation flows, AI contexts (AI replies from your own knowledge), and agent transfers to hand chats to humans. Chatbot settings hold greeting/fallback messages, business hours and the AI provider.
- WhatsApp Flows: native WhatsApp forms.
- Calling: call logs, IVR flows and call transfers for WhatsApp Business calling.
- Wallet: prepaid balance. Every billable message is charged at the moment it is sent using the rate card; failed messages are refunded automatically. Clients cannot top up themselves: the platform owner (account manager) adds funds.
- Contacts (import/export CSV, tags), Users, Roles, Teams, API keys, Webhooks, Custom actions, SSO and Audit logs live under Settings.
- Connect a WhatsApp number: Settings > Accounts > Add Account (Embedded Signup with Facebook, or enter Phone Number ID, WABA ID and access token manually).
- Common send errors: error 190 / OAuth = the access token expired, so reconnect the number; 131047 = 24-hour window closed, so use a template; insufficient balance = wallet empty.
- Platform owners (super admins) also have the Owner Panel: clients, plans, rate card and transactions.

How to answer:
- Reply in the same language and style as the user (English, Hindi or Hinglish).
- Be short and concrete: at most 5 sentences or a short numbered list of steps. Use **bold** for button and menu names.
- Never invent features, prices or settings that are not described above. If you are unsure, say so and suggest the closest page.
- Only link to pages from the AVAILABLE PAGES list, by id. If the user asks for a page that is not in the list, tell them they don't have access and should ask their admin.
- Respond with JSON only, no markdown fences: {"reply": "<answer>", "links": ["<page id>", ...]} with at most 3 links, most relevant first.`

// AssistantChat answers a help question with the configured AI provider.
// Returns 503 when no AI provider is configured so the client can fall back
// to its built-in answers.
func (a *App) AssistantChat(r *fastglue.Request) error {
	orgID, _, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	var req AssistantChatRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	turns := req.Messages
	if len(turns) > assistantMaxTurns {
		turns = turns[len(turns)-assistantMaxTurns:]
	}
	if len(turns) == 0 || turns[len(turns)-1].Role != "user" || strings.TrimSpace(turns[len(turns)-1].Content) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Ask a question first", nil, "")
	}

	provider, apiKey, model, err := a.resolveTemplateAI(orgID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "AI assistant is not configured", nil, "ai_not_configured")
	}

	allowed := make(map[string]bool, len(req.Destinations))
	var pages strings.Builder
	for i, d := range req.Destinations {
		if i == assistantMaxDestinations {
			break
		}
		id := truncateRunes(strings.TrimSpace(d.ID), 60)
		if id == "" {
			continue
		}
		allowed[id] = true
		fmt.Fprintf(&pages, "- %s: %s. %s\n", id, truncateRunes(d.Title, 80), truncateRunes(d.Description, 200))
	}

	var prompt strings.Builder
	prompt.WriteString("AVAILABLE PAGES (id: title. description):\n")
	prompt.WriteString(pages.String())
	if req.CurrentPage != "" {
		fmt.Fprintf(&prompt, "\nThe user is currently on: %s\n", truncateRunes(req.CurrentPage, 200))
	}
	prompt.WriteString("\nCONVERSATION:\n")
	for _, t := range turns {
		role := "User"
		if t.Role == "assistant" {
			role = "Assistant"
		}
		fmt.Fprintf(&prompt, "%s: %s\n", role, truncateRunes(strings.TrimSpace(t.Content), assistantMaxMessageRunes))
	}
	prompt.WriteString("\nAnswer the user's last message as JSON.")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	raw, err := a.callTemplateLLM(ctx, provider, apiKey, model, assistantSystemPrompt, prompt.String())
	if err != nil {
		a.Log.Error("Assistant AI call failed", "error", err, "provider", provider)
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, "The assistant could not answer right now. Please try again.", nil, "")
	}

	return r.SendEnvelope(parseAssistantReply(raw, allowed))
}

// parseAssistantReply extracts the JSON answer, tolerating code fences or a
// plain-text reply, and drops links to pages the user was not offered.
func parseAssistantReply(raw string, allowed map[string]bool) AssistantChatResponse {
	text := strings.TrimSpace(tplJSONFences.ReplaceAllString(strings.TrimSpace(raw), ""))
	var out AssistantChatResponse
	if start, end := strings.Index(text, "{"), strings.LastIndex(text, "}"); start >= 0 && end > start {
		if json.Unmarshal([]byte(text[start:end+1]), &out) != nil {
			out = AssistantChatResponse{}
		}
	}
	if strings.TrimSpace(out.Reply) == "" {
		out.Reply = text
		out.Links = nil
	}
	links := make([]string, 0, assistantMaxLinks)
	seen := map[string]bool{}
	for _, id := range out.Links {
		if allowed[id] && !seen[id] && len(links) < assistantMaxLinks {
			links = append(links, id)
			seen[id] = true
		}
	}
	out.Reply = strings.TrimSpace(out.Reply)
	out.Links = links
	return out
}
