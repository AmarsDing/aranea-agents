package service

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

// embedUsageRecorder adapts the knowledge embedder's usage hook to
// UsageUsecase.RecordAuxLLMUsage (P1-3, 2026-08-19).
//
// Late binding: the embedder sits UPSTREAM of UsageUsecase in the wire graph
// (same cycle constraint as the session-title generator), so the recorder
// resolves the usecase lazily via UsageUsecaseRef at record time. Recording
// is best-effort — persistence failures are Warn-logged, never surfaced to
// the embedding caller.
//
// Attribution: embedding is a platform-shared resource (one embedder serves
// all agents/collections), so events carry no AgentID/SessionID; the
// metadata's task_type distinguishes ingest batches from retrieval queries.
type embedUsageRecorder struct {
	usageRef *biz.UsageUsecaseRef
	lg       loggateway.Logger
}

// NewEmbedUsageRecorder builds the recorder; nil usageRef disables recording
// (RecordEmbedUsage no-ops until the ref is populated post-startup).
func NewEmbedUsageRecorder(usageRef *biz.UsageUsecaseRef, lg loggateway.Logger) knowledge.EmbedUsageRecorder {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &embedUsageRecorder{usageRef: usageRef, lg: lg}
}

func (r *embedUsageRecorder) RecordEmbedUsage(ctx context.Context, in knowledge.EmbedUsageInput) {
	if r == nil || r.usageRef == nil {
		return
	}
	uc := r.usageRef.Get()
	if uc == nil {
		return
	}
	status := "success"
	errMsg := ""
	if in.Err != nil {
		status = "failed"
		errMsg = in.Err.Error()
	}
	meta, _ := json.Marshal(map[string]any{
		"task_type":  in.TaskType,
		"batch_size": in.BatchSize,
	})
	if err := uc.RecordAuxLLMUsage(ctx, biz.AuxLLMUsageInput{
		Kind:         biz.UsageKindAuxEmbedding,
		Provider:     strings.TrimSpace(in.Provider),
		Model:        strings.TrimSpace(in.Model),
		Status:       status,
		EmbeddingTok: in.Tokens,
		UsageSource:  in.UsageSource,
		Latency:      in.Latency,
		ErrMsg:       errMsg,
		MetadataJSON: string(meta),
	}); err != nil {
		r.lg.Warn("embed usage record failed",
			loggateway.StepID("knowledge.embed_usage"),
			loggateway.Str("provider", in.Provider),
			loggateway.Str("model", in.Model),
			loggateway.Err(err))
	}
}
