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
		// Local paths are rejected for security (no arbitrary code execution from local filesystem).
		{"absolute path", "/tmp/repo", true},
		{"relative path dot", "./repo", true},
		{"relative path dotdot", "../repo", true},
		{"windows path", "C:\\repo", true},
		{"file scheme", "file:///tmp/repo", true},
		{"empty scheme", "github.com/user/repo", false},
		{"single letter scheme drive", "C:/repo", true},
		// SSRF bypass: single-char scheme that's not a Windows drive letter
		// Note: X is an alpha char, so X:// is treated as Windows drive letter by current code.
		// This is a known limitation - the code allows any single alpha char as drive letter.
		{"SSRF single-char scheme X (alpha=drive letter)", "X://evil.com/repo", true},
		{"SSRF single-char scheme 1 (non-alpha)", "1:/path", true},
		// Windows drive letters are local paths — rejected for security.
		{"windows drive C", "C:/path", true},
		{"windows drive D", "D:\\repo", true},
		{"windows drive lowercase", "c:/path", true},
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
			err := ValidateRepoURL(tt.url, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRepoURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRepoURL_DisallowedScheme(t *testing.T) {
	err := ValidateRepoURL("ftp://example.com/repo", nil)
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
}

func TestValidateRepoURLPrivateHostWhitelist(t *testing.T) {
	// Private IP hosts are rejected by default (SSRF guard).
	if err := ValidateRepoURL("https://192.168.1.10/org/repo", nil); err == nil {
		t.Fatal("ValidateRepoURL() error = nil, want private host rejection")
	}
	// Exact whitelist match allows the private host.
	if err := ValidateRepoURL("https://192.168.1.10/org/repo", []string{"192.168.1.10"}); err != nil {
		t.Fatalf("ValidateRepoURL() error = %v, want whitelisted host allowed", err)
	}
	// A different private host is still rejected.
	if err := ValidateRepoURL("https://10.0.0.9/org/repo", []string{"192.168.1.10"}); err == nil {
		t.Fatal("ValidateRepoURL() error = nil, want non-whitelisted private host rejected")
	}
	// SCP-style URL with whitelisted host.
	if err := ValidateRepoURL("192.168.1.10/org/repo", []string{"192.168.1.10"}); err != nil {
		t.Fatalf("ValidateRepoURL() error = %v, want whitelisted SCP-style host allowed", err)
	}
	// Hostname match is case-insensitive and trimmed.
	if err := ValidateRepoURL("https://GitLab.CORP.local/org/repo", []string{" gitlab.corp.local "}); err != nil {
		t.Fatalf("ValidateRepoURL() error = %v, want case-insensitive whitelist match", err)
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
