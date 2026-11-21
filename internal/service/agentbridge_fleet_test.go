package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/pkg/apierror"
)

// 装在运行台 CLI 根目录里的 ACP 适配器（如 claude-agent-acp）不依赖 PATH 也能解析成全路径；
// 不在根目录里的命令（npx 等）原样交 PATH，不改 Codex 现有的启动方式。
func TestResolveLaunchCommandFindsCLIRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ARANEA_FLEET_CLI_ROOT", root)
	name := "claude-agent-acp"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	_ = os.MkdirAll(filepath.Join(root, "npm"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "npm", name), []byte("@echo off\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolveLaunchCommand("claude-agent-acp"); !strings.HasPrefix(got, root) {
		t.Fatalf("adapter in CLI root should resolve to full path, got %q", got)
	}
	if got := resolveLaunchCommand("npx"); got != "npx" {
		t.Fatalf("commands outside CLI root must stay bare, got %q", got)
	}
}

// fleet:<工具 id> 的编码 agent 不走 ACP：运行台没登记时明确报不可用，而不是去 spawn 一个叫 "fleet:x" 的进程。
func TestACPSessionFactoryRoutesFleetAgents(t *testing.T) {
	prev := fleetBridge.Load()
	fleetBridge.Store(nil)
	t.Cleanup(func() { fleetBridge.Store(prev) })
	_, err := NewACPSessionFactory().Spawn(context.Background(), &agentbridge.CodingAgent{Command: "fleet:cursor"})
	if !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("want Unavailable without fleet bridge, got %v", err)
	}
}
