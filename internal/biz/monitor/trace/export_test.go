package trace

import (
	"context"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

func SpanKindFromStep(stepID, domain string) string {
	return spanKindFromStep(stepID, domain)
}

func MetaStr(m map[string]any, key string) string {
	return metaStr(m, key)
}

func CoalesceStr(a, b string) string {
	return coalesceStr(a, b)
}

func (p *TraceProjector) EnsureTraceExposed(ctx context.Context, traceID, sessionID, runID, agentID, provider, model, teamID, domain string) {
	p.ensureTrace(ctx, traceID, sessionID, runID, agentID, provider, model, teamID, domain)
}

func (p *TraceProjector) EvictStaleTracesExposed() {
	p.evictStaleTraces()
}

func (p *TraceProjector) HandleExposed(ctx context.Context, ev contract.MonitorEvent) {
	p.handle(ctx, ev)
}

func (p *TraceProjector) SetUpsertWarnIntervalForTest(d time.Duration) {
	p.upsertWarnInterval = d
}

// SetRepoWarnIntervalForTest replaces the insert/completion/usage-agg Warn
// throttles with fresh ones using the given window, so tests can exercise
// both the in-window suppression and the post-window re-emission.
func (p *TraceProjector) SetRepoWarnIntervalForTest(d time.Duration) {
	p.insertWarnThrottle = loggateway.NewThrottle(d)
	p.completionWarnThrottle = loggateway.NewThrottle(d)
	p.usageAggWarnThrottle = loggateway.NewThrottle(d)
}

func (p *TraceProjector) AddTestTrace(traceID string, createdAt time.Time) {
	p.mu.Lock()
	p.traces[traceID] = &activeTrace{
		traceID:   traceID,
		createdAt: createdAt,
	}
	p.mu.Unlock()
}

// RecordEventForTest updates the projector's last-event timestamp the
// same way handle() does for a real envelope. It exists so the
// self-check signal plumbing can be unit-tested without spinning up a
// real bus subscription.
func (p *TraceProjector) RecordEventForTest() {
	if p == nil {
		return
	}
	p.lastEventUnixNano.Store(time.Now().UnixNano())
}

// MarkStartedForTest flips the started flag without going through
// Start(). The signal is otherwise exclusively controlled by the
// production Start() path; the test helper exists for symmetry with
// RecordEventForTest.
func (p *TraceProjector) MarkStartedForTest() {
	if p == nil {
		return
	}
	p.started.Store(true)
}

func (a *FlowFileAppender) OnMonitorEventExposed(ev contract.MonitorEvent) {
	a.onMonitorEvent(ev)
}

func (a *FlowFileAppender) CompressOldFilesExposed() int {
	return a.compressOldFiles()
}

func (a *FlowFileAppender) PurgeExpiredFilesExposed() int {
	return a.purgeExpiredFiles()
}

func (a *FlowFileAppender) PurgeTmpFilesExposed() {
	a.purgeTmpFiles()
}

func (a *FlowFileAppender) SyncOpenFilesExposed() {
	a.syncOpenFiles()
}

func (a *FlowFileAppender) MaintenanceExposed() {
	a.maintenance()
}

func (a *FlowFileAppender) SetCompressAge(d time.Duration) {
	a.compressAge = d
}

func (a *FlowFileAppender) SetRetentionDays(days int) {
	a.retentionDays = days
}

func (a *FlowFileAppender) SetMaxBackups(n int) {
	a.maxBackups = n
}

// SetWriteMutedUntilForTest overrides the circuit-breaker mute deadline so
// tests can exercise the half-open probe path without waiting out the window.
func (a *FlowFileAppender) SetWriteMutedUntilForTest(t time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.writeMutedUntil = t
}

func (a *FlowFileAppender) PurgeExcessBackupsExposed() int {
	return a.purgeExcessBackups()
}

func (a *FlowFileAppender) CloseAllFiles() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, f := range []*rotatingFile{a.flowFile, a.systemFile, a.traceFile, a.alertFile, a.logFile} {
		if f != nil {
			f.Close()
		}
	}
}

func (a *FlowFileAppender) RotatingFilePaths() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var paths []string
	for _, f := range []*rotatingFile{a.flowFile, a.systemFile, a.traceFile, a.alertFile, a.logFile} {
		if f != nil && f.file != nil {
			paths = append(paths, f.currentPath())
		}
	}
	return paths
}
