package agent

import (
	"aranea-agents/internal/biz"
	"context"
	"encoding/json"
)

// marshalSynthesisOutput serializes a SynthesisOutput to JSON.
func marshalSynthesisOutput(output *biz.SynthesisOutput) (string, error) {
	type jsonOutput struct {
		Content       string                    `json:"content"`
		Strategy      string                    `json:"strategy"`
		TeamResults   []biz.TeamSynthesisResult `json:"team_results"`
		SynthesizedAt string                    `json:"synthesized_at"`
	}
	jo := jsonOutput{
		Content:       output.Content,
		Strategy:      string(output.Strategy),
		TeamResults:   output.TeamResults,
		SynthesizedAt: output.SynthesizedAt,
	}
	b, err := json.Marshal(jo)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// publishOrchestrationInterrupted publishes the spirit_orchestration_interrupted event as a v2 SystemNoticeEvent.
func (o *TaskOrchestratorImpl) publishOrchestrationInterrupted(ctx context.Context, handle *biz.OrchestrationHandle) {
	if o.eventBus == nil || handle == nil {
		return
	}
	spiritSessionID := handle.SpiritSessionID

	meta := map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": spiritSessionID,
		"status":            string(handle.Status),
		"notice_type":       "warning",
	}
	if handle.CancelReason != "" {
		meta["cancel_reason"] = string(handle.CancelReason)
	}
	o.eventBus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_interrupted", "任务编排已中断", meta))
}
