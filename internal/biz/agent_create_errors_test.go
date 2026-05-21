package biz

import (
	"errors"
	"testing"
)

func TestIsAgentKeyDuplicate(t *testing.T) {
	if isAgentKeyDuplicate(nil) {
		t.Fatal("nil should not be duplicate")
	}
	if isAgentKeyDuplicate(errors.New("other error")) {
		t.Fatal("unrelated error should not match")
	}
	if !isAgentKeyDuplicate(errors.New("UNIQUE constraint failed: agents.agent_key")) {
		t.Fatal("expected unique agent_key message to match")
	}
}
