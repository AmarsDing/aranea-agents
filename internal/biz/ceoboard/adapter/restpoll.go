// REST 轮询器(自 ceo-dashboard/internal/adapter 逐字移植)。
// 统一限流(每 host QPS 池)+ 超时 + 连续失败断路 + last_ok_at 陈旧判定;
// 可选挂接登录型 token 源(NT/HCF admin 网关),401 时 Invalidate 并重试一次。
// PollState/CircuitOpen 运行态视图在 types.go(agg 依赖面)。
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// PollerConfig 是轮询器配置。
type PollerConfig struct {
	QPSPerHost    float64       // 每平台(按 host)回源 QPS 池上限,默认 5
	Timeout       time.Duration // 单次请求超时
	FailThreshold int           // 连续失败多少次断开断路器,默认 3
	OpenDuration  time.Duration // 断路器断开时长,默认 60s
	MaxBodyBytes  int64         // 响应体上限
}

// Poller 是统一的 REST 轮询器:QPS 池 + 超时 + 断路器 + last_ok_at。
type Poller struct {
	cfg       PollerConfig
	client    *http.Client
	mu        sync.Mutex
	states    map[string]*PollState
	nextAllow map[string]time.Time // host 维度 QPS 池

	authMu     sync.Mutex
	authByHost map[string]*TokenProvider // host:port → 登录型 token 源
}

// SetAuthProvider 为某 host:port 挂接 Bearer token 源;nil 表示移除。
// 命中规则的请求自动带 Authorization;收到 401 时 Invalidate 并重试一次。
func (p *Poller) SetAuthProvider(hostPort string, tp *TokenProvider) {
	p.authMu.Lock()
	defer p.authMu.Unlock()
	if tp == nil {
		delete(p.authByHost, hostPort)
		return
	}
	p.authByHost[hostPort] = tp
}

// AuthStatus 返回某 host 的 token 源状态(展示用);未挂接返回 ok=false。
func (p *Poller) AuthStatus(hostPort string) (ok, loggedIn bool, failCount int, lastErr string) {
	p.authMu.Lock()
	tp := p.authByHost[hostPort]
	p.authMu.Unlock()
	if tp == nil {
		return false, false, 0, ""
	}
	loggedIn, failCount, lastErr = tp.Status()
	return true, loggedIn, failCount, lastErr
}

func (p *Poller) authFor(rawURL string) *TokenProvider {
	p.authMu.Lock()
	defer p.authMu.Unlock()
	return p.authByHost[hostOf(rawURL)]
}

// NewPoller 构造轮询器。
func NewPoller(cfg PollerConfig) *Poller {
	if cfg.QPSPerHost <= 0 {
		cfg.QPSPerHost = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.FailThreshold <= 0 {
		cfg.FailThreshold = 3
	}
	if cfg.OpenDuration <= 0 {
		cfg.OpenDuration = 60 * time.Second
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
	}
	return &Poller{
		cfg:        cfg,
		client:     &http.Client{Timeout: cfg.Timeout},
		states:     map[string]*PollState{},
		nextAllow:  map[string]time.Time{},
		authByHost: map[string]*TokenProvider{},
	}
}

// Lookup 读取某目标当前状态(不存在返回 nil)。
func (p *Poller) Lookup(key string) *PollState {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.states[key]; ok {
		cp := *s
		return &cp
	}
	return nil
}

// waitTurn 按 host 维度限速(最小间隔 = 1/QPS)。
func (p *Poller) waitTurn(rawURL string) {
	host := hostOf(rawURL)
	p.mu.Lock()
	now := time.Now()
	na := p.nextAllow[host]
	if na.Before(now) {
		na = now
	}
	p.nextAllow[host] = na.Add(time.Duration(float64(time.Second) / p.cfg.QPSPerHost))
	p.mu.Unlock()
	if d := time.Until(na); d > 0 {
		time.Sleep(d)
	}
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "_unknown"
	}
	return u.Host
}

// PollOnce 对某目标执行一次轮询并更新状态。断路器断开期间直接跳过(不消耗源)。
// 返回值是锁内快照(评审 2026-11-09 L3):此前返回内部共享指针——断路判定
// 无锁读 OpenUntil,且调用方持活指针可与 record 的写并发交错(读脏/改写
// 串话);多协程同 key 轮询时该面被并发放大。快照保证各节点/各调用方互不
// 串话,内部状态只能经 record 在锁内推进。
func (p *Poller) PollOnce(ctx context.Context, key, method, rawURL string) *PollState {
	p.mu.Lock()
	st, ok := p.states[key]
	if !ok {
		st = &PollState{Key: key, URL: rawURL}
		p.states[key] = st
	}
	if st.CircuitOpen(time.Now()) {
		cp := *st
		p.mu.Unlock()
		return &cp // 断路中:不打扰源,保持陈旧标记
	}
	p.mu.Unlock()

	p.waitTurn(rawURL)
	start := time.Now()
	body, status, err := p.doRequest(ctx, method, rawURL)
	if err != nil {
		p.record(key, st, false, start, err, nil, "")
		return p.Lookup(key)
	}
	// 401 且挂了 token 源:作废缓存 token,重登后重试一次(仍失败按普通失败记账)
	if status == http.StatusUnauthorized {
		if tp := p.authFor(rawURL); tp != nil {
			tp.Invalidate()
			body, status, err = p.doRequest(ctx, method, rawURL)
		}
	}
	if err != nil {
		p.record(key, st, false, start, err, nil, "")
		return p.Lookup(key)
	}
	if status >= 400 {
		p.record(key, st, false, start, fmt.Errorf("HTTP %d", status), nil, snippet(body))
		return p.Lookup(key)
	}
	items := extractItems(body)
	p.record(key, st, true, start, nil, items, snippet(body))
	return p.Lookup(key)
}

// doRequest 发一次请求(命中 auth 规则时带 Bearer),返回 (body, status, err)。
func (p *Poller) doRequest(ctx context.Context, method, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("构造请求失败: %w", err)
	}
	if tp := p.authFor(rawURL); tp != nil {
		h, err := tp.Header(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("凭据获取失败: %w", err)
		}
		req.Header.Set("Authorization", h)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, p.cfg.MaxBodyBytes))
	return body, resp.StatusCode, nil
}

func snippet(b []byte) string {
	const n = 300
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}

// extractItems 把响应归一为列表:{items:[...]} / 顶层数组 / 单对象包一层。
func extractItems(body []byte) []map[string]any {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil
	}
	switch t := v.(type) {
	case []any:
		return toMapSlice(t)
	case map[string]any:
		if items, ok := t["items"].([]any); ok {
			return toMapSlice(items)
		}
		return []map[string]any{t}
	}
	return nil
}

func toMapSlice(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func (p *Poller) record(key string, st *PollState, ok bool, start time.Time, err error, items []map[string]any, raw string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if ok {
		st.LastOKAt = now
		st.AsOf = now
		st.LatencyMs = time.Since(start).Milliseconds()
		st.FailCount = 0
		st.LastErr = ""
		st.Items = items
		st.RawSnippet = raw
	} else {
		st.LastErrAt = now
		st.LatencyMs = time.Since(start).Milliseconds()
		st.FailCount++
		if err != nil {
			st.LastErr = err.Error()
		}
		st.RawSnippet = raw
		if st.FailCount >= p.cfg.FailThreshold {
			st.OpenUntil = now.Add(p.cfg.OpenDuration) // 连续失败 → 断路,防止拖垮源
		}
	}
	_ = key
}

// RunLoop 以固定间隔轮询某目标,直到 ctx 取消。每个目标独立协程(隔离)。
func (p *Poller) RunLoop(ctx context.Context, key, method, rawURL string, interval time.Duration) {
	p.PollOnce(ctx, key, method, rawURL)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.PollOnce(ctx, key, method, rawURL)
		}
	}
}
