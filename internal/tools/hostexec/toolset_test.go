package hostexec

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestBuildHostexecToolSetInjectsAndRedactsEnvironment(t *testing.T) {
	const (
		marker = "hostexec-env-injected"
		secret = "hostexec-injected-secret-987"
	)
	env := map[string]string{
		"ARANEA_TEST_MARKER":    marker,
		"ARANEA_TEST_API_TOKEN": secret,
	}
	toolSet, err := BuildHostexecToolSet(t.TempDir(), env)
	if err != nil {
		t.Fatalf("BuildHostexecToolSet() error = %v", err)
	}
	if closer, ok := toolSet.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}

	var execTool trpctool.CallableTool
	for _, candidate := range toolSet.Tools(context.Background()) {
		if candidate.Declaration().Name == "exec_command" {
			execTool, _ = candidate.(trpctool.CallableTool)
			break
		}
	}
	if execTool == nil {
		t.Fatal("exec_command callable tool not found")
	}

	command := `printf '%s\n%s\n' "$ARANEA_TEST_MARKER" "$ARANEA_TEST_API_TOKEN"`
	if runtime.GOOS == "windows" {
		command = `echo %ARANEA_TEST_MARKER% & echo %ARANEA_TEST_API_TOKEN%`
	}
	args, err := json.Marshal(map[string]any{
		"command":       command,
		"yield-time_ms": 0,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	value, err := execTool.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("exec_command error = %v", err)
	}
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("exec_command result type = %T, want map[string]any", value)
	}
	output, _ := result["output"].(string)
	if !strings.Contains(output, marker) {
		t.Errorf("output = %q, want injected marker %q", output, marker)
	}
	if strings.Contains(output, secret) {
		t.Errorf("output leaked configured secret: %q", output)
	}
	if !strings.Contains(output, "[REDACTED") {
		t.Errorf("output = %q, want secret redaction marker", output)
	}
}
