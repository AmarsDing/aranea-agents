package acp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess 是子进程替身：echo stdin 到 stdout；收到 "quit" 行退出码 0。
// 收到 "sleep" 后阻塞（用于 Kill 测试）。
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_ACP_HELPER") != "1" {
		return
	}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "quit":
			os.Exit(0)
		case line == "sleep":
			fmt.Println("sleeping")
			time.Sleep(time.Hour) // 带定时器阻塞，避开死锁检测；等待被杀
		case strings.HasPrefix(line, "err:"):
			fmt.Fprintln(os.Stderr, "stderr:"+line[4:]) // S3 诊断材料验证
		case strings.HasPrefix(line, "env:"): // S2 白名单验证：回报宿主注入键是否可见
			v, ok := os.LookupEnv(line[4:])
			fmt.Printf("env:%s=%s:%v\n", line[4:], v, ok)
		default:
			fmt.Println("echo:" + line)
		}
	}
	os.Exit(0)
}

func helperCommand(t *testing.T) (string, []string, map[string]string) {
	t.Helper()
	return os.Args[0], []string{"-test.run=^TestHelperProcess$"}, map[string]string{"GO_WANT_ACP_HELPER": "1"}
}

func TestProcessSpawnEchoRoundTrip(t *testing.T) {
	cmd, args, env := helperCommand(t)
	p, err := Spawn(context.Background(), SpawnOptions{Command: cmd, Args: args, Env: env})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Kill()

	if _, err := p.Stdin().Write([]byte("hello\n")); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	line, err := bufio.NewReader(p.Stdout()).ReadString('\n')
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if strings.TrimSpace(line) != "echo:hello" {
		t.Fatalf("want echo:hello, got %q", line)
	}
}

func TestProcessCleanExit(t *testing.T) {
	cmd, args, env := helperCommand(t)
	p, err := Spawn(context.Background(), SpawnOptions{Command: cmd, Args: args, Env: env})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, err := p.Stdin().Write([]byte("quit\n")); err != nil {
		t.Fatalf("write quit: %v", err)
	}
	select {
	case <-p.Done():
		if err := p.ExitErr(); err != nil {
			t.Fatalf("clean exit should yield nil, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Done not closed after quit")
	}
}

func TestProcessKillTerminates(t *testing.T) {
	cmd, args, env := helperCommand(t)
	p, err := Spawn(context.Background(), SpawnOptions{Command: cmd, Args: args, Env: env})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, err := p.Stdin().Write([]byte("sleep\n")); err != nil {
		t.Fatalf("write sleep: %v", err)
	}
	// 等 helper 确认已进入阻塞（避免竞态：进程还没开始读就被 Kill）
	line, err := bufio.NewReader(p.Stdout()).ReadString('\n')
	if err != nil {
		t.Fatalf("read sleeping ack: %v", err)
	}
	if strings.TrimSpace(line) != "sleeping" {
		t.Fatalf("want sleeping ack, got %q", line)
	}
	p.Kill()
	select {
	case <-p.Done():
		if err := p.ExitErr(); err == nil {
			t.Fatal("killed process must yield non-nil error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Done not closed after Kill")
	}
}

func TestProcessSpawnMissingCommand(t *testing.T) {
	_, err := Spawn(context.Background(), SpawnOptions{Command: "definitely-not-exist-cmd-xyz-123"})
	if err == nil {
		t.Fatal("want error for missing command")
	}
}

func TestLookPathProbe(t *testing.T) {
	if err := ProbeCommand("definitely-not-exist-cmd-xyz-123"); err == nil {
		t.Fatal("want probe error for missing command")
	}
	self, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	_ = exec.Command(self)
	if err := ProbeCommand(self); err != nil {
		t.Fatalf("probe self executable should pass: %v", err)
	}
}

// ---------- M4 P0-2 S2：子进程 env 白名单 ----------

func TestBuildChildEnv_FiltersHostSecrets(t *testing.T) {
	t.Setenv("MY_API_KEY", "host-secret-value")
	t.Setenv("MY_AUTH_TOKEN", "host-token-value")
	t.Setenv("SOME_PASSWORD", "host-pw-value")
	t.Setenv("NO_PROXY", "127.0.0.1")

	env := buildChildEnv(map[string]string{"DEEPSEEK_API_KEY": "sk-explicit"})
	got := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		got[strings.ToUpper(k)] = v
	}
	for _, k := range []string{"MY_API_KEY", "MY_AUTH_TOKEN", "SOME_PASSWORD"} {
		if _, leaked := got[k]; leaked {
			t.Fatalf("host sensitive key %s must not reach child env", k)
		}
	}
	if got["DEEPSEEK_API_KEY"] != "sk-explicit" {
		t.Fatal("explicit injected env must pass through")
	}
	if got["NO_PROXY"] != "127.0.0.1" {
		t.Fatal("whitelisted proxy env must pass through")
	}
	if _, ok := got["PATH"]; !ok && os.Getenv("PATH") != "" && os.Getenv("Path") != "" {
		t.Fatal("PATH must pass through when present on host")
	}
}

func TestBuildChildEnv_ExplicitOverridesWhitelist(t *testing.T) {
	cmd, args, env := helperCommand(t)
	env["PATH"] = "C:\\override-path"
	p, err := Spawn(context.Background(), SpawnOptions{Command: cmd, Args: args, Env: env})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Kill()
	if _, err := p.Stdin().Write([]byte("env:PATH\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	line, err := bufio.NewReader(p.Stdout()).ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.TrimSpace(line) != "env:PATH=C:\\override-path:true" {
		t.Fatalf("explicit PATH override lost, got %q", line)
	}
}

func TestChildDoesNotSeeHostSecret(t *testing.T) {
	t.Setenv("AGENTBRIDGE_SECRET_KEY", "must-not-leak")
	cmd, args, env := helperCommand(t)
	p, err := Spawn(context.Background(), SpawnOptions{Command: cmd, Args: args, Env: env})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Kill()
	if _, err := p.Stdin().Write([]byte("env:AGENTBRIDGE_SECRET_KEY\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	line, err := bufio.NewReader(p.Stdout()).ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := strings.TrimSpace(line); got != "env:AGENTBRIDGE_SECRET_KEY=:false" {
		t.Fatalf("host secret leaked into child: %q", got)
	}
}

// ---------- M4 P0-2 S3：stderr 尾部诊断材料 ----------

func TestProcessStderrTail(t *testing.T) {
	cmd, args, env := helperCommand(t)
	p, err := Spawn(context.Background(), SpawnOptions{Command: cmd, Args: args, Env: env})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Kill()
	if _, err := p.Stdin().Write([]byte("err:diag-marker-42\nhello\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	line, err := bufio.NewReader(p.Stdout()).ReadString('\n')
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if strings.TrimSpace(line) != "echo:hello" {
		t.Fatalf("want echo:hello, got %q", line)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(p.StderrTail(), "diag-marker-42") {
		if time.Now().After(deadline) {
			t.Fatalf("stderr tail missing marker: %q", p.StderrTail())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStderrRingRoundTrip(t *testing.T) {
	r := &stderrRing{}
	big := make([]byte, stderrRingCap*2)
	for i := range big {
		big[i] = 'a'
	}
	const marker = "tail-marker-32-bytes-long-exact!"
	copy(big[len(big)-len(marker):], marker)
	if _, err := r.Write(big[:stderrRingCap]); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := r.Write(big[stderrRingCap:]); err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(r.buf) > stderrRingCap {
		t.Fatalf("ring must stay within cap, got %d", len(r.buf))
	}
	if !strings.HasSuffix(r.String(), marker) {
		t.Fatalf("ring must keep newest tail, got suffix %q", r.String())
	}
}

// ---------- M4 P0-4 S4：孤儿进程树按 PID 收割 ----------

// TestReapOrphanPID：按 PID 收割存活孤儿树；PID 已退出的重复收割幂等成功（重启恢复场景）。
func TestReapOrphanPID(t *testing.T) {
	cmd, args, env := helperCommand(t)
	p, err := Spawn(context.Background(), SpawnOptions{Command: cmd, Args: args, Env: env})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, err := p.Stdin().Write([]byte("sleep\n")); err != nil {
		t.Fatalf("write sleep: %v", err)
	}
	if _, err := bufio.NewReader(p.Stdout()).ReadString('\n'); err != nil {
		t.Fatalf("read sleeping ack: %v", err)
	}
	if err := ReapOrphanPID(p.PID()); err != nil {
		t.Fatalf("reap live orphan: %v", err)
	}
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("orphan tree not reaped")
	}
	// 幂等：PID 已退出（taskkill 128 / ESRCH）不视为失败
	if err := ReapOrphanPID(p.PID()); err != nil {
		t.Fatalf("reap dead pid must be tolerated: %v", err)
	}
	if err := ReapOrphanPID(0); err != nil {
		t.Fatalf("reap pid 0 must be no-op: %v", err)
	}
}

// ---------- M4 P3-3 前置：`_aranea/ping` 协议探测 ----------

func TestClientPingMethodNotFoundIsAlive(t *testing.T) {
	c, err := Start(context.Background(), fakeAgentSpawn(t), nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("-32601 must count as alive, got %v", err)
	}

	c.Close() // 关闭传输后 ping 必须失败（对端失联）
	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	if err := c.Ping(ctx2); err == nil {
		t.Fatal("closed transport must fail ping")
	}
}
