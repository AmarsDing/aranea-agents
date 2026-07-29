package service

import (
	"net/http/httptest"
	"testing"
)

func TestAuditMetaFromRequest_ForwardedFor(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/agents", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	r.Header.Set("User-Agent", "test-agent/1.0")
	r.Header.Set("X-Request-Id", "rid-1")

	ip, ua, rid := auditMetaFromRequest(r)
	if ip != "1.2.3.4" {
		t.Errorf("ip = %q, want first X-Forwarded-For hop %q", ip, "1.2.3.4")
	}
	if ua != "test-agent/1.0" {
		t.Errorf("ua = %q, want %q", ua, "test-agent/1.0")
	}
	if rid != "rid-1" {
		t.Errorf("rid = %q, want %q", rid, "rid-1")
	}
}

func TestAuditMetaFromRequest_RealIPFallback(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/agents", nil)
	r.Header.Set("X-Real-IP", "9.9.9.9")

	ip, _, _ := auditMetaFromRequest(r)
	if ip != "9.9.9.9" {
		t.Errorf("ip = %q, want %q", ip, "9.9.9.9")
	}
}

func TestAuditMetaFromRequest_RemoteAddrFallback(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/agents", nil)
	r.RemoteAddr = "192.168.1.10:54321"

	ip, _, _ := auditMetaFromRequest(r)
	if ip != "192.168.1.10" {
		t.Errorf("ip = %q, want host part %q", ip, "192.168.1.10")
	}
}

func TestAuditMetaFromRequest_Empty(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/agents", nil)
	r.RemoteAddr = ""

	ip, ua, rid := auditMetaFromRequest(r)
	if ip != "" || ua != "" || rid != "" {
		t.Errorf("unexpected meta: ip=%q ua=%q rid=%q", ip, ua, rid)
	}
}
