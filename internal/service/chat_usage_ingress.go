package service

import (
	"context"
	"os"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"
)

// chatIngressRecordingEnabled is true only when CHAT_RECORD_USAGE_INGRESS is explicitly on.
// 主路径 recordTurnUsage 已写入；默认关闭 ingress 避免双写。
func chatIngressRecordingEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("CHAT_RECORD_USAGE_INGRESS")))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func chatIngressRecordingDisabled() bool {
	return !chatIngressRecordingEnabled()
}

// roughTokenEstimateFromText ~4 chars per token for CJK/Latin mix (display-only when API omits usage).
func roughTokenEstimateFromText(s string) int {
	return chatagent.RoughTokenEstimate(s)
}

// recordChatIngressUsage writes one model_token_usage_events row (legacy / 备用路径).
// 主路径为 trpc_turn.recordTurnUsage；native SendChatMessage 已不再调用本函数。
// 若需启用：在专用入口显式调用，并设置 CHAT_RECORD_USAGE_INGRESS=1（默认视为关闭语义：仅显式开启时写入）。
func recordChatIngressUsage(ctx context.Context, uc *biz.UsageUsecase, req *chatv1.SendChatMessageRequest, am map[string]any, streamEnabled bool) {
	if uc == nil {
		return
	}
	if chatIngressRecordingDisabled() {
		return
	}
	if am == nil {
		return
	}
	status := strutil.FirstNonEmpty(jsonutil.IfaceStr(am, "status"), "success")
	tin := jsonutil.IfaceInt(am, "token_in")
	tout := jsonutil.IfaceInt(am, "token_out")
	if tin <= 0 && tout <= 0 && status == "success" {
		tout = roughTokenEstimateFromText(jsonutil.IfaceStr(am, "content_markdown"))
	}

	latency := jsonutil.IfaceInt(am, "latency_ms")
	tps := 0.0
	if latency > 0 && tout > 0 {
		tps = float64(tout) / (float64(latency) / 1000.0)
	}

	sess := strings.TrimSpace(req.GetSessionId())
	if sess == "" {
		sess = jsonutil.IfaceStr(am, "session_id")
	}

	ev := biz.TokenUsageEvent{
		ID:               uuid.NewString(),
		SessionID:        sess,
		TeamID:           strings.TrimSpace(req.GetTeamId()),
		AgentKey:         strings.TrimSpace(req.GetAgentKey()),
		MessageID:        jsonutil.IfaceStr(am, "id"),
		ModelAPIID:       jsonutil.IfaceStr(am, "model_name"),
		ModelDisplayName: jsonutil.IfaceStr(am, "model_name"),
		InputTokens:      tin,
		OutputTokens:     tout,
		TotalTokens:      tin + tout,
		LatencyMS:        latency,
		TokensPerSecond:  tps,
		Status:           status,
		StreamEnabled:    streamEnabled,
		UsageKind:        "chat",
		MetadataJSON:     `{"source":"chat_ingress_native"}`,
	}
	if streamEnabled {
		ev.MetadataJSON = `{"source":"chat_ingress_native_stream"}`
	}
	ev.ErrorMessage = strutil.FirstNonEmpty(jsonutil.IfaceStr(am, "error_message"), jsonutil.IfaceStr(am, "errorMessage"))
	ev.ErrorCode = strutil.FirstNonEmpty(jsonutil.IfaceStr(am, "error_code"), jsonutil.IfaceStr(am, "errorCode"))
	opts := req.GetOptions()
	if opts != nil {
		if ev.ModelAPIID == "" {
			ev.ModelAPIID = strings.TrimSpace(opts.GetModel())
			ev.ModelDisplayName = strings.TrimSpace(opts.GetModel())
		}
		ev.ProviderCode = strings.TrimSpace(opts.GetProvider())
		ev.PromptMode = strings.TrimSpace(opts.GetDialogMode())
	}
	// Request ctx may already be expired after a long LLM call; usage insert uses its own deadline.
	recCtx, recCancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer recCancel()
	if _, err := uc.RecordTokenUsageEvent(recCtx, ev); err != nil {
		// best-effort; do not fail chat
		return
	}
}
