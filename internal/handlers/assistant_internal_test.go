package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseAssistantReply(t *testing.T) {
	allowed := map[string]bool{"wallet": true, "templates": true}

	t.Run("json with fences, unknown and duplicate links dropped", func(t *testing.T) {
		out := parseAssistantReply("```json\n{\"reply\":\"Go to **Wallet**.\",\"links\":[\"wallet\",\"admin_clients\",\"wallet\",\"templates\"]}\n```", allowed)
		assert.Equal(t, "Go to **Wallet**.", out.Reply)
		assert.Equal(t, []string{"wallet", "templates"}, out.Links)
	})

	t.Run("plain text falls back to the raw answer", func(t *testing.T) {
		out := parseAssistantReply("Open Settings > Accounts.", allowed)
		assert.Equal(t, "Open Settings > Accounts.", out.Reply)
		assert.Empty(t, out.Links)
	})

	t.Run("links capped at three", func(t *testing.T) {
		many := map[string]bool{"a": true, "b": true, "c": true, "d": true}
		out := parseAssistantReply(`{"reply":"x","links":["a","b","c","d"]}`, many)
		assert.Len(t, out.Links, 3)
	})
}
