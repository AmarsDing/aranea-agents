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

func Evaluate(enabled bool, configJSON string) TestResult {
	if !enabled {
		return TestResult{OK: false, Status: "unknown", Message: "MCP 服务器已停用，未执行连接测试"}
	}
	cfg, err := config.ParseServerConfigJSON(configJSON)
	if err != nil {
		return TestResult{OK: false, Status: "error", Message: "config_json 格式错误: " + err.Error()}
	}
	switch cfg.Transport {
	case config.TransportStdio:
		return evaluateStdio(cfg)
	case config.TransportSSE, config.TransportStreamable:
		return evaluateHTTP(cfg)
	default:
		return TestResult{
			OK:      false,
			Status:  "error",
			Message: fmt.Sprintf("transport 必须是 %v（收到 %q）", config.KnownTransports(), cfg.Transport),
		}
	}
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
	rawURL := strings.TrimSpace(cfg.URL)
	if rawURL == "" {
		return TestResult{OK: false, Status: "error", Message: "HTTP 传输需要填写 URL"}
	}
	if err := outboundguard.ValidateURL(rawURL); err != nil {
		return TestResult{OK: false, Status: "error", Message: "URL 校验失败: " + err.Error()}
	}

	timeout := config.DurationSec(cfg.TimeoutSec)
	if timeout <= 0 || timeout > time.Duration(mcp.DefaultProbeTimeoutSec)*time.Second {
		timeout = time.Duration(mcp.DefaultProbeTimeoutSec) * time.Second
	}
	return doHTTPProbe(rawURL, cfg.Headers, outboundguard.NewClient(timeout))
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
