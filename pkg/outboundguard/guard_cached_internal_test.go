package outboundguard

import (
	"errors"
	"net"
	"testing"
	"time"
)

// fakeDNS 是可编程的 DNS lookup 替身：按 host 返回预设结果或错误，
// 并记录调用次数以验证缓存命中行为。
type fakeDNS struct {
	calls int
	ips   []net.IP
	err   error
}

func (f *fakeDNS) lookup(string) ([]net.IP, error) {
	f.calls++
	return f.ips, f.err
}

// 00:52 会话运行时取证：trpc_llm 在每次 agent 构建时对 provider base_url 做
// 实时 DNS 校验；一次瞬时 DNS 失败（lookup api.deepseek.com: no such host）
// 会直接导致整个 team graph build 失败。修复：成功校验按 host 缓存（TTL），
// 缓存命中期间不再重复解析；失败永不缓存。
func TestCachedValidator_CachesSuccessfulLookup(t *testing.T) {
	dns := &fakeDNS{ips: []net.IP{net.ParseIP("39.156.8.67")}}
	v := newCachedValidator(5*time.Minute, dns.lookup)

	if err := v.ValidateURL("https://api.deepseek.com/v1"); err != nil {
		t.Fatalf("first validation should succeed, got %v", err)
	}
	if dns.calls != 1 {
		t.Fatalf("first validation must perform DNS lookup, calls=%d", dns.calls)
	}

	// TTL 内第二次校验：即使 DNS 开始失败，也必须命中缓存直接放行。
	dns.err = errors.New("lookup api.deepseek.com: no such host")
	dns.ips = nil
	if err := v.ValidateURL("https://api.deepseek.com/v1"); err != nil {
		t.Fatalf("cached validation must succeed within TTL despite DNS failure, got %v", err)
	}
	if dns.calls != 1 {
		t.Fatalf("cached validation must not re-resolve DNS within TTL, calls=%d", dns.calls)
	}
}

func TestCachedValidator_NeverCachesFailure(t *testing.T) {
	dns := &fakeDNS{err: errors.New("no such host")}
	v := newCachedValidator(5*time.Minute, dns.lookup)

	if err := v.ValidateURL("https://api.deepseek.com/v1"); err == nil {
		t.Fatal("validation with failing DNS must return error")
	}
	// DNS 恢复后必须立即成功（失败结果不得入缓存）。
	dns.err = nil
	dns.ips = []net.IP{net.ParseIP("39.156.8.67")}
	if err := v.ValidateURL("https://api.deepseek.com/v1"); err != nil {
		t.Fatalf("validation must retry DNS after failure (no negative cache), got %v", err)
	}
	if dns.calls != 2 {
		t.Fatalf("failure must not be cached: expected 2 DNS calls, got %d", dns.calls)
	}
}

func TestCachedValidator_NeverCachesBlockedIP(t *testing.T) {
	// DNS 先返回私网 IP（阻断），后恢复为公网 IP：阻断结果不得入缓存。
	dns := &fakeDNS{ips: []net.IP{net.ParseIP("192.168.1.1")}}
	v := newCachedValidator(5*time.Minute, dns.lookup)

	if err := v.ValidateURL("https://example.com/"); err == nil {
		t.Fatal("private IP resolution must be blocked")
	}
	dns.ips = []net.IP{net.ParseIP("93.184.216.34")}
	if err := v.ValidateURL("https://example.com/"); err != nil {
		t.Fatalf("blocked result must not be cached; revalidation should succeed, got %v", err)
	}
	if dns.calls != 2 {
		t.Fatalf("blocked result must not be cached: expected 2 DNS calls, got %d", dns.calls)
	}
}

func TestCachedValidator_ExpiresAfterTTL(t *testing.T) {
	dns := &fakeDNS{ips: []net.IP{net.ParseIP("39.156.8.67")}}
	v := newCachedValidator(50*time.Millisecond, dns.lookup)

	if err := v.ValidateURL("https://api.deepseek.com/v1"); err != nil {
		t.Fatalf("first validation should succeed, got %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if err := v.ValidateURL("https://api.deepseek.com/v1"); err != nil {
		t.Fatalf("post-TTL revalidation should succeed, got %v", err)
	}
	if dns.calls != 2 {
		t.Fatalf("expired cache entry must trigger fresh DNS lookup, calls=%d", dns.calls)
	}
}

func TestCachedValidator_LiteralIPSkipsCacheAndDNS(t *testing.T) {
	dns := &fakeDNS{err: errors.New("must not be called")}
	v := newCachedValidator(5*time.Minute, dns.lookup)

	if err := v.ValidateURL("https://93.184.216.34/v1"); err != nil {
		t.Fatalf("literal public IP must pass without DNS, got %v", err)
	}
	if err := v.ValidateURL("https://10.0.0.1/v1"); err == nil {
		t.Fatal("literal private IP must be blocked without DNS")
	}
	if dns.calls != 0 {
		t.Fatalf("literal IP path must not touch DNS, calls=%d", dns.calls)
	}
}
