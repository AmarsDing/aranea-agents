package discord

import "testing"

func TestTextSender_ID(t *testing.T) {
	s := &TextSender{}
	if got := s.ID(); got != "discord" {
		t.Fatalf("expected 'discord', got %q", got)
	}
}
