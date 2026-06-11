package agenticons

import (
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestLoadPNG_EmptyKey(t *testing.T) {
	_, err := LoadPNG("")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T", err)
	}
	if ae.Domain != "AGENT_ICONS" {
		t.Fatalf("domain = %q, want %q", ae.Domain, "AGENT_ICONS")
	}
}

func TestLoadPNG_NonexistentKey(t *testing.T) {
	_, err := LoadPNG("nonexistent_icon")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestLoadPNG_ValidKey(t *testing.T) {
	data, err := LoadPNG("avatar_career_01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty PNG data")
	}
	if len(data) < 8 {
		t.Fatal("PNG data too short to be a valid PNG")
	}
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i, b := range pngHeader {
		if data[i] != b {
			t.Fatalf("byte %d = 0x%02X, want 0x%02X (not a valid PNG header)", i, data[i], b)
		}
	}
}
