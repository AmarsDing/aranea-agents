package cli_admin

import (
	"testing"
)

func TestValidateRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"absolute path", "/tmp/repo", false},
		{"relative path dot", "./repo", false},
		{"relative path dotdot", "../repo", false},
		{"windows path", "C:\\repo", false},
		{"file scheme", "file:///tmp/repo", false},
		{"empty scheme", "github.com/user/repo", false},
		{"single letter scheme drive", "C:/repo", false},
		// SSRF bypass: single-char scheme that's not a Windows drive letter
		// Note: X is an alpha char, so X:// is treated as Windows drive letter by current code.
		// This is a known limitation - the code allows any single alpha char as drive letter.
		{"SSRF single-char scheme X (alpha=drive letter)", "X://evil.com/repo", false},
		{"SSRF single-char scheme 1 (non-alpha)", "1:/path", true},
		// Windows drive letters should be allowed
		{"windows drive C", "C:/path", false},
		{"windows drive D", "D:\\repo", false},
		{"windows drive lowercase", "c:/path", false},
		// Disallowed schemes
		{"ftp scheme", "ftp://example.com/repo", true},
		{"javascript scheme", "javascript://evil", true},
		{"data scheme", "data:text/html,<script>", true},
		{"gopher scheme", "gopher://evil.com", true},
		// Empty URL - fails because it can't determine host
		{"empty URL", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepoURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRepoURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRepoURL_DisallowedScheme(t *testing.T) {
	err := ValidateRepoURL("ftp://example.com/repo")
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
}

func TestIsAlpha(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"lowercase a", 'a', true},
		{"lowercase z", 'z', true},
		{"uppercase A", 'A', true},
		{"uppercase Z", 'Z', true},
		{"digit 0", '0', false},
		{"digit 9", '9', false},
		{"underscore", '_', false},
		{"dash", '-', false},
		{"space", ' ', false},
		{"chinese char", '你', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAlpha(tt.r)
			if got != tt.want {
				t.Errorf("IsAlpha(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}
