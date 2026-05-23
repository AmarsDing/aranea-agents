package agent

import (
	"testing"
)

func TestDisplayMarkdownFromStream_prefersReply(t *testing.T) {
	var r EventStreamResult
	_, _ = r.Reply.WriteString("hello")
	_, _ = r.Reasoning.WriteString("think")
	if got := DisplayMarkdownFromStream(r); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestDisplayMarkdownFromStream_reasoningFallback(t *testing.T) {
	var r EventStreamResult
	_, _ = r.Reasoning.WriteString("only reasoning")
	if got := DisplayMarkdownFromStream(r); got != "only reasoning" {
		t.Fatalf("got %q", got)
	}
}

func TestEstimateTokensIfMissing_skipsWhenUsagePresent(t *testing.T) {
	in, out := EstimateTokensIfMissing(10, 5, "in", "out")
	if in != 10 || out != 5 {
		t.Fatalf("in=%d out=%d", in, out)
	}
}
