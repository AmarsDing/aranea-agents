package config_test

import (
	"strings"
	"testing"

	"aranea-agents/internal/cli/config"
)

func TestTokenDisplay_ShowFull(t *testing.T) {
	got := config.TokenDisplay("secret", true)
	if got != "secret" {
		t.Errorf("TokenDisplay(%q, true) = %q, want %q", "secret", got, "secret")
	}
}

func TestTokenDisplay_Masked(t *testing.T) {
	got := config.TokenDisplay("secret", false)
	if got != "***cret" {
		t.Errorf("TokenDisplay(%q, false) = %q, want %q", "secret", got, "***cret")
	}
}

func TestTokenDisplay_Empty(t *testing.T) {
	got := config.TokenDisplay("", false)
	if got != "" {
		t.Errorf("TokenDisplay(%q, false) = %q, want %q", "", got, "")
	}
}

func TestStripBearerPrefix_WithPrefix(t *testing.T) {
	got := config.StripBearerPrefix("Bearer abc123")
	if got != "abc123" {
		t.Errorf("StripBearerPrefix(%q) = %q, want %q", "Bearer abc123", got, "abc123")
	}
}

func TestStripBearerPrefix_NoPrefix(t *testing.T) {
	got := config.StripBearerPrefix("abc123")
	if got != "abc123" {
		t.Errorf("StripBearerPrefix(%q) = %q, want %q", "abc123", got, "abc123")
	}
}

func TestStripBearerPrefix_Empty(t *testing.T) {
	got := config.StripBearerPrefix("")
	if got != "" {
		t.Errorf("StripBearerPrefix(%q) = %q, want %q", "", got, "")
	}
}

func TestMaskToken_LongToken(t *testing.T) {
	token := strings.Repeat("x", 40)
	got := config.MaskToken(token)
	want := "***xxxx"
	if got != want {
		t.Errorf("MaskToken(40-char) = %q, want %q", got, want)
	}
}

func TestMaskToken_VeryLongToken(t *testing.T) {
	token := strings.Repeat("x", 100)
	got := config.MaskToken(token)
	want := "***xxxx"
	if got != want {
		t.Errorf("MaskToken(100-char) = %q, want %q", got, want)
	}
}

func TestMaskToken_Exactly10Chars(t *testing.T) {
	token := strings.Repeat("x", 10)
	got := config.MaskToken(token)
	want := "***xxxx"
	if got != want {
		t.Errorf("MaskToken(10-char) = %q, want %q", got, want)
	}
}

func TestMaskToken_11Chars(t *testing.T) {
	token := strings.Repeat("x", 10) + "y"
	got := config.MaskToken(token)
	want := "***xxxy"
	if got != want {
		t.Errorf("MaskToken(11-char) = %q, want %q", got, want)
	}
}
