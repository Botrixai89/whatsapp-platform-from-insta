package worker

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"strings"

	"github.com/Botrixai89/botrixai/internal/models"
)

const linkCodeAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newLinkCode() string {
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(linkCodeAlphabet))))
		b[i] = linkCodeAlphabet[n.Int64()]
	}
	return string(b)
}

func isWebURL(v string) bool {
	u, err := url.Parse(strings.TrimSpace(v))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && !strings.ContainsAny(v, " \n")
}

// withTrackedLinks returns a copy of the recipient whose URL parameters point at
// per-recipient short links, so clicks can be counted and tied to the contact.
// Without a public URL configured the recipient is returned unchanged.
func (w *Worker) withTrackedLinks(campaign *models.BulkMessageCampaign, recipient *models.BulkMessageRecipient) *models.BulkMessageRecipient {
	if !campaign.TrackLinks || w.Config == nil {
		return recipient
	}
	base := strings.TrimRight(w.Config.Server.PublicURL, "/")
	if base == "" {
		return recipient
	}
	out := *recipient
	out.TemplateParams = models.JSONB{}
	for key, raw := range recipient.TemplateParams {
		value := fmt.Sprintf("%v", raw)
		if !isWebURL(value) {
			out.TemplateParams[key] = raw
			continue
		}
		// Reuse the link on retries so a recipient keeps one code per URL
		var link models.TrackedLink
		err := w.DB.Where("recipient_id = ? AND target_url = ?", recipient.ID, value).First(&link).Error
		if err != nil {
			campaignID, recipientID := campaign.ID, recipient.ID
			link = models.TrackedLink{
				OrganizationID: campaign.OrganizationID,
				Code:           newLinkCode(),
				TargetURL:      value,
				CampaignID:     &campaignID,
				RecipientID:    &recipientID,
				ContactPhone:   recipient.PhoneNumber,
			}
			if err := w.DB.Create(&link).Error; err != nil {
				w.Log.Warn("Could not create tracked link; sending the original URL", "error", err)
				out.TemplateParams[key] = raw
				continue
			}
		}
		out.TemplateParams[key] = base + "/l/" + link.Code
	}
	return &out
}
