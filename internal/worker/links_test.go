package worker

import (
	"testing"

	"github.com/Botrixai89/botrixai/internal/config"
	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithTrackedLinks(t *testing.T) {
	w := testWorker(t)
	_, _, _, campaign := createMinimalCampaignData(t, w, models.CampaignStatusProcessing)
	recipient := &models.BulkMessageRecipient{
		CampaignID: campaign.ID, PhoneNumber: "919800000001",
		TemplateParams: models.JSONB{"1": "Rohit", "2": "https://shop.example.com/sale?x=1"},
	}
	require.NoError(t, w.DB.Create(recipient).Error)

	// Tracking off, or no public URL: unchanged
	assert.Same(t, recipient, w.withTrackedLinks(campaign, recipient))
	campaign.TrackLinks = true
	w.Config = &config.Config{}
	assert.Same(t, recipient, w.withTrackedLinks(campaign, recipient))

	w.Config.Server.PublicURL = "https://app.example.com/"
	out := w.withTrackedLinks(campaign, recipient)
	assert.Equal(t, "Rohit", out.TemplateParams["1"], "plain text is left alone")
	short := out.TemplateParams["2"].(string)
	assert.Regexp(t, `^https://app\.example\.com/l/[A-Za-z0-9]{8}$`, short)
	assert.Equal(t, "https://shop.example.com/sale?x=1", recipient.TemplateParams["2"], "the stored recipient is not modified")

	// A retry reuses the same short link
	again := w.withTrackedLinks(campaign, recipient)
	assert.Equal(t, short, again.TemplateParams["2"])
	var count int64
	w.DB.Model(&models.TrackedLink{}).Where("recipient_id = ?", recipient.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}
