package llmcompat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
		func(piece string) error { received += piece; return nil })
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
