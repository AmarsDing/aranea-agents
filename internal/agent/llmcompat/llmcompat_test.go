package llmcompat

import (
	"errors"
	"testing"

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
