package watch

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

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
}

type monitorSyncReporter struct {
	mon        *biz.MonitorUsecase
	monitorBus contract.MonitorBus
	lg         loggateway.Logger
}

// NewMonitorSyncReporter writes monitor events and publishes monitor bus events.
func NewMonitorSyncReporter(mon *biz.MonitorUsecase, monitorBus contract.MonitorBus, lg loggateway.Logger) SyncReporter {
	if mon == nil && monitorBus == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &monitorSyncReporter{mon: mon, monitorBus: monitorBus, lg: lg}
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
	if r.mon != nil {
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
		r.mon.RecordAdminAudit(ctx, "skill.filesystem.sync", "skill", slug, message)
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
