// ceoboard 存储指标(自 ceo-dashboard/internal/store/metrics.go 移植,指标名保持
// 不变以便双跑对比与既有告警规则复用)。
// 移植差异:SQLite 文件大小 → pg_database_size(rawDB 为空时记 0);WAL 文件大小 →
// pg_ls_waldir()(权限不足等失败时静默记 0,不阻断指标)。
package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/ceoboardevent"
)

// MetricsText 输出 Prometheus 文本格式的存储指标。
func (s *Store) MetricsText(ctx context.Context) (string, error) {
	events, err := s.TotalEvents(ctx)
	if err != nil {
		return "", err
	}
	var dbSize, walSize int64
	if s.rawDB != nil {
		if err := s.rawDB.QueryRowContext(ctx, `SELECT pg_database_size(current_database())`).Scan(&dbSize); err != nil {
			dbSize = 0
		}
		// pg_ls_waldir 需要 pg_monitor/超级用户;失败静默记 0(不阻断主指标)。
		if err := s.rawDB.QueryRowContext(ctx, `SELECT COALESCE(SUM(size),0) FROM pg_ls_waldir()`).Scan(&walSize); err != nil {
			walSize = 0
		}
	}
	var spoolBytes int64
	if s.spool != nil {
		spoolBytes = s.spool.pendingBytes()
	}
	lag := 0
	oldest, err := s.client.CeoboardEvent.Query().
		Order(ent.Asc(ceoboardevent.FieldReceivedAt)).
		First(ctx)
	switch {
	case err == nil:
		lag = int(time.Since(oldest.ReceivedAt).Hours() / 24)
		if lag < 0 {
			lag = 0
		}
	case ent.IsNotFound(err):
		// 空表:无滞后
	default:
		return "", err
	}
	capacity := s.dbCapacityBytes
	if capacity <= 0 {
		capacity = 10 * 1024 * 1024 * 1024
	}
	var b strings.Builder
	fmt.Fprintf(&b, "ceoboard_spool_write_failed_total %d\n", s.spoolWriteFailed.Load())
	fmt.Fprintf(&b, "ceoboard_spool_pending_bytes %d\n", spoolBytes)
	fmt.Fprintf(&b, "ceoboard_db_size_bytes %d\n", dbSize)
	fmt.Fprintf(&b, "ceoboard_wal_size_bytes %d\n", walSize)
	fmt.Fprintf(&b, "ceoboard_events_rows %d\n", events)
	fmt.Fprintf(&b, "ceoboard_archive_lag_days %d\n", lag)
	ratio := float64(dbSize+walSize) / float64(capacity)
	fmt.Fprintf(&b, "ceoboard_db_capacity_used_ratio %.8f\n", ratio)
	alert := 0
	if ratio >= 0.8 {
		alert = 1
	}
	fmt.Fprintf(&b, "ceoboard_db_capacity_alert %d\n", alert)
	fmt.Fprintf(&b, "ceoboard_audit_write_failed_total %d\n", s.auditWriteFailed.Load())
	return b.String(), nil
}
