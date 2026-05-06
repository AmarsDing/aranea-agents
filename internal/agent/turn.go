package agent

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// Deps are agent-runtime dependencies (biz facades).
type Deps struct {
	Agents  biz.AgentRepository
	AgentUC *biz.AgentUsecase
	Catalog *biz.LlmProviderModelUsecase
	HTTP    *http.Client
}

// SessionPersistence loads history and appends rows (typically *biz.SessionUsecase).
type SessionPersistence interface {
	ListMessages(ctx context.Context, sessionID string) ([]biz.ChatMessage, error)
	AppendChatTurn(ctx context.Context, sessionID string, user, assistant biz.ChatMessage) error
	AppendChatMessage(ctx context.Context, sessionID string, msg biz.ChatMessage, bumpModelCall bool) error
}

// StreamEmitter is optional SSE bridge (e.g. chat stream).
type StreamEmitter interface {
	Emit(event string, payload any) error
}

// TurnInput is one user utterance against a resolved catalog agent.
type TurnInput struct {
	SessionID        string
	Agent            biz.Agent
	UserContent      string
	DialogMode       string
	Provider         string
	Model            string
	AgentKeyFromReq  string
	AttachmentsCount int
	ContextRatio     float64
	TeamMember       *TeamMemberAnchor
}

// RFC3339Now returns UTC RFC3339 timestamp string for message rows.
func RFC3339Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ExecuteOpenAICompatTurn runs one model call and persists like legacy native chat.
func ExecuteOpenAICompatTurn(ctx context.Context, d Deps, sp SessionPersistence, in TurnInput, stream StreamEmitter) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	sessionID := strings.TrimSpace(in.SessionID)
	content := strings.TrimSpace(in.UserContent)
	if sessionID == "" || content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "session_id and content are required")
	}
	ag := in.Agent
	if ak := strings.TrimSpace(in.AgentKeyFromReq); ak != "" && in.TeamMember == nil && !strings.EqualFold(ak, ag.AgentKey) {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.Forbidden("CHAT_AGENT", "agent_key does not match this session")
	}

	dialogMode := strings.TrimSpace(in.DialogMode)
	prov := strings.TrimSpace(in.Provider)
	mod := strings.TrimSpace(in.Model)

	pm, err := d.Catalog.GetByProviderAndModel(ctx, prov, mod)
	if err != nil {
		if err == sql.ErrNoRows {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_MODEL", fmt.Sprintf("unknown provider/model: %s / %s (add it under LLM provider models)", prov, mod))
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	var cfg providerAPIConfig
	MergeProviderConfigJSON(pm.ConfigJSON, &cfg)
	if IsLikelyAnthropicNativeAPI(cfg.APIBaseURL, cfg.ProviderType) {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "Native admin chat uses OpenAI-compatible /chat/completions only; use an OpenAI-compatible base URL or set LEGACY_REST_ORIGIN.")
	}

	promptFiles, err := PromptFilesForAgent(ctx, d, ag)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	system := BuildSystemPrompt(ag, promptFiles)
	if cue := RuntimeCapabilityCue(ctx, d, ag); cue != "" {
		system = system + "\n\n" + cue
	}

	history, err := sp.ListMessages(ctx, sessionID)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	var oaMsgs []OpenAICompatMessage
	if system != "" {
		oaMsgs = append(oaMsgs, OpenAICompatMessage{Role: "system", Content: system})
	}
	for _, row := range history {
		role := strings.TrimSpace(strings.ToLower(row.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		body := strings.TrimSpace(row.ContentMarkdown)
		rc := strings.TrimSpace(ReasoningFromMessageOptionsJSON(row.OptionsJSON))
		if body == "" && rc == "" {
			continue
		}
		oaMsgs = append(oaMsgs, OpenAICompatMessage{Role: role, Content: body, ReasoningContent: rc})
	}
	oaMsgs = append(oaMsgs, OpenAICompatMessage{Role: "user", Content: content})

	userOpts, err := UserOptionsJSON(ag, dialogMode, prov, mod, in.ContextRatio, in.TeamMember)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	now := RFC3339Now()
	userMsg = biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sessionID,
		Role:             "user",
		ContentMarkdown:  content,
		Status:           "ok",
		OptionsJSON:      userOpts,
		CreatedAt:        now,
		AttachmentsCount: in.AttachmentsCount,
	}

	assistantOptsStr, err := AssistantOptionsJSON(ag, in.TeamMember)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	t0 := time.Now()

	if stream != nil {
		if err := sp.AppendChatMessage(ctx, sessionID, userMsg, false); err != nil {
			return biz.ChatMessage{}, biz.ChatMessage{}, err
		}
		_ = stream.Emit("user_message", userMsg)
	}

	var reply string
	var reasoning string
	var tin, tout int
	var callErr error
	reply, reasoning, tin, tout, callErr = CompleteOpenAIModelReply(ctx, d, cfg, pm.Model, ag, oaMsgs, stream)
	reply = strings.TrimSpace(reply)
	if assistantOptsStr, err = MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, reasoning); err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	latency := int(time.Since(t0).Milliseconds())
	assistantMsg = biz.ChatMessage{
		ID:              uuid.NewString(),
		SessionID:       sessionID,
		Role:            "assistant",
		ContentMarkdown: reply,
		ModelName:       pm.Model,
		TokenIn:         tin,
		TokenOut:        tout,
		LatencyMS:       latency,
		Status:          "ok",
		OptionsJSON:     assistantOptsStr,
		CreatedAt:       RFC3339Now(),
	}
	if callErr != nil {
		assistantMsg.Status = "error"
		assistantMsg.ErrorMessage = callErr.Error()
		assistantMsg.ContentMarkdown = "对话生成失败。"
		assistantMsg.TokenIn = 0
		assistantMsg.TokenOut = 0
		assistantMsg.LatencyMS = latency
	}

	pctx, pcancel := ChatPersistCtx(ctx)
	defer pcancel()
	if stream != nil {
		if err := sp.AppendChatMessage(pctx, sessionID, assistantMsg, true); err != nil {
			return userMsg, assistantMsg, err
		}
		_ = stream.Emit("done", assistantMsg)
		return userMsg, assistantMsg, nil
	}

	if err := sp.AppendChatTurn(pctx, sessionID, userMsg, assistantMsg); err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	return userMsg, assistantMsg, nil
}

const teamRelayCue = "（团队顺序流转）请根据上文继续履行你在团队中的职责，输出本轮结果。"

func endHistoryRole(history []biz.ChatMessage) string {
	for i := len(history) - 1; i >= 0; i-- {
		r := strings.TrimSpace(strings.ToLower(history[i].Role))
		if r == "" {
			continue
		}
		return r
	}
	return ""
}

// RelayStepInput is a follow-up model call in the same session (user line already persisted).
type RelayStepInput struct {
	SessionID       string
	Agent           biz.Agent
	DialogMode      string
	Provider        string
	Model           string
	AgentKeyFromReq string
	ContextRatio    float64
	TeamMember      *TeamMemberAnchor
	// RelayUserContent, if set, is appended as a user message after thread history (OpenAI payload only).
	RelayUserContent string
}

// ExecuteOpenAIRelayStep runs OpenAI-compat completion using existing message history only,
// optionally appending a synthetic user relay line when the thread ends with assistant.
func ExecuteOpenAIRelayStep(ctx context.Context, d Deps, sp SessionPersistence, in RelayStepInput, stream StreamEmitter) (assistantMsg biz.ChatMessage, err error) {
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "session_id is required")
	}
	ag := in.Agent
	if ak := strings.TrimSpace(in.AgentKeyFromReq); ak != "" && in.TeamMember == nil && !strings.EqualFold(ak, ag.AgentKey) {
		return biz.ChatMessage{}, kerrors.Forbidden("CHAT_AGENT", "agent_key does not match this session")
	}

	dialogMode := strings.TrimSpace(in.DialogMode)
	prov := strings.TrimSpace(in.Provider)
	mod := strings.TrimSpace(in.Model)

	pm, err := d.Catalog.GetByProviderAndModel(ctx, prov, mod)
	if err != nil {
		if err == sql.ErrNoRows {
			return biz.ChatMessage{}, kerrors.BadRequest("CHAT_MODEL", fmt.Sprintf("unknown provider/model: %s / %s (add it under LLM provider models)", prov, mod))
		}
		return biz.ChatMessage{}, err
	}

	var cfg providerAPIConfig
	MergeProviderConfigJSON(pm.ConfigJSON, &cfg)
	if IsLikelyAnthropicNativeAPI(cfg.APIBaseURL, cfg.ProviderType) {
		return biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "Native admin chat uses OpenAI-compatible /chat/completions only; use an OpenAI-compatible base URL or set LEGACY_REST_ORIGIN.")
	}

	promptFiles, err := PromptFilesForAgent(ctx, d, ag)
	if err != nil {
		return biz.ChatMessage{}, err
	}
	system := BuildSystemPrompt(ag, promptFiles)
	if cue := RuntimeCapabilityCue(ctx, d, ag); cue != "" {
		system = system + "\n\n" + cue
	}

	history, err := sp.ListMessages(ctx, sessionID)
	if err != nil {
		return biz.ChatMessage{}, err
	}
	var oaMsgs []OpenAICompatMessage
	if system != "" {
		oaMsgs = append(oaMsgs, OpenAICompatMessage{Role: "system", Content: system})
	}
	for _, row := range history {
		role := strings.TrimSpace(strings.ToLower(row.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		body := strings.TrimSpace(row.ContentMarkdown)
		rc := strings.TrimSpace(ReasoningFromMessageOptionsJSON(row.OptionsJSON))
		if body == "" && rc == "" {
			continue
		}
		oaMsgs = append(oaMsgs, OpenAICompatMessage{Role: role, Content: body, ReasoningContent: rc})
	}
	if cue := strings.TrimSpace(in.RelayUserContent); cue != "" {
		oaMsgs = append(oaMsgs, OpenAICompatMessage{Role: "user", Content: cue})
	} else if endHistoryRole(history) == "assistant" {
		oaMsgs = append(oaMsgs, OpenAICompatMessage{Role: "user", Content: teamRelayCue})
	}

	assistantOptsStr, err := AssistantOptionsJSON(ag, in.TeamMember)
	if err != nil {
		return biz.ChatMessage{}, err
	}

	t0 := time.Now()
	var reply string
	var reasoning string
	var tin, tout int
	var callErr error
	reply, reasoning, tin, tout, callErr = CompleteOpenAIModelReply(ctx, d, cfg, pm.Model, ag, oaMsgs, stream)
	reply = strings.TrimSpace(reply)
	if assistantOptsStr, err = MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, reasoning); err != nil {
		return biz.ChatMessage{}, err
	}
	latency := int(time.Since(t0).Milliseconds())
	assistantMsg = biz.ChatMessage{
		ID:              uuid.NewString(),
		SessionID:       sessionID,
		Role:            "assistant",
		ContentMarkdown: reply,
		ModelName:       pm.Model,
		TokenIn:         tin,
		TokenOut:        tout,
		LatencyMS:       latency,
		Status:          "ok",
		OptionsJSON:     assistantOptsStr,
		CreatedAt:       RFC3339Now(),
	}
	if callErr != nil {
		assistantMsg.Status = "error"
		assistantMsg.ErrorMessage = callErr.Error()
		assistantMsg.ContentMarkdown = "对话生成失败。"
		assistantMsg.TokenIn = 0
		assistantMsg.TokenOut = 0
		assistantMsg.LatencyMS = latency
	}

	_ = dialogMode

	pctx, pcancel := ChatPersistCtx(ctx)
	defer pcancel()
	if err := sp.AppendChatMessage(pctx, sessionID, assistantMsg, true); err != nil {
		return biz.ChatMessage{}, err
	}
	if stream != nil {
		_ = stream.Emit("done", assistantMsg)
	}
	return assistantMsg, nil
}
