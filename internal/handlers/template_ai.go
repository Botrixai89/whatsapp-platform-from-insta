package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// AI template agent: turns a natural-language brief into ready-to-submit
// WhatsApp template drafts that follow Meta's template rules.

// Meta template limits
const (
	tplMaxBody       = 1024
	tplMaxHeaderText = 60
	tplMaxFooter     = 60
	tplMaxButtonText = 25
	tplMaxButtons    = 3 // matches the template editor
	tplMaxPrompt     = 1024
	tplMaxVariations = 3
)

// Default models when the platform key is used. Clients that configured their
// own chatbot AI provider use their own model instead.
var defaultTemplateModels = map[models.AIProvider]string{
	models.AIProviderAnthropic: "claude-sonnet-5-5",
	models.AIProviderOpenAI:    "gpt-4o-mini",
	models.AIProviderGoogle:    "gemini-2.5-flash",
}

var templateLanguageNames = map[string]string{
	"en": "English", "en_US": "English (US)", "en_GB": "English (UK)", "hi": "Hindi", "mr": "Marathi",
	"gu": "Gujarati", "ta": "Tamil", "te": "Telugu", "kn": "Kannada", "ml": "Malayalam", "bn": "Bengali",
	"pa": "Punjabi", "ur": "Urdu", "ar": "Arabic", "es": "Spanish", "pt_BR": "Portuguese (Brazil)",
	"fr": "French", "de": "German", "id": "Indonesian",
}

var styleGuides = map[string]string{
	"normal":   "clear, friendly and professional",
	"poetic":   "lyrical and evocative, with a light poetic rhythm, while staying easy to read",
	"exciting": "energetic and enthusiastic, creating urgency and excitement (use a few fitting emojis)",
	"funny":    "playful and witty with light humour (use a few fitting emojis), never offensive",
}

var optimizeGuides = map[string]string{
	"click": "Optimise for click rate: one clear benefit, a strong call to action, and a URL button as the main action.",
	"reply": "Optimise for reply rate: end with an inviting question and offer quick-reply buttons so the customer can answer in one tap.",
}

// TemplateAIRequest is the brief for the AI template agent.
type TemplateAIRequest struct {
	Prompt      string `json:"prompt"`
	Category    string `json:"category"`     // MARKETING, UTILITY or AUTO
	Language    string `json:"language"`     // template language code, e.g. en, hi
	Style       string `json:"style"`        // normal, poetic, exciting, funny
	OptimizeFor string `json:"optimize_for"` // click, reply
	HeaderType  string `json:"header_type"`  // AUTO, NONE, TEXT, IMAGE, VIDEO, DOCUMENT
	Variations  int    `json:"variations"`   // 1-3
}

// TemplateAIButton mirrors the template editor's button shape.
type TemplateAIButton struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	URL         string `json:"url,omitempty"`
	Example     string `json:"example,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

// TemplateAISample mirrors the template editor's sample_values entries.
type TemplateAISample struct {
	Component string `json:"component"`
	Index     int    `json:"index"`
	Value     string `json:"value"`
}

// TemplateAIDraft is one generated template, ready to load into the editor.
type TemplateAIDraft struct {
	Name          string             `json:"name"`
	DisplayName   string             `json:"display_name"`
	Category      string             `json:"category"`
	Language      string             `json:"language"`
	HeaderType    string             `json:"header_type"`
	HeaderContent string             `json:"header_content"`
	BodyContent   string             `json:"body_content"`
	FooterContent string             `json:"footer_content"`
	Buttons       []TemplateAIButton `json:"buttons"`
	SampleValues  []TemplateAISample `json:"sample_values"`
	Warnings      []string           `json:"warnings"`
}

// aiTemplateOutput is the JSON shape the model is asked to return.
type aiTemplateOutput struct {
	Variations []struct {
		Name          string             `json:"name"`
		DisplayName   string             `json:"display_name"`
		Category      string             `json:"category"`
		HeaderType    string             `json:"header_type"`
		HeaderText    string             `json:"header_text"`
		Body          string             `json:"body"`
		Footer        string             `json:"footer"`
		Buttons       []TemplateAIButton `json:"buttons"`
		HeaderExample string             `json:"header_example"`
		BodyExamples  []string           `json:"body_examples"`
	} `json:"variations"`
}

const templateAISystemPrompt = `You are an expert WhatsApp Business copywriter who writes message templates that Meta approves on the first try.

Follow Meta's template rules strictly:
- Body: max 1024 characters. Use positional variables {{1}}, {{2}}, ... in order, each used once. A variable must NOT be the very first or very last thing in the body. Do not put two variables next to each other. Keep the variable-to-text ratio low.
- WhatsApp formatting only: *bold*, _italic_, ~strikethrough~. No markdown headings, no HTML. Line breaks are allowed.
- Header (optional): TEXT header max 60 characters with at most one variable {{1}}; no emojis, no formatting. Or a media header IMAGE, VIDEO or DOCUMENT (leave header_text empty).
- Footer (optional): max 60 characters, no variables.
- Buttons (optional, max 3): QUICK_REPLY (text only), URL (https URL; a single {{1}} may appear only at the end of the URL), PHONE_NUMBER (international format with +), COPY_CODE (marketing only; put the coupon code in "example"). Button text max 25 characters. Do not mix QUICK_REPLY with other button types.
- UTILITY templates must be about a specific transaction or account update (order, booking, payment, delivery, reminder) and must not contain promotional language. Anything promotional is MARKETING.
- Never invent real phone numbers, URLs, prices or discount codes the user did not give; use variables or obvious placeholders like https://example.com instead.
- "name" is lowercase snake_case, max 50 characters, describing the template.

Reply with JSON only (no prose, no code fences) in exactly this shape:
{"variations":[{"name":"","display_name":"","category":"MARKETING|UTILITY","header_type":"NONE|TEXT|IMAGE|VIDEO|DOCUMENT","header_text":"","body":"","footer":"","buttons":[{"type":"QUICK_REPLY|URL|PHONE_NUMBER|COPY_CODE","text":"","url":"","example":"","phone_number":""}],"header_example":"","body_examples":["sample for {{1}}","sample for {{2}}"]}]}
body_examples must contain exactly one realistic sample value per body variable, in order.`

func buildTemplateAIUserPrompt(req TemplateAIRequest) string {
	lang := templateLanguageNames[req.Language]
	if lang == "" {
		lang = req.Language
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Write %d distinct variation(s) of a WhatsApp template.\n\n", req.Variations)
	fmt.Fprintf(&b, "Brief from the business:\n%s\n\n", req.Prompt)
	if req.Category == "AUTO" {
		b.WriteString("Category: choose MARKETING or UTILITY correctly for this brief.\n")
	} else {
		fmt.Fprintf(&b, "Category: %s\n", req.Category)
	}
	fmt.Fprintf(&b, "Language: write all text in %s (template language code %q).\n", lang, req.Language)
	fmt.Fprintf(&b, "Tone: %s.\n", styleGuides[req.Style])
	b.WriteString(optimizeGuides[req.OptimizeFor] + "\n")
	switch req.HeaderType {
	case "AUTO":
		b.WriteString("Header: pick whatever fits best (or none).\n")
	case "NONE":
		b.WriteString("Header: none.\n")
	default:
		fmt.Fprintf(&b, "Header: %s.\n", req.HeaderType)
	}
	if req.Variations > 1 {
		b.WriteString("Make the variations meaningfully different in angle and wording.\n")
	}
	return b.String()
}

// resolveTemplateAI picks the AI provider: the organization's own chatbot AI
// settings first, then the platform keys from config.
func (a *App) resolveTemplateAI(orgID uuid.UUID) (provider models.AIProvider, apiKey, model string, err error) {
	var settings models.ChatbotSettings
	if e := a.DB.Where("organization_id = ? AND (whats_app_account = '' OR whats_app_account IS NULL)", orgID).
		First(&settings).Error; e == nil && settings.AI.Provider != "" && settings.AI.APIKey != "" {
		model = settings.AI.Model
		if model == "" {
			model = defaultTemplateModels[settings.AI.Provider]
		}
		return settings.AI.Provider, settings.AI.APIKey, model, nil
	}
	cfg := a.Config.AI
	switch {
	case cfg.AnthropicKey != "":
		provider, apiKey = models.AIProviderAnthropic, cfg.AnthropicKey
	case cfg.OpenAIKey != "":
		provider, apiKey = models.AIProviderOpenAI, cfg.OpenAIKey
	case cfg.GoogleKey != "":
		provider, apiKey = models.AIProviderGoogle, cfg.GoogleKey
	default:
		return "", "", "", fmt.Errorf("AI is not configured. Add an AI provider in Chatbot settings, or ask the platform owner to set an AI key")
	}
	model = cfg.TemplateModel
	if model == "" {
		model = defaultTemplateModels[provider]
	}
	return provider, apiKey, model, nil
}

// callTemplateLLM sends one system+user prompt to the provider and returns the text reply.
func (a *App) callTemplateLLM(ctx context.Context, provider models.AIProvider, apiKey, model, system, user string) (string, error) {
	var (
		url     string
		payload map[string]any
		headers = map[string]string{"Content-Type": "application/json"}
	)
	switch provider {
	case models.AIProviderAnthropic:
		url = "https://api.anthropic.com/v1/messages"
		headers["x-api-key"] = apiKey
		headers["anthropic-version"] = "2023-06-01"
		payload = map[string]any{
			"model":      model,
			"max_tokens": 4096,
			"system":     system,
			"messages":   []map[string]string{{"role": "user", "content": user}},
		}
	case models.AIProviderOpenAI:
		url = "https://api.openai.com/v1/chat/completions"
		headers["Authorization"] = "Bearer " + apiKey
		payload = map[string]any{
			"model":           model,
			"response_format": map[string]string{"type": "json_object"},
			"messages": []map[string]string{
				{"role": "system", "content": system},
				{"role": "user", "content": user},
			},
		}
	case models.AIProviderGoogle:
		url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
		headers["x-goog-api-key"] = apiKey
		payload = map[string]any{
			"systemInstruction": map[string]any{"parts": []map[string]string{{"text": system}}},
			"contents":          []map[string]any{{"role": "user", "parts": []map[string]string{{"text": user}}}},
			"generationConfig":  map[string]any{"responseMimeType": "application/json"},
		}
	default:
		return "", fmt.Errorf("unsupported AI provider: %s", provider)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("AI provider returned %d: %s", resp.StatusCode, truncateRunes(string(raw), 300))
	}

	switch provider {
	case models.AIProviderAnthropic:
		var r struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return "", err
		}
		var sb strings.Builder
		for _, c := range r.Content {
			if c.Type == "text" {
				sb.WriteString(c.Text)
			}
		}
		return sb.String(), nil
	case models.AIProviderOpenAI:
		var r struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &r); err != nil || len(r.Choices) == 0 {
			return "", fmt.Errorf("unexpected OpenAI response")
		}
		return r.Choices[0].Message.Content, nil
	default:
		var r struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(raw, &r); err != nil || len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("unexpected Google response")
		}
		return r.Candidates[0].Content.Parts[0].Text, nil
	}
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

var (
	tplVarRe      = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)
	tplNameBadRe  = regexp.MustCompile(`[^a-z0-9_]+`)
	tplUnderRe    = regexp.MustCompile(`_+`)
	tplJSONFences = regexp.MustCompile("(?s)^```(?:json)?\\s*|\\s*```$")
)

// sanitizeTemplateName makes a Meta-valid template name.
func sanitizeTemplateName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = tplNameBadRe.ReplaceAllString(strings.ReplaceAll(s, " ", "_"), "_")
	s = strings.Trim(tplUnderRe.ReplaceAllString(s, "_"), "_")
	if len(s) > 50 {
		s = strings.Trim(s[:50], "_")
	}
	if s == "" {
		s = "ai_template"
	}
	return s
}

// renumberVariables rewrites variables to {{1}}, {{2}}, ... in order of
// appearance and returns the original variable names in that order.
func renumberVariables(text string) (string, []string) {
	var names []string
	out := tplVarRe.ReplaceAllStringFunc(text, func(m string) string {
		names = append(names, strings.TrimSpace(tplVarRe.FindStringSubmatch(m)[1]))
		return fmt.Sprintf("{{%d}}", len(names))
	})
	return out, names
}

// normalizeTemplateDraft enforces Meta's limits on a generated draft and
// records anything the user should review.
func normalizeTemplateDraft(d *TemplateAIDraft, headerExample string, bodyExamples []string) {
	warn := func(s string) { d.Warnings = append(d.Warnings, s) }

	d.Name = sanitizeTemplateName(d.Name)
	if d.DisplayName == "" {
		d.DisplayName = strings.ReplaceAll(d.Name, "_", " ")
	}
	d.Category = strings.ToUpper(strings.TrimSpace(d.Category))
	if d.Category != "MARKETING" && d.Category != "UTILITY" {
		d.Category = "MARKETING"
	}

	// Header
	d.HeaderType = strings.ToUpper(strings.TrimSpace(d.HeaderType))
	switch d.HeaderType {
	case "TEXT":
		header, names := renumberVariables(strings.TrimSpace(d.HeaderContent))
		if len(names) > 1 {
			header = tplVarRe.ReplaceAllStringFunc(header, func(m string) string {
				if m == "{{1}}" {
					return m
				}
				return ""
			})
			warn("Header had more than one variable; extra variables were removed.")
		}
		header = strings.Join(strings.Fields(header), " ")
		if utf8.RuneCountInString(header) > tplMaxHeaderText {
			header = truncateRunes(header, tplMaxHeaderText)
			warn("Header was shortened to 60 characters.")
		}
		d.HeaderContent = header
		if header == "" {
			d.HeaderType = "NONE"
		} else if strings.Contains(header, "{{1}}") {
			if headerExample == "" {
				headerExample = "Sample"
			}
			d.SampleValues = append(d.SampleValues, TemplateAISample{Component: "header", Index: 1, Value: headerExample})
		}
	case "IMAGE", "VIDEO", "DOCUMENT":
		d.HeaderContent = ""
		warn(fmt.Sprintf("Upload a sample %s for the header before submitting.", strings.ToLower(d.HeaderType)))
	default:
		d.HeaderType = "NONE"
		d.HeaderContent = ""
	}

	// Body
	body, names := renumberVariables(strings.TrimSpace(d.BodyContent))
	if strings.HasPrefix(body, "{{") {
		body = "Hi " + body
		warn("Body started with a variable, so \"Hi\" was added in front (Meta rejects that).")
	}
	if strings.HasSuffix(body, "}}") {
		body += "."
		warn("Body ended with a variable, so a full stop was added (Meta rejects that).")
	}
	if utf8.RuneCountInString(body) > tplMaxBody {
		body = truncateRunes(body, tplMaxBody)
		warn("Body was shortened to 1024 characters; please review the ending.")
	}
	d.BodyContent = body
	for i := range names {
		value := ""
		if i < len(bodyExamples) {
			value = strings.TrimSpace(bodyExamples[i])
		}
		if value == "" {
			value = names[i]
			if _, err := strconv.Atoi(value); err == nil {
				value = fmt.Sprintf("Sample %d", i+1)
			}
		}
		d.SampleValues = append(d.SampleValues, TemplateAISample{Component: "body", Index: i + 1, Value: value})
	}

	// Footer
	footer := strings.TrimSpace(tplVarRe.ReplaceAllString(d.FooterContent, ""))
	if utf8.RuneCountInString(footer) > tplMaxFooter {
		footer = truncateRunes(footer, tplMaxFooter)
		warn("Footer was shortened to 60 characters.")
	}
	d.FooterContent = footer

	// Buttons
	var buttons []TemplateAIButton
	hasQuickReply, hasCTA := false, false
	for _, btn := range d.Buttons {
		btn.Type = strings.ToUpper(strings.TrimSpace(btn.Type))
		btn.Text = truncateRunes(strings.TrimSpace(btn.Text), tplMaxButtonText)
		switch btn.Type {
		case "QUICK_REPLY":
			if btn.Text == "" {
				continue
			}
			btn.URL, btn.PhoneNumber, btn.Example = "", "", ""
			hasQuickReply = true
		case "URL":
			if btn.Text == "" || !strings.HasPrefix(btn.URL, "https://") {
				warn("A URL button was dropped because it had no valid https:// link.")
				continue
			}
			btn.PhoneNumber = ""
			if strings.Contains(btn.URL, "{{") {
				btn.URL = tplVarRe.ReplaceAllString(btn.URL, "{{1}}")
				if btn.Example == "" || strings.Contains(btn.Example, "{{") {
					btn.Example = strings.ReplaceAll(btn.URL, "{{1}}", "123")
				}
			} else {
				btn.Example = ""
			}
			hasCTA = true
		case "PHONE_NUMBER":
			if btn.Text == "" || !strings.HasPrefix(strings.TrimSpace(btn.PhoneNumber), "+") {
				warn("A phone button was dropped because it had no number in +country format; add your number manually.")
				continue
			}
			btn.URL, btn.Example = "", ""
			hasCTA = true
		case "COPY_CODE":
			if d.Category != "MARKETING" || btn.Example == "" {
				continue
			}
			btn.Text = "Copy offer code"
			btn.URL, btn.PhoneNumber = "", ""
			hasCTA = true
		default:
			continue
		}
		buttons = append(buttons, btn)
		if len(buttons) == tplMaxButtons {
			break
		}
	}
	if hasQuickReply && hasCTA {
		// Keep whichever kind comes first, matching how WhatsApp groups them
		first := buttons[0].Type == "QUICK_REPLY"
		var kept []TemplateAIButton
		for _, b := range buttons {
			if (b.Type == "QUICK_REPLY") == first {
				kept = append(kept, b)
			}
		}
		buttons = kept
		warn("Quick-reply and call-to-action buttons were mixed; only one kind was kept.")
	}
	if buttons == nil {
		buttons = []TemplateAIButton{}
	}
	d.Buttons = buttons
	if d.SampleValues == nil {
		d.SampleValues = []TemplateAISample{}
	}
	if d.Warnings == nil {
		d.Warnings = []string{}
	}
}

// GenerateTemplateWithAI drafts template variations from a natural-language brief.
func (a *App) GenerateTemplateWithAI(r *fastglue.Request) error {
	orgID, userID, err := a.requireAuth(r, models.ResourceTemplates, models.ActionWrite)
	if err != nil {
		return nil
	}
	var req TemplateAIRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Please describe the template you want", nil, "")
	}
	if utf8.RuneCountInString(req.Prompt) > tplMaxPrompt {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Prompt must be at most 1024 characters", nil, "")
	}
	req.Category = strings.ToUpper(strings.TrimSpace(req.Category))
	switch req.Category {
	case "MARKETING", "UTILITY", "AUTO":
	case "AUTHENTICATION":
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Authentication templates use Meta's fixed OTP text and cannot be AI-generated", nil, "")
	default:
		req.Category = "AUTO"
	}
	if req.Language == "" {
		req.Language = "en"
	}
	req.Style = strings.ToLower(req.Style)
	if _, ok := styleGuides[req.Style]; !ok {
		req.Style = "normal"
	}
	req.OptimizeFor = strings.ToLower(req.OptimizeFor)
	if _, ok := optimizeGuides[req.OptimizeFor]; !ok {
		req.OptimizeFor = "click"
	}
	req.HeaderType = strings.ToUpper(req.HeaderType)
	switch req.HeaderType {
	case "NONE", "TEXT", "IMAGE", "VIDEO", "DOCUMENT":
	default:
		req.HeaderType = "AUTO"
	}
	if req.Variations < 1 {
		req.Variations = 1
	}
	if req.Variations > tplMaxVariations {
		req.Variations = tplMaxVariations
	}

	provider, apiKey, model, err := a.resolveTemplateAI(orgID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	// Remember the prompt for "Previous prompts"
	a.DB.Create(&models.TemplateAIPrompt{
		OrganizationID: orgID, UserID: userID, Prompt: req.Prompt, Category: req.Category,
		Language: req.Language, Style: req.Style, OptimizeFor: req.OptimizeFor, HeaderType: req.HeaderType,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	reply, err := a.callTemplateLLM(ctx, provider, apiKey, model, templateAISystemPrompt, buildTemplateAIUserPrompt(req))
	if err != nil {
		a.Log.Error("AI template generation failed", "error", err, "provider", provider, "model", model)
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, "AI could not generate the template: "+err.Error(), nil, "")
	}

	var out aiTemplateOutput
	text := strings.TrimSpace(tplJSONFences.ReplaceAllString(strings.TrimSpace(reply), ""))
	if start, end := strings.Index(text, "{"), strings.LastIndex(text, "}"); start >= 0 && end > start {
		text = text[start : end+1]
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil || len(out.Variations) == 0 {
		a.Log.Error("AI template reply was not valid JSON", "error", err, "reply", truncateRunes(reply, 500))
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, "AI returned an unexpected answer. Please try again.", nil, "")
	}

	drafts := make([]TemplateAIDraft, 0, len(out.Variations))
	seen := map[string]int{}
	for _, v := range out.Variations {
		category := v.Category
		if req.Category != "AUTO" {
			category = req.Category
		}
		headerType := v.HeaderType
		if req.HeaderType != "AUTO" {
			headerType = req.HeaderType
		}
		d := TemplateAIDraft{
			Name: v.Name, DisplayName: v.DisplayName, Category: category, Language: req.Language,
			HeaderType: headerType, HeaderContent: v.HeaderText, BodyContent: v.Body,
			FooterContent: v.Footer, Buttons: v.Buttons,
		}
		normalizeTemplateDraft(&d, v.HeaderExample, v.BodyExamples)
		if d.BodyContent == "" {
			continue
		}
		if n := seen[d.Name]; n > 0 {
			d.Name = fmt.Sprintf("%s_%d", d.Name, n+1)
		}
		seen[d.Name]++
		drafts = append(drafts, d)
		if len(drafts) == req.Variations {
			break
		}
	}
	if len(drafts) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, "AI returned an empty template. Please try again.", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"variations": drafts,
		"provider":   provider,
		"model":      model,
	})
}

// ListTemplateAIPrompts returns the current user's recent prompts.
func (a *App) ListTemplateAIPrompts(r *fastglue.Request) error {
	orgID, userID, err := a.requireAuth(r, models.ResourceTemplates, models.ActionRead)
	if err != nil {
		return nil
	}
	var prompts []models.TemplateAIPrompt
	a.DB.Where("organization_id = ? AND user_id = ?", orgID, userID).
		Order("created_at DESC").Limit(20).Find(&prompts)
	return r.SendEnvelope(map[string]any{"prompts": prompts})
}
