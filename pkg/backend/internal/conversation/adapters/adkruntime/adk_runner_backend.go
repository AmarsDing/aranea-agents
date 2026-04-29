package adkruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"arenea/backend/internal/kernel/runctx"
)

// 工具调用治理常量。取值偏保守，仅在模型异常时生效——正常回合很少超过数次工具调用。
// 作为安全网，防止模型陷入工具循环从而拖垮会话或耗尽提供商额度。
const (
	toolCallBudgetPerTurn   = 8
	toolFailureBudgetPerArg = 2
)

type runnerRuntimeBackend struct {
	adapter *ADKRuntimeAdapter
}

func newRunnerRuntimeBackend(adapter *ADKRuntimeAdapter) runtimeBackend {
	return &runnerRuntimeBackend{adapter: adapter}
}

func (b *runnerRuntimeBackend) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	return b.run(ctx, req, nil)
}

func (b *runnerRuntimeBackend) StreamGenerate(ctx context.Context, req GenerateRequest, onDelta DeltaFunc) (GenerateResult, error) {
	return b.run(ctx, req, onDelta)
}

func (b *runnerRuntimeBackend) run(ctx context.Context, req GenerateRequest, onDelta DeltaFunc) (GenerateResult, error) {
	if strings.TrimSpace(req.Input) == "" {
		return GenerateResult{}, fmt.Errorf("empty input")
	}
	started := time.Now()
	emittedPartial := false
	modelDelta := func(delta string) error {
		emittedPartial = true
		if onDelta == nil {
			return nil
		}
		return onDelta(delta)
	}
	rootAgent, err := b.buildAgent(ctx, req, modelDelta)
	if err != nil {
		return GenerateResult{}, err
	}
	plugins, err := b.adapter.builtinPlugins(ctx)
	if err != nil {
		return GenerateResult{}, err
	}
	r, err := runner.New(runner.Config{
		AppName:           "aranea",
		Agent:             rootAgent,
		SessionService:    session.InMemoryService(),
		PluginConfig:      runner.PluginConfig{Plugins: plugins},
		AutoCreateSession: true,
	})
	if err != nil {
		return GenerateResult{}, err
	}

	// 仅当 run-config 显式请求 StreamingModeSSE 时，ADK 才会以 stream=true 调用底层模型。
	// 否则 providerModelLLM.GenerateContent 会走 generateDirect，前端无法收到单 Agent 或团队成员的 SSE 增量。
	runConfig := agent.RunConfig{}
	if onDelta != nil {
		runConfig.StreamingMode = agent.StreamingModeSSE
	}

	var finalText string
	for event, runErr := range r.Run(ctx, "aranea-user", runnerSessionID(req), genai.NewContentFromText(req.Input, genai.RoleUser), runConfig) {
		if runErr != nil {
			return GenerateResult{}, runErr
		}
		if event == nil {
			continue
		}
		text := llmResponseText(&event.LLMResponse)
		if text == "" {
			continue
		}
		// providerModelLLM.GenerateContent 每回合仅产生一个终结 LLMResponse——
		// 增量 token 在 streamDirect 内通过 modelDelta 透出。最终非 partial 事件携带待持久化的全文。
		if !event.LLMResponse.Partial {
			finalText = text
		}
	}
	if strings.TrimSpace(finalText) == "" {
		return GenerateResult{}, fmt.Errorf("adk runner returned empty response")
	}
	if onDelta != nil && !emittedPartial {
		if err = onDelta(finalText); err != nil {
			return GenerateResult{}, err
		}
	}
	cfg, _ := parseProviderConfig(req.ProviderModel.ConfigJSON)
	return GenerateResult{
		Content:          finalText,
		ModelName:        firstNonEmpty(req.ProviderModel.Model, req.ProviderModel.Name),
		PromptTokens:     estimatePromptTokens(req, cfg),
		CompletionTokens: estimateTokens(finalText),
		LatencyMS:        int(time.Since(started).Milliseconds()),
	}, nil
}

func (b *runnerRuntimeBackend) buildAgent(ctx context.Context, req GenerateRequest, onDelta DeltaFunc) (agent.Agent, error) {
	tools, err := b.adapter.runtimeTools(ctx, req)
	if err != nil {
		return nil, err
	}
	rc := enrichRuntimeContextWithTools(req.RuntimeContext, tools)
	return llmagent.New(llmagent.Config{
		Name:        adkAgentName(req),
		Description: strings.TrimSpace(req.Agent.AgentDescription),
		Instruction: buildSystemPrompt(req.Agent, rc),
		Model:       newProviderModelLLM(b.adapter, req.Agent, req.ProviderModel, rc, onDelta),
		Tools:       tools,
	})
}

// enrichRuntimeContextWithTools 将上下文的 Tools 切片重写为本回合实际装配的工具面。
// 若无此步，渲染的策略块会列出被 Agent profile 静默过滤的工具，误导模型。
func enrichRuntimeContextWithTools(rc *RuntimeContext, tools []tool.Tool) *RuntimeContext {
	if len(tools) == 0 {
		if rc == nil {
			return &RuntimeContext{}
		}
		clone := *rc
		clone.Tools = nil
		return &clone
	}
	hints := make([]runctx.ToolHint, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		hints = append(hints, runctx.ToolHint{Name: t.Name(), Description: t.Description()})
	}
	if rc == nil {
		return &RuntimeContext{Tools: hints}
	}
	clone := *rc
	clone.Tools = hints
	return &clone
}

func runnerToolCallbacks(req GenerateRequest) (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
	var mu sync.Mutex
	started := map[string]time.Time{}
	totalCalls := 0
	failures := map[string]int{} // 键 = tool|argsFingerprint -> 失败次数

	emitEvent := func(id string, phase, status, name string, args, result map[string]any, toolErr error, durationMS int) {
		if req.OnToolEvent == nil {
			return
		}
		_ = req.OnToolEvent(newRunnerToolEvent(req, id, phase, status, name, args, result, toolErr, durationMS))
	}

	before := func(ctx tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
		if t == nil {
			return nil, nil
		}
		name := t.Name()
		id := toolEventID(ctx, name)
		fingerprint := name + "|" + toolArgsFingerprint(args)

		mu.Lock()
		if totalCalls >= toolCallBudgetPerTurn {
			mu.Unlock()
			result := map[string]any{
				"status":  "blocked",
				"reason":  fmt.Sprintf("tool call budget for this turn exceeded (max %d calls)", toolCallBudgetPerTurn),
				"hint":    "Stop calling tools. Answer the user from context and explain that you reached the per-turn tool call limit.",
				"tool":    name,
				"blocked": true,
			}
			emitEvent(id, "before", "blocked", name, args, nil, nil, 0)
			emitEvent(id, "after", "blocked", name, args, result, nil, 0)
			return result, nil
		}
		if count := failures[fingerprint]; count >= toolFailureBudgetPerArg {
			mu.Unlock()
			result := map[string]any{
				"status":  "blocked",
				"reason":  fmt.Sprintf("tool %q already failed %d times with these arguments; further attempts are suppressed", name, count),
				"hint":    "Do not retry. Report the limitation to the user and continue without this tool.",
				"tool":    name,
				"blocked": true,
			}
			emitEvent(id, "before", "blocked", name, args, nil, nil, 0)
			emitEvent(id, "after", "blocked", name, args, result, nil, 0)
			return result, nil
		}
		totalCalls++
		started[id] = time.Now()
		mu.Unlock()

		emitEvent(id, "before", "running", name, args, nil, nil, 0)
		return nil, nil
	}
	after := func(ctx tool.Context, t tool.Tool, args, result map[string]any, toolErr error) (map[string]any, error) {
		if t == nil {
			return nil, nil
		}
		name := t.Name()
		id := toolEventID(ctx, name)
		fingerprint := name + "|" + toolArgsFingerprint(args)

		mu.Lock()
		durationMS := 0
		if at, ok := started[id]; ok {
			durationMS = int(time.Since(at).Milliseconds())
			delete(started, id)
		}
		if toolErr != nil {
			failures[fingerprint]++
		}
		mu.Unlock()

		status := "success"
		if toolErr != nil {
			status = "failed"
		}
		emitEvent(id, "after", status, name, args, result, toolErr, durationMS)
		return nil, nil
	}
	return before, after
}

// toolArgsFingerprint 为工具参数 map 生成稳定标识，以便在不依赖 map 遍历顺序时识别「相同调用」。
// 故意不全量纳入大字段（上游 sanitizeToolArgs 已截断）——同路径不同正文仍计为重复，符合失败预算语义。
func toolArgsFingerprint(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		raw, err := json.Marshal(args[k])
		if err != nil {
			parts = append(parts, k+"=?")
			continue
		}
		value := string(raw)
		if len(value) > 96 {
			value = value[:96]
		}
		parts = append(parts, k+"="+value)
	}
	return strings.Join(parts, "&")
}

func newRunnerToolEvent(req GenerateRequest, id string, phase string, status string, toolName string, args map[string]any, result map[string]any, toolErr error, durationMS int) ToolEvent {
	event := ToolEvent{
		ID:         id,
		Phase:      phase,
		Status:     status,
		AgentID:    req.Agent.ID,
		AgentKey:   req.Agent.AgentKey,
		AgentName:  firstNonEmpty(req.Agent.DisplayName, req.Agent.AgentKey, req.Agent.ID),
		AgentIcon:  req.Agent.Icon,
		ToolName:   toolName,
		ToolLabel:  toolDisplayLabel(toolName),
		Arguments:  sanitizeToolArgs(args),
		Result:     summarizeToolResult(result),
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		DurationMS: durationMS,
	}
	if toolErr != nil {
		event.Error = toolErr.Error()
	}
	if phase == "before" {
		event.MessageHint = fmt.Sprintf("%s 正在使用 %s", event.AgentName, event.ToolLabel)
	} else if status == "success" {
		event.MessageHint = fmt.Sprintf("%s 已完成 %s", event.AgentName, event.ToolLabel)
	} else {
		event.MessageHint = fmt.Sprintf("%s 使用 %s 失败", event.AgentName, event.ToolLabel)
	}
	return event
}

func toolEventID(ctx tool.Context, fallback string) string {
	if ctx != nil && strings.TrimSpace(ctx.FunctionCallID()) != "" {
		return strings.TrimSpace(ctx.FunctionCallID())
	}
	return strings.TrimSpace(fallback)
}

func toolDisplayLabel(name string) string {
	switch name {
	case "read_file":
		return "读取文件"
	case "write_file":
		return "写入文件"
	case "list_files":
		return "列出文件"
	case "edit_file":
		return "编辑文件"
	default:
		return name
	}
}

func sanitizeToolArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := map[string]any{}
	for k, v := range args {
		if strings.EqualFold(k, "content") || strings.EqualFold(k, "new_string") || strings.EqualFold(k, "old_string") {
			text := fmt.Sprint(v)
			if len([]rune(text)) > 80 {
				runes := []rune(text)
				text = string(runes[:80]) + "..."
			}
			out[k] = text
			continue
		}
		out[k] = v
	}
	return out
}

func summarizeToolResult(result map[string]any) map[string]any {
	if len(result) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{"path", "written", "replacements", "size"} {
		if value, ok := result[key]; ok {
			out[key] = value
		}
	}
	if items, ok := result["items"].([]map[string]any); ok {
		out["items_count"] = len(items)
	} else if items, ok := result["items"].([]any); ok {
		out["items_count"] = len(items)
	}
	return out
}

func runnerSessionID(req GenerateRequest) string {
	key := strings.TrimSpace(req.Agent.ID)
	if key == "" {
		key = strings.TrimSpace(req.Agent.AgentKey)
	}
	if key == "" {
		key = "default"
	}
	return "runtime-" + sanitizeIdentifier(key)
}

func adkAgentName(req GenerateRequest) string {
	name := firstNonEmpty(req.Agent.AgentKey, req.Agent.ID, req.Agent.DisplayName, "aranea_agent")
	return sanitizeIdentifier(name)
}

var unsafeIdentifierChars = regexp.MustCompile(`[^A-Za-z0-9_]`)

func sanitizeIdentifier(value string) string {
	value = unsafeIdentifierChars.ReplaceAllString(strings.TrimSpace(value), "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "aranea_agent"
	}
	if value[0] >= '0' && value[0] <= '9' {
		value = "agent_" + value
	}
	return value
}
