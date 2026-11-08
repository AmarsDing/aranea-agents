// TokenProvider 单测(P2-6 验收:token 源)。httptest 模拟 NT/HCF 两种登录协议,
// 覆盖:登录体形状、token 缓存、401 Invalidate 重登、失败冷却不刷屏、Status 展示口径。
package adapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// loginServer 脚本化登录端:校验请求体形状后按 kind 应答 token。
func loginServer(t *testing.T, kind string, token string, calls *atomic.Int32, failFirst int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		var doc map[string]string
		if err := json.Unmarshal(raw, &doc); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch kind {
		case "nt-login":
			if doc["user_name"] != "u" || doc["password"] != "p" {
				t.Errorf("nt-login 体形状错误: %s", raw)
			}
		case "hcf-login":
			if doc["grant_type"] != "password" || doc["username"] != "u" || doc["password"] != "p" {
				t.Errorf("hcf-login 体形状错误: %s", raw)
			}
		}
		if int(n) <= failFirst {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if kind == "hcf-login" {
			_, _ = w.Write([]byte(`{"access_token":"` + token + `"}`))
			return
		}
		_, _ = w.Write([]byte(`{"token":"` + token + `"}`))
	}))
}

func TestTokenProvider_NTLoginCachesToken(t *testing.T) {
	var calls atomic.Int32
	srv := loginServer(t, "nt-login", "tok-nt", &calls, 0)
	defer srv.Close()

	tp, err := NewTokenProvider("nt-login", srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	h, err := tp.Header(context.Background())
	if err != nil || h != "Bearer tok-nt" {
		t.Fatalf("首次登录: h=%q err=%v", h, err)
	}
	// 缓存期内第二次不再请求
	if _, err := tp.Header(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("缓存期内不应重复登录,实际 %d 次", calls.Load())
	}
	loggedIn, failCount, lastErr := tp.Status()
	if !loggedIn || failCount != 0 || lastErr != "" {
		t.Fatalf("Status 应为已登录无失败: %v %d %q", loggedIn, failCount, lastErr)
	}
}

func TestTokenProvider_HCFLoginAccessToken(t *testing.T) {
	var calls atomic.Int32
	srv := loginServer(t, "hcf-login", "tok-hcf", &calls, 0)
	defer srv.Close()

	tp, err := NewTokenProvider("hcf-login", srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	h, err := tp.Header(context.Background())
	if err != nil || h != "Bearer tok-hcf" {
		t.Fatalf("hcf-login 应取 access_token: h=%q err=%v", h, err)
	}
}

func TestTokenProvider_InvalidateForcesRelogin(t *testing.T) {
	var calls atomic.Int32
	srv := loginServer(t, "nt-login", "tok-2", &calls, 0)
	defer srv.Close()

	tp, _ := NewTokenProvider("nt-login", srv.URL, "u", "p")
	if _, err := tp.Header(context.Background()); err != nil {
		t.Fatal(err)
	}
	tp.Invalidate() // 模拟请求方收到 401
	if _, err := tp.Header(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("Invalidate 后应强制重登,实际 %d 次", calls.Load())
	}
}

func TestTokenProvider_FailureCooldownSuppressesRetry(t *testing.T) {
	var calls atomic.Int32
	srv := loginServer(t, "nt-login", "tok", &calls, 99) // 永远 401
	defer srv.Close()

	tp, _ := NewTokenProvider("nt-login", srv.URL, "u", "p")
	if _, err := tp.Header(context.Background()); err == nil {
		t.Fatal("登录被拒应返回错误")
	}
	// 冷却期内第二次直接返回上次错误,不打扰源
	if _, err := tp.Header(context.Background()); err == nil {
		t.Fatal("冷却期应返回错误")
	}
	if calls.Load() != 1 {
		t.Fatalf("冷却期内不应重试刷屏,实际 %d 次", calls.Load())
	}
	loggedIn, failCount, lastErr := tp.Status()
	if loggedIn || failCount != 1 || lastErr == "" {
		t.Fatalf("Status 应记录失败: %v %d %q", loggedIn, failCount, lastErr)
	}
}

func TestTokenProvider_Validation(t *testing.T) {
	if _, err := NewTokenProvider("nt-login", "", "u", "p"); err == nil {
		t.Fatal("缺 login_url 应报错")
	}
	if _, err := NewTokenProvider("nt-login", "http://x", "", "p"); err == nil {
		t.Fatal("缺 user 应报错")
	}
	if _, err := NewTokenProvider("nt-login", "http://x", "u", ""); err == nil {
		t.Fatal("缺 pass 应报错")
	}
	if _, err := NewTokenProvider("oauth2", "http://x", "u", "p"); err == nil {
		t.Fatal("未知类型应报错")
	}
	if tp, err := NewTokenProvider("", "http://x", "u", "p"); err != nil || tp.Type != "nt-login" {
		t.Fatalf("空类型应默认 nt-login: %v %v", tp, err)
	}
}

// Poller 集成:挂接 token 源后请求自动带 Bearer;401 → Invalidate → 重登重试一次成功。
func TestPoller_AuthRetryOn401(t *testing.T) {
	var loginCalls, apiCalls atomic.Int32
	loginSrv := loginServer(t, "nt-login", "fresh-tok", &loginCalls, 0)
	defer loginSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := apiCalls.Add(1)
		auth := r.Header.Get("Authorization")
		if auth != "Bearer fresh-tok" {
			t.Errorf("请求应带 Bearer fresh-tok,实际 %q", auth)
		}
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized) // 首次 401 → Invalidate → 重登重试
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"state":"queued"}]}`))
	}))
	defer apiSrv.Close()

	tp, err := NewTokenProvider("nt-login", loginSrv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	// 预登录建立缓存 token(首次轮询携带;401 后 Invalidate 强制重登)
	if _, err := tp.Header(context.Background()); err != nil {
		t.Fatal(err)
	}

	p := NewPoller(PollerConfig{QPSPerHost: 1000, Timeout: time.Second})
	host := hostOf(apiSrv.URL)
	p.SetAuthProvider(host, tp)

	st := p.PollOnce(context.Background(), "k", "GET", apiSrv.URL)
	if st.LastErr != "" || st.FailCount != 0 {
		t.Fatalf("401 重试后应成功: %+v", st)
	}
	if len(st.Items) != 1 {
		t.Fatalf("应归一出 1 条 item: %+v", st.Items)
	}
	if loginCalls.Load() != 2 {
		t.Fatalf("401 应触发一次重登,实际登录 %d 次", loginCalls.Load())
	}
	ok, loggedIn, failCount, _ := p.AuthStatus(host)
	if !ok || !loggedIn || failCount != 0 {
		t.Fatalf("AuthStatus 应为挂接且已登录: %v %v %d", ok, loggedIn, failCount)
	}
	if ok, _, _, _ := p.AuthStatus("127.0.0.1:1"); ok {
		t.Fatal("未挂接 host 应 ok=false")
	}
}
