// NT 风格"登录签发 Bearer token"的凭据提供器(自 ceo-dashboard/internal/adapter 逐字移植)。
// 背景:NovaTrader 对外 API 统一经 admin 网关(192.168.4.44:22001),除 /api/v1/login 外
// 全部要求 Authorization: Bearer <token>(gate.go:333-344);token 由 POST /api/v1/login
// 签发(auth.go:39-75),设计文档 M11 记载 7 天有效。看板只发 GET(只读),凭据密码走环境
// 变量,不入版本库。
// 行为:首次用凭据登录拿 token 并缓存(保守按 24h 上限强制重登,NT 侧实际 7 天);
// 请求方收到 401 时调 Invalidate() 使下次强制重新登录;登录失败不缓存、不重试刷屏
// (冷却 30s,失败计数供状态展示)。
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// TokenProvider 是一个"登录→缓存→自动续期"的 Bearer token 源。
// Type 支持两种登录协议:
//   - nt-login :POST {user_name,password} → {token}          (NovaTrader admin 网关)
//   - hcf-login:POST {grant_type:"password",username,password} → {access_token} (HotCraftFlow admin)
type TokenProvider struct {
	Type     string // nt-login(默认)| hcf-login
	LoginURL string // 例: http://192.168.4.44:22001/api/v1/login
	User     string
	Pass     string // 来自环境变量(配置里只写 pass_env 名)

	client *http.Client

	mu        sync.Mutex
	token     string
	fetchedAt time.Time
	failCount int
	lastErr   string
	nextTryAt time.Time // 登录失败冷却
}

// NewTokenProvider 构造;pass 为空时返回错误(调用方应跳过装配并记状态)。
func NewTokenProvider(typ, loginURL, user, pass string) (*TokenProvider, error) {
	if loginURL == "" || user == "" || pass == "" {
		return nil, fmt.Errorf("token provider 缺 login_url/user/pass")
	}
	if typ == "" {
		typ = "nt-login"
	}
	if typ != "nt-login" && typ != "hcf-login" {
		return nil, fmt.Errorf("未知登录类型 %q", typ)
	}
	return &TokenProvider{Type: typ, LoginURL: loginURL, User: user, Pass: pass, client: &http.Client{Timeout: 10 * time.Second}}, nil
}

const (
	tokenMaxAge    = 24 * time.Hour // 保守上限;NT 实际 7 天,提前换
	loginCooldown  = 30 * time.Second
	loginBodyLimit = 1 << 20
)

// Header 返回可用的 Authorization 头值(Bearer xxx);失败返回错误(冷却期内直接返回上次错误)。
func (t *TokenProvider) Header(ctx context.Context) (string, error) {
	t.mu.Lock()
	if t.token != "" && time.Since(t.fetchedAt) < tokenMaxAge {
		tok := t.token
		t.mu.Unlock()
		return "Bearer " + tok, nil
	}
	if time.Now().Before(t.nextTryAt) {
		err := t.lastErr
		t.mu.Unlock()
		return "", fmt.Errorf("登录冷却中(上次失败: %s)", err)
	}
	t.mu.Unlock()
	return t.login(ctx)
}

// Invalidate 使缓存 token 失效(收到 401 时调用),下次 Header 强制重新登录。
func (t *TokenProvider) Invalidate() {
	t.mu.Lock()
	t.token = ""
	t.mu.Unlock()
}

// Status 返回展示用状态(不含敏感信息)。
func (t *TokenProvider) Status() (loggedIn bool, failCount int, lastErr string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.token != "", t.failCount, t.lastErr
}

func (t *TokenProvider) login(ctx context.Context) (string, error) {
	var body []byte
	if t.Type == "hcf-login" {
		body, _ = json.Marshal(map[string]string{"grant_type": "password", "username": t.User, "password": t.Pass})
	} else {
		body, _ = json.Marshal(map[string]string{"user_name": t.User, "password": t.Pass})
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.LoginURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		t.recordFail(err.Error())
		return "", fmt.Errorf("登录请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, loginBodyLimit))
	if resp.StatusCode != http.StatusOK {
		t.recordFail(fmt.Sprintf("HTTP %d", resp.StatusCode))
		return "", fmt.Errorf("登录被拒: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.recordFail("响应解析失败")
		return "", fmt.Errorf("登录响应解析失败")
	}
	tok := out.Token
	if tok == "" {
		tok = out.AccessToken
	}
	if tok == "" {
		t.recordFail("响应无 token")
		return "", fmt.Errorf("登录响应解析失败")
	}
	t.mu.Lock()
	t.token = tok
	t.fetchedAt = time.Now()
	t.failCount = 0
	t.lastErr = ""
	t.mu.Unlock()
	return "Bearer " + tok, nil
}

func (t *TokenProvider) recordFail(msg string) {
	t.mu.Lock()
	t.failCount++
	t.lastErr = msg
	t.nextTryAt = time.Now().Add(loginCooldown)
	t.mu.Unlock()
}
