package service

import (
	"testing"
)

func TestTrimKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"spaces_only", "   ", ""},
		{"tabs_only", "\t\t", ""},
		{"leading_spaces", "  hello", "hello"},
		{"trailing_spaces", "hello  ", "hello"},
		{"leading_tab", "\thello", "hello"},
		{"trailing_tab", "hello\t", "hello"},
		{"both_sides", "  hello  ", "hello"},
		{"both_tabs", "\thello\t", "hello"},
		{"mixed_whitespace", " \thello \t", "hello"},
		{"inner_space", "hello world", "hello world"},
		{"inner_tab", "hello\tworld", "hello\tworld"},
		{"surrounding_with_inner", "  hello world  ", "hello world"},
		{"single_char_leading", " a", "a"},
		{"single_char_trailing", "a ", "a"},
		{"single_char_both", " a ", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimKey(tt.input); got != tt.want {
				t.Errorf("trimKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
