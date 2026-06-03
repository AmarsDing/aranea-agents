package hostexec

import (
	"strings"
	"testing"
)

func TestRedactOutput(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		input    string
		contains string   // output must contain this
		omits    []string // output must NOT contain these
	}{
		{
			name:     "basic env var redaction",
			env:      map[string]string{"API_TOKEN": "supersecret123"},
			input:    "API_TOKEN=supersecret123",
			contains: "[REDACTED]",
			omits:    []string{"supersecret123"},
		},
		{
			name:     "assignment line redaction",
			env:      map[string]string{"MY_SECRET": "mysecretvalue"},
			input:    "export MY_SECRET=mysecretvalue",
			contains: "[REDACTED]",
			omits:    []string{"mysecretvalue"},
		},
		{
			name:     "colon line redaction",
			env:      map[string]string{"DB_PASSWORD": "dbpass123456"},
			input:    `"DB_PASSWORD": "dbpass123456"`,
			contains: "[REDACTED]",
			omits:    []string{"dbpass123456"},
		},
		{
			name:     "empty output",
			env:      map[string]string{"TOKEN": "abc123"},
			input:    "",
			contains: "",
			omits:    nil,
		},
		{
			name:     "whitespace only output",
			env:      map[string]string{"TOKEN": "abc123"},
			input:    "   ",
			contains: "   ",
			omits:    nil,
		},
		{
			name:     "non-sensitive env not redacted",
			env:      map[string]string{"PATH": "/usr/bin"},
			input:    "PATH=/usr/bin",
			contains: "/usr/bin",
			omits:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactOutput(tt.env, tt.input)
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("RedactOutput() = %q, want to contain %q", got, tt.contains)
			}
			for _, omit := range tt.omits {
				if strings.Contains(got, omit) {
					t.Errorf("RedactOutput() = %q, should NOT contain %q", got, omit)
				}
			}
		})
	}
}

func TestRedactSensitiveValues(t *testing.T) {
	t.Run("long value uses word boundary", func(t *testing.T) {
		values := []sensitiveValue{
			{Name: "TOKEN", Value: "longsecretvalue123", AllowShort: false},
		}
		// Value in unrelated context should NOT be replaced (no word boundary)
		output := "prefix_longsecretvalue123_suffix"
		got := redactSensitiveValues(output, values)
		if strings.Contains(got, "[REDACTED:TOKEN]") {
			t.Errorf("long value in unrelated context should not be replaced, got: %q", got)
		}
	})

	t.Run("long value standalone is replaced", func(t *testing.T) {
		values := []sensitiveValue{
			{Name: "TOKEN", Value: "longsecretvalue123", AllowShort: false},
		}
		output := "the token is longsecretvalue123 here"
		got := redactSensitiveValues(output, values)
		if !strings.Contains(got, "[REDACTED:TOKEN]") {
			t.Errorf("standalone long value should be replaced, got: %q", got)
		}
		if strings.Contains(got, "longsecretvalue123") {
			t.Errorf("original value should be replaced, got: %q", got)
		}
	})

	t.Run("short value uses ReplaceAll", func(t *testing.T) {
		values := []sensitiveValue{
			{Name: "PASS", Value: "abc", AllowShort: true},
		}
		output := "password is abc and also prefixabc"
		got := redactSensitiveValues(output, values)
		// Short values with AllowShort=true use ReplaceAll, so both occurrences are replaced
		count := strings.Count(got, "[REDACTED:PASS]")
		if count < 1 {
			t.Errorf("expected at least 1 replacement, got: %q", got)
		}
	})

	t.Run("empty value is skipped", func(t *testing.T) {
		values := []sensitiveValue{
			{Name: "TOKEN", Value: "", AllowShort: false},
		}
		output := "some text"
		got := redactSensitiveValues(output, values)
		if got != output {
			t.Errorf("empty value should be skipped, got: %q", got)
		}
	})
}

func TestReplaceValueWithBoundary(t *testing.T) {
	t.Run("long value word boundary match", func(t *testing.T) {
		output := "token=secretlongvalue123 end"
		got := replaceValueWithBoundary(output, "secretlongvalue123", "[REDACTED]")
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("expected replacement, got: %q", got)
		}
		if strings.Contains(got, "secretlongvalue123") {
			t.Errorf("value should be replaced, got: %q", got)
		}
	})

	t.Run("long value in quoted context", func(t *testing.T) {
		output := `"secretlongvalue123"`
		got := replaceValueWithBoundary(output, "secretlongvalue123", "[REDACTED]")
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("expected replacement in quoted context, got: %q", got)
		}
	})

	t.Run("long value partial match not replaced", func(t *testing.T) {
		output := "prefixsecretlongvalue123suffix"
		got := replaceValueWithBoundary(output, "secretlongvalue123", "[REDACTED]")
		if strings.Contains(got, "[REDACTED]") {
			t.Errorf("partial match should not be replaced, got: %q", got)
		}
	})

	t.Run("short value uses ReplaceAll", func(t *testing.T) {
		output := "abc and abcdef"
		got := replaceValueWithBoundary(output, "abc", "[R]")
		// Short value (< 6 chars) uses ReplaceAll, so both occurrences are replaced
		if strings.Contains(got, "abc") {
			t.Errorf("short value should use ReplaceAll, got: %q", got)
		}
	})
}

func TestIsSensitiveEnvName(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want bool
	}{
		{"TOKEN", "API_TOKEN", true},
		{"SECRET", "MY_SECRET", true},
		{"PASSWORD", "DB_PASSWORD", true},
		{"PASSWD", "SYSTEM_PASSWD", true},
		{"API_KEY", "API_KEY", true},
		{"ACCESS_KEY", "ACCESS_KEY", true},
		{"PRIVATE_KEY", "PRIVATE_KEY", true},
		{"lowercase token", "api_token", true},
		{"mixed case", "Api_Secret", true},
		{"PATH not sensitive", "PATH", false},
		{"HOME not sensitive", "HOME", false},
		{"USER not sensitive", "USER", false},
		{"LANG not sensitive", "LANG", false},
		{"TOKEN in middle", "MY_TOKEN_VALUE", true},
		{"PASSWORD suffix", "DB_PASSWORD", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSensitiveEnvName(tt.env)
			if got != tt.want {
				t.Errorf("isSensitiveEnvName(%q) = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

func TestRedactedStructuredValue(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"double quoted value", `"secret123"`, `"[REDACTED]"`},
		{"single quoted value", `'secret123'`, `'[REDACTED]'`},
		{"unquoted value", `secret123`, `[REDACTED]`},
		{"quoted with trailing comma", `"secret123",`, `"[REDACTED]",`},
		{"unquoted with trailing comma", `secret123,`, `[REDACTED],`},
		{"quoted with trailing space", `"secret123" `, `"[REDACTED]" `},
		{"unquoted with trailing space", `secret123 `, `[REDACTED] `},
		{"empty string", ``, `[REDACTED]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactedStructuredValue(tt.raw)
			if got != tt.want {
				t.Errorf("redactedStructuredValue(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestKnownSensitiveValues(t *testing.T) {
	t.Run("env values with AllowShort bypass length check", func(t *testing.T) {
		env := map[string]string{
			"API_TOKEN": "longsecretvalue", // 15 chars, >= 6
			"MY_SECRET": "abc",             // 3 chars, < 6 but AllowShort=true from env
			"DB_PASSWD": "short",           // 5 chars, < 6 but AllowShort=true from env
		}
		values := knownSensitiveValues(env)

		found := make(map[string]bool)
		for _, v := range values {
			found[v.Name] = true
		}
		// All env values should survive because addSensitiveEnvValues sets AllowShort=true
		if !found["API_TOKEN"] {
			t.Error("expected API_TOKEN in results")
		}
		if !found["MY_SECRET"] {
			t.Error("expected MY_SECRET in results (AllowShort=true from env)")
		}
		if !found["DB_PASSWD"] {
			t.Error("expected DB_PASSWD in results (AllowShort=true from env)")
		}
	})

	t.Run("empty env returns nil", func(t *testing.T) {
		values := knownSensitiveValues(nil)
		if values != nil {
			t.Errorf("expected nil for empty env, got %v", values)
		}
	})

	t.Run("sorted by value length descending", func(t *testing.T) {
		env := map[string]string{
			"TOKEN_A": "mediumsecret",  // 12 chars
			"TOKEN_B": "verylongsecretvalue", // 19 chars
			"TOKEN_C": "shortval",      // 8 chars
		}
		values := knownSensitiveValues(env)
		for i := 1; i < len(values); i++ {
			if len(values[i].Value) > len(values[i-1].Value) {
				t.Errorf("values not sorted by length descending: %v", values)
			}
		}
	})
}
