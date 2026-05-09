package adksvc

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestRewriteSnapshotWithCompression_trimsEventsAndPrependsSummary(t *testing.T) {
	snapshot := `{"app_name":"aranea","user_id":"local","root_agent_name":"demo","state":{},"events":[{"actions":{},"llm_response":{"content":{"parts":[{"text":"old"}],"role":"user"}},"author":"user"}],"updated_at":"2025-01-01T00:00:00Z"}`
	tail := []biz.ChatMessage{
		{TurnIndex: 10, Role: "user", ContentMarkdown: "hello"},
		{TurnIndex: 11, Role: "assistant", ContentMarkdown: "hi"},
	}
	out, err := RewriteSnapshotWithCompression(snapshot, "## Facts\nok", tail, "demo")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := unmarshalBundle(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Events) != 3 {
		t.Fatalf("events len=%d want 3 (summary + 2 tail)", len(bundle.Events))
	}
	c0 := bundle.Events[0].LLMResponse.Content
	if c0 == nil || !strings.EqualFold(strings.TrimSpace(c0.Role), "system") {
		t.Fatalf("first event should be system content, got %#v", c0)
	}
}
