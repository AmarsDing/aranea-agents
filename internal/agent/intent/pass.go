// Package intent runs an optional LLM pass to refine user goals before main ADK execution.
package intent

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/event"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
)

// Artifact is the structured output of the intent pass (subset of design doc).
type Artifact struct {
	RefinedGoal     string   `json:"refined_goal"`
	IntentKind      string   `json:"intent_kind"`
	SuccessCriteria []string `json:"success_criteria"`
	Ambiguities     []string `json:"ambiguities"`
	SearchHints     []string `json:"search_hints"`
	RiskFlags       []string `json:"risk_flags"`
}

const intentSystem = `You classify and restate the user's request for a coding assistant. Reply with ONE JSON object only, no markdown fences, no commentary. Keys:
- refined_goal (string): one clear sentence of what the user wants.
- intent_kind (string): one of code_change, explain, debug, doc, research, other.
- success_criteria (array of strings): measurable checks (e.g. "tests pass").
- ambiguities (array of strings): questions that still need human clarification, or [].
- search_hints (array of strings): short literals useful for codebase search (identifiers, error substrings, file name fragments).
- risk_flags (array of strings): e.g. touches_auth, migrations, or [].`

// PassEffective returns whether the intent pass should run (extra LLM call).
// Per-agent default comes from agent_runtime_settings.intent_pass_enabled (UI / API, default true).
// ARANEA_INTENT_PASS: unset → follow agent; "0"/"false"/"off"/"no" → off; "1"/"true"/"on"/"yes" → on; other non-empty values → follow agent.
func PassEffective(agentIntentPassEnabled bool) bool {
	v := strings.TrimSpace(os.Getenv("ARANEA_INTENT_PASS"))
	if v == "" {
		return agentIntentPassEnabled
	}
	v = strings.ToLower(v)
	if v == "0" || v == "false" || v == "off" || v == "no" {
		return false
	}
	if v == "1" || v == "true" || v == "on" || v == "yes" {
		return true
	}
	return agentIntentPassEnabled
}

// IntentPassFromAgent returns persisted intent-pass preference; default true when settings are missing.
func IntentPassFromAgent(ag biz.Agent) bool {
	if ag.Settings != nil {
		return ag.Settings.IntentPassEnabled
	}
	return true
}

// RunResult is the outcome of Run (for main path wiring and TeamRunEvent / monitor).
type RunResult struct {
	Artifact *Artifact
	RawJSON  string
	Duration time.Duration
	// Outcome is one of: completed, skipped_disabled, skipped_preflight, skipped_catalog, skipped_llm, skipped_parse
	Outcome string
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
func Run(ctx context.Context, agentIntentPassEnabled bool, catalog *biz.LlmProviderModelUsecase, httpClient *http.Client, provider, model, userText string) (res RunResult) {
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
	row, err := catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		res.Outcome = "skipped_catalog"
		return
	}
	var cfg agent.ProviderAPIConfig
	agent.MergeProviderConfigJSON(row.ConfigJSON, &cfg)

	msgs := []agent.OpenAICompatMessage{
		{Role: "system", Content: intentSystem},
		{Role: "user", Content: "User message:\n\n" + userText},
	}
	callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	text, _, _, _, err := agent.CallOpenAICompatChat(callCtx, httpClient, cfg, model, msgs)
	if err != nil {
		res.Outcome = "skipped_llm"
		return
	}
	text = stripFences(text)
	art, raw := parseArtifactJSON(text)
	if art == nil {
		res.Outcome = "skipped_parse"
		return
	}
	event.CtxFlowLogDone(ctx, "chat.intent.pass", "意图识别完成", event.P("intent_kind", art.IntentKind), event.P("refined_goal_len", len(art.RefinedGoal)))
	res.Artifact = art
	res.RawJSON = raw
	res.Outcome = "completed"
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
	if json.Unmarshal([]byte(text), &art) != nil {
		return nil, ""
	}
	if strings.TrimSpace(art.RefinedGoal) == "" {
		return nil, ""
	}
	return &art, text
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
