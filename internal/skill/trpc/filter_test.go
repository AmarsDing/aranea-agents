package trpc

import (
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

func TestCanonicalSlug(t *testing.T) {
	if got := canonicalSlug("My-Skill"); got != "my-skill" {
		t.Fatalf("expected my-skill, got %q", got)
	}
	// NormalizeSlug replaces spaces with hyphens for consistent slug matching.
	if got := canonicalSlug("  Hello World  "); got != "hello-world" {
		t.Fatalf("expected 'hello-world', got %q", got)
	}
	if got := canonicalSlug(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	// Verify that non-slug inputs are properly normalized.
	if got := canonicalSlug("My Skill"); got != "my-skill" {
		t.Fatalf("expected 'my-skill', got %q", got)
	}
	if got := canonicalSlug("MY_SKILL"); got != "my_skill" {
		t.Fatalf("expected 'my_skill', got %q", got)
	}
}

func TestNewDBRepositoryAdapter_DefaultTTL(t *testing.T) {
	adapter := NewDBRepositoryAdapter(nil, 0, loggateway.NewNoop())
	if adapter.ttl != 2*time.Minute {
		t.Fatalf("expected 2m default TTL, got %v", adapter.ttl)
	}
}

func TestNewDBRepositoryAdapter_CustomTTL(t *testing.T) {
	adapter := NewDBRepositoryAdapter(nil, 5*time.Minute, loggateway.NewNoop())
	if adapter.ttl != 5*time.Minute {
		t.Fatalf("expected 5m TTL, got %v", adapter.ttl)
	}
}

func TestDBRepositoryAdapter_Invalidate(t *testing.T) {
	adapter := NewDBRepositoryAdapter(nil, time.Minute, loggateway.NewNoop())
	now := time.Now()
	adapter.loaded = now
	adapter.Invalidate()
	if !adapter.loaded.IsZero() {
		t.Fatal("Invalidate should reset loaded to zero")
	}
}

func TestNewFSRepositoryAdapter_InvalidRoot(t *testing.T) {
	adapter, err := NewFSRepositoryAdapter("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adapter.Summaries()) != 0 {
		t.Fatal("expected no summaries for nonexistent root")
	}
}

func TestWrapWithArtifactSave_Nil(t *testing.T) {
	result := WrapWithArtifactSave(nil, loggateway.NewNoop())
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestSkillNotFoundError(t *testing.T) {
	err := &skillNotFoundError{name: "test-skill"}
	if err.Error() != "skill not found: test-skill" {
		t.Fatalf("unexpected error: %q", err.Error())
	}
}
