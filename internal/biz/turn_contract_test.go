package biz

import "testing"

func TestTurnIntentCanonicalize(t *testing.T) {
	intent := TurnIntent{
		SessionID: " sess-1 ",
		TeamID:    " team-1 ",
		Content:   " hello ",
		EntryConfig: TurnEntryPointConfig{
			EntryPoint:  EntryPointChannel,
			AllowStream: true,
			Platform:    "slack",
		},
	}

	got := intent.Canonicalize()
	if got.Source != TurnSourceChannel {
		t.Fatalf("Source = %q, want %q", got.Source, TurnSourceChannel)
	}
	if got.TargetType != ConversationTargetTeam {
		t.Fatalf("TargetType = %q, want %q", got.TargetType, ConversationTargetTeam)
	}
	if got.SessionID != "sess-1" || got.TeamID != "team-1" || got.Content != "hello" {
		t.Fatalf("Canonicalize did not trim fields: %+v", got)
	}

	input := got.TurnInput()
	if input.EntryConfig.EntryPoint != EntryPointChannel {
		t.Fatalf("TurnInput EntryPoint = %q, want %q", input.EntryConfig.EntryPoint, EntryPointChannel)
	}
	if !input.EntryConfig.AllowStream || input.EntryConfig.Platform != "slack" {
		t.Fatalf("TurnInput lost entry config: %+v", input.EntryConfig)
	}
}

func TestTurnStatusFromNativeOutcome(t *testing.T) {
	cases := map[NativeTurnOutcome]TurnStatus{
		NativeTurnOutcomeCompleted: TurnStatusCompleted,
		NativeTurnOutcomeQueued:    TurnStatusQueued,
		NativeTurnOutcomeFailed:    TurnStatusFailed,
	}
	for outcome, want := range cases {
		if got := TurnStatusFromNativeOutcome(outcome); got != want {
			t.Fatalf("TurnStatusFromNativeOutcome(%q) = %q, want %q", outcome, got, want)
		}
	}
}

func TestDeliveryStatusFromChannelRecord(t *testing.T) {
	cases := map[string]DeliveryStatus{
		"queued":            DeliveryStatusPending,
		"streamed":          DeliveryStatusSending,
		"sent":              DeliveryStatusDelivered,
		"error":             DeliveryStatusFailed,
		"skipped_duplicate": DeliveryStatusSkipped,
	}
	for input, want := range cases {
		if got := DeliveryStatusFromChannelRecord(input); got != want {
			t.Fatalf("DeliveryStatusFromChannelRecord(%q) = %q, want %q", input, got, want)
		}
	}
}
