package handlers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkText(t *testing.T) {
	assert.Equal(t, []string{"short"}, chunkText("short", 100, 10))

	para := strings.Repeat("Our shop opens at 10 am and closes at 9 pm on all days. ", 60)
	chunks := chunkText(para, 400, 50)
	require.Greater(t, len(chunks), 3)
	for _, c := range chunks {
		assert.LessOrEqual(t, len([]rune(c)), 400)
		assert.NotEmpty(t, c)
	}
	// Nothing is lost: the text starts and ends where the source does, and each
	// chunk starts inside the previous one (the overlap) or right after it.
	assert.True(t, strings.HasPrefix(para, chunks[0]))
	assert.True(t, strings.HasSuffix(strings.TrimSpace(para), chunks[len(chunks)-1]))
	pos := 0
	for _, c := range chunks {
		i := strings.Index(para[pos:], c)
		require.GreaterOrEqual(t, i, 0, "chunk not found in order")
		pos += i + 1
	}
}

func TestHTMLToText(t *testing.T) {
	got := normalizeKnowledgeText(htmlToText([]byte(`<html><head><style>.x{}</style><script>alert(1)</script></head>
		<body><nav>Home | About</nav><h1>Returns</h1><p>Return within <b>7 days</b>.</p><footer>© Shop</footer></body></html>`)))
	assert.Contains(t, got, "Returns")
	assert.Contains(t, got, "Return within 7 days")
	assert.NotContains(t, got, "alert")
	assert.NotContains(t, got, "Home | About")
	assert.NotContains(t, got, "© Shop")
}

func TestFetchPageTextRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("secret")) }))
	defer srv.Close()
	_, err := fetchPageText(context.Background(), srv.URL)
	require.Error(t, err, "localhost must not be reachable")
	_, err = fetchPageText(context.Background(), "file:///etc/passwd")
	require.Error(t, err)

	assert.False(t, isPublicIP(net.ParseIP("10.0.0.5")))
	assert.False(t, isPublicIP(net.ParseIP("169.254.169.254")))
	assert.False(t, isPublicIP(net.ParseIP("100.64.1.1")))
	assert.False(t, isPublicIP(net.ParseIP("::1")))
	assert.True(t, isPublicIP(net.ParseIP("142.250.183.14")))
}

func TestExtractDocumentText(t *testing.T) {
	txt, err := extractDocumentText("faq.txt", []byte("Delivery takes 3 days"))
	require.NoError(t, err)
	assert.Equal(t, "Delivery takes 3 days", txt)
	_, err = extractDocumentText("photo.jpg", []byte{1, 2})
	assert.Error(t, err)
	_, err = extractDocumentText("broken.pdf", []byte("not a pdf"))
	assert.Error(t, err)
}

func TestRelevantKnowledge(t *testing.T) {
	app := newProcessorTestApp(t)
	org, _ := createProcessorTestOrg(t, app)
	aiCtx := &models.AIContext{OrganizationID: org.ID, Name: "Shop FAQ", ContextType: models.ContextTypeKnowledge, IsEnabled: true}
	require.NoError(t, app.DB.Create(aiCtx).Error)
	texts := []string{
		"Sharma Traders sells tractors and spare parts in Uttar Pradesh.",
		"Warranty: every tractor has a 2 year warranty on the engine.",
		"Delivery: we deliver within 5 days to any village in the district.",
		"Payment: we accept UPI, cash and bank loans from SBI.",
	}
	for i, tx := range texts {
		require.NoError(t, app.DB.Create(&models.AIKnowledgeChunk{OrganizationID: org.ID, ContextID: aiCtx.ID, Position: i, Content: tx}).Error)
	}
	got := app.relevantKnowledge(aiCtx.ID, "How many days for delivery to my village?")
	assert.Contains(t, got, "deliver within 5 days")
	assert.NotContains(t, got, "UPI")

	// No matching words: falls back to the opening chunk
	assert.Contains(t, app.relevantKnowledge(aiCtx.ID, "hello"), "Sharma Traders")
}
