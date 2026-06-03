package probe

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"aranea-agents/internal/mcp"
	"aranea-agents/internal/mcp/config"
	"aranea-agents/pkg/outboundguard"
)

type TestResult struct {
	OK      bool           `json:"ok"`
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

var defaultProber = NewProber(nil)

func Evaluate(ctx context.Context, enabled bool, configJSON string) TestResult {
	return defaultProber.Evaluate(ctx, enabled, configJSON)
}

func evaluateStdio(cfg config.ServerConfig) TestResult {
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return TestResult{OK: false, Status: "error", Message: "stdio 传输需要填写 command"}
	}
	if _, err := exec.LookPath(command); err != nil {
		return TestResult{OK: false, Status: "error", Message: "command 不可执行或不在 PATH 中: " + err.Error()}
	}
	return TestResult{
		OK:      true,
		Status:  "ok",
		Message: "stdio 命令校验通过，未在测试中启动子进程",
		Details: map[string]any{"command": command, "args": cfg.Args},
	}
}

func evaluateHTTP(cfg config.ServerConfig) TestResult {
	rawURL, timeout, errResult := validateHTTPConfig(cfg)
	if errResult != nil {
		return *errResult
	}
	return doHTTPProbe(rawURL, cfg.Headers, outboundguard.NewClient(timeout))
}

func validateHTTPConfig(cfg config.ServerConfig) (string, time.Duration, *TestResult) {
	rawURL := strings.TrimSpace(cfg.URL)
	if rawURL == "" {
		return "", 0, &TestResult{OK: false, Status: "error", Message: "HTTP 传输需要填写 URL"}
	}
	if err := outboundguard.ValidateURL(rawURL); err != nil {
		return "", 0, &TestResult{OK: false, Status: "error", Message: "URL 校验失败: " + err.Error()}
	}
	timeout := config.DurationSec(cfg.TimeoutSec)
	if timeout <= 0 || timeout > time.Duration(mcp.DefaultProbeTimeoutSec)*time.Second {
		timeout = time.Duration(mcp.DefaultProbeTimeoutSec) * time.Second
	}
	return rawURL, timeout, nil
}

func doHTTPProbe(rawURL string, headers map[string]string, client *http.Client) TestResult {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return TestResult{OK: false, Status: "error", Message: "创建测试请求失败: " + err.Error()}
	}
	for key, value := range headers {
		if strings.TrimSpace(key) != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return TestResult{OK: false, Status: "error", Message: "连接失败: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return TestResult{
			OK:      true,
			Status:  "ok",
			Message: "连接测试成功",
			Details: map[string]any{"status_code": resp.StatusCode},
		}
	}
	// ConnectivityProbe: 401/403 means network is reachable but auth is needed.
	// OK=true because the probe only validates network connectivity, not auth.
	// AuthAwareProbe returns OK=false for the same status (auth failure).
	// The health runner handles this difference: isHardFailure excludes auth_required.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return TestResult{
			OK:      true,
			Status:  "auth_required",
			Message: fmt.Sprintf("连接成功，服务器要求鉴权（HTTP %d）；探针仅校验网络连通性，运行时将使用配置的凭据", resp.StatusCode),
			Details: map[string]any{"status_code": resp.StatusCode},
		}
	}
	return TestResult{
		OK:      false,
		Status:  "error",
		Message: fmt.Sprintf("连接返回非成功状态: HTTP %d", resp.StatusCode),
		Details: map[string]any{"status_code": resp.StatusCode},
	}
}
