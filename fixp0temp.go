//go:build ignore
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	files := []struct {
		path   string
		oldStr string
		newStr string
	}{
		// P0-4: memory_episode_backfill.go - add stats field
		{
			"f:\\aranea-agents\\internal\\cronrunner\\jobs\\memory_episode_backfill.go",
			"\tsys         biz.SystemSettingRepo\n\tlg          loggateway.Logger\n}",
			"\tsys         biz.SystemSettingRepo\n\tstats       *biz.MemoryWorkerStats\n\tlg          loggateway.Logger\n}",
		},
		// P0-4: memory_episode_backfill.go - update constructor signature
		{
			"f:\\aranea-agents\\internal\\cronrunner\\jobs\\memory_episode_backfill.go",
			"func NewMemoryEpisodeBackfillWorker(interval time.Duration, reader biz.MemoryEpisodeBackfillReader, episodeSync biz.EpisodeIndexSyncer, sys biz.SystemSettingRepo, lg loggateway.Logger) *MemoryEpisodeBackfillWorker {",
			"func NewMemoryEpisodeBackfillWorker(interval time.Duration, reader biz.MemoryEpisodeBackfillReader, episodeSync biz.EpisodeIndexSyncer, sys biz.SystemSettingRepo, stats *biz.MemoryWorkerStats, lg loggateway.Logger) *MemoryEpisodeBackfillWorker {",
		},
		// P0-4: memory_episode_backfill.go - update constructor body
		{
			"f:\\aranea-agents\\internal\\cronrunner\\jobs\\memory_episode_backfill.go",
			"return &MemoryEpisodeBackfillWorker{interval: interval, reader: reader, episodeSync: episodeSync, sys: sys, lg: lg}",
			"return &MemoryEpisodeBackfillWorker{interval: interval, reader: reader, episodeSync: episodeSync, sys: sys, stats: stats, lg: lg}",
		},
		// P0-4: memory_episode_backfill.go - replace global call
		{
			"f:\\aranea-agents\\internal\\cronrunner\\jobs\\memory_episode_backfill.go",
			"biz.MemoryWorkerStatsGlobal().RecordEpisodeBackfill(n)",
			"w.stats.RecordEpisodeBackfill(n)",
		},
	}

	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			fmt.Printf("ERROR reading %s: %v\n", f.path, err)
			continue
		}
		content := string(data)
		if !strings.Contains(content, f.oldStr) {
			fmt.Printf("SKIP %s: old string not found\n", f.path)
			continue
		}
		if strings.Contains(content, f.newStr) {
			fmt.Printf("SKIP %s: new string already present\n", f.path)
			continue
		}
		content = strings.Replace(content, f.oldStr, f.newStr, 1)
		if err := os.WriteFile(f.path, []byte(content), 0644); err != nil {
			fmt.Printf("ERROR writing %s: %v\n", f.path, err)
			continue
		}
		fmt.Printf("OK %s\n", f.path)
	}
}
