package biz

import "testing"

func TestFormatCompositeRecallLine(t *testing.T) {
	t.Parallel()
	if got := formatCompositeRecallLine(CompositeRecallStoreRow{
		Layer: "L2", Title: "Dark mode", Summary: "User prefers dark theme",
	}); got != "Dark mode: User prefers dark theme" {
		t.Fatalf("L2 line = %q", got)
	}
	if got := formatCompositeRecallLine(CompositeRecallStoreRow{
		Layer: "L3", Statement: "User name is Alice",
	}); got != "User name is Alice" {
		t.Fatalf("L3 line = %q", got)
	}
}
