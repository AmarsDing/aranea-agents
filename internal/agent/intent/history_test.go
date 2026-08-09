package intent

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuildUserMessageContent_NoHistory(t *testing.T) {
	got := buildUserMessageContent("帮我做个 CRM", nil)
	want := "User message:\n\n帮我做个 CRM"
	if got != want {
		t.Errorf("no-history content = %q, want %q", got, want)
	}
	// 空切片与 nil 同语义
	if got := buildUserMessageContent("帮我做个 CRM", []HistoryMessage{}); got != want {
		t.Errorf("empty-history content = %q, want %q", got, want)
	}
}

func TestBuildUserMessageContent_WithHistory(t *testing.T) {
	history := []HistoryMessage{
		{Role: "user", Content: "我们团队在选型 CRM"},
		{Role: "assistant", Content: "推荐先看开源方案"},
		{Role: "system", Content: "system notice"}, // 非 user/assistant 角色应过滤
		{Role: "user", Content: "   "},             // 空内容应过滤
	}
	got := buildUserMessageContent("它支持私有化部署吗？", history)

	if !strings.Contains(got, "Recent conversation") {
		t.Errorf("missing history section header: %q", got)
	}
	// 历史先旧后新，当前消息在末尾
	iu := strings.Index(got, "user: 我们团队在选型 CRM")
	ia := strings.Index(got, "assistant: 推荐先看开源方案")
	im := strings.Index(got, "User message:\n\n它支持私有化部署吗？")
	if iu < 0 || ia < 0 || im < 0 || !(iu < ia && ia < im) {
		t.Errorf("history order wrong (want oldest-first, current last): %q", got)
	}
	if strings.Contains(got, "system notice") {
		t.Errorf("non user/assistant role should be filtered: %q", got)
	}
}

func TestBuildUserMessageContent_KeepsMostRecentWithinCap(t *testing.T) {
	history := make([]HistoryMessage, 0, MaxIntentHistoryMessages+3)
	for i := 0; i < MaxIntentHistoryMessages+3; i++ {
		history = append(history, HistoryMessage{Role: "user", Content: fmt.Sprintf("msg-%d", i)})
	}
	got := buildUserMessageContent("当前问题", history)
	// 超限丢弃最旧：9 条留最近 6 条（msg-3..msg-8）
	for _, dropped := range []string{"msg-0", "msg-1", "msg-2"} {
		if strings.Contains(got, dropped) {
			t.Errorf("oldest message %q beyond cap should be dropped: %q", dropped, got)
		}
	}
	if !strings.Contains(got, "msg-8") {
		t.Errorf("most recent history message missing: %q", got)
	}
	if MaxIntentHistoryMessages != 6 {
		t.Errorf("MaxIntentHistoryMessages = %d, want 6", MaxIntentHistoryMessages)
	}
}

func TestBuildUserMessageContent_TruncatesLongHistoryContent(t *testing.T) {
	long := strings.Repeat("长", intentHistoryMaxContentRunes+50)
	got := buildUserMessageContent("Q", []HistoryMessage{{Role: "user", Content: long}})
	if strings.Contains(got, long) {
		t.Error("long history content should be truncated")
	}
	if !strings.Contains(got, strings.Repeat("长", intentHistoryMaxContentRunes)) {
		t.Error("truncated content prefix missing")
	}
}
