package a2a

import (
	"testing"

	"trpc.group/trpc-go/trpc-a2a-go/protocol"
)

func TestMessageResultText_message(t *testing.T) {
	t.Parallel()
	msg := protocol.NewMessage(protocol.MessageRoleAgent, []protocol.Part{protocol.NewTextPart("hello")})
	text := messageResultText(&protocol.MessageResult{Result: &msg})
	if text != "hello" {
		t.Fatalf("got %q", text)
	}
}

func TestMessageResultText_nil(t *testing.T) {
	t.Parallel()
	if messageResultText(nil) != "" {
		t.Fatal("expected empty")
	}
}
