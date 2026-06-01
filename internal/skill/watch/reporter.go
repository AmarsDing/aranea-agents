package watch

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
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
	mon *biz.MonitorUsecase
	bus event.Bus
}

// NewMonitorSyncReporter writes monitor events and publishes bus envelopes.
func NewMonitorSyncReporter(mon *biz.MonitorUsecase, bus event.Bus) SyncReporter {
	if mon == nil && bus == nil {
		return nil
	}
	return &monitorSyncReporter{mon: mon, bus: bus}
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
		_ = r.mon.RecordMonitorEvent(ctx, biz.MonitorEventWrite{
			EventKey:     eventKey,
			Name:         slug,
			Description:  message,
			Status:       severity,
			MetadataJSON: string(meta),
		})
		r.mon.RecordAdminAudit(ctx, "skill.filesystem.sync", "skill", slug, message)
	}
	if r.bus != nil {
		env := event.NewEnvelope(contract.EnvelopeType(eventKey), "skill.watch", "")
		env.Channel = "monitor"
		env.Metadata = map[string]any{
			"slug":     slug,
			"message":  message,
			"severity": severity,
		}
		if message != "" {
			env.Content = &event.EnvelopeContent{Text: message}
		}
		r.bus.Publish(ctx, env)
	}
}
