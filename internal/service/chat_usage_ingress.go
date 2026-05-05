package service

import (
	"context"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

// chatIngressRecordingDisabled is true only when CHAT_RECORD_USAGE_INGRESS is explicitly off
// (legacy dual-stack with pkg/backend may set this to avoid duplicate rows).
func chatIngressRecordingDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("CHAT_RECORD_USAGE_INGRESS")))
	return raw == "0" || raw == "false" || raw == "no" || raw == "off"
}

// roughTokenEstimateFromText ~4 chars per token for CJK/Latin mix (display-only when API omits usage).
func roughTokenEstimateFromText(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n := utf8.RuneCountInString(s)
	if n < 1 {
		return 0
	}
	est := n / 4
	if est < 1 {
		return 1
	}
	return est
}

// recordChatIngressUsage writes one model_token_usage_events row for native admin chat.
// Recording is ON by default; set CHAT_RECORD_USAGE_INGRESS=0|false|no|off to disable.
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
	status := firstNonEmptyString(jsonString(am["status"]), "success")
	tin := jsonNumberInt(am["token_in"])
	tout := jsonNumberInt(am["token_out"])
	if tin <= 0 && tout <= 0 && status == "success" {
		// Streaming providers often omit usage unless stream_options.include_usage is supported.
		tout = roughTokenEstimateFromText(jsonString(am["content_markdown"]))
	}

	latency := jsonNumberInt(am["latency_ms"])
	tps := 0.0
	if latency > 0 && tout > 0 {
		tps = float64(tout) / (float64(latency) / 1000.0)
	}

	sess := strings.TrimSpace(req.GetSessionId())
	if sess == "" {
		sess = jsonString(am["session_id"])
	}

	ev := biz.TokenUsageEvent{
		ID:               uuid.NewString(),
		SessionID:        sess,
		TeamID:           strings.TrimSpace(req.GetTeamId()),
		AgentKey:         strings.TrimSpace(req.GetAgentKey()),
		MessageID:        jsonString(am["id"]),
		ModelAPIID:       jsonString(am["model_name"]),
		ModelDisplayName: jsonString(am["model_name"]),
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
	ev.ErrorMessage = firstNonEmptyString(jsonString(am["error_message"]), jsonString(am["errorMessage"]))
	ev.ErrorCode = firstNonEmptyString(jsonString(am["error_code"]), jsonString(am["errorCode"]))
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

func jsonString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		return ""
	}
}

func jsonNumberInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	default:
		return 0
	}
}

func firstNonEmptyString(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
