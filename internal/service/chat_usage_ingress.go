package service

import (
	"context"
	"os"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

// recordChatIngressUsage forwards one token-usage row to admin SQLite when explicitly enabled:
// CHAT_RECORD_USAGE_INGRESS=1 (avoids doubling legacy pkg/backend inserts by default).
func recordChatIngressUsage(ctx context.Context, uc *biz.UsageUsecase, req *chatv1.SendChatMessageRequest, am map[string]any) {
	if uc == nil {
		return
	}
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("CHAT_RECORD_USAGE_INGRESS")))
	if raw != "1" && raw != "true" && raw != "yes" {
		return
	}
	if am == nil {
		return
	}
	tin := jsonNumberInt(am["token_in"])
	tout := jsonNumberInt(am["token_out"])
	if tin <= 0 && tout <= 0 {
		return
	}

	sess := strings.TrimSpace(req.GetSessionId())
	if sess == "" {
		sess = jsonString(am["session_id"])
	}

	ev := biz.TokenUsageEvent{
		ID:               uuid.NewString(),
		SessionID:      sess,
		TeamID:           strings.TrimSpace(req.GetTeamId()),
		AgentKey:         strings.TrimSpace(req.GetAgentKey()),
		MessageID:        jsonString(am["id"]),
		ModelAPIID:       jsonString(am["model_name"]),
		ModelDisplayName: jsonString(am["model_name"]),
		InputTokens:      tin,
		OutputTokens:     tout,
		TotalTokens:      tin + tout,
		LatencyMS:        jsonNumberInt(am["latency_ms"]),
		Status:           firstNonEmptyString(jsonString(am["status"]), "success"),
		StreamEnabled:    false,
		UsageKind:        "chat",
		MetadataJSON:     `{"source":"chat_ingress_unary"}`,
	}
	opts := req.GetOptions()
	if opts != nil {
		if ev.ModelAPIID == "" {
			ev.ModelAPIID = strings.TrimSpace(opts.GetModel())
			ev.ModelDisplayName = strings.TrimSpace(opts.GetModel())
		}
		ev.ProviderCode = strings.TrimSpace(opts.GetProvider())
		ev.PromptMode = strings.TrimSpace(opts.GetDialogMode())
	}
	if _, err := uc.RecordTokenUsageEvent(ctx, ev); err != nil {
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
