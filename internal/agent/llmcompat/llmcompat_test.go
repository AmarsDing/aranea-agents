package llmcompat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// The wrapped error must preserve the original *trpcmodel.ResponseError in the
// chain so upstream retry classification can inspect Type/Code.
func TestProviderResponseError_PreservesOriginal(t *testing.T) {
	code := "context_length_exceeded"
	orig := &trpcmodel.ResponseError{
		Message: "context length exceeded",
		Type:    "invalid_request_error",
		Code:    &code,
	}
	err := providerResponseError(orig)
	var respErr *trpcmodel.ResponseError
	if !errors.As(err, &respErr) {
		t.Fatalf("errors.As failed to recover *trpcmodel.ResponseError from %v", err)
	}
	if respErr.Code == nil || *respErr.Code != code {
		t.Fatalf("expected code %q preserved, got %v", code, respErr.Code)
	}
}

func TestProviderResponseError_Nil(t *testing.T) {
	if err := providerResponseError(nil); err != nil {
		t.Fatalf("expected nil for nil input, got %v", err)
	}
}

// Phase 9: 多模态消息必须携带 ContentParts（文本 + 图片）进入 trpc 请求，
// 不能退化为纯文本 Content（否则视觉模型收不到图片）。
func TestOpenAICompatToTRPCMessages_ContentParts(t *testing.T) {
	text := "描述这张图"
	msgs := []OpenAICompatMessage{
		{Role: "system", Content: "你是视觉助手"},
		{
			Role: "user",
			ContentParts: []trpcmodel.ContentPart{
				{Type: trpcmodel.ContentTypeText, Text: &text},
				{Type: trpcmodel.ContentTypeImage, Image: &trpcmodel.Image{Data: []byte{1, 2, 3}, Format: "png"}},
			},
		},
	}
	out := openAICompatToTRPCMessages(msgs)
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
	user := out[1]
	if len(user.ContentParts) != 2 {
		t.Fatalf("expected 2 content parts preserved, got %d", len(user.ContentParts))
	}
	if user.ContentParts[1].Type != trpcmodel.ContentTypeImage || user.ContentParts[1].Image == nil {
		t.Fatalf("image part lost in conversion: %+v", user.ContentParts[1])
	}
	if string(user.ContentParts[1].Image.Data) != string([]byte{1, 2, 3}) {
		t.Errorf("image data corrupted: %v", user.ContentParts[1].Image.Data)
	}
}

// 00:52 会话根因（B1+B2）：openai.go 在 ctx 取消时不发射错误响应（非流式
// 直接 silent return；流式错误响应与 ctx.Done() 竞态可被丢弃），导致
// CallOpenAICompatChat* 在 LLM 超时后返回 nil error —— TaskPlanner 分解
// 静默产空、用户 60s 无任何反馈。修复：流结束后显式检查 ctx.Err()。
func TestCallOpenAICompatChat_ContextDeadline_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 永不响应，直到客户端因 ctx 超时断开（5s 兜底防 httptest.Close 挂起）。
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	// CloseClientConnections 强制断开活跃连接，否则 handler 阻塞在
	// r.Context().Done() 时 srv.Close() 会一直等待。
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})

	cfg := ProviderAPIConfig{ProviderType: "openai", APIBaseURL: srv.URL, APIKey: "test-key"}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_, _, _, _, err := CallOpenAICompatChat(ctx, srv.Client(), cfg, "test-model",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error when ctx deadline exceeded, got nil (silent timeout)")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded in error chain, got %v", err)
	}
}

func TestCallOpenAICompatChatStream_ContextDeadlineMidStream_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 先吐一个 partial chunk（模拟 LLM 推理中途），然后挂起直到超时。
		_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"par\"},\"finish_reason\":null}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})

	cfg := ProviderAPIConfig{ProviderType: "openai", APIBaseURL: srv.URL, APIKey: "test-key"}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var received string
	_, _, _, _, err := CallOpenAICompatChatStream(ctx, srv.Client(), cfg, "test-model",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}},
		StreamCallbacks{OnContent: func(piece string) error { received += piece; return nil }})
	if err == nil {
		t.Fatal("expected error when ctx deadline exceeded mid-stream, got nil (silent timeout)")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded in error chain, got %v", err)
	}
	if received != "par" {
		t.Fatalf("expected partial delta %q delivered before timeout, got %q", "par", received)
	}
}

// P3：idle 停滞守卫——流式调用超过 streamIdleTimeout 无任何响应（增量/聚合/
// 错误帧）即判定停滞，中止本次尝试并返回 *StreamIdleError。取代旧的「60s
// 总超时掐断健康慢流」：有数据流动的流不设总时长上限。
func TestCallOpenAICompatChatStream_IdleStall_ReturnsStreamIdleError(t *testing.T) {
	old := streamIdleTimeout
	streamIdleTimeout = 200 * time.Millisecond
	t.Cleanup(func() { streamIdleTimeout = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 先吐一个 partial chunk，然后停滞（不再发送任何帧）直到客户端断开。
		_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"par\"},\"finish_reason\":null}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})

	cfg := ProviderAPIConfig{ProviderType: "openai", APIBaseURL: srv.URL, APIKey: "test-key"}
	start := time.Now()
	_, _, _, _, err := CallOpenAICompatChatStream(context.Background(), srv.Client(), cfg, "test-model",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}}, StreamCallbacks{})
	if err == nil {
		t.Fatal("expected idle stall error, got nil")
	}
	var idleErr *StreamIdleError
	if !errors.As(err, &idleErr) {
		t.Fatalf("expected *StreamIdleError, got %T: %v", err, err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("idle guard fired too late: %v", elapsed)
	}
}

// P3 对照组：帧间隔小于停滞窗口但总时长远超窗口的慢流必须正常完成——
// 停滞守卫只掐「不流动」的流，不掐「慢但健康」的流。
func TestCallOpenAICompatChatStream_SlowFlowingStream_Succeeds(t *testing.T) {
	old := streamIdleTimeout
	streamIdleTimeout = 150 * time.Millisecond
	t.Cleanup(func() { streamIdleTimeout = old })

	chunk := func(text string) string {
		return "{\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"" + text + "\"},\"finish_reason\":null}]}"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, s := range []string{"a", "b", "c", "d"} {
			_, _ = io.WriteString(w, "data: "+chunk(s)+"\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			// 帧间隔 100ms < 窗口 150ms；总时长 400ms > 窗口——健康慢流。
			time.Sleep(100 * time.Millisecond)
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})

	cfg := ProviderAPIConfig{ProviderType: "openai", APIBaseURL: srv.URL, APIKey: "test-key"}
	full, _, _, _, err := CallOpenAICompatChatStream(context.Background(), srv.Client(), cfg, "test-model",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}}, StreamCallbacks{})
	if err != nil {
		t.Fatalf("slow but flowing stream must not be killed by idle guard: %v", err)
	}
	if full != "abcd" {
		t.Fatalf("full = %q, want %q", full, "abcd")
	}
}

// sseServer 返回一个按给定 chunks 逐条发射 SSE 的测试服务器。
// chunks 中每个元素是一帧 data: 载荷（不含前缀）。
func sseServer(t *testing.T, chunks []string, captureBody *[]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if captureBody != nil {
			body, _ := io.ReadAll(r.Body)
			*captureBody = body
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, c := range chunks {
			_, _ = io.WriteString(w, "data: "+c+"\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})
	return srv
}

// P1a：推理段可见性——流式调用必须把 reasoning_content 增量通过
// StreamCallbacks.OnReasoning 回调送出（而非仅静默累积），使 planner 能将
// 分解思考过程实时发布给用户。同时验证 OnContent/OnReasoning 两路独立、
// 顺序保持。
func TestCallOpenAICompatChatStream_ReasoningCallback_ReceivesReasoningDeltas(t *testing.T) {
	chunks := []string{
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"想一"},"finish_reason":null}]}`,
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"reasoning_content":"想二"},"finish_reason":null}]}`,
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"答一"},"finish_reason":null}]}`,
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"答二"},"finish_reason":null}]}`,
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	}
	srv := sseServer(t, chunks, nil)

	cfg := ProviderAPIConfig{ProviderType: "openai", APIBaseURL: srv.URL, APIKey: "test-key"}
	var reasoning, content []string
	full, reason, _, _, err := CallOpenAICompatChatStream(context.Background(), srv.Client(), cfg, "test-model",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}},
		StreamCallbacks{
			OnContent:   func(p string) error { content = append(content, p); return nil },
			OnReasoning: func(p string) error { reasoning = append(reasoning, p); return nil },
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reasoning) != 2 || reasoning[0] != "想一" || reasoning[1] != "想二" {
		t.Fatalf("OnReasoning deltas = %v, want [想一 想二]", reasoning)
	}
	if len(content) != 2 || content[0] != "答一" || content[1] != "答二" {
		t.Fatalf("OnContent deltas = %v, want [答一 答二]", content)
	}
	if full != "答一答二" {
		t.Fatalf("fullText = %q, want 答一答二", full)
	}
	if reason != "想一想二" {
		t.Fatalf("reasoningText = %q, want 想一想二", reason)
	}
}

// P1a：nil 回调安全——调用方可以不关心推理流（如 allocator），
// StreamCallbacks 零值不得 panic。
func TestCallOpenAICompatChatStream_NilCallbacks_NoPanic(t *testing.T) {
	chunks := []string{
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"r","content":"c"},"finish_reason":null}]}`,
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	}
	srv := sseServer(t, chunks, nil)
	cfg := ProviderAPIConfig{ProviderType: "openai", APIBaseURL: srv.URL, APIKey: "test-key"}
	full, reason, _, _, err := CallOpenAICompatChatStream(context.Background(), srv.Client(), cfg, "test-model",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}}, StreamCallbacks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if full != "c" || reason != "r" {
		t.Fatalf("full=%q reason=%q, want c/r", full, reason)
	}
}

// P1a 治理项：config_json 的 thinking_disabled=true 时，deepseek 系请求体
// 必须注入 "thinking":{"type":"disabled"}（框架 GenerationConfig.ThinkingEnabled
// 的 DeepSeek variant 映射）。未配置时不得注入该字段（保持 provider 默认）。
func TestCallOpenAICompatChatStream_ThinkingDisabled_InjectsThinkingObject(t *testing.T) {
	var body []byte
	chunks := []string{
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`,
	}
	srv := sseServer(t, chunks, &body)

	cfg := ProviderAPIConfig{ProviderType: "deepseek", APIBaseURL: srv.URL, APIKey: "test-key", ThinkingDisabled: true}
	_, _, _, _, err := CallOpenAICompatChatStream(context.Background(), srv.Client(), cfg, "deepseek-v4-flash",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}}, StreamCallbacks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(body), `"thinking"`) || !strings.Contains(string(body), `"disabled"`) {
		t.Fatalf("request body missing thinking.disabled injection: %s", string(body))
	}
}

func TestCallOpenAICompatChatStream_ThinkingDefault_NoInjection(t *testing.T) {
	var body []byte
	chunks := []string{
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`,
	}
	srv := sseServer(t, chunks, &body)

	cfg := ProviderAPIConfig{ProviderType: "deepseek", APIBaseURL: srv.URL, APIKey: "test-key"}
	_, _, _, _, err := CallOpenAICompatChatStream(context.Background(), srv.Client(), cfg, "deepseek-v4-flash",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}}, StreamCallbacks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(body), `"thinking"`) {
		t.Fatalf("request body must not contain thinking field by default: %s", string(body))
	}
}

// P1a：MergeProviderConfigJSON 必须透传 thinking_disabled。
func TestMergeProviderConfigJSON_ThinkingDisabled(t *testing.T) {
	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(`{"provider_type":"deepseek","thinking_disabled":true}`, &cfg)
	if !cfg.ThinkingDisabled {
		t.Fatal("thinking_disabled not merged from config_json")
	}
	if cfg.ProviderType != "deepseek" {
		t.Fatalf("provider_type = %q, want deepseek", cfg.ProviderType)
	}
}
