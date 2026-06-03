package preview

import (
	"strings"
	"testing"
)

func TestRedactAndTruncate_EmailRedaction(t *testing.T) {
	input := `contact user@example.com for details`
	got := RedactAndTruncate(input, 0)
	if strings.Contains(got, "user@example.com") {
		t.Errorf("email not redacted: %q", got)
	}
	if !strings.Contains(got, "[email redacted]") {
		t.Errorf("expected [email redacted] in output: %q", got)
	}
}

func TestRedactAndTruncate_PhoneRedaction(t *testing.T) {
	input := `call +1-555-123-4567 now`
	got := RedactAndTruncate(input, 0)
	if strings.Contains(got, "555-123-4567") {
		t.Errorf("phone not redacted: %q", got)
	}
	if !strings.Contains(got, "[phone redacted]") {
		t.Errorf("expected [phone redacted] in output: %q", got)
	}
}

func TestRedactAndTruncate_SecretRedaction(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // substring that should be in output
	}{
		{
			"api_key assignment",
			`api_key=sk-1234567890`,
			"[secret redacted]",
		},
		{
			"token colon",
			`token: abcdefghijklmn`,
			"[secret redacted]",
		},
		{
			"password assignment",
			`password=mypass123`,
			"[secret redacted]",
		},
		{
			"JSON secret key",
			`"api_key": "sk-1234567890"`,
			`"[secret redacted]"`,
		},
		{
			"bearer token",
			`bearer: tok_abc123def456`,
			"[secret redacted]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactAndTruncate(tt.input, 0)
			if !strings.Contains(got, tt.want) {
				t.Errorf("RedactAndTruncate(%q) = %q, want to contain %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRedactAndTruncate_UTF8Truncation(t *testing.T) {
	// Multi-byte characters: each Chinese char is 3 bytes in UTF-8
	input := "你好世界再见你好世界再见你好世界再见"
	// Limit to 5 runes
	got := RedactAndTruncate(input, 5)
	if len([]rune(got)) != 5 {
		t.Errorf("expected 5 runes, got %d: %q", len([]rune(got)), got)
	}
	want := "你好世界再"
	if got != want {
		t.Errorf("RedactAndTruncate(%q, 5) = %q, want %q", input, got, want)
	}
}

func TestRedactAndTruncate_MaxLenZero(t *testing.T) {
	input := strings.Repeat("a", 3000)
	got := RedactAndTruncate(input, 0)
	// maxLen=0 should use default (2000)
	if len(got) > 2000 {
		t.Errorf("expected output <= 2000 chars, got %d", len(got))
	}
}

func TestRedactAndTruncate_MaxLenNegative(t *testing.T) {
	input := strings.Repeat("a", 3000)
	got := RedactAndTruncate(input, -1)
	// Negative maxLen should also use default
	if len(got) > 2000 {
		t.Errorf("expected output <= 2000 chars, got %d", len(got))
	}
}

func TestRedactAndTruncate_EmptyInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"whitespace only", "   \t\n  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactAndTruncate(tt.input, 100)
			if got != tt.want {
				t.Errorf("RedactAndTruncate(%q, 100) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRedactAndTruncate_ChineseTruncation(t *testing.T) {
	input := "这是一个包含中文字符的长字符串用于测试截断功能是否正常工作"
	got := RedactAndTruncate(input, 10)
	runes := []rune(got)
	if len(runes) != 10 {
		t.Errorf("expected 10 runes, got %d: %q", len(runes), got)
	}
}

func TestRedactAndTruncate_ExactLength(t *testing.T) {
	input := "hello"
	got := RedactAndTruncate(input, 5)
	if got != "hello" {
		t.Errorf("RedactAndTruncate(%q, 5) = %q, want %q", input, got, "hello")
	}
}

func TestRedactAndTruncate_ShortInput(t *testing.T) {
	input := "hi"
	got := RedactAndTruncate(input, 100)
	if got != "hi" {
		t.Errorf("RedactAndTruncate(%q, 100) = %q, want %q", input, got, "hi")
	}
}
