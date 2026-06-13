package monitor

import (
	"context"
	"time"

	"aranea-agents/internal/event/contract"
)

func RecoveryThreshold(rule AlertRule) float64 {
	return recoveryThreshold(rule)
}

func ParseMetadataJSON(raw string) map[string]any {
	return parseMetadataJSON(raw)
}

func NonEmpty(ss ...string) []string {
	return nonEmpty(ss...)
}

func MatchStepID(pattern, stepID string) bool {
	return matchStepID(pattern, stepID)
}

func MatchPrerequisite(pre Prerequisite, metadata map[string]any) bool {
	return matchPrerequisite(pre, metadata)
}

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

func (a *FlowFileAppender) OnEnvelopeExposed(env contract.Envelope) {
	a.onEnvelope(env)
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

func (a *FlowFileAppender) CloseAllFiles() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, f := range []*rotatingFile{a.flowFile, a.systemFile, a.traceFile, a.alertFile} {
		if f != nil {
			f.Close()
		}
	}
}

func (a *FlowFileAppender) RotatingFilePaths() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var paths []string
	for _, f := range []*rotatingFile{a.flowFile, a.systemFile, a.traceFile, a.alertFile} {
		if f != nil && f.file != nil {
			paths = append(paths, f.currentPath())
		}
	}
	return paths
}

// SetCooldownForTest allows tests to manipulate the cooldown state of PredictiveHealUsecase.
func (uc *PredictiveHealUsecase) SetCooldownForTest(actionType string, t time.Time) {
	uc.setCooldown(actionType, t)
}
