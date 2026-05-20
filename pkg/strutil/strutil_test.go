package strutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateRunes_utf8Safe(t *testing.T) {
	s := strings.Repeat("中", 100)
	got := TruncateRunes(s, 50)
	if !utf8.ValidString(got) {
		t.Fatal("invalid utf-8 after truncate")
	}
	if len([]rune(got)) != 50 {
		t.Fatalf("want 50 runes, got %d", len([]rune(got)))
	}
	// Byte slice at 200 would break a 3-byte rune (legacy bug).
	broken := s[:200]
	if utf8.ValidString(broken) {
		t.Fatal("expected broken legacy slice for this sample")
	}
}

func TestValidUTF8(t *testing.T) {
	s := strings.Repeat("中", 66) + string([]byte{0xff})
	got := ValidUTF8(s)
	if !utf8.ValidString(got) {
		t.Fatal("expected valid utf-8")
	}
}

func TestProtoPreview(t *testing.T) {
	got := ProtoPreview(strings.Repeat("文", 300), 200)
	if len([]rune(got)) != 200 {
		t.Fatalf("want 200 runes, got %d", len([]rune(got)))
	}
	if !utf8.ValidString(got) {
		t.Fatal("invalid utf-8")
	}
}
