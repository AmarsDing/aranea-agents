package adkruntime

import (
	"context"
	"strings"
	"testing"

	"arenea/backend/internal/domain"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

func TestADKRuntimeAdapterBackendSwitch(t *testing.T) {
	t.Setenv("RUNTIME_BACKEND", "")
	adapter := NewADKRuntimeAdapter()
	if _, ok := adapter.activeBackend().(*runnerRuntimeBackend); !ok {
		t.Fatalf("default backend should be runner so ADK tools are available")
	}

	t.Setenv("RUNTIME_BACKEND", "direct")
	if _, ok := adapter.activeBackend().(*directRuntimeBackend); !ok {
		t.Fatalf("RUNTIME_BACKEND=direct should select direct backend")
	}
}

func TestADKRuntimeAdapterRunnerBackendStub(t *testing.T) {
	t.Setenv("RUNTIME_BACKEND", "adk_runner")
	t.Setenv("ADK_RUNNER_PLUGINS", "runtime_audit,sensitive_data_mask,confirmation_guard,retry_and_reflect,skill_usage_tracker")
	adapter := NewADKRuntimeAdapter()
	result, err := adapter.Generate(context.Background(), GenerateRequest{
		Agent: domain.Agent{
			ID:               "agent_test",
			AgentKey:         "test_agent",
			DisplayName:      "Test Agent",
			AgentDescription: "Reply briefly.",
		},
		ProviderModel: domain.PlatformResource{Model: "stub-model"},
		Input:         "hello",
	})
	if err != nil {
		t.Fatalf("runner backend Generate() failed: %v", err)
	}
	if !strings.Contains(result.Content, "[stub/test_agent]") {
		t.Fatalf("expected stub response from provider model adapter, got %q", result.Content)
	}
}

func TestADKRuntimeAdapterStreamUsesActiveBackend(t *testing.T) {
	t.Setenv("RUNTIME_BACKEND", "adk_runner")
	adapter := NewADKRuntimeAdapter()
	backend := &fakeRuntimeBackend{result: GenerateResult{Content: "runner stream", ModelName: "fake"}}
	adapter.runner = backend

	var deltas []string
	result, err := adapter.StreamGenerate(context.Background(), GenerateRequest{Input: "hello"}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamGenerate() failed: %v", err)
	}
	if !backend.streamCalled {
		t.Fatalf("expected StreamGenerate() to use active backend")
	}
	if result.Content != "runner stream" {
		t.Fatalf("unexpected result content %q", result.Content)
	}
	if strings.Join(deltas, "") != "runner stream" {
		t.Fatalf("unexpected deltas %v", deltas)
	}
}

func TestNormalizeBuiltinPluginKeys(t *testing.T) {
	keys, err := normalizeBuiltinPluginKeys("logging,redaction,confirm,retry,skill_usage,cost,router")
	if err != nil {
		t.Fatalf("normalizeBuiltinPluginKeys() failed: %v", err)
	}
	expected := []string{"runtime_audit", "sensitive_data_mask", "confirmation_guard", "retry_and_reflect", "skill_usage_tracker", "cost_guard", "model_router"}
	if strings.Join(keys, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected keys: got %v want %v", keys, expected)
	}
}

func TestSensitiveDataMaskPlugin(t *testing.T) {
	p, err := newSensitiveDataMaskPlugin()
	if err != nil {
		t.Fatalf("newSensitiveDataMaskPlugin() failed: %v", err)
	}
	content := genai.NewContentFromText("email a@example.com token=abc123 sk-testsecret123456", genai.RoleUser)
	masked, err := p.OnUserMessageCallback()(nil, content)
	if err != nil {
		t.Fatalf("OnUserMessageCallback() failed: %v", err)
	}
	text := contentText(masked)
	for _, forbidden := range []string{"a@example.com", "abc123", "sk-testsecret123456"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("expected %q to be redacted from %q", forbidden, text)
		}
	}
}

func TestConfirmationGuardBlocksHighRiskTool(t *testing.T) {
	p, err := newConfirmationGuardPlugin()
	if err != nil {
		t.Fatalf("newConfirmationGuardPlugin() failed: %v", err)
	}
	result, err := p.BeforeToolCallback()(nil, fakeTool{name: "delete_file"}, map[string]any{"path": "important.txt"})
	if err != nil {
		t.Fatalf("BeforeToolCallback() failed: %v", err)
	}
	if result["status"] != "blocked" {
		t.Fatalf("expected high-risk tool to be blocked, got %v", result)
	}
}

func TestPermissionGuardDeniesConfiguredTool(t *testing.T) {
	t.Setenv("ADK_PERMISSION_DENY_TOOLS", "read_secret")
	p, err := newPermissionGuardPlugin()
	if err != nil {
		t.Fatalf("newPermissionGuardPlugin() failed: %v", err)
	}
	result, err := p.BeforeToolCallback()(nil, fakeTool{name: "read_secret"}, map[string]any{})
	if err != nil {
		t.Fatalf("BeforeToolCallback() failed: %v", err)
	}
	if result["status"] != "blocked" {
		t.Fatalf("expected permission guard to block tool, got %v", result)
	}
}

func TestOutputPolicyBlocksDangerousOutput(t *testing.T) {
	p, err := newOutputPolicyPlugin()
	if err != nil {
		t.Fatalf("newOutputPolicyPlugin() failed: %v", err)
	}
	resp := &model.LLMResponse{Content: genai.NewContentFromText("run rm -rf / to fix it", genai.RoleModel)}
	updated, err := p.AfterModelCallback()(nil, resp, nil)
	if err != nil {
		t.Fatalf("AfterModelCallback() failed: %v", err)
	}
	if updated == nil || updated.ErrorCode != "OUTPUT_POLICY_BLOCKED" {
		t.Fatalf("expected output policy block, got %#v", updated)
	}
}

func TestSkillUsageTrackerRecordsSkillTool(t *testing.T) {
	p, err := newSkillUsageTrackerPlugin()
	if err != nil {
		t.Fatalf("newSkillUsageTrackerPlugin() failed: %v", err)
	}
	args := map[string]any{"skill_name": "data analyst"}
	if result, err := p.BeforeToolCallback()(nil, fakeTool{name: "load_skill"}, args); err != nil || result != nil {
		t.Fatalf("BeforeToolCallback() result=%v err=%v", result, err)
	}
	if result, err := p.AfterToolCallback()(nil, fakeTool{name: "load_skill"}, args, map[string]any{"ok": true}, nil); err != nil || result != nil {
		t.Fatalf("AfterToolCallback() result=%v err=%v", result, err)
	}
	skillUsageStats.Lock()
	record := skillUsageStats.records["load_skill"]
	skillUsageStats.Unlock()
	if record == nil || record.Success == 0 {
		t.Fatalf("expected skill usage success to be recorded, got %#v", record)
	}
}

func TestCostGuardFallbackForBlockedPremiumModel(t *testing.T) {
	t.Setenv("ADK_COST_FALLBACK_MODEL", "cheap-model")
	t.Setenv("ADK_COST_BLOCK_PREMIUM_MODELS", "true")
	p, err := newCostGuardPlugin()
	if err != nil {
		t.Fatalf("newCostGuardPlugin() failed: %v", err)
	}
	req := &model.LLMRequest{
		Model:    "gpt-5-pro",
		Contents: []*genai.Content{genai.NewContentFromText("hello", genai.RoleUser)},
	}
	resp, err := p.BeforeModelCallback()(nil, req)
	if err != nil {
		t.Fatalf("BeforeModelCallback() failed: %v", err)
	}
	if resp != nil {
		t.Fatalf("expected fallback to modify request, got response %#v", resp)
	}
	if req.Model != "cheap-model" {
		t.Fatalf("expected fallback model, got %q", req.Model)
	}
}

func TestCostGuardBlocksOverBudgetWithoutFallback(t *testing.T) {
	t.Setenv("ADK_COST_MAX_PROMPT_TOKENS", "1")
	t.Setenv("ADK_COST_FALLBACK_MODEL", "")
	t.Setenv("ADK_COST_BLOCK_PREMIUM_MODELS", "false")
	p, err := newCostGuardPlugin()
	if err != nil {
		t.Fatalf("newCostGuardPlugin() failed: %v", err)
	}
	req := &model.LLMRequest{
		Model:    "cheap-model",
		Contents: []*genai.Content{genai.NewContentFromText("this prompt is intentionally long enough", genai.RoleUser)},
	}
	resp, err := p.BeforeModelCallback()(nil, req)
	if err != nil {
		t.Fatalf("BeforeModelCallback() failed: %v", err)
	}
	if resp == nil || resp.ErrorCode != "MODEL_POLICY_BLOCKED" {
		t.Fatalf("expected model policy block response, got %#v", resp)
	}
}

func TestModelRouterRoutesCodeAndLongContext(t *testing.T) {
	t.Setenv("ADK_ROUTER_CODE_MODEL", "code-model")
	t.Setenv("ADK_ROUTER_LONG_CONTEXT_MODEL", "long-model")
	t.Setenv("ADK_ROUTER_LONG_CONTEXT_TOKENS", "100")
	p, err := newModelRouterPlugin()
	if err != nil {
		t.Fatalf("newModelRouterPlugin() failed: %v", err)
	}
	codeReq := &model.LLMRequest{
		Model:    "default",
		Contents: []*genai.Content{genai.NewContentFromText("please debug this python function", genai.RoleUser)},
	}
	if resp, err := p.BeforeModelCallback()(nil, codeReq); err != nil || resp != nil {
		t.Fatalf("code route callback resp=%#v err=%v", resp, err)
	}
	if codeReq.Model != "code-model" {
		t.Fatalf("expected code model, got %q", codeReq.Model)
	}

	t.Setenv("ADK_ROUTER_LONG_CONTEXT_TOKENS", "5")
	p, err = newModelRouterPlugin()
	if err != nil {
		t.Fatalf("newModelRouterPlugin() failed: %v", err)
	}
	longReq := &model.LLMRequest{
		Model:    "default",
		Contents: []*genai.Content{genai.NewContentFromText(strings.Repeat("long context ", 20), genai.RoleUser)},
	}
	if resp, err := p.BeforeModelCallback()(nil, longReq); err != nil || resp != nil {
		t.Fatalf("long route callback resp=%#v err=%v", resp, err)
	}
	if longReq.Model != "long-model" {
		t.Fatalf("expected long context model, got %q", longReq.Model)
	}
}

func TestProviderModelLLMUsesRoutedRequestModel(t *testing.T) {
	llm := newProviderModelLLM(NewADKRuntimeAdapter(), domain.Agent{AgentKey: "router_test"}, domain.PlatformResource{Model: "base-model"}, nil, nil)
	req := &model.LLMRequest{
		Model:    "routed-model",
		Contents: []*genai.Content{genai.NewContentFromText("hello", genai.RoleUser)},
	}
	var resp *model.LLMResponse
	for item, err := range llm.GenerateContent(context.Background(), req, false) {
		if err != nil {
			t.Fatalf("GenerateContent() failed: %v", err)
		}
		resp = item
	}
	if resp == nil || resp.ModelVersion != "routed-model" {
		t.Fatalf("expected routed model version, got %#v", resp)
	}
}

type fakeTool struct {
	name string
}

func (f fakeTool) Name() string        { return f.name }
func (f fakeTool) Description() string { return "fake tool" }
func (f fakeTool) IsLongRunning() bool { return false }

type fakeRuntimeBackend struct {
	result         GenerateResult
	generateCalled bool
	streamCalled   bool
}

func (f *fakeRuntimeBackend) Generate(context.Context, GenerateRequest) (GenerateResult, error) {
	f.generateCalled = true
	return f.result, nil
}

func (f *fakeRuntimeBackend) StreamGenerate(_ context.Context, _ GenerateRequest, onDelta DeltaFunc) (GenerateResult, error) {
	f.streamCalled = true
	if onDelta != nil {
		if err := onDelta(f.result.Content); err != nil {
			return GenerateResult{}, err
		}
	}
	return f.result, nil
}
