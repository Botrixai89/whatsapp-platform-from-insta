package whatsapp

// Heltar provider.
//
// An account with Provider == ProviderHeltar talks to a Heltar-compatible API
// (Heltar itself or a white-label of it, e.g. https://newapi.botrixai.com)
// instead of Meta's Graph API. The rest of the client keeps building Meta
// payloads: the URL builders return an internal heltar:// target for such
// accounts and doRequest hands those to doHeltar, which translates the Meta
// request into Heltar's format and the response back into Meta's shape.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// Account providers
const (
	ProviderMeta   = "meta"
	ProviderHeltar = "heltar"
)

// DefaultHeltarURL is the BotrixAI white-label API host.
const DefaultHeltarURL = "https://newapi.botrixai.com"

const heltarScheme = "heltar://"

// ErrNotSupportedByProvider is returned for Meta Graph features the provider API does not expose.
var ErrNotSupportedByProvider = errors.New("this feature is not available for accounts connected through the BotrixAI API")

// IsHeltar reports whether the account sends through a Heltar-compatible API.
func (a *Account) IsHeltar() bool { return a != nil && a.Provider == ProviderHeltar }

func (a *Account) heltarBase() string {
	if a.ProviderURL != "" {
		return strings.TrimRight(a.ProviderURL, "/")
	}
	return DefaultHeltarURL
}

// heltarTarget encodes "which Heltar resource on which host" as an internal URL
// that doRequest recognises; callers may append a query string as usual.
func heltarTarget(account *Account, kind string) string {
	return heltarScheme + kind + "/" + url.PathEscape(account.heltarBase())
}

// heltarMediaMeta remembers name/MIME for files we uploaded, keyed by their public
// URL, because Heltar's media message needs both and Meta's payload carries only the ID.
var heltarMediaMeta sync.Map // url -> heltarMediaInfo

type heltarMediaInfo struct{ name, mime string }

// HeltarBusiness is the subset of GET /v1/business used to connect an account.
type HeltarBusiness struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	BusinessAccountID string `json:"businessAccountId"`
	BizWhatsappNumber string `json:"bizWhatsappNumber"`
	PhoneNumberID     string `json:"phoneNumberId"`
}

type heltarEnvelope struct {
	Message      string          `json:"message"`
	ErrorMessage string          `json:"errorMessage"`
	Data         json.RawMessage `json:"data"`
	ErrorRaw     json.RawMessage `json:"errorRaw"`
}

// heltarCall performs one request against the Heltar API and returns the envelope's data.
func (c *Client) heltarCall(ctx context.Context, method, fullURL string, body any, apiKey string) (json.RawMessage, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}
	var env heltarEnvelope
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusPartialContent {
		msg := env.ErrorMessage
		if msg == "" {
			msg = env.Message
		}
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		if len(env.ErrorRaw) > 0 {
			return env.ErrorRaw, resp.StatusCode, fmt.Errorf("BotrixAI API error (%d): %s", resp.StatusCode, msg)
		}
		return nil, resp.StatusCode, fmt.Errorf("BotrixAI API error (%d): %s", resp.StatusCode, msg)
	}
	return env.Data, resp.StatusCode, nil
}

// GetHeltarBusiness validates an API key and returns the connected number's details.
func (c *Client) GetHeltarBusiness(ctx context.Context, baseURL, apiKey string) (*HeltarBusiness, error) {
	if baseURL == "" {
		baseURL = DefaultHeltarURL
	}
	data, _, err := c.heltarCall(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/business", nil, apiKey)
	if err != nil {
		return nil, err
	}
	var biz HeltarBusiness
	if err := json.Unmarshal(data, &biz); err != nil {
		return nil, fmt.Errorf("unexpected business response: %w", err)
	}
	if biz.PhoneNumberID == "" {
		return nil, errors.New("no WhatsApp number is connected to this API key yet")
	}
	return &biz, nil
}

// HeltarWebhook is one webhook registration on the provider side.
type HeltarWebhook struct {
	URL       string            `json:"url"`
	IsEnabled bool              `json:"isEnabled"`
	Fields    []string          `json:"fields"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// RegisterHeltarWebhook asks the provider to forward raw Meta webhooks to url.
// The provider upserts by URL, so other webhooks the business has are kept.
func (c *Client) RegisterHeltarWebhook(ctx context.Context, account *Account, hook HeltarWebhook) error {
	if len(hook.Fields) == 0 {
		hook.Fields = []string{"metaWebhooks"}
	}
	_, _, err := c.heltarCall(ctx, http.MethodPost, account.heltarBase()+"/v1/business/webhook-url",
		map[string]any{"urls": []HeltarWebhook{hook}}, account.AccessToken)
	return err
}

// doHeltar serves a heltar:// target produced by the URL builders.
func (c *Client) doHeltar(ctx context.Context, method, target string, body any, apiKey string) ([]byte, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid provider target: %w", err)
	}
	base, err := url.PathUnescape(strings.TrimPrefix(u.Path, "/"))
	if err != nil || base == "" {
		return nil, fmt.Errorf("invalid provider target")
	}
	switch u.Host {
	case "messages":
		return c.heltarSend(ctx, base, body, apiKey)
	case "templates":
		return c.heltarTemplates(ctx, method, base, u.Query(), body, apiKey)
	default:
		return nil, ErrNotSupportedByProvider
	}
}

func (c *Client) heltarSend(ctx context.Context, base string, body any, apiKey string) ([]byte, error) {
	payload, ok := body.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected message payload")
	}
	// Read receipts: the provider marks the chat read when we reply, and its
	// mark-read endpoint needs the customer's number rather than a message ID.
	if payload["status"] == "read" {
		return []byte(`{"success":true}`), nil
	}
	msg, err := translateMetaMessage(payload)
	if err != nil {
		return nil, err
	}
	// priority=true keeps sends synchronous so we always get the real wamid back
	// (async mode returns a temporary ID that status webhooks would not match).
	data, _, err := c.heltarCall(ctx, http.MethodPost, base+"/v1/messages/send?priority=true",
		map[string]any{"messages": []map[string]any{msg}}, apiKey)
	var result struct {
		Success map[string]struct {
			Message struct {
				Wamid string `json:"wamid"`
			} `json:"message"`
		} `json:"success"`
		Fail map[string]struct {
			ErrorCode int             `json:"errorCode"`
			ErrorData json.RawMessage `json:"errorData"`
		} `json:"fail"`
	}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &result)
	}
	if f, failed := result.Fail["0"]; failed {
		return nil, heltarSendError(f.ErrorCode, f.ErrorData)
	}
	if err != nil {
		return nil, err
	}
	s, ok := result.Success["0"]
	if !ok || s.Message.Wamid == "" {
		return nil, fmt.Errorf("BotrixAI API accepted the message but returned no message ID")
	}
	return json.Marshal(MetaAPIResponse{Messages: []struct {
		ID string `json:"id"`
	}{{ID: s.Message.Wamid}}})
}

// heltarSendError keeps Meta's error codes when the provider passes them through.
func heltarSendError(code int, raw json.RawMessage) error {
	if code == 0 {
		code = http.StatusBadRequest
	}
	var metaErr struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	if json.Unmarshal(raw, &metaErr) == nil && metaErr.Message != "" {
		return ParseMetaAPIError(code, mustJSON(map[string]any{"error": json.RawMessage(raw)}))
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return fmt.Errorf("BotrixAI API error (%d): %s", code, s)
	}
	return fmt.Errorf("BotrixAI API error (%d): %s", code, strings.TrimSpace(string(raw)))
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// translateMetaMessage converts a Graph API /messages payload into one Heltar message.
func translateMetaMessage(p map[string]any) (map[string]any, error) {
	to, _ := p["to"].(string)
	if to == "" {
		to, _ = p["recipient"].(string) // BSUID; the provider accepts user IDs here too
	}
	if to == "" {
		return nil, fmt.Errorf("message has no recipient")
	}
	typ, _ := p["type"].(string)
	out := map[string]any{"clientWaNumber": to}
	if ctxObj, ok := p["context"].(map[string]any); ok && typ != "template" {
		if id, _ := ctxObj["message_id"].(string); id != "" {
			out["contextId"] = id
		}
	}

	switch typ {
	case "text":
		t, _ := p["text"].(map[string]any)
		out["messageType"] = "text"
		out["message"] = t["body"]
	case "template":
		t, _ := p["template"].(map[string]any)
		out["messageType"] = "template"
		out["templateName"] = t["name"]
		if lang, ok := t["language"].(map[string]any); ok {
			out["languageCode"] = lang["code"]
		}
		if comps, ok := t["components"]; ok && comps != nil {
			out["variables"] = comps
		} else {
			out["variables"] = []any{}
		}
	case "interactive", "location", "contacts":
		out["messageType"] = typ
		out[typ] = p[typ]
	case "image", "video", "audio", "document", "sticker":
		m, _ := p[typ].(map[string]any)
		link, _ := m["link"].(string)
		if link == "" {
			link, _ = m["id"].(string) // our uploads return the public URL as the "ID"
		}
		if !strings.HasPrefix(link, "http") {
			return nil, fmt.Errorf("media must be uploaded through the BotrixAI API before sending")
		}
		name, mimeType := mediaNameAndType(link, m)
		out["messageType"] = "media"
		out["mediaType"] = typ
		out["url"] = link
		out["name"] = name
		out["mimeType"] = mimeType
		if caption, _ := m["caption"].(string); caption != "" && typ != "audio" && typ != "sticker" {
			out["caption"] = caption
		}
	default:
		return nil, fmt.Errorf("%q messages are not supported through the BotrixAI API", typ)
	}
	return out, nil
}

func mediaNameAndType(link string, m map[string]any) (string, string) {
	var info heltarMediaInfo
	if v, ok := heltarMediaMeta.Load(link); ok {
		info = v.(heltarMediaInfo)
	}
	name, _ := m["filename"].(string)
	if name == "" {
		name = info.name
	}
	if name == "" {
		if u, err := url.Parse(link); err == nil {
			name = path.Base(u.Path)
		}
	}
	mimeType := info.mime
	if mimeType == "" {
		mimeType = mime.TypeByExtension(path.Ext(name))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return name, mimeType
}

func (c *Client) heltarTemplates(ctx context.Context, method, base string, q url.Values, body any, apiKey string) ([]byte, error) {
	switch method {
	case http.MethodGet:
		data, _, err := c.heltarCall(ctx, http.MethodGet, base+"/v1/templates?refresh=true", nil, apiKey)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]json.RawMessage{"data": data})
	case http.MethodPost:
		data, _, err := c.heltarCall(ctx, http.MethodPost, base+"/v1/templates", body, apiKey)
		if err != nil {
			return nil, err
		}
		return data, nil // {id, status, category}, the same shape Meta returns
	case http.MethodDelete:
		// Meta deletes every language of a name in one call; the provider needs each ID.
		name := q.Get("name")
		data, _, err := c.heltarCall(ctx, http.MethodGet, base+"/v1/templates", nil, apiKey)
		if err != nil {
			return nil, err
		}
		var list []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(data, &list)
		deleted := 0
		for _, t := range list {
			if t.Name != name {
				continue
			}
			if _, _, err := c.heltarCall(ctx, http.MethodDelete,
				base+"/v1/templates/"+url.PathEscape(t.Name)+"/"+url.PathEscape(t.ID), nil, apiKey); err != nil {
				return nil, err
			}
			deleted++
		}
		if deleted == 0 {
			return nil, fmt.Errorf("template %q not found", name)
		}
		return []byte(`{"success":true}`), nil
	}
	return nil, ErrNotSupportedByProvider
}

// heltarUploadMedia hosts a file with the provider and returns its public URL,
// which the rest of the code treats as the media ID.
func (c *Client) heltarUploadMedia(ctx context.Context, account *Account, data []byte, mimeType, filename string) (string, error) {
	q := url.Values{"file_name": {filename}, "file_type": {mimeType}}
	raw, _, err := c.heltarCall(ctx, http.MethodGet, account.heltarBase()+"/v1/messages/presigned-url?"+q.Encode(), nil, account.AccessToken)
	if err != nil {
		return "", err
	}
	var signed struct {
		SignedRequest string `json:"signedRequest"`
		URL           string `json:"url"`
	}
	if err := json.Unmarshal(raw, &signed); err != nil || signed.SignedRequest == "" || signed.URL == "" {
		return "", fmt.Errorf("unexpected upload URL response")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, signed.SignedRequest, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mimeType)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("media upload failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("media upload failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	heltarMediaMeta.Store(signed.URL, heltarMediaInfo{name: filename, mime: mimeType})
	return signed.URL, nil
}

// heltarMediaURL resolves an incoming message's media to the provider's
// permanent CDN link. ref is the message's wamid; the CDN copy can lag the
// webhook by a few seconds, so this retries briefly.
func (c *Client) heltarMediaURL(ctx context.Context, account *Account, ref string) (string, error) {
	if strings.HasPrefix(ref, "http") {
		return ref, nil
	}
	endpoint := account.heltarBase() + "/v1/messages?wamid=" + url.QueryEscape(ref)
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(3 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+account.AccessToken)
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		// The doc shows the message at the top level; accept an envelope too.
		var msg struct {
			AwsLink string `json:"awsLink"`
			Data    struct {
				AwsLink string `json:"awsLink"`
			} `json:"data"`
		}
		_ = json.Unmarshal(raw, &msg)
		if link := msg.AwsLink + msg.Data.AwsLink; link != "" {
			return link, nil
		}
	}
	return "", fmt.Errorf("media for message %s is not available yet", ref)
}
