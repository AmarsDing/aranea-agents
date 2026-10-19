package intent

import (
	"aranea-agents/internal/agent/llmcompat"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// RunResult is the outcome of Run (for main path wiring and TeamRunEvent / monitor).
type RunResult struct {
	Artifact *Artifact
	RawJSON  string
	Duration time.Duration
	// Outcome is one of: completed, skipped_disabled, skipped_preflight, skipped_catalog, skipped_llm, skipped_parse
	Outcome string
	// PromptTok/CompletionTok are provider-reported usage of the intent call
	// (0 when the call never happened or the provider returned no usage).
	// Surfaced so callers can record aux LLM usage (P1-2, 2026-08-19).
	PromptTok     int
	CompletionTok int
}

// RunMeta carries IDs duplicated into intent_pass event payload (snake_case) for clients that only read payload.
type RunMeta struct {
	AgentID   string
	SessionID string
	RunID     string
	TeamID    string
}

// BuildIntentPassPayload builds TeamRunEvent.Payload for type "intent_pass" (orchestration facts; no raw user text).
func BuildIntentPassPayload(r RunResult, meta RunMeta) map[string]any {
	out := map[string]any{
		"outcome":     r.Outcome,
		"duration_ms": r.Duration.Milliseconds(),
	}
	if meta.SessionID != "" {
		out["session_id"] = meta.SessionID
	}
	if meta.TeamID != "" {
		out["team_id"] = meta.TeamID
	}
	if meta.RunID != "" {
		out["run_id"] = meta.RunID
	}
	if meta.AgentID != "" {
		out["agent_id"] = meta.AgentID
	}
	if r.Artifact != nil {
		out["intent_kind"] = strings.TrimSpace(r.Artifact.IntentKind)
		out["refined_goal_len"] = len(strings.TrimSpace(r.Artifact.RefinedGoal))
		out["search_hints_count"] = len(r.Artifact.SearchHints)

		out["sub_intents_count"] = len(r.Artifact.SubIntents)
	}
	return out
}

func MonitorLogEntry(r RunResult, scope string, meta RunMeta) (level, msg string) {
	var sb strings.Builder
	sb.WriteString("intent_pass[")
	sb.WriteString(scope)
	sb.WriteString("] outcome=")
	sb.WriteString(r.Outcome)
	fmt.Fprintf(&sb, " duration_ms=%d", r.Duration.Milliseconds())
	if meta.SessionID != "" {
		fmt.Fprintf(&sb, " session_id=%s", meta.SessionID)
	}
	if meta.TeamID != "" {
		fmt.Fprintf(&sb, " team_id=%s", meta.TeamID)
	}
	if meta.RunID != "" {
		fmt.Fprintf(&sb, " run_id=%s", meta.RunID)
	}
	if meta.AgentID != "" {
		fmt.Fprintf(&sb, " agent_id=%s", meta.AgentID)
	}
	if r.Artifact != nil && strings.TrimSpace(r.Artifact.IntentKind) != "" {
		fmt.Fprintf(&sb, " intent_kind=%s", strings.TrimSpace(r.Artifact.IntentKind))
	}
	level = "INFO"
	if r.Outcome == "skipped_llm" || r.Outcome == "skipped_parse" {
		level = "WARN"
	}
	return level, sb.String()
}

// Run calls a small chat completion to produce an Artifact. On skip or failure Artifact is nil with Outcome set.
// history 是近期对话消息（先旧后新），用于解析当前输入中的指代/省略；nil 表示无历史注入。
func Run(ctx context.Context, agentIntentPassEnabled bool, catalog biz.TeamModelCatalog, httpClient *http.Client, provider, model, userText string, history []HistoryMessage, lg loggateway.Logger) (res RunResult) {
	return runWithSystem(ctx, agentIntentPassEnabled, intentSystemCoding, catalog, httpClient, provider, model, userText, history, lg)
}

// RunForAgent runs the intent pass with agent-aware gating and prompt template selection.
// history 语义同 Run。
func RunForAgent(ctx context.Context, ag biz.Agent, catalog biz.TeamModelCatalog, httpClient *http.Client, provider, model, userText string, history []HistoryMessage, lg loggateway.Logger) (res RunResult) {
	if !ShouldRun(ag, userText) {
		res.Outcome = "skipped_disabled"
		return res
	}
	return runWithSystem(ctx, IntentPassFromAgent(ag), IntentSystemForAgent(ag), catalog, httpClient, provider, model, userText, history, lg)
}

func runWithSystem(ctx context.Context, agentIntentPassEnabled bool, systemPrompt string, catalog biz.TeamModelCatalog, httpClient *http.Client, provider, model, userText string, history []HistoryMessage, lg loggateway.Logger) (res RunResult) {
	start := time.Now()
	defer func() { res.Duration = time.Since(start) }()

	if !PassEffective(agentIntentPassEnabled) || catalog == nil || httpClient == nil {
		res.Outcome = "skipped_disabled"
		return
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	userText = strings.TrimSpace(userText)
	if provider == "" || model == "" || userText == "" {
		res.Outcome = "skipped_preflight"
		return
	}

	sessionID := SessionIDFromCtx(ctx)
	cacheKey := CacheKey(sessionID, userText, provider, model, systemPrompt)
	if cached, ok := globalRunCache().Get(cacheKey); ok {
		cached.Outcome = "cached"
		return cached
	}

	row, err := catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		res.Outcome = "skipped_catalog"
		return
	}
	var cfg llmcompat.ProviderAPIConfig
	llmcompat.MergeProviderConfigJSON(row.ConfigJSON, &cfg)
	llmcompat.ApplyThinkingCapability(&cfg, row.CapabilitiesExplicit, row.Capabilities.Thinking)

	cfg.ThinkingDisabled = true

	msgs := []llmcompat.OpenAICompatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: buildUserMessageContent(userText, history)},
	}

	callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	text, _, promptTok, completionTok, err := llmcompat.CallOpenAICompatChat(callCtx, httpClient, cfg, model, msgs)
	if err != nil {
		res.Outcome = "skipped_llm"
		return
	}
	res.PromptTok = promptTok
	res.CompletionTok = completionTok
	text = stripFences(text)
	art, raw := parseArtifactJSON(text)
	if art == nil {
		res.Outcome = "skipped_parse"
		return
	}

	ForceDestructiveFlag(userText, art)
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	lg.Info("意图识别完成", loggateway.StepID("chat.intent.pass"), loggateway.Str("flow_status", "done"), loggateway.Str("intent_kind", art.IntentKind), loggateway.Int("refined_goal_len", len(art.RefinedGoal)))
	res.Artifact = art
	res.RawJSON = raw
	res.Outcome = "completed"

	globalRunCache().Put(cacheKey, res)
	return
}

func parseArtifactJSON(text string) (*Artifact, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ""
	}
	if i := strings.Index(text, "{"); i >= 0 {
		if j := strings.LastIndex(text, "}"); j > i {
			text = text[i : j+1]
		}
	}
	var art Artifact
	if err := json.Unmarshal([]byte(text), &art); err != nil {

		if retry, ok := stripJSONTopKey(text, "sub_intents"); ok {
			if json.Unmarshal([]byte(retry), &art) != nil {
				return nil, ""
			}
			art.SubIntents = nil
		} else {
			return nil, ""
		}
	}
	if strings.TrimSpace(art.RefinedGoal) == "" {
		return nil, ""
	}
	art.SubIntents = sanitizeSubIntents(art.SubIntents)
	return &art, text
}

// stripJSONTopKey 删除顶层 JSON 对象的指定键（经 map[RawMessage] 往返，不依赖
// 脆弱的正则切括号）。输入非法 JSON 时返回 ok=false。
func stripJSONTopKey(text, key string) (string, bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(text), &m) != nil {
		return "", false
	}
	if _, exists := m[key]; !exists {
		return "", false
	}
	delete(m, key)
	out, err := json.Marshal(m)
	if err != nil {
		return "", false
	}
	return string(out), true
}

// sanitizeSubIntents 条目级清洗：空 goal 条目丢弃；<2 条视为非复合（置 nil，
// 防 sub_intents_count 监控口径失真）；超出 MaxSubIntents 截断。
func sanitizeSubIntents(subs []SubIntent) []SubIntent {
	if len(subs) == 0 {
		return nil
	}
	out := make([]SubIntent, 0, len(subs))
	for _, s := range subs {
		if strings.TrimSpace(s.Goal) == "" {
			continue
		}
		s.IntentKind = strings.TrimSpace(s.IntentKind)
		out = append(out, s)
	}
	if len(out) < 2 {
		return nil
	}
	if len(out) > MaxSubIntents {
		out = out[:MaxSubIntents]
	}
	return out
}

// WrapUserMessage embeds the artifact for the main model (design: extend user turn without replacing).
func WrapUserMessage(original string, art *Artifact) string {
	original = strings.TrimSpace(original)
	if art == nil {
		return original
	}
	b, _ := json.Marshal(art)
	return fmt.Sprintf("Original user message:\n%s\n\n---\nDerived intent (align your plan and tools to this JSON):\n%s", original, string(b))
}

// MergeIntoUserOptionsJSON adds intent_artifact for audit replay.
func MergeIntoUserOptionsJSON(optionsJSON string, artifactJSON string) (string, error) {
	artifactJSON = strings.TrimSpace(artifactJSON)
	if artifactJSON == "" {
		return optionsJSON, nil
	}
	var opts map[string]any
	if strings.TrimSpace(optionsJSON) == "" {
		opts = map[string]any{}
	} else if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
		return optionsJSON, err
	}
	opts["intent_artifact"] = json.RawMessage([]byte(artifactJSON))
	out, err := json.Marshal(opts)
	if err != nil {
		return optionsJSON, err
	}
	return string(out), nil
}

var fenceRE = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")

// stripFences removes optional markdown code fences from model output.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if m := fenceRE.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return s
}
