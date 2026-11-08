package probe

import (
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
	return doHTTPProbe(rawURL, cfg.Headers, newProbeClient(timeout))
}

// newProbeClient 探针 HTTP 客户端构造。测试用 httptest.Server 监听 127.0.0.1，
// outboundguard.NewClient 的 SSRF 拦截会拒绝——探针语义是「验证用户配置的
// 目标可达」，目标是否私网由部署方负责（与 validateHTTPConfig 的 URL 校验
// 是两层闸：URL 层挡明显非法，网络层放行由探针自己负责）。
var newProbeClient = newProbeClientImpl

func newProbeClientImpl(timeout time.Duration) *http.Client {
	return outboundguard.NewClient(timeout)
}

// validateHTTPConfigURL 测试可替换的 URL 校验钩子（默认 = outboundguard.ValidateURL）。
var validateHTTPConfigURL = outboundguard.ValidateURL

func validateHTTPConfig(cfg config.ServerConfig) (string, time.Duration, *TestResult) {
	rawURL := strings.TrimSpace(cfg.URL)
	if rawURL == "" {
		return "", 0, &TestResult{OK: false, Status: "error", Message: "HTTP 传输需要填写 URL"}
	}
	if err := validateHTTPConfigURL(rawURL); err != nil {
		return "", 0, &TestResult{OK: false, Status: "error", Message: "URL 校验失败: " + err.Error()}
	}
	timeout := config.DurationSec(cfg.TimeoutSec)
	if timeout <= 0 || timeout > time.Duration(mcp.DefaultProbeTimeoutSec)*time.Second {
		timeout = time.Duration(mcp.DefaultProbeTimeoutSec) * time.Second
	}
	return rawURL, timeout, nil
}

func doHTTPProbe(rawURL string, headers map[string]string, client *http.Client) TestResult {
	return doHTTPProbeWithMethod(rawURL, http.MethodGet, headers, client)
}

// doStreamableInitializeProbe 对 streamable 传输发 POST initialize 探活——
// trpc-mcp-go 等 streamable 服务对裸 GET 回 400（2026-11-03 44 实录），
// 但真实握手 POST initialize 返回 200。与 GET 探针同一出口纪律：
// 2xx/3xx=通，401/403=auth_required，其余=error。
func doStreamableInitializeProbe(rawURL string, headers map[string]string, client *http.Client) TestResult {
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"aranea-probe","version":"1.0"}}}`)
	req, err := http.NewRequest(http.MethodPost, rawURL, body)
	if err != nil {
		return TestResult{OK: false, Status: "error", Message: "创建测试请求失败: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
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
			Message: "连接测试成功（streamable initialize）",
			Details: map[string]any{"status_code": resp.StatusCode, "method": "POST initialize"},
		}
	}
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

func doHTTPProbeWithMethod(rawURL, method string, headers map[string]string, client *http.Client) TestResult {
	req, err := http.NewRequest(method, rawURL, nil)
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
