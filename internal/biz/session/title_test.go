package session

import (
	"strings"
	"testing"
)

func TestSessionTitleFromUserSnippet(t *testing.T) {
	longInput := strings.Repeat("a", 60)
	longExpected := strings.Repeat("a", 56) + "…"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"whitespace only", "   \t  ", ""},
		{"short string", "hello world", "hello world"},
		{"long string truncates", longInput, longExpected},
		{"exactly 56 chars no truncation", strings.Repeat("a", 56), strings.Repeat("a", 56)},
		{"57 chars truncates", strings.Repeat("a", 57), strings.Repeat("a", 56) + "…"},
		{"newline replaced with space", "hello\nworld", "hello world"},
		{"carriage return removed", "hello\r\nworld", "hello world"},
		{"multiple newlines", "line1\n\nline2\nline3", "line1 line2 line3"},
		{"multiple spaces compressed", "hello   world   foo", "hello world foo"},
		{"tabs compressed", "hello\t\tworld", "hello world"},
		{"leading trailing whitespace", "  hello world  ", "hello world"},
		{"mixed whitespace", "  hello \n \r\n world  ", "hello world"},
		{"chinese characters", "你好世界", "你好世界"},
		{"long chinese truncated", strings.Repeat("你", 60), strings.Repeat("你", 56) + "…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SessionTitleFromUserSnippet(tt.input)
			if got != tt.expected {
				t.Errorf("SessionTitleFromUserSnippet(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestShouldAutoNameSession(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		expected bool
	}{
		{"empty title", "", true},
		{"whitespace only", "   ", true},
		{"untitled lowercase", "untitled", true},
		{"untitled uppercase", "Untitled", true},
		{"untitled mixed case", "UNTITLEd", true},
		{"new chat lowercase", "new chat", true},
		{"new chat mixed case", "New Chat", true},
		{"new chat uppercase", "NEW CHAT", true},
		{"新会话", "新会话", true},
		{"未命名对话", "未命名对话", true},
		{"新对话", "新对话", true},
		{"新会话 with prefix", "我的新会话", true},
		{"未命名 with suffix", "未命名项目", true},
		{"custom title", "My Project Discussion", false},
		{"custom chinese title", "项目讨论", false},
		{"title with spaces", "  untitled  ", true},
		{"title resembling but not match", "new chatroom", false},
		{"title with untitled substring but not exact", "my untitled doc", false},
		{"single char", "a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldAutoNameSession(tt.title)
			if got != tt.expected {
				t.Errorf("ShouldAutoNameSession(%q) = %v, want %v", tt.title, got, tt.expected)
			}
		})
	}
}
