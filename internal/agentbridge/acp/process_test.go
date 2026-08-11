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
		switch line {
		case "quit":
			os.Exit(0)
		case "sleep":
			select {} // 永不返回，等待被杀
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
	case err := <-p.Done():
		if err != nil {
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
	time.Sleep(100 * time.Millisecond) // 等 helper 进入阻塞
	p.Kill()
	select {
	case err := <-p.Done():
		if err == nil {
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
