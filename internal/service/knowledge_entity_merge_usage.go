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

// entityMergeUsageRecorder adapts the knowledge entity-merge adjudicator's
// usage hook to UsageUsecase.RecordAuxLLMUsage (G-37 ADR-F, 2026-10-21).
//
// Same discipline as mocUsageRecorder: late binding via UsageUsecaseRef
// (curate sits upstream of UsageUsecase in the wire graph), best-effort
// recording (Warn-logged persistence failures never reach the governance
// round), async via safego so the curate round never blocks on a DB INSERT.
type entityMergeUsageRecorder struct {
	usageRef *biz.UsageUsecaseRef
	lg       loggateway.Logger
}

// NewEntityMergeUsageRecorder builds the recorder; nil usageRef disables
// recording (RecordEntityMergeUsage no-ops until the ref is populated).
func NewEntityMergeUsageRecorder(usageRef *biz.UsageUsecaseRef, lg loggateway.Logger) knowledge.EntityMergeUsageRecorder {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &entityMergeUsageRecorder{usageRef: usageRef, lg: lg}
}

func (r *entityMergeUsageRecorder) RecordEntityMergeUsage(ctx context.Context, in knowledge.EntityMergeUsageInput) {
	if r == nil || r.usageRef == nil {
		return
	}
	uc := r.usageRef.Get()
	if uc == nil {
		return
	}
	safego.Go(ctx, "knowledge.entity_merge_usage", func() {
		r.record(ctx, uc, in)
	})
}

// record persists one entity-merge adjudication usage row; runs inside the
// safego goroutine. Metadata carries task_type + pair ids/names (agent 出口
// 2048 cap 纪律：摘要+锚点，零全文零长证据）。
func (r *entityMergeUsageRecorder) record(ctx context.Context, uc *biz.UsageUsecase, in knowledge.EntityMergeUsageInput) {
	status := "success"
	errMsg := ""
	if in.Err != nil {
		status = "failed"
		errMsg = in.Err.Error()
	}
	meta, _ := json.Marshal(map[string]any{
		"task_type":   "entity_merge_adjudication",
		"keeper_id":   in.KeeperID,
		"keeper_name": in.KeeperName,
		"mergee_id":   in.MergeeID,
		"mergee_name": in.MergeeName,
		"similarity":  in.Similarity,
	})
	if err := uc.RecordAuxLLMUsage(ctx, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxEntityMerge,
		Provider:      strings.TrimSpace(in.Provider),
		Model:         strings.TrimSpace(in.Model),
		Status:        status,
		CompletionTok: in.Tokens,
		UsageSource:   biz.UsageSourceResponse,
		Latency:       in.Latency,
		ErrMsg:        errMsg,
		MetadataJSON:  string(meta),
	}); err != nil {
		r.lg.Warn("entity merge adjudication usage record failed",
			loggateway.StepID("knowledge.entity_merge_usage"),
			loggateway.Str("provider", in.Provider),
			loggateway.Str("model", in.Model),
			loggateway.Err(err))
	}
}
