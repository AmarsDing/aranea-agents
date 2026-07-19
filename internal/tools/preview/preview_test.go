package preview

import (
	"strings"
	"testing"
)

// TestRedactAndTruncate_APIKeyPatterns verifies that common API key
// patterns are redacted. These patterns cover the most common secrets
// that might accidentally leak into logs or tool previews.
func TestRedactAndTruncate_APIKeyPatterns(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"OpenAI style key", "sk-abc123def456ghi789"},
		{"xAI key", "xai-abc123def456"},
		{"AWS AKIA", "AKIAIOSFODNN7EXAMPLE"},
		{"GitHub PAT", "ghp_xxxxxxxxxxxxxxxxxxxxxxxx"},
		{"Google API key", "AIzaSyA1234567890abcdefghijklmnopqrstuv"},
		{"Bearer token", "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
		{"JWT token", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
		{"api_key in JSON", `"api_key": "sk-1234567890abcdefghij"`},
		{"token assignment", "token=ghp_abcdefghijklmnopqrstuvwxyz12"},
		{"secret with colon", "secret: sk-ant-api03-abc123def456"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactAndTruncate(tt.input, 10000)
			// Should contain a redaction marker.
			if !strings.Contains(got, "[secret redacted]") && !strings.Contains(got, "[email redacted]") {
				t.Errorf("RedactAndTruncate(%q) did not redact secret, got: %q", tt.input, got)
			}
			// Should not contain the original secret.
			if strings.Contains(got, tt.input) {
				t.Errorf("RedactAndTruncate(%q) still contains original secret, got: %q", tt.input, got)
			}
		})
	}
}

// TestRedactAndTruncate_NoFalsePositives verifies that common non-secret
// strings are NOT redacted (e.g. task-, disk-, risk- prefixed words).
func TestRedactAndTruncate_NoFalsePositives(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"task- prefix", "task-1234567890"},
		{"disk- prefix", "disk-abcdefgh"},
		{"risk- prefix", "risk-987654321"},
		{"normal text", "this is a normal message without secrets"},
		{"short token", "token=abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactAndTruncate(tt.input, 10000)
			// Should not be redacted.
			if strings.Contains(got, "[secret redacted]") {
				t.Errorf("RedactAndTruncate(%q) incorrectly redacted non-secret, got: %q", tt.input, got)
			}
		})
	}
}

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

// TestRedactAndTruncate_MultipleSecrets verifies that when one specific
// pattern already matched, other secrets in the same text are still redacted
// by the generic secretRE. This is a regression test for the "one hit skips
// generic" logic flaw.
func TestRedactAndTruncate_MultipleSecrets(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		mustNotHave []string // substrings that must NOT appear in output
	}{
		{
			"OpenAI key + password",
			`key is sk-abc123def456ghi789 and password=mysecret123`,
			[]string{"sk-abc123def456ghi789", "mysecret123"},
		},
		{
			"JWT + api_key assignment",
			`token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U api_key=sk-livekey123456789abcdef`,
			[]string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", "sk-livekey123456789abcdef"},
		},
		{
			"Bearer + generic secret",
			`Authorization: Bearer tok_abc123def456 secret: myverysecretvalue`,
			[]string{"tok_abc123def456", "myverysecretvalue"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactAndTruncate(tt.input, 10000)
			for _, secret := range tt.mustNotHave {
				if strings.Contains(got, secret) {
					t.Errorf("RedactAndTruncate(%q) still contains secret %q, got: %q", tt.input, secret, got)
				}
			}
		})
	}
}

// TestRedactAndTruncate_ExtendedPatterns verifies newly added secret patterns:
// Anthropic sk-ant, Slack xoxb/xoxp, Stripe sk_live/rk_live, private key blocks,
// DSN embedded passwords.
func TestRedactAndTruncate_ExtendedPatterns(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Anthropic key", "sk-ant-api03-abc123def456ghi789"},
		{"Slack bot token", "xoxb-" + "1234567890-abcdefghijklmnop"},
		{"Slack user token", "xoxp-" + "1234567890-abcdefghijklmnop"},
		{"Stripe live secret key", "sk_live_abc123def456ghi789jkl0"},
		{"Stripe live restricted key", "rk_live_abc123def456ghi789jkl0"},
		{"RSA private key block", "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA7\n-----END RSA PRIVATE KEY-----"},
		{"EC private key block", "-----BEGIN EC PRIVATE KEY-----\nMHQCAQEEIA==\n-----END EC PRIVATE KEY-----"},
		{"Postgres DSN with password", "postgres://admin:secretpass123@db.example.com:5432/mydb"},
		{"MySQL DSN with password", "mysql://root:secretpass123@localhost:3306/mydb"},
		{"Redis DSN with password", "redis://default:secretpass123@redis.example.com:6379"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactAndTruncate(tt.input, 10000)
			if strings.Contains(got, tt.input) {
				t.Errorf("RedactAndTruncate(%q) did not redact secret, got: %q", tt.input, got)
			}
			// Must contain a redaction marker.
			if !strings.Contains(got, "[secret redacted]") {
				t.Errorf("RedactAndTruncate(%q) missing redaction marker, got: %q", tt.input, got)
			}
		})
	}
}
