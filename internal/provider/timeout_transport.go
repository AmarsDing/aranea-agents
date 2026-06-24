package provider

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

// timeoutTransport 在 transport 层为请求添加超时控制。
//
// 背景：wire.go 通过 http.Client.Timeout 设置超时，但 buildProviderOptions
// 只提取 rt.HTTP.Transport（通常为 nil），框架 DefaultNewHTTPClient 创建新
// 客户端时不设置 Timeout，导致超时丢失。本 wrapper 通过 context.WithTimeout
// 在 RoundTrip 入口注入超时，确保无论框架如何创建客户端，超时都生效。
//
// 与 http.Client.Timeout 的差异：
//   - http.Client.Timeout 覆盖整个请求生命周期（含连接、TLS、body 读取）
//   - timeoutTransport 通过 context deadline 控制，效果近似，但仅在 transport
//     层生效（不含连接池获取阶段，实际差异可忽略）
//
// 与 retryTransport/circuitBreakerTransport 的交互：
//   - 传输链执行顺序（从外到内）：circuit-breaker → retry → rate-limit → timeoutTransport → base
//   - timeout 在最内层，确保每次重试都受超时约束
//   - 组装顺序（buildProviderOptions 中）：timeout → rate-limit → retry → circuit-breaker
//     （每层包裹前一层，最终 circuit-breaker 在最外层）
type timeoutTransport struct {
	base    http.RoundTripper
	timeout time.Duration
}

// newTimeoutTransport 包装 base transport，为每个请求添加超时。
// timeout <= 0 时直接返回 base（无超时）。
func newTimeoutTransport(base http.RoundTripper, timeout time.Duration) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if timeout <= 0 {
		return base
	}
	return &timeoutTransport{base: base, timeout: timeout}
}

func (t *timeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), t.timeout)
	resp, err := t.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.Body == nil {
		cancel()
		return resp, nil
	}
	// http.Client.Timeout covers the entire request including response body
	// reads. A naive defer cancel() here would cancel the context as soon as
	// RoundTrip returns (after response headers), breaking streaming bodies.
	// Delay cancellation until the body is fully read/closed to match the
	// expected lifecycle.
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// cancelOnClose wraps an io.ReadCloser and cancels the associated context once
// the body is closed.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.once.Do(c.cancel)
	return err
}
