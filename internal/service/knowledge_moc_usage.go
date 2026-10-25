package service

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// mocUsageRecorder adapts the knowledge MOC summarizer's usage hook to
// UsageUsecase.RecordAuxLLMUsage (G-34 ADR-C, 2026-10-21).
//
// Same discipline as embedUsageRecorder: late binding via UsageUsecaseRef
// (curate sits upstream of UsageUsecase in the wire graph), best-effort
// recording (Warn-logged persistence failures never reach the governance
// round), async via safego so the curate round never blocks on a DB INSERT.
// Tokens land in CompletionTok (biz.LLMCaller collapses the prompt/completion
// breakdown) with UsageSourceResponse.
type mocUsageRecorder struct {
	usageRef *biz.UsageUsecaseRef
	lg       loggateway.Logger
}

// NewMOCUsageRecorder builds the recorder; nil usageRef disables recording
// (RecordMOCSummaryUsage no-ops until the ref is populated post-startup).
func NewMOCUsageRecorder(usageRef *biz.UsageUsecaseRef, lg loggateway.Logger) knowledge.MOCUsageRecorder {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &mocUsageRecorder{usageRef: usageRef, lg: lg}
}

func (r *mocUsageRecorder) RecordMOCSummaryUsage(ctx context.Context, in knowledge.MOCUsageInput) {
	if r == nil || r.usageRef == nil {
		return
	}
	uc := r.usageRef.Get()
	if uc == nil {
		return
	}
	safego.Go(ctx, "knowledge.moc_summary_usage", func() {
		r.record(ctx, uc, in)
	})
}

// record persists one MOC summary usage row; runs inside the safego goroutine.
func (r *mocUsageRecorder) record(ctx context.Context, uc *biz.UsageUsecase, in knowledge.MOCUsageInput) {
	status := "success"
	errMsg := ""
	if in.Err != nil {
		status = "failed"
		errMsg = in.Err.Error()
	}
	meta, _ := json.Marshal(map[string]any{
		"task_type": "moc_summary",
		"doc_id":    in.DocID,
		"rel_path":  in.RelPath,
	})
	if err := uc.RecordAuxLLMUsage(ctx, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxMOCSummary,
		Provider:      strings.TrimSpace(in.Provider),
		Model:         strings.TrimSpace(in.Model),
		Status:        status,
		CompletionTok: in.Tokens,
		UsageSource:   biz.UsageSourceResponse,
		Latency:       in.Latency,
		ErrMsg:        errMsg,
		MetadataJSON:  string(meta),
	}); err != nil {
		r.lg.Warn("moc summary usage record failed",
			loggateway.StepID("knowledge.moc_summary_usage"),
			loggateway.Str("provider", in.Provider),
			loggateway.Str("model", in.Model),
			loggateway.Err(err))
	}
}
