package agent

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// ParallelWorkerInput runs one member against a single user line (no prior assistant context).
type ParallelWorkerInput struct {
	SessionID       string
	Agent           biz.Agent
	UserLine        string
	DialogMode      string
	Provider        string
	Model           string
	AgentKeyFromReq string
	TeamMember      *TeamMemberAnchor
	// Stream when set requests OpenAI SSE chunks forwarded as delta events (then done if persisted).
	Stream StreamEmitter
	// SkipPersist skips AppendChatMessage so the caller can order writes (parallel team).
	SkipPersist bool
}

// ExecuteOpenAIParallelMember runs an isolated system+user completion and appends only the assistant row.
func ExecuteOpenAIParallelMember(ctx context.Context, d Deps, sp SessionPersistence, in ParallelWorkerInput) (biz.ChatMessage, error) {
	sessionID := strings.TrimSpace(in.SessionID)
	userLine := strings.TrimSpace(in.UserLine)
	if sessionID == "" || userLine == "" {
		return biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "session_id and user line are required")
	}
	ag := in.Agent
	if ak := strings.TrimSpace(in.AgentKeyFromReq); ak != "" && in.TeamMember == nil && !strings.EqualFold(ak, ag.AgentKey) {
		return biz.ChatMessage{}, kerrors.Forbidden("CHAT_AGENT", "agent_key does not match this session")
	}
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
	var oaMsgs []OpenAICompatMessage
	if system != "" {
		oaMsgs = append(oaMsgs, OpenAICompatMessage{Role: "system", Content: system})
	}
	oaMsgs = append(oaMsgs, OpenAICompatMessage{Role: "user", Content: userLine})

	assistantOptsStr, err := AssistantOptionsJSON(ag, in.TeamMember)
	if err != nil {
		return biz.ChatMessage{}, err
	}

	t0 := time.Now()
	reply, reasoning, tin, tout, callErr := CompleteOpenAIModelReply(ctx, d, cfg, pm.Model, ag, oaMsgs, in.Stream)
	reply = strings.TrimSpace(reply)
	assistantOptsStr, err = MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, reasoning)
	if err != nil {
		return biz.ChatMessage{}, err
	}
	latency := int(time.Since(t0).Milliseconds())
	assistantMsg := biz.ChatMessage{
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

	_ = in.DialogMode

	if !in.SkipPersist {
		pctx, pcancel := ChatPersistCtx(ctx)
		defer pcancel()
		if err := sp.AppendChatMessage(pctx, sessionID, assistantMsg, true); err != nil {
			return biz.ChatMessage{}, err
		}
		if in.Stream != nil {
			_ = in.Stream.Emit("done", assistantMsg)
		}
	}
	return assistantMsg, nil
}
