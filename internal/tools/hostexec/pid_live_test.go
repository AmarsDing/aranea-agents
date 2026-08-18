package hostexec

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strconv"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestWrapSessionEnhance_BackgroundHasPID(t *testing.T) {
	ts, err := BuildHostexecToolSet("", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ts.Close() })
	var execTool, stdin, kill trpctool.CallableTool
	for _, candidate := range ts.Tools(context.Background()) {
		ct, ok := candidate.(trpctool.CallableTool)
		if !ok || candidate.Declaration() == nil {
			continue
		}
		switch candidate.Declaration().Name {
		case "exec_command":
			execTool = ct
		case "write_stdin":
			stdin = ct
		case "kill_session":
			kill = ct
		}
	}
	if execTool == nil || stdin == nil {
		t.Fatal("missing exec_command or write_stdin")
	}
	command := "sleep 8"
	if runtime.GOOS == "windows" {
		command = "ping -n 8 127.0.0.1"
	}
	args, _ := json.Marshal(map[string]any{
		"command":       command,
		"yield_time_ms": 400,
	})
	value, err := execTool.Call(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%T", value)
	}
	if result["status"] != "running" {
		t.Fatalf("status=%v output=%v", result["status"], result["output"])
	}
	pid := jsonInt(result["pid"])
	if pid <= 0 {
		t.Fatalf("pid=%v keys=%v", result["pid"], result)
	}
	if _, ok := result["running_for_ms"]; !ok {
		t.Fatal("missing running_for_ms")
	}
	sid, _ := result["session_id"].(string)
	if sid == "" {
		t.Fatal("missing session_id")
	}
	t.Cleanup(func() {
		if kill != nil {
			killArgs, _ := json.Marshal(map[string]any{"session_id": sid})
			_, _ = kill.Call(context.Background(), killArgs)
		}
		if runtime.GOOS == "windows" && pid > 0 {
			_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
		}
	})
	pollArgs, _ := json.Marshal(map[string]any{
		"session_id":    sid,
		"chars":         "",
		"yield_time_ms": 0,
	})
	pollValue, err := stdin.Call(context.Background(), pollArgs)
	if err != nil {
		t.Fatal(err)
	}
	poll, ok := pollValue.(map[string]any)
	if !ok {
		t.Fatalf("%T", pollValue)
	}
	pollPID := jsonInt(poll["pid"])
	if pollPID <= 0 {
		t.Fatalf("poll pid=%v keys=%v", poll["pid"], poll)
	}
}

func jsonInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
