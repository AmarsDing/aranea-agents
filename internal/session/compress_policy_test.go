package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestSessionCompressEnabled(t *testing.T) {
	if !sessionCompressEnabled(biz.Agent{Settings: &biz.AgentRuntimeSettings{L0SnapshotMode: "on_warning"}}) {
		t.Fatal("legacy on_warning should enable compress")
	}
	if sessionCompressEnabled(biz.Agent{Settings: &biz.AgentRuntimeSettings{L0SnapshotMode: "off"}}) {
		t.Fatal("legacy off should disable compress")
	}
	if !sessionCompressEnabled(biz.Agent{Settings: &biz.AgentRuntimeSettings{ContextCompactionEnabled: true, L0SnapshotMode: "off"}}) {
		t.Fatal("context_compaction_enabled should enable compress even when snapshot mode is off")
	}
}

func TestFilterMessagesForTruncateStrategy_dropsTool(t *testing.T) {
	msgs := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "hi"},
		{Role: "tool", ContentMarkdown: "result"},
	}
	out := filterMessagesForTruncateStrategy(msgs, "drop_tool_results")
	if len(out) != 1 || out[0].Role != "user" {
		t.Fatalf("got %#v", out)
	}
}

func TestCompressMinGapFromAgent_default(t *testing.T) {
	got := compressMinGapFromAgent(biz.Agent{})
	if got != DefaultCompressMinGap {
		t.Fatalf("default gap = %v want %v", got, DefaultCompressMinGap)
	}
}

func TestCompressMinGapFromAgent_custom(t *testing.T) {
	got := compressMinGapFromAgent(biz.Agent{
		Settings: &biz.AgentRuntimeSettings{L0CompressMinGapSec: 120},
	})
	if got != 2*time.Minute {
		t.Fatalf("custom gap = %v want 2m", got)
	}
}

func TestCompressDebounceActive_withinGap(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	last := now.Add(-5 * time.Minute).Format(time.RFC3339)
	if !compressDebounceActive(last, 10*time.Minute, now) {
		t.Fatal("expected debounce within min gap")
	}
}

func TestCompressProviderModel_agentOverride(t *testing.T) {
	p, m := compressProviderModel(
		biz.Session{DefaultProvider: "openai", DefaultModel: "gpt-4"},
		biz.Agent{
			Settings: &biz.AgentRuntimeSettings{
				L0CompressProvider: "deepseek",
				L0CompressModel:    "deepseek-chat",
			},
		},
	)
	if p != "deepseek" || m != "deepseek-chat" {
		t.Fatalf("got %q/%q", p, m)
	}
}

func TestRewriteSnapshotWithCompression_tailEvents(t *testing.T) {
	raw, err := RewriteSnapshotWithCompression(
		`{"state":{}}`,
		"merged summary",
		[]biz.ChatMessage{
			{Role: "user", ContentMarkdown: "hello", CreatedAt: "2026-05-24T10:00:00Z"},
			{Role: "assistant", ContentMarkdown: "hi", CreatedAt: "2026-05-24T10:00:01Z"},
		},
		"my-agent",
	)
	if err != nil {
		t.Fatal(err)
	}
	var bundle map[string]any
	if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
		t.Fatal(err)
	}
	events, ok := bundle["events"].([]any)
	if !ok || len(events) != 3 {
		t.Fatalf("events = %#v", bundle["events"])
	}
	first, ok := events[0].(map[string]any)
	if !ok || !strings.Contains(first["content"].(string), "merged summary") {
		t.Fatalf("summary event = %#v", events[0])
	}
}

func TestKey_defaultsAppName(t *testing.T) {
	k := Key("user-1", "sess-1")
	if k.AppName != DefaultAppName || k.UserID != "user-1" || k.SessionID != "sess-1" {
		t.Fatalf("unexpected key %+v", k)
	}
}
