package memory_butler

import (
	"context"
	"errors"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestAnalyzeMemoryQuality_RequiresAgentID(t *testing.T) {
	tl, ok := newAnalyzeMemoryQualityTool(Deps{}).(trpctool.CallableTool)
	if !ok {
		t.Fatal("analyze_quality must be callable")
	}
	if _, err := tl.Call(context.Background(), []byte(`{"agent_id":""}`)); !errors.Is(err, ErrAgentIDRequired) {
		t.Fatalf("expected ErrAgentIDRequired, got %v", err)
	}
}
