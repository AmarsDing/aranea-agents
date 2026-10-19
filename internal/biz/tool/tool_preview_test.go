package tool

import (
	"strings"
	"testing"
)

func TestRedactToolPreview_masksSecrets(t *testing.T) {
	in := `{"api_key":"sk-live-secret","email":"a@b.com"}`
	out := RedactToolPreview(in, 2000)
	if strings.Contains(out, "sk-live-secret") || strings.Contains(out, "a@b.com") {
		t.Fatalf("expected redaction, got %q", out)
	}
}

func TestRedactToolPreview_masksJWT(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	out := RedactToolPreview("Authorization: Bearer "+jwt, 2000)
	if strings.Contains(out, jwt) || strings.Contains(out, "eyJhbGci") {
		t.Fatalf("expected JWT redacted, got %q", out)
	}
}

func TestRedactToolPreview_masksAWSAndVendorTokens(t *testing.T) {
	cases := []string{
		"aws key AKIAIOSFODNN7EXAMPLE here",
		"token ASIAIOSFODNN7EXAMPLE here",
		"key sk-abcdefghijklmnopqrstuvwxyz0123 end",
		"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 done",
		"github_pat_11ABCDEFGH_abcdefghijklmnopqrstuvwxyz0123456789 end",
		"glpat-ABCDEFGHIJKLMNOPQRSTuv end",
		"hf_ABCDEFGHIJKLMNOPQRSTUVWXYZ12 end",
	}
	for _, in := range cases {
		out := RedactToolPreview(in, 2000)
		// A4 单源化后标记统一为 preview 包的 "[secret redacted]"。
		if !strings.Contains(out, "[secret redacted]") {
			t.Fatalf("expected token redaction for %q, got %q", in, out)
		}
	}
}

func TestRedactToolPreview_masksPEM(t *testing.T) {
	full := "cfg -----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA7\n-----END RSA PRIVATE KEY----- end"
	out := RedactToolPreview(full, 2000)
	if strings.Contains(out, "MIIEpAIBAAKCAQEA7") || strings.Contains(out, "PRIVATE KEY") {
		t.Fatalf("expected PEM block redacted, got %q", out)
	}
	// 截断预览缺少 END 行时，BEGIN 行也要被兜底脱敏。
	truncated := "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA"
	out = RedactToolPreview(truncated, 2000)
	if strings.Contains(out, "PRIVATE KEY") {
		t.Fatalf("expected truncated PEM head redacted, got %q", out)
	}
}

func TestRedactToolPreview_phone(t *testing.T) {
	// 带前缀或分隔符的号码应脱敏。
	phones := []string{
		"call +86 138 0000 0000 now",
		"tel 138-0000-0000",
		"code 010-1234-5678 x",
		"+8613800138000",
	}
	for _, in := range phones {
		out := RedactToolPreview(in, 2000)
		if !strings.Contains(out, "[phone redacted]") {
			t.Fatalf("expected phone redaction for %q, got %q", in, out)
		}
	}
	// 裸 10/13 位时间戳必须保留。
	timestamps := []string{"1700000000", "1700000000000"}
	for _, ts := range timestamps {
		out := RedactToolPreview("event ts="+ts+" ok", 2000)
		if !strings.Contains(out, ts) {
			t.Fatalf("timestamp %s must survive redaction, got %q", ts, out)
		}
	}
}
