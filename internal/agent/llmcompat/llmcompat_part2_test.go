package llmcompat

import (
	"context"
	"strings"
	"testing"
)

// catalog 静态 thinking_disabled=true 被调用方显式 effort 覆盖（P2-1 同约：
// 显式路由策略优先于静态配置）。
func TestCallOpenAICompatChatStream_ThinkingEffort_OverridesThinkingDisabled(t *testing.T) {
	var body []byte
	chunks := []string{
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`,
	}
	srv := sseServer(t, chunks, &body)

	cfg := ProviderAPIConfig{ProviderType: "deepseek", APIBaseURL: srv.URL, APIKey: "test-key",
		ThinkingDisabled: true, ThinkingEffort: "max"}
	_, _, _, _, err := CallOpenAICompatChatStream(context.Background(), srv.Client(), cfg, "deepseek-v4-flash",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}}, StreamCallbacks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(body), `"reasoning_effort"`) {
		t.Fatalf("explicit effort must inject reasoning_effort: %s", string(body))
	}
	if strings.Contains(string(body), `"disabled"`) {
		t.Fatalf("explicit effort must override thinking_disabled: %s", string(body))
	}
}
