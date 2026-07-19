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
