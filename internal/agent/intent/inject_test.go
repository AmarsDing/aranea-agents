package intent

import (
	"strings"
	"testing"
)

func TestSystemContextMessage(t *testing.T) {
	msg := SystemContextMessage(&Artifact{RefinedGoal: "fix tests", IntentKind: "debug"})
	if msg.Role != "system" {
		t.Fatalf("role: %s", msg.Role)
	}
	if !strings.Contains(msg.Content, intentContextHeader) {
		t.Fatalf("missing header: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, "fix tests") {
		t.Fatalf("missing goal: %q", msg.Content)
	}
}

func TestIsIntentContextContent(t *testing.T) {
	if !IsIntentContextContent(SystemContextMessage(&Artifact{RefinedGoal: "x"}).Content) {
		t.Fatal("expected intent context")
	}
}
