package service

import (
	"aranea-agents/internal/biz"
	"context"
	"testing"
)

func TestRecentIntentHistory_FiltersDedupesAndCaps(t *testing.T) {
	msgs := []biz.ChatMessage{
		{Role: "system", ContentMarkdown: "系统通知"},
		{Role: "user", ContentMarkdown: "   "},
		{Role: "user", ContentMarkdown: "先做个 Web 版"},
		{Role: "assistant", ContentMarkdown: "好的，用 React"},
		{Role: "user", ContentMarkdown: "它支持导出吗？"},
	}
	orch := newIntentHistoryTestOrch(&stubRecentMessageLister{msgs: msgs})
	got := orch.recentIntentHistory(context.Background(), "sess-1", "它支持导出吗？")
	if len(got) != 2 {
		t.Fatalf("history len = %d, want 2 (filtered + deduped): %+v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Content != "先做个 Web 版" {
		t.Errorf("history[0] = %+v", got[0])
	}
	if got[1].Role != "assistant" || got[1].Content != "好的，用 React" {
		t.Errorf("history[1] = %+v", got[1])
	}
}
