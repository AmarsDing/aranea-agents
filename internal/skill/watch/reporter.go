package watch

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// reportDedupWindow bounds how often the same (eventKey, slug) governance
// event is persisted. reconcile runs every 5 minutes by default; without
// dedup a persistently missing/invalid skill dir writes monitor_events +
// admin_audit rows every round. Within the window only the first row lands.
const reportDedupWindow = time.Hour

// monitorEventPersister is the narrow persistence dependency of the reporter
// (*biz.MonitorUsecase satisfies it). Narrowed for testability.
type monitorEventPersister interface {
	RecordMonitorEvent(ctx context.Context, ev biz.MonitorEventWrite) error
	RecordAdminAudit(ctx context.Context, e biz.AdminAuditEntry)
}

// SyncReporter persists filesystem sync notifications (monitor + optional bus).
type SyncReporter interface {
	ReportFilesystemSync(ctx context.Context, report SyncReport)
}

// SyncReport describes one filesystem sync notification.
type SyncReport struct {
	EventKey string
	Slug     string
	Message  string
	Severity string
	// SkipPersist=true 时仅发布 MonitorBus 事件，不落 monitor_events / admin_audit。
	// 用于高频低价值的常规同步（skill.filesystem.updated/info）——
	// 完整审计已由 skill_invocations 承担，落库只会淹没 Events 页。
	SkipPersist bool
}

type monitorSyncReporter struct {
	mon        monitorEventPersister
	monitorBus contract.MonitorBus
	lg         loggateway.Logger

	// dedupLast records the last persisted time per (eventKey, slug) key so
	// repeated reconcile rounds for a persistently broken skill don't flood
	// monitor_events / admin_audit. Guarded by dedupMu.
	dedupMu   sync.Mutex
	dedupLast map[string]time.Time
}

// NewMonitorSyncReporter writes monitor events and publishes monitor bus events.
func NewMonitorSyncReporter(mon *biz.MonitorUsecase, monitorBus contract.MonitorBus, lg loggateway.Logger) SyncReporter {
	if mon == nil && monitorBus == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var persister monitorEventPersister
	if mon != nil {
		persister = mon
	}
	return &monitorSyncReporter{mon: persister, monitorBus: monitorBus, lg: lg, dedupLast: map[string]time.Time{}}
}

// shouldPersist reports whether this (eventKey, slug) pair may be persisted
// now: the first occurrence within reportDedupWindow passes, repeats are
// dropped. Expired entries are swept on each call to bound map growth.
func (r *monitorSyncReporter) shouldPersist(eventKey, slug string) bool {
	key := eventKey + "|" + slug
	now := time.Now()
	r.dedupMu.Lock()
	defer r.dedupMu.Unlock()
	if r.dedupLast == nil {
		r.dedupLast = map[string]time.Time{}
	}
	for k, ts := range r.dedupLast {
		if now.Sub(ts) >= reportDedupWindow {
			delete(r.dedupLast, k)
		}
	}
	if last, ok := r.dedupLast[key]; ok && now.Sub(last) < reportDedupWindow {
		return false
	}
	r.dedupLast[key] = now
	return true
}

func (r *monitorSyncReporter) ReportFilesystemSync(ctx context.Context, report SyncReport) {
	if r == nil {
		return
	}
	eventKey := strings.TrimSpace(report.EventKey)
	if eventKey == "" {
		return
	}
	slug := strings.TrimSpace(report.Slug)
	severity := strings.TrimSpace(report.Severity)
	if severity == "" {
		severity = "info"
	}
	message := strings.TrimSpace(report.Message)
	// Dedup only gates DB persistence; the MonitorBus publish below still
	// fires every time so the UI keeps realtime visibility of repeats.
	if r.mon != nil && !report.SkipPersist && r.shouldPersist(eventKey, slug) {
		meta, _ := json.Marshal(map[string]any{
			"slug":    slug,
			"message": message,
		})
		if err := r.mon.RecordMonitorEvent(ctx, biz.MonitorEventWrite{
			EventKey:     eventKey,
			Name:         slug,
			Description:  message,
			Status:       severity,
			MetadataJSON: string(meta),
		}); err != nil {
			r.lg.Warn("RecordMonitorEvent failed",
				loggateway.StepID("skill.watch"),
				loggateway.Err(err))
		}
		r.mon.RecordAdminAudit(ctx, biz.AdminAuditEntry{
			Action:     biz.AuditAction(biz.AuditVerbSync, "skill"),
			Resource:   "skill",
			ResourceID: slug,
			Summary:    message,
		})
	}
	if r.monitorBus != nil {
		ev := contract.NewMonitorEvent(mapEventKeyToMonitorType(eventKey), "skill.watch")
		ev.Metadata = map[string]any{
			"slug":     slug,
			"message":  message,
			"severity": severity,
		}
		if message != "" {
			ev.Message = message
		}
		r.monitorBus.Publish(ctx, ev)
	}
}

// mapEventKeyToMonitorType maps the legacy filesystem event key strings to
// their corresponding contract.MonitorEventType constants.
func mapEventKeyToMonitorType(eventKey string) contract.MonitorEventType {
	switch eventKey {
	case "skill.filesystem.updated":
		return contract.MonitorEventTypeSkillFilesystemUpdated
	case "skill.filesystem.recovered":
		return contract.MonitorEventTypeSkillFilesystemRecovered
	case "skill.filesystem.imported":
		return contract.MonitorEventTypeSkillFilesystemImported
	default:
		// Unknown event keys fall back to the generic "updated" type so that
		// ad-hoc keys (e.g. "skill.filesystem.missing"/".rejected"/".similarity_warn")
		// still propagate without losing the event.
		return contract.MonitorEventTypeSkillFilesystemUpdated
	}
}
