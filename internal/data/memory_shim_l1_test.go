package data

import (
	"testing"
)

// TestEstimateTokens tests the CJK-aware token estimation.
func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name  string
		texts []string
		want  int
	}{
		{name: "empty", texts: []string{""}, want: 0},
		{name: "single ASCII char", texts: []string{"a"}, want: 1},
		{name: "hello (5 non-CJK)", texts: []string{"hello"}, want: 1},
		{name: "Chinese 4 chars (CJK)", texts: []string{"你好世界"}, want: 4},
		{name: "8 ASCII chars", texts: []string{"abcdefgh"}, want: 2},
		{name: "mixed CJK+ASCII", texts: []string{"你好ab"}, want: 2},
		{name: "multiple texts combined", texts: []string{"hello", "你好"}, want: 3},
		{name: "single CJK char", texts: []string{"中"}, want: 1},
		{name: "CJK + lots of ASCII", texts: []string{"你好hello world"}, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.texts...)
			if got != tt.want {
				t.Errorf("estimateTokens(%v) = %d, want %d", tt.texts, got, tt.want)
			}
		})
	}
}

// TestIsCJKRune tests CJK character detection.
func TestIsCJKRune(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'中', true},  // CJK Unified Ideographs
		{'あ', true},  // Hiragana
		{'ア', true},  // Katakana
		{'한', true},  // Hangul
		{'a', false}, // ASCII
		{'é', false}, // Latin extended
		{'0', false}, // Digit
		{' ', false}, // Space
	}
	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			if got := isCJKRune(tt.r); got != tt.want {
				t.Errorf("isCJKRune(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}
