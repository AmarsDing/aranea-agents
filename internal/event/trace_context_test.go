package event

import (
	"context"
	"regexp"
	"testing"
)

func TestTraceIDContext_Roundtrip(t *testing.T) {
	if _, ok := TraceIDFromContext(context.Background()); ok {
		t.Fatal("empty ctx should not carry a trace id")
	}
	ctx := ContextWithTraceID(context.Background(), "abc123")
	id, ok := TraceIDFromContext(ctx)
	if !ok || id != "abc123" {
		t.Fatalf("roundtrip: id=%q ok=%v", id, ok)
	}
	// Empty / nil-safety
	if got := ContextWithTraceID(nil, "x"); got != nil {
		// nil ctx passes through unchanged (no panic)
		_ = got
	}
	if got := ContextWithTraceID(context.Background(), ""); got == nil {
		t.Fatal("empty trace id should keep ctx non-nil")
	} else if _, ok := TraceIDFromContext(got); ok {
		t.Fatal("empty trace id must not be stored")
	}
}

func TestGenerateTraceID_OTelHexFormat(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{32}$`)
	for i := 0; i < 4; i++ {
		id := GenerateTraceID()
		if !re.MatchString(id) {
			t.Fatalf("GenerateTraceID = %q, want 32 lowercase hex chars", id)
		}
	}
}

func TestNewTraceContext_PrefersExplicitTraceID(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	tc := NewTraceContext(ctx, TraceOpts{SessionID: "s1"})
	if tc.TraceID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("explicit ctx trace id must win, got %q", tc.TraceID)
	}
}

func TestNewTraceContext_GeneratesHexWhenMissing(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{32}$`)
	tc := NewTraceContext(context.Background(), TraceOpts{SessionID: "s1"})
	if !re.MatchString(tc.TraceID) {
		t.Fatalf("fallback trace id must be OTel hex, got %q", tc.TraceID)
	}
}
