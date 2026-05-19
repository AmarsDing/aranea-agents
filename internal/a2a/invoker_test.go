package a2a

import (
	"testing"
)

func TestPayloadToInput(t *testing.T) {
	t.Parallel()
	if got := PayloadToInput(`{"message":"hello"}`, "chat"); got != "hello" {
		t.Fatalf("got %q", got)
	}
	if got := PayloadToInput("{}", "summarize"); got != "summarize" {
		t.Fatalf("got %q", got)
	}
	if got := PayloadToInput(`{"foo":1}`, "chat"); got != `chat: {"foo":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestValidateSameWorkspace(t *testing.T) {
	t.Parallel()
	if err := ValidateSameWorkspace("ws-a", "ws-a"); err != nil {
		t.Fatalf("same workspace should pass: %v", err)
	}
	if err := ValidateSameWorkspace("", "ws-a"); err != nil {
		t.Fatalf("empty caller workspace should pass: %v", err)
	}
	if err := ValidateSameWorkspace("ws-a", "ws-b"); err == nil {
		t.Fatal("expected cross-workspace error")
	}
}
