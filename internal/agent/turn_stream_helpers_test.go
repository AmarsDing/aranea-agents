package agent

import (
	"testing"
)

func TestDisplayMarkdownFromStream_prefersReply(t *testing.T) {
	var r EventStreamResult
	_, _ = r.Reply.WriteString("hello")
	_, _ = r.Reasoning.WriteString("think")
	got, fallback := DisplayMarkdownFromStream(r)
	if got != "hello" {
		t.Fatalf("got %q", got)
	}
	if fallback {
		t.Fatal("expected fallback=false for reply content")
	}
}

func TestDisplayMarkdownFromStream_reasoningFallback(t *testing.T) {
	var r EventStreamResult
	_, _ = r.Reasoning.WriteString("only reasoning")
	got, fallback := DisplayMarkdownFromStream(r)
	if got != "only reasoning" {
		t.Fatalf("got %q", got)
	}
	if !fallback {
		t.Fatal("expected fallback=true for reasoning-only content")
	}
}

func TestDisplayMarkdownFromStream_emptyResult(t *testing.T) {
	var r EventStreamResult
	got, fallback := DisplayMarkdownFromStream(r)
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
	if fallback {
		t.Fatal("expected fallback=false for empty result")
	}
}

func TestEstimateTokensIfMissing_skipsWhenUsagePresent(t *testing.T) {
	in, out := EstimateTokensIfMissing(10, 5, "in", "out")
	if in != 10 || out != 5 {
		t.Fatalf("in=%d out=%d", in, out)
	}
}
