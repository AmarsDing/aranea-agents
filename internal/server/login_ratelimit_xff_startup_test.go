package server

import (
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// warnCaptureLogger 捕获 Warn 调用次数的测试 Logger（loggateway.Logger 最小实现）。
type warnCaptureLogger struct {
	mu       sync.Mutex
	warns    int
	lastWarn string
}

func (c *warnCaptureLogger) Debug(string, ...loggateway.Field) {}
func (c *warnCaptureLogger) Info(string, ...loggateway.Field)  {}
func (c *warnCaptureLogger) Error(string, ...loggateway.Field) {}
func (c *warnCaptureLogger) Warn(msg string, _ ...loggateway.Field) {
	c.mu.Lock()
	c.warns++
	c.lastWarn = msg
	c.mu.Unlock()
}
func (c *warnCaptureLogger) With(...loggateway.Field) loggateway.Logger { return c }

func (c *warnCaptureLogger) warnCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.warns
}

// V 桶 minor（P0 XFF 门默认配置兑现）pin：未配置 KRATOS_TRUSTED_PROXY_CIDRS
// 时启动期必须 Warn——离机 frps/反代拓扑下 XFF 被忽略，登录限流坍缩为代理
// 单桶（暴破即全员锁死），静默默认曾让该风险不可见。
func TestWarnXFFTrustedProxyStartup(t *testing.T) {
	t.Run("unset env warns", func(t *testing.T) {
		t.Setenv("KRATOS_TRUSTED_PROXY_CIDRS", "")
		lg := &warnCaptureLogger{}
		warnXFFTrustedProxyStartup(lg)
		if lg.warnCount() != 1 {
			t.Fatalf("warns = %d, want 1 (unset allowlist must surface at startup)", lg.warnCount())
		}
	})

	t.Run("configured env stays silent", func(t *testing.T) {
		t.Setenv("KRATOS_TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 192.168.1.10")
		lg := &warnCaptureLogger{}
		warnXFFTrustedProxyStartup(lg)
		if lg.warnCount() != 0 {
			t.Fatalf("warns = %d, want 0 when allowlist configured", lg.warnCount())
		}
	})

	t.Run("whitespace-only env still warns", func(t *testing.T) {
		t.Setenv("KRATOS_TRUSTED_PROXY_CIDRS", "   ")
		lg := &warnCaptureLogger{}
		warnXFFTrustedProxyStartup(lg)
		if lg.warnCount() != 1 {
			t.Fatalf("warns = %d, want 1 (whitespace-only is not configured)", lg.warnCount())
		}
	})
}
