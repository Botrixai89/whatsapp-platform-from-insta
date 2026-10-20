package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"golang.org/x/net/html"
	"gorm.io/gorm"
)

const (
	knowledgeMaxFileBytes = 10 << 20 // 10 MB upload
	knowledgeMaxPageBytes = 3 << 20  // 3 MB web page
	knowledgeMaxChars     = 400_000  // per context, after extraction
	knowledgeChunkChars   = 1200
	knowledgeChunkOverlap = 150
	knowledgeTopChunks    = 4
	knowledgeMaxPrompt    = 6000 // characters of knowledge placed in one AI prompt
)

// ---------------------------------------------------------------------------
// Ingestion
// ---------------------------------------------------------------------------

// UploadKnowledge replaces a knowledge context's text with an uploaded file
// (multipart field "file": PDF, TXT, MD or CSV) or a web page (form field "url").
func (a *App) UploadKnowledge(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceChatbotAI, models.ActionWrite)
	if err != nil {
		return nil
	}
	id, err := parsePathUUID(r, "id", "context")
	if err != nil {
		return nil
	}
	aiCtx, err := findByIDAndOrg[models.AIContext](a.DB, r, id, orgID, "AI context")
	if err != nil {
		return nil
	}

	var text, sourceName, sourceType string
	if form, ferr := r.RequestCtx.MultipartForm(); ferr == nil && len(form.File["file"]) > 0 {
		fh := form.File["file"][0]
		if fh.Size > knowledgeMaxFileBytes {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "File is larger than 10 MB", nil, "")
		}
		f, err := fh.Open()
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Could not read the file", nil, "")
		}
		data, _ := io.ReadAll(io.LimitReader(f, knowledgeMaxFileBytes+1))
		_ = f.Close()
		text, err = extractDocumentText(fh.Filename, data)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
		}
		sourceName, sourceType = filepath.Base(fh.Filename), "file"
	} else if raw := strings.TrimSpace(string(r.RequestCtx.FormValue("url"))); raw != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		text, err = fetchPageText(ctx, raw)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
		}
		sourceName, sourceType = raw, "url"
	} else {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Upload a file or enter a website address", nil, "")
	}

	text = normalizeKnowledgeText(text)
	if len([]rune(text)) < 20 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "No readable text was found (scanned PDFs without text are not supported)", nil, "")
	}
	if len([]rune(text)) > knowledgeMaxChars {
		text = string([]rune(text)[:knowledgeMaxChars])
	}
	chunks := chunkText(text, knowledgeChunkChars, knowledgeChunkOverlap)

	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("context_id = ?", aiCtx.ID).Delete(&models.AIKnowledgeChunk{}).Error; err != nil {
			return err
		}
		rows := make([]models.AIKnowledgeChunk, len(chunks))
		for i, c := range chunks {
			rows[i] = models.AIKnowledgeChunk{OrganizationID: orgID, ContextID: aiCtx.ID, Position: i, Content: c}
		}
		if err := tx.CreateInBatches(rows, 200).Error; err != nil {
			return err
		}
		return tx.Model(aiCtx).Updates(map[string]any{
			"context_type": models.ContextTypeKnowledge, "source_name": truncateRunes(sourceName, 500),
			"source_type": sourceType, "source_chars": len([]rune(text)),
		}).Error
	})
	if err != nil {
		a.Log.Error("Failed to save knowledge", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save the knowledge", nil, "")
	}
	a.InvalidateAIContextsCache(orgID)
	return r.SendEnvelope(map[string]any{
		"source_name": sourceName, "source_type": sourceType,
		"chars": len([]rune(text)), "chunks": len(chunks),
		"preview": truncateRunes(text, 600),
	})
}

// extractDocumentText reads PDF, text, Markdown and CSV files.
func extractDocumentText(name string, data []byte) (string, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf":
		reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", errors.New("This PDF could not be opened")
		}
		var b strings.Builder
		for i := 1; i <= reader.NumPage(); i++ {
			page := reader.Page(i)
			if page.V.IsNull() {
				continue
			}
			t, err := page.GetPlainText(nil)
			if err != nil {
				continue
			}
			b.WriteString(t)
			b.WriteString("\n\n")
		}
		return b.String(), nil
	case ".txt", ".md", ".markdown", ".csv":
		return string(data), nil
	}
	return "", errors.New("Use a PDF, TXT, MD or CSV file")
}

// fetchPageText downloads a public web page and returns its readable text.
// Requests to private, loopback or link-local addresses are refused, including
// via redirects or DNS tricks, so the feature can't be used to probe our network.
func fetchPageText(ctx context.Context, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("Enter a full http:// or https:// address")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || !isPublicIP(ip) {
			return errors.New("address is not public")
		}
		return nil
	}}
	client := &http.Client{
		Timeout:   20 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext, Proxy: nil},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", errors.New("Invalid address")
	}
	req.Header.Set("User-Agent", "BotrixAI-KnowledgeBot/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("Could not open that page (it must be a public website)")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("The page returned an error (%d)", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, knowledgeMaxPageBytes))
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/plain") {
		return string(body), nil
	}
	if !strings.Contains(ct, "html") && ct != "" {
		return "", errors.New("That address is not a web page")
	}
	return htmlToText(body), nil
}

func isPublicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.Equal(net.ParseIP("169.254.169.254")) ||
		// carrier-grade NAT 100.64.0.0/10
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xC0 == 64))
}

// htmlToText keeps visible text, dropping scripts, styles and navigation chrome.
func htmlToText(body []byte) string {
	z := html.NewTokenizer(bytes.NewReader(body))
	var b strings.Builder
	skip := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.StartTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "noscript", "svg", "nav", "footer", "header", "template":
				skip++
			case "p", "div", "br", "li", "h1", "h2", "h3", "h4", "tr", "section", "article":
				b.WriteString("\n")
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "noscript", "svg", "nav", "footer", "header", "template":
				if skip > 0 {
					skip--
				}
			}
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
				b.WriteString(" ")
			}
		}
	}
}

// normalizeKnowledgeText collapses runs of spaces and blank lines.
func normalizeKnowledgeText(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, l := range lines {
		l = strings.Join(strings.Fields(l), " ")
		if l == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// chunkText splits text into ~size-rune pieces on paragraph/sentence boundaries,
// overlapping slightly so an answer cut at a boundary is still found.
func chunkText(text string, size, overlap int) []string {
	runes := []rune(text)
	if len(runes) <= size {
		return []string{text}
	}
	var chunks []string
	for start := 0; start < len(runes); {
		end := start + size
		if end >= len(runes) {
			chunks = append(chunks, strings.TrimSpace(string(runes[start:])))
			break
		}
		// Prefer to cut at a paragraph, then a sentence, in the last third of the window
		cut := end
		window := string(runes[start+size*2/3 : end])
		if i := strings.LastIndex(window, "\n\n"); i >= 0 {
			cut = start + size*2/3 + len([]rune(window[:i]))
		} else if i := strings.LastIndexAny(window, ".!?\n"); i >= 0 {
			cut = start + size*2/3 + len([]rune(window[:i])) + 1
		}
		chunks = append(chunks, strings.TrimSpace(string(runes[start:cut])))
		next := cut - overlap
		if next <= start {
			next = cut // always move forward
		}
		start = next
	}
	return chunks
}

// ---------------------------------------------------------------------------
// Retrieval
// ---------------------------------------------------------------------------

var knowledgeStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "you": true, "your": true, "are": true, "with": true, "this": true,
	"that": true, "what": true, "how": true, "can": true, "have": true, "from": true, "will": true, "does": true,
	"kya": true, "hai": true, "hain": true, "kaise": true, "mein": true, "aur": true, "kar": true, "nahi": true,
}

func knowledgeTerms(s string) []string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	out := words[:0]
	for _, w := range words {
		if len([]rune(w)) >= 3 && !knowledgeStopwords[w] {
			out = append(out, w)
		}
	}
	return out
}

// relevantKnowledge returns the chunks of a context that best match the question
// (term-frequency × inverse-document-frequency over the context's own chunks).
func (a *App) relevantKnowledge(contextID uuid.UUID, question string) string {
	var chunks []models.AIKnowledgeChunk
	a.DB.Where("context_id = ?", contextID).Order("position").Find(&chunks)
	if len(chunks) == 0 {
		return ""
	}
	terms := knowledgeTerms(question)
	lowered := make([]string, len(chunks))
	for i, c := range chunks {
		lowered[i] = strings.ToLower(c.Content)
	}
	type scored struct {
		i     int
		score float64
	}
	var results []scored
	if len(terms) > 0 {
		df := map[string]int{}
		for _, term := range terms {
			for _, c := range lowered {
				if strings.Contains(c, term) {
					df[term]++
				}
			}
		}
		for i, c := range lowered {
			score := 0.0
			for _, term := range terms {
				if n := strings.Count(c, term); n > 0 {
					idf := math.Log(1 + float64(len(chunks))/float64(df[term]))
					score += (1 + math.Log(float64(n))) * idf
				}
			}
			if score > 0 {
				results = append(results, scored{i, score})
			}
		}
		sort.Slice(results, func(x, y int) bool { return results[x].score > results[y].score })
	}
	if len(results) == 0 {
		// Nothing matched: the opening of the document usually says what the business is
		results = []scored{{0, 0}}
	}
	if len(results) > knowledgeTopChunks {
		results = results[:knowledgeTopChunks]
	}
	// Keep document order so the AI reads the passages naturally
	sort.Slice(results, func(x, y int) bool { return results[x].i < results[y].i })
	var b strings.Builder
	for _, r := range results {
		if b.Len()+len(chunks[r.i].Content) > knowledgeMaxPrompt {
			break
		}
		if b.Len() > 0 {
			b.WriteString("\n…\n")
		}
		b.WriteString(chunks[r.i].Content)
	}
	return b.String()
}
