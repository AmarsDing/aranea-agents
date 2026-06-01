package cli_admin

import (
	"net"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"private 10.x", "10.0.0.1", true},
		{"private 10.255.255.255", "10.255.255.255", true},
		{"private 172.16.x", "172.16.0.1", true},
		{"private 172.31.x", "172.31.255.255", true},
		{"public 172.15.x", "172.15.0.1", false},
		{"public 172.32.x", "172.32.0.1", false},
		{"private 192.168.x", "192.168.0.1", true},
		{"private 192.168.255.255", "192.168.255.255", true},
		{"loopback 127.0.0.1", "127.0.0.1", true},
		{"link-local 169.254.x", "169.254.0.1", true},
		{"public 8.8.8.8", "8.8.8.8", false},
		{"public 1.1.1.1", "1.1.1.1", false},
		{"unspecified 0.0.0.0", "0.0.0.0", true},
		{"ipv6 loopback ::1", "::1", true},
		{"ipv6 private fc00::", "fc00::1", true},
		{"ipv6 link-local fe80::", "fe80::1", true},
		{"ipv6 public 2001:db8::1", "2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %q", tt.ip)
			}
			got := IsPrivateIP(ip)
			if got != tt.want {
				t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

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

func TestValidateRepoURL_EmptyURL(t *testing.T) {
	err := ValidateRepoURL("")
	if err != nil {
		t.Fatalf("expected no error for empty URL, got %v", err)
	}
}

func TestValidateRepoURL_DisallowedScheme(t *testing.T) {
	err := ValidateRepoURL("ftp://example.com/repo")
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
}
