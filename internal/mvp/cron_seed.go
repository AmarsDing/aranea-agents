package mvp

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"aranea-agents/internal/biz"
)

// SeedRSSIngestCronTask registers an optional mvp_rss_ingest cron when
// ARANEA_MVP_RSS_FEEDS is set (comma-separated URLs). Idempotent by task_key.
// Enabled only when feeds are present — ops can edit feed_urls in admin UI later.
func SeedRSSIngestCronTask(ctx context.Context, cronRepo biz.CronRepo) error {
	if cronRepo == nil {
		return nil
	}
	feeds := parseFeedEnv(os.Getenv("ARANEA_MVP_RSS_FEEDS"))
	tasks, err := cronRepo.ListCronTasks(ctx)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.TaskKey == "mvp-rss-ingest" {
			return nil
		}
	}
	cfg := map[string]any{
		"target_type":      "mvp_rss_ingest",
		"interval_seconds": 3600,
		"feed_urls":        feeds,
		"workspace_id":     strings.TrimSpace(os.Getenv("ARANEA_MVP_WORKSPACE_ID")),
		"message":          "MVP RSS → Content Inbox",
	}
	raw, _ := json.Marshal(cfg)
	enabled := len(feeds) > 0
	_, err = cronRepo.CreateCronTask(ctx, biz.CronTask{
		ID:         "cron_mvp_rss_ingest",
		TaskKey:    "mvp-rss-ingest",
		Name:       "MVP RSS Content Ingest",
		Enabled:    enabled,
		Status:     "active",
		ConfigJSON: string(raw),
	})
	return err
}

func parseFeedEnv(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
