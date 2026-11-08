package service

import (
	"os"
	"testing"
)

// TestResolveLaunchCommand_Self "@self" 哨兵解析为宿主可执行文件绝对路径
// （通用 shim 零新增二进制接入的注册表约定）。
func TestResolveLaunchCommand_Self(t *testing.T) {
	got := resolveLaunchCommand("@self")
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	if got != exe {
		t.Fatalf("resolveLaunchCommand(@self) = %q, want %q", got, exe)
	}
}

// TestResolveLaunchCommand_Passthrough 非哨兵 command 原样透传（含空白变体不命中）。
func TestResolveLaunchCommand_Passthrough(t *testing.T) {
	for _, in := range []string{"npx", "claude-agent-acp", " @selfish", "@selfx", ""} {
		if got := resolveLaunchCommand(in); got != in {
			t.Fatalf("resolveLaunchCommand(%q) = %q, want passthrough", in, got)
		}
	}
	// 带首尾空白的 "@self" 命中哨兵（注册表手填容错）。
	if exe, err := os.Executable(); err == nil {
		if got := resolveLaunchCommand("  @self  "); got != exe {
			t.Fatalf("expected @self resolution for padded input, got %q", got)
		}
	}
}
