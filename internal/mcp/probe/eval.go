package probe

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type TestResult struct {
	OK      bool           `json:"ok"`
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type serverConfig struct {
	Transport              string            `json:"transport"`
	URL                    string            `json:"url"`
	Command                string            `json:"command"`
	Args                   []string          `json:"args"`
	Headers                map[string]string `json:"headers"`
	Env                    map[string]string `json:"env"`
	ToolPrefix             string            `json:"tool_prefix"`
	TimeoutSec             int               `json:"timeout_sec"`
	RequireUserCredentials bool              `json:"require_user_credentials"`
}

func defaultJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

func Evaluate(enabled bool, configJSON string) TestResult {
	if !enabled {
		return TestResult{OK: false, Status: "unknown", Message: "MCP 服务器已停用，未执行连接测试"}
	}
	var cfg serverConfig
	if err := json.Unmarshal([]byte(defaultJSON(configJSON)), &cfg); err != nil {
		return TestResult{OK: false, Status: "error", Message: "config_json 格式错误: " + err.Error()}
	}
	switch cfg.Transport {
	case "stdio":
		return evaluateStdio(cfg)
	case "sse", "streamable_http":
		return evaluateHTTP(cfg)
	default:
		return TestResult{OK: false, Status: "error", Message: "transport 必须是 stdio、sse 或 streamable_http"}
	}
}

func evaluateStdio(cfg serverConfig) TestResult {
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

func evaluateHTTP(cfg serverConfig) TestResult {
	rawURL := strings.TrimSpace(cfg.URL)
	if rawURL == "" {
		return TestResult{OK: false, Status: "error", Message: "HTTP 传输需要填写 URL"}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return TestResult{OK: false, Status: "error", Message: "URL 格式错误"}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return TestResult{OK: false, Status: "error", Message: "URL 仅支持 http 或 https"}
	}
	if err := validatePublicHost(parsed.Hostname()); err != nil {
		return TestResult{OK: false, Status: "error", Message: "URL 校验失败: " + err.Error()}
	}

	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	client := http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return TestResult{OK: false, Status: "error", Message: "创建测试请求失败: " + err.Error()}
	}
	for key, value := range cfg.Headers {
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
	return TestResult{
		OK:      false,
		Status:  "error",
		Message: fmt.Sprintf("连接返回非成功状态: HTTP %d", resp.StatusCode),
		Details: map[string]any{"status_code": resp.StatusCode},
	}
}

func validatePublicHost(host string) error {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("localhost is not allowed")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("private or local address is not allowed")
		}
	}
	return nil
}
