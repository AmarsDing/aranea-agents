package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestTurnStatusFromNativeOutcome(t *testing.T) {
	cases := []struct {
		name    string
		outcome biz.NativeTurnOutcome
		want    biz.TurnStatus
	}{
		{"completed", biz.NativeTurnOutcomeCompleted, biz.TurnStatusCompleted},
		{"queued", biz.NativeTurnOutcomeQueued, biz.TurnStatusQueued},
		{"rejected", biz.NativeTurnOutcomeRejected, biz.TurnStatusRejected},
		{"failed", biz.NativeTurnOutcomeFailed, biz.TurnStatusFailed},
		{"unknown", biz.NativeTurnOutcome("unknown"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.TurnStatusFromNativeOutcome(tc.outcome)
			if got != tc.want {
				t.Fatalf("TurnStatusFromNativeOutcome(%q) = %q, want %q", tc.outcome, got, tc.want)
			}
		})
	}
}

func TestDeliveryStatusFromChannelRecord(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  biz.DeliveryStatus
	}{
		{"queued", "queued", biz.DeliveryStatusPending},
		{"pending", "pending", biz.DeliveryStatusPending},
		{"sending", "sending", biz.DeliveryStatusSending},
		{"streaming", "streaming", biz.DeliveryStatusSending},
		{"streamed", "streamed", biz.DeliveryStatusSending},
		{"sent", "sent", biz.DeliveryStatusDelivered},
		{"delivered", "delivered", biz.DeliveryStatusDelivered},
		{"ok", "ok", biz.DeliveryStatusDelivered},
		{"success", "success", biz.DeliveryStatusDelivered},
		{"failed", "failed", biz.DeliveryStatusFailed},
		{"error", "error", biz.DeliveryStatusFailed},
		{"timeout", "timeout", biz.DeliveryStatusFailed},
		{"skipped", "skipped", biz.DeliveryStatusSkipped},
		{"skipped_duplicate", "skipped_duplicate", biz.DeliveryStatusSkipped},
		{"skipped_access", "skipped_access", biz.DeliveryStatusSkipped},
		{"skipped_empty", "skipped_empty", biz.DeliveryStatusSkipped},
		{"unknown", "unknown_status", ""},
		{"empty", "", ""},
		{"case insensitive", "DELIVERED", biz.DeliveryStatusDelivered},
		{"with spaces", "  sent  ", biz.DeliveryStatusDelivered},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.DeliveryStatusFromChannelRecord(tc.input)
			if got != tc.want {
				t.Fatalf("DeliveryStatusFromChannelRecord(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCanonicalTurnSource(t *testing.T) {
	cases := []struct {
		name   string
		source biz.TurnSource
		entry  biz.TurnEntryPoint
		want   biz.TurnSource
	}{
		{"source takes priority", biz.TurnSourceWeb, biz.EntryPointWS, biz.TurnSourceWeb},
		{"empty source with WS entry", "", biz.EntryPointWS, biz.TurnSourceWS},
		{"empty source with channel entry", "", biz.EntryPointChannel, biz.TurnSourceChannel},
		{"empty source with cron entry", "", biz.EntryPointCron, biz.TurnSourceCron},
		{"empty source with a2a entry", "", biz.EntryPointA2A, biz.TurnSourceA2A},
		{"empty source with durable entry", "", biz.EntryPointDurable, biz.TurnSourceDurable},
		{"empty source with web entry", "", biz.EntryPointWeb, biz.TurnSourceWeb},
		{"empty source with unknown entry", "", biz.TurnEntryPoint("unknown"), biz.TurnSourceWeb},
		{"both empty defaults to web", "", "", biz.TurnSourceWeb},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.CanonicalTurnSource(tc.source, tc.entry)
			if got != tc.want {
				t.Fatalf("CanonicalTurnSource(%q, %q) = %q, want %q", tc.source, tc.entry, got, tc.want)
			}
		})
	}
}

func TestEntryPointFromTurnSource(t *testing.T) {
	cases := []struct {
		source biz.TurnSource
		want   biz.TurnEntryPoint
	}{
		{biz.TurnSourceWS, biz.EntryPointWS},
		{biz.TurnSourceChannel, biz.EntryPointChannel},
		{biz.TurnSourceCron, biz.EntryPointCron},
		{biz.TurnSourceA2A, biz.EntryPointA2A},
		{biz.TurnSourceDurable, biz.EntryPointDurable},
		{biz.TurnSourceWeb, biz.EntryPointWeb},
		{biz.TurnSource("unknown"), biz.EntryPointWeb},
		{"", biz.EntryPointWeb},
	}
	for _, tc := range cases {
		t.Run(string(tc.source), func(t *testing.T) {
			got := biz.EntryPointFromTurnSource(tc.source)
			if got != tc.want {
				t.Fatalf("EntryPointFromTurnSource(%q) = %q, want %q", tc.source, got, tc.want)
			}
		})
	}
}

func TestTurnIntentCanonicalize(t *testing.T) {
	t.Run("fills TargetType from TeamID", func(t *testing.T) {
		intent := biz.TurnIntent{
			TeamID:    "team-1",
			SessionID: "  sess-1  ",
			Content:   "  hello  ",
			AgentKey:  "  key-1  ",
		}
		got := intent.Canonicalize()
		if got.TargetType != biz.ConversationTargetTeam {
			t.Fatalf("TargetType = %q, want team", got.TargetType)
		}
		if got.SessionID != "sess-1" {
			t.Fatalf("SessionID = %q, want sess-1", got.SessionID)
		}
		if got.Content != "hello" {
			t.Fatalf("Content = %q, want hello", got.Content)
		}
		if got.AgentKey != "key-1" {
			t.Fatalf("AgentKey = %q, want key-1", got.AgentKey)
		}
	})

	t.Run("fills TargetType as agent when no TeamID", func(t *testing.T) {
		intent := biz.TurnIntent{AgentID: "agent-1"}
		got := intent.Canonicalize()
		if got.TargetType != biz.ConversationTargetAgent {
			t.Fatalf("TargetType = %q, want agent", got.TargetType)
		}
	})

	t.Run("preserves explicit TargetType", func(t *testing.T) {
		intent := biz.TurnIntent{
			TargetType: biz.ConversationTargetTeam,
			AgentID:    "agent-1",
		}
		got := intent.Canonicalize()
		if got.TargetType != biz.ConversationTargetTeam {
			t.Fatalf("TargetType = %q, want team", got.TargetType)
		}
	})

	t.Run("trims all string fields", func(t *testing.T) {
		intent := biz.TurnIntent{
			SessionID:      "  s1  ",
			AgentID:        "  a1  ",
			AgentKey:       "  k1  ",
			TeamID:         "  t1  ",
			Content:        "  hi  ",
			IdempotencyKey: "  idem  ",
		}
		got := intent.Canonicalize()
		if got.SessionID != "s1" || got.AgentID != "a1" || got.AgentKey != "k1" {
			t.Fatalf("string fields not trimmed: %+v", got)
		}
		if got.TeamID != "t1" || got.Content != "hi" || got.IdempotencyKey != "idem" {
			t.Fatalf("string fields not trimmed: %+v", got)
		}
	})

	t.Run("source from entry point", func(t *testing.T) {
		intent := biz.TurnIntent{
			EntryConfig: biz.TurnEntryPointConfig{EntryPoint: biz.EntryPointWS},
		}
		got := intent.Canonicalize()
		if got.Source != biz.TurnSourceWS {
			t.Fatalf("Source = %q, want ws", got.Source)
		}
	})
}

func TestTurnIntentTurnInput(t *testing.T) {
	t.Run("converts intent to TurnInput", func(t *testing.T) {
		intent := biz.TurnIntent{
			SessionID: "sess-1",
			Content:   "hello",
			AgentKey:  "key-1",
			TeamID:    "team-1",
			EntryConfig: biz.TurnEntryPointConfig{
				EntryPoint:  biz.EntryPointWS,
				AllowQueue:  true,
				AllowStream: true,
				Platform:    "feishu",
			},
		}
		got := intent.TurnInput()
		if got.SessionID != "sess-1" {
			t.Fatalf("SessionID = %q, want sess-1", got.SessionID)
		}
		if got.Content != "hello" {
			t.Fatalf("Content = %q, want hello", got.Content)
		}
		if got.AgentKey != "key-1" {
			t.Fatalf("AgentKey = %q, want key-1", got.AgentKey)
		}
		if got.TeamID != "team-1" {
			t.Fatalf("TeamID = %q, want team-1", got.TeamID)
		}
		if got.EntryConfig.EntryPoint != biz.EntryPointWS {
			t.Fatalf("EntryPoint = %q, want ws", got.EntryConfig.EntryPoint)
		}
		if !got.EntryConfig.AllowQueue {
			t.Fatalf("AllowQueue should be true")
		}
		if !got.EntryConfig.AllowStream {
			t.Fatalf("AllowStream should be true")
		}
		if got.EntryConfig.Platform != "feishu" {
			t.Fatalf("Platform = %q, want feishu", got.EntryConfig.Platform)
		}
	})

	t.Run("source maps to entry point", func(t *testing.T) {
		intent := biz.TurnIntent{
			Source:    biz.TurnSourceChannel,
			SessionID: "s1",
			Content:   "hi",
		}
		got := intent.TurnInput()
		if got.EntryConfig.EntryPoint != biz.EntryPointChannel {
			t.Fatalf("EntryPoint = %q, want channel", got.EntryConfig.EntryPoint)
		}
	})
}
