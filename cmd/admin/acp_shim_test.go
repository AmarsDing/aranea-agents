package main

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestParseACPShimArgs_Required 必填校验：缺 --base-url / --model 直接报错
// （子命令启动即失败要比握手期失败更可诊断）。
func TestParseACPShimArgs_Required(t *testing.T) {
	if _, err := parseACPShimArgs(nil, io.Discard); err == nil {
		t.Fatal("expected error when --base-url missing")
	}
	if _, err := parseACPShimArgs([]string{"--base-url=https://api.example.com"}, io.Discard); err == nil {
		t.Fatal("expected error when --model missing")
	}
	cfg, err := parseACPShimArgs([]string{
		"--base-url=https://api.example.com/v1", "--model=code-x",
	}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.baseURL != "https://api.example.com/v1" || cfg.model != "code-x" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	// 默认值钉住（注册表模板与文档以这些默认为准）。
	if cfg.apiKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("default api-key-env = %q, want OPENAI_API_KEY", cfg.apiKeyEnv)
	}
	if cfg.provider != "generic" {
		t.Fatalf("default provider = %q, want generic", cfg.provider)
	}
	if cfg.heartbeat != 30*time.Second {
		t.Fatalf("default heartbeat = %v, want 30s", cfg.heartbeat)
	}
	if cfg.fallbackNonStream {
		t.Fatal("default fallback-non-stream should be false")
	}
}

// TestParseACPShimArgs_Full 全参透传。
func TestParseACPShimArgs_Full(t *testing.T) {
	cfg, err := parseACPShimArgs([]string{
		"--base-url=https://api.deepseek.com", "--model=deepseek-chat",
		"--api-key-env=DEEPSEEK_API_KEY", "--provider=deepseek",
		"--system=你是编程助手", "--heartbeat=0", "--log-level=debug",
		"--fallback-non-stream",
	}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.apiKeyEnv != "DEEPSEEK_API_KEY" || cfg.provider != "deepseek" ||
		cfg.system != "你是编程助手" || cfg.heartbeat != 0 ||
		cfg.logLevel != "debug" || !cfg.fallbackNonStream {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

// TestParseACPShimArgs_BadFlag 未知 flag 报错（flag.ContinueOnError 不退出进程）。
func TestParseACPShimArgs_BadFlag(t *testing.T) {
	_, err := parseACPShimArgs([]string{"--nope"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected unknown flag error, got %v", err)
	}
}

// TestMaybeRunACPShim_NotShim 非子命令调用不拦截（正常服务启动路径不受影响）。
func TestMaybeRunACPShim_NotShim(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"admin", "-conf", "../../configs"}
	if handled, _ := maybeRunACPShim(); handled {
		t.Fatal("should not handle non-acp-shim invocation")
	}
}
