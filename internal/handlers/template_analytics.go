package handlers

import (
	"strings"
	"time"

	"github.com/Botrixai89/botrixai/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// TemplateUsageRow is one template's performance in the selected period.
type TemplateUsageRow struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	DisplayName     string     `json:"display_name"`
	Language        string     `json:"language"`
	Category        string     `json:"category"`
	Status          string     `json:"status"`
	WhatsAppAccount string     `json:"whatsapp_account"`
	QualityRating   string     `json:"quality_rating"`
	Sent            int64      `json:"sent"`
	Delivered       int64      `json:"delivered"`
	Read            int64      `json:"read"`
	Failed          int64      `json:"failed"`
	Spend           float64    `json:"spend"`
	LastSentAt      *time.Time `json:"last_sent_at"`
}

// GetTemplateAnalytics returns template status counts plus per-template
// sent/delivered/read/failed/spend for a date range (?from=&to=YYYY-MM-DD,
// default last 30 days; optional ?account=).
func (a *App) GetTemplateAnalytics(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceTemplates, models.ActionRead)
	if err != nil {
		return nil
	}
	args := r.RequestCtx.QueryArgs()
	now := time.Now()
	from := dayStart(now).AddDate(0, 0, -29)
	to := endOfDay(dayStart(now))
	if fs, ts := string(args.Peek("from")), string(args.Peek("to")); fs != "" || ts != "" {
		f, t, msg := parseDateRange(fs, ts)
		if msg != "" {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
		}
		from, to = f, t
	}
	if to.Sub(from) > 366*24*time.Hour {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Date range cannot exceed one year", nil, "")
	}
	account := string(args.Peek("account"))

	// Status counts (all templates, not limited by date)
	type statusRow struct {
		Status string
		N      int64
	}
	var statusRows []statusRow
	sq := a.DB.Model(&models.Template{}).Select("UPPER(COALESCE(status, '')) AS status, COUNT(*) AS n").
		Where("organization_id = ?", orgID)
	if account != "" {
		sq = sq.Where("whats_app_account = ?", account)
	}
	sq.Group("UPPER(COALESCE(status, ''))").Scan(&statusRows)
	counts := map[string]int64{"total": 0, "approved": 0, "pending": 0, "rejected": 0, "paused": 0, "disabled": 0, "draft": 0}
	for _, s := range statusRows {
		counts["total"] += s.N
		key := strings.ToLower(s.Status)
		if key == "" {
			key = "draft"
		}
		if _, ok := counts[key]; ok {
			counts[key] += s.N
		} else {
			counts["other"] += s.N
		}
	}

	// Per-template usage from sent messages, joined with wallet charges
	accountFilter := ""
	params := []any{orgID, models.DirectionOutgoing, models.MessageTypeTemplate, from, to, orgID}
	if account != "" {
		accountFilter = " AND t.whats_app_account = ?"
		params = append(params, account)
	}
	var rows []TemplateUsageRow
	if err := a.DB.Raw(`
		WITH u AS (
			SELECT m.template_name, m.whats_app_account,
				COUNT(*) AS sent,
				COUNT(*) FILTER (WHERE m.status IN ('delivered', 'read')) AS delivered,
				COUNT(*) FILTER (WHERE m.status = 'read') AS read,
				COUNT(*) FILTER (WHERE m.status = 'failed') AS failed,
				COALESCE(SUM(mc.amount) FILTER (WHERE mc.state = 'charged'), 0) AS spend,
				MAX(m.created_at) AS last_sent_at
			FROM messages m
			LEFT JOIN message_charges mc ON mc.wa_message_id = m.whats_app_message_id AND m.whats_app_message_id <> ''
			WHERE m.organization_id = ? AND m.direction = ? AND m.message_type = ?
				AND m.created_at BETWEEN ? AND ? AND m.deleted_at IS NULL AND m.template_name <> ''
			GROUP BY m.template_name, m.whats_app_account
		)
		SELECT t.id, t.name, t.display_name, t.language, t.category, t.status, t.whats_app_account, t.quality_rating,
			COALESCE(u.sent, 0) AS sent, COALESCE(u.delivered, 0) AS delivered, COALESCE(u.read, 0) AS read,
			COALESCE(u.failed, 0) AS failed, COALESCE(u.spend, 0) AS spend, u.last_sent_at
		FROM templates t
		LEFT JOIN u ON u.template_name = t.name AND u.whats_app_account = t.whats_app_account
		WHERE t.organization_id = ? AND t.deleted_at IS NULL`+accountFilter+`
		ORDER BY COALESCE(u.sent, 0) DESC, t.name ASC`, params...).Scan(&rows).Error; err != nil {
		a.Log.Error("Failed to load template analytics", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load template analytics", nil, "")
	}

	var totals struct {
		Sent      int64   `json:"sent"`
		Delivered int64   `json:"delivered"`
		Read      int64   `json:"read"`
		Failed    int64   `json:"failed"`
		Spend     float64 `json:"spend"`
	}
	for _, row := range rows {
		totals.Sent += row.Sent
		totals.Delivered += row.Delivered
		totals.Read += row.Read
		totals.Failed += row.Failed
		totals.Spend += row.Spend
	}

	// Daily template sends
	type dayRow struct {
		Day       time.Time
		Sent      int64
		Delivered int64
		Read      int64
	}
	var dayRows []dayRow
	dq := a.DB.Model(&models.Message{}).
		Select(`date_trunc('day', created_at) AS day, COUNT(*) AS sent,
			COUNT(*) FILTER (WHERE status IN ('delivered', 'read')) AS delivered,
			COUNT(*) FILTER (WHERE status = 'read') AS read`).
		Where("organization_id = ? AND direction = ? AND message_type = ? AND created_at BETWEEN ? AND ?",
			orgID, models.DirectionOutgoing, models.MessageTypeTemplate, from, to)
	if account != "" {
		dq = dq.Where("whats_app_account = ?", account)
	}
	dq.Group("date_trunc('day', created_at)").Scan(&dayRows)
	byDay := map[string]dayRow{}
	for _, d := range dayRows {
		byDay[d.Day.Format("2006-01-02")] = d
	}
	type dailyPoint struct {
		Date      string `json:"date"`
		Sent      int64  `json:"sent"`
		Delivered int64  `json:"delivered"`
		Read      int64  `json:"read"`
	}
	var daily []dailyPoint
	for d := dayStart(from); !d.After(to); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		row := byDay[key]
		daily = append(daily, dailyPoint{Date: key, Sent: row.Sent, Delivered: row.Delivered, Read: row.Read})
	}

	if rows == nil {
		rows = []TemplateUsageRow{}
	}
	return r.SendEnvelope(map[string]any{
		"from":          from.Format("2006-01-02"),
		"to":            to.Format("2006-01-02"),
		"status_counts": counts,
		"totals":        totals,
		"templates":     rows,
		"daily":         daily,
		"currency":      a.Config.Billing.Currency,
	})
}
