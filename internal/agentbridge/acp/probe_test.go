// probe_test.go M4 P4-2：分级探针五级断言（设计 §G L1–L5，AC-12）。
package acp

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestFakeACPAgentSilent 假 agent：进程存活但永不回 initialize（L3 素材）。
func TestFakeACPAgentSilent(t *testing.T) {
	if os.Getenv("GO_WANT_ACP_FAKE_SILENT") != "1" {
		return
	}
	_, _ = io.Copy(io.Discard, bufio.NewReader(os.Stdin))
	os.Exit(0)
}

func silentAgentSpawn() SpawnOptions {
	return SpawnOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestFakeACPAgentSilent$"},
		Env:     map[string]string{"GO_WANT_ACP_FAKE_SILENT": "1"},
	}
}

// withHandshakeBudget 缩短握手预算（L3 用例等不起默认 2min 下限）。
func withHandshakeBudget(t *testing.T, b time.Duration) {
	t.Helper()
	prev := probeHandshakeBudget
	probeHandshakeBudget = func(time.Duration) time.Duration { return b }
	t.Cleanup(func() { probeHandshakeBudget = prev })
}

func TestProbe_NotInstalled(t *testing.T) {
	level, detail := Probe(context.Background(), SpawnOptions{Command: "aranea-no-such-cmd-xyzzy-996"}, 0)
	if level != ProbeNotInstalled {
		t.Fatalf("level = %s, want not_installed", level)
	}
	if detail == "" || strings.Contains(detail, os.Getenv("HOME")) {
		t.Fatalf("detail 应含中文引导且无敏感信息，got %q", detail)
	}
}

func TestProbe_SpawnFailed_ImmediateExit(t *testing.T) {
	// 测试二进制以不存在的 -test.run 启动：立即退出、stdout 无协议帧。
	opt := SpawnOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestNoSuchTestZZZ$"},
	}
	level, detail := Probe(context.Background(), opt, 0)
	if level != ProbeSpawnFailed {
		t.Fatalf("level = %s, want spawn_failed（detail=%q）", level, detail)
	}
}

func TestProbe_HandshakeFailed_Timeout(t *testing.T) {
	withHandshakeBudget(t, 80*time.Millisecond)
	level, detail := Probe(context.Background(), silentAgentSpawn(), time.Minute)
	if level != ProbeHandshakeFailed {
		t.Fatalf("level = %s, want handshake_failed（detail=%q）", level, detail)
	}
	if !strings.Contains(detail, "握手超时") {
		t.Fatalf("detail 应指明握手超时，got %q", detail)
	}
}

func TestProbe_VersionIncompatible(t *testing.T) {
	opt := SpawnOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestFakeACPAgentBadVersion$"},
		Env:     map[string]string{"GO_WANT_ACP_FAKE_BADVER": "1"},
	}
	level, detail := Probe(context.Background(), opt, 0)
	if level != ProbeVersionIncompatible {
		t.Fatalf("level = %s, want version_incompatible（detail=%q）", level, detail)
	}
	if !strings.Contains(detail, "999") {
		t.Fatalf("detail 应含对端版本号，got %q", detail)
	}
}

func TestProbe_Warming(t *testing.T) {
	opt := SpawnOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestFakeACPAgent$"},
		Env: map[string]string{
			"GO_WANT_ACP_FAKE":            "1",
			"GO_WANT_ACP_FAKE_INIT_DELAY": "150ms",
		},
	}
	// warm=10ms ≪ 150ms 首轮延时，但握手预算保持默认下限（L5 须先于超时触发）。
	level, detail := Probe(context.Background(), opt, 10*time.Millisecond)
	if level != ProbeWarming {
		t.Fatalf("level = %s, want warming（detail=%q）", level, detail)
	}
	if !level.OKish() {
		t.Fatal("warming 应视为可派发")
	}
}

func TestProbe_OK(t *testing.T) {
	level, detail := Probe(context.Background(), fakeAgentSpawn(t), 0)
	if level != ProbeOK {
		t.Fatalf("level = %s, want ok（detail=%q）", level, detail)
	}
	if detail != "" {
		t.Fatalf("ok 级别 detail 应为空，got %q", detail)
	}
}
