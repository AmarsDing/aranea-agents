package intent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// stubCatalog 实现 biz.TeamModelCatalog，返回固定 ProviderModel。
type stubCatalog struct{ row biz.ProviderModel }

func (s stubCatalog) GetByProviderAndModel(_ context.Context, _, _ string) (biz.ProviderModel, error) {
	return s.row, nil
}
func (s stubCatalog) List(_ context.Context) ([]biz.ProviderModel, error) {
	return []biz.ProviderModel{s.row}, nil
}

// Voice Fast-Path（2026-08-09）：意图识别是分类任务，callsite 必须强制关闭
// thinking——即便 catalog 行未配置 thinking_disabled。真机实测 deepseek-v4-flash
// 开思考时意图识别 3.7-26.6s（分类前先烧数千推理 token），是语音轮次最大延迟源。
func TestRunForAgent_ForcesThinkingDisabled(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","object":"chat.completion","created":0,"model":"m",` +
			`"choices":[{"index":0,"message":{"role":"assistant","content":"{\"intent_kind\":\"question\",\"refined_goal\":\"查北京天气\"}"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(srv.Close)

	cat := stubCatalog{row: biz.ProviderModel{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		// catalog 行刻意不配 thinking_disabled——callsite 策略必须兜底。
		ConfigJSON: `{"provider_type":"deepseek","api_base_url":"` + srv.URL + `","api_key":"k"}`,
	}}
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{IntentPassEnabled: true}}
	res := RunForAgent(context.Background(), ag, cat, srv.Client(), "deepseek", "deepseek-v4-flash", "北京天气怎么样", nil, loggateway.NewNoop())
	if res.Outcome != "completed" {
		t.Fatalf("outcome = %q, want completed", res.Outcome)
	}
	if !strings.Contains(string(body), `"thinking"`) || !strings.Contains(string(body), `"disabled"`) {
		t.Fatalf("intent pass must force thinking disabled in request body: %s", string(body))
	}
}
