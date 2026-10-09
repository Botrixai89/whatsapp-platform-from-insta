package handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeTemplateName(t *testing.T) {
	assert.Equal(t, "diwali_sale_20_off", sanitizeTemplateName("Diwali Sale - 20% OFF!"))
	assert.Equal(t, "ai_template", sanitizeTemplateName("!!!"))
	assert.LessOrEqual(t, len(sanitizeTemplateName(strings.Repeat("abc ", 40))), 50)
}

func TestNormalizeTemplateDraft_RenumbersVariablesAndSamples(t *testing.T) {
	d := TemplateAIDraft{
		Name: "Order Update", Category: "utility", HeaderType: "text",
		HeaderContent: "Order {{order}} for {{name}}",
		BodyContent:   "{{name}}, your order {{order_id}} ships on {{date}}",
		FooterContent: "Thanks {{1}}",
	}
	normalizeTemplateDraft(&d, "#123", []string{"Rahul", "A-77", "Monday"})

	assert.Equal(t, "order_update", d.Name)
	assert.Equal(t, "UTILITY", d.Category)
	assert.Equal(t, "Order {{1}} for", d.HeaderContent, "only one header variable allowed")
	assert.Equal(t, "Hi {{1}}, your order {{2}} ships on {{3}}.", d.BodyContent, "no variable at the start or end")
	assert.Equal(t, "Thanks", d.FooterContent, "footer cannot have variables")
	require.Len(t, d.SampleValues, 4)
	assert.Equal(t, TemplateAISample{Component: "header", Index: 1, Value: "#123"}, d.SampleValues[0])
	assert.Equal(t, TemplateAISample{Component: "body", Index: 2, Value: "A-77"}, d.SampleValues[2])
	assert.NotEmpty(t, d.Warnings)
}

func TestNormalizeTemplateDraft_Buttons(t *testing.T) {
	d := TemplateAIDraft{
		Category:    "MARKETING",
		BodyContent: "Big sale this weekend on all shoes",
		Buttons: []TemplateAIButton{
			{Type: "url", Text: "Shop now and save big today!!", URL: "https://shop.example.com/{{code}}"},
			{Type: "PHONE_NUMBER", Text: "Call us"},               // no number -> dropped
			{Type: "URL", Text: "Bad", URL: "http://insecure.io"}, // not https -> dropped
			{Type: "QUICK_REPLY", Text: "Stop"},                   // mixed with CTA -> dropped
			{Type: "COPY_CODE", Text: "x", Example: "SAVE20"},
		},
	}
	normalizeTemplateDraft(&d, "", nil)

	require.Len(t, d.Buttons, 2)
	assert.Equal(t, "URL", d.Buttons[0].Type)
	assert.LessOrEqual(t, len([]rune(d.Buttons[0].Text)), 25)
	assert.Equal(t, "https://shop.example.com/{{1}}", d.Buttons[0].URL)
	assert.Equal(t, "https://shop.example.com/123", d.Buttons[0].Example)
	assert.Equal(t, "COPY_CODE", d.Buttons[1].Type)
	assert.Equal(t, "NONE", d.HeaderType)
}

func TestNormalizeTemplateDraft_MediaHeaderAndLongBody(t *testing.T) {
	d := TemplateAIDraft{
		Category: "MARKETING", HeaderType: "IMAGE", HeaderContent: "ignored",
		BodyContent: strings.Repeat("नमस्ते ", 300),
	}
	normalizeTemplateDraft(&d, "", nil)
	assert.Equal(t, "", d.HeaderContent)
	assert.LessOrEqual(t, len([]rune(d.BodyContent)), 1024)
	assert.True(t, strings.HasPrefix(d.BodyContent, "नमस्ते"), "Hindi text must be cut on character boundaries")
}
