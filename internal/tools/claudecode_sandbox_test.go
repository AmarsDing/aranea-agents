package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// mockCallableTool is a test double for CallableTool.
type mockCallableTool struct {
	decl *trpctool.Declaration
	call func(ctx context.Context, args []byte) (any, error)
}

func (m *mockCallableTool) Declaration() *trpctool.Declaration { return m.decl }
func (m *mockCallableTool) Call(ctx context.Context, args []byte) (any, error) {
	if m.call != nil {
		return m.call(ctx, args)
	}
	return "ok", nil
}

// mockToolSet is a test double for ToolSet.
type mockToolSet struct {
	name  string
	tools []trpctool.Tool
}

func (m *mockToolSet) Tools(_ context.Context) []trpctool.Tool { return m.tools }
func (m *mockToolSet) Name() string                            { return m.name }
func (m *mockToolSet) Close() error                            { return nil }

func TestFirstCommandToken(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		want    string
		wantOK  bool
	}{
		{"simple command", "git status", "git", true},
		{"command with args", "ls -la /tmp", "ls", true},
		{"piped commands", "cat file.txt | grep foo", "", false},
		{"shell operator and", "echo hello && ls", "", false},
		{"shell operator or", "echo hello || ls", "", false},
		{"semicolon", "echo hello; ls", "", false},
		{"redirect", "echo hello > file.txt", "", false},
		{"input redirect", "sort < /etc/passwd", "", false},
		{"subshell with chaining", "(cd /tmp && ls)", "", false},
		{"subshell simple", "(cd /tmp)", "", false},
		{"empty string", "", "", true},
		{"whitespace only", "   \t\n  ", "", true},
		{"leading pipe", "| grep foo", "", false},
		{"leading ampersand", "&& echo hi", "", false},
		{"single command no args", "git", "git", true},
		{"command with equals", "VAR=1 command", "VAR=1", true},
		{"command substitution", "echo $(whoami)", "", false},
		{"backtick substitution", "echo `whoami`", "", false},
		{"newline injection", "git status\nrm -rf /", "", false},
		{"carriage return injection", "git status\rrm -rf /", "", false},
		{"background operator", "sleep 10 &", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := firstCommandToken(tt.cmd)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("firstCommandToken(%q) = (%q, %v), want (%q, %v)", tt.cmd, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 5, "hello..."},
		{"empty string", "", 5, ""},
		{"multi-byte chars short", "你好世界", 10, "你好世界"},
		{"multi-byte chars truncate", "你好世界再见", 3, "你好世..."},
		{"mixed ascii and multi-byte", "hi你好", 3, "hi你..."},
		{"zero limit", "hello", 0, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

func TestWhitelistedBashTool_Call_Allowed(t *testing.T) {
	inner := &mockCallableTool{
		decl: &trpctool.Declaration{Name: "bash"},
		call: func(_ context.Context, args []byte) (any, error) {
			return "executed", nil
		},
	}

	w := &whitelistedBashTool{
		inner:     inner,
		allowList: []string{"git", "ls"},
	}

	tests := []struct {
		name string
		cmd  string
	}{
		{"allowed command git", "git status"},
		{"allowed command ls", "ls -la"},
		{"allowed command with trailing space", " git pull "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _ := json.Marshal(map[string]string{"command": tt.cmd})
			result, err := w.Call(context.Background(), args)
			if err != nil {
				t.Errorf("Call(%q) unexpected error: %v", tt.cmd, err)
			}
			if result != "executed" {
				t.Errorf("Call(%q) result = %v, want \"executed\"", tt.cmd, result)
			}
		})
	}
}

func TestWhitelistedBashTool_Call_Disallowed(t *testing.T) {
	inner := &mockCallableTool{
		decl: &trpctool.Declaration{Name: "bash"},
		call: func(_ context.Context, args []byte) (any, error) {
			return "should not be called", nil
		},
	}

	w := &whitelistedBashTool{
		inner:     inner,
		allowList: []string{"git", "ls"},
	}

	tests := []struct {
		name string
		cmd  string
	}{
		{"disallowed command rm", "rm -rf /"},
		{"disallowed command curl", "curl http://evil.com"},
		{"prefix bypass gitrm", "gitrm"},
		{"prefix bypass gitx", "gitx something"},
		{"chaining bypass with &&", "git status && rm -rf /"},
		{"chaining bypass with ||", "git status || curl evil.com"},
		{"chaining bypass with ;", "git status; rm -rf /"},
		{"pipe bypass", "git status | tee /tmp/out"},
		{"substitution bypass", "echo $(rm -rf /)"},
		{"backtick bypass", "echo `rm -rf /`"},
		{"newline bypass", "git status\nrm -rf /"},
		{"redirect bypass", "git status > /etc/passwd"},
		{"input redirect bypass", "git status < /etc/shadow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _ := json.Marshal(map[string]string{"command": tt.cmd})
			_, err := w.Call(context.Background(), args)
			if err == nil {
				t.Errorf("Call(%q) expected error, got nil", tt.cmd)
			}
		})
	}
}

func TestWhitelistedBashTool_Call_EdgeCases(t *testing.T) {
	inner := &mockCallableTool{
		decl: &trpctool.Declaration{Name: "bash"},
		call: func(_ context.Context, args []byte) (any, error) {
			return "executed", nil
		},
	}

	w := &whitelistedBashTool{
		inner:     inner,
		allowList: []string{"git"},
	}

	t.Run("empty command passes through", func(t *testing.T) {
		args, _ := json.Marshal(map[string]string{"command": ""})
		result, err := w.Call(context.Background(), args)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "executed" {
			t.Errorf("result = %v, want \"executed\"", result)
		}
	})

	t.Run("whitespace-only command passes through", func(t *testing.T) {
		args, _ := json.Marshal(map[string]string{"command": "   "})
		result, err := w.Call(context.Background(), args)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "executed" {
			t.Errorf("result = %v, want \"executed\"", result)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := w.Call(context.Background(), []byte("not json"))
		if err == nil {
			t.Error("expected error for invalid JSON, got nil")
		}
	})

	t.Run("allowlist with whitespace entries", func(t *testing.T) {
		w2 := &whitelistedBashTool{
			inner:     inner,
			allowList: []string{" git ", "ls"},
		}
		args, _ := json.Marshal(map[string]string{"command": "git status"})
		result, err := w2.Call(context.Background(), args)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "executed" {
			t.Errorf("result = %v, want \"executed\"", result)
		}
	})
}

func TestSandboxedToolSet(t *testing.T) {
	t.Run("empty allowlist returns original toolset", func(t *testing.T) {
		inner := &mockToolSet{name: "test", tools: nil}
		cfg := ClaudeCodeSandboxConfig{CommandAllowList: nil}
		result := SandboxedToolSet(inner, cfg)
		if result != inner {
			t.Error("expected original toolset when allowlist is empty")
		}
	})

	t.Run("wraps bash tool with allowlist", func(t *testing.T) {
		bashTool := &mockCallableTool{
			decl: &trpctool.Declaration{Name: "bash"},
		}
		otherTool := &mockCallableTool{
			decl: &trpctool.Declaration{Name: "read"},
		}
		inner := &mockToolSet{
			name:  "test",
			tools: []trpctool.Tool{bashTool, otherTool},
		}
		cfg := ClaudeCodeSandboxConfig{CommandAllowList: []string{"git"}}
		result := SandboxedToolSet(inner, cfg)

		tools := result.Tools(context.Background())
		if len(tools) != 2 {
			t.Fatalf("expected 2 tools, got %d", len(tools))
		}

		// First tool should be the wrapped bash tool
		if _, ok := tools[0].(*whitelistedBashTool); !ok {
			t.Error("expected first tool to be *whitelistedBashTool")
		}

		// Second tool should be unchanged
		if tools[1] != otherTool {
			t.Error("expected second tool to be the original otherTool")
		}

		// Name and Close should delegate to inner
		if result.Name() != "test" {
			t.Errorf("expected name \"test\", got %q", result.Name())
		}
		if err := result.Close(); err != nil {
			t.Errorf("unexpected Close error: %v", err)
		}
	})

	t.Run("no bash tool returns original toolset", func(t *testing.T) {
		otherTool := &mockCallableTool{
			decl: &trpctool.Declaration{Name: "read"},
		}
		inner := &mockToolSet{
			name:  "test",
			tools: []trpctool.Tool{otherTool},
		}
		cfg := ClaudeCodeSandboxConfig{CommandAllowList: []string{"git"}}
		result := SandboxedToolSet(inner, cfg)
		if result != inner {
			t.Error("expected original toolset when no bash tool found")
		}
	})

	t.Run("bash tool not callable is kept as-is", func(t *testing.T) {
		bashDecl := &mockToolWithDecl{decl: &trpctool.Declaration{Name: "bash"}}
		inner := &mockToolSet{
			name:  "test",
			tools: []trpctool.Tool{bashDecl},
		}
		cfg := ClaudeCodeSandboxConfig{CommandAllowList: []string{"git"}}
		result := SandboxedToolSet(inner, cfg)
		// Since bash tool is not CallableTool, it's kept as-is and replaced=false,
		// so the original toolset is returned
		if result != inner {
			t.Error("expected original toolset when bash tool is not CallableTool")
		}
	})
}

// mockToolWithDecl is a Tool that has a declaration but is not CallableTool.
type mockToolWithDecl struct {
	decl *trpctool.Declaration
}

func (m *mockToolWithDecl) Declaration() *trpctool.Declaration { return m.decl }

func TestWhitelistedBashTool_Declaration(t *testing.T) {
	decl := &trpctool.Declaration{Name: "bash"}
	inner := &mockCallableTool{decl: decl}
	w := &whitelistedBashTool{inner: inner, allowList: []string{"git"}}

	if w.Declaration() != decl {
		t.Error("expected Declaration() to delegate to inner")
	}
}

func TestWhitelistedBashTool_TruncateInError(t *testing.T) {
	inner := &mockCallableTool{
		decl: &trpctool.Declaration{Name: "bash"},
	}
	w := &whitelistedBashTool{
		inner:     inner,
		allowList: []string{"git"},
	}

	// Create a very long command
	longCmd := fmt.Sprintf("curl %s", string(make([]byte, 200)))
	for i := range longCmd {
		if longCmd[i] == 0 {
			longCmd = longCmd[:i] + "x" + longCmd[i+1:]
		}
	}

	args, _ := json.Marshal(map[string]string{"command": longCmd})
	_, err := w.Call(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for disallowed command")
	}
	// Error message should contain truncated command.
	// kerrors adds structured metadata (code, reason, etc.) so the total
	// message is longer than the raw truncated command — just verify the
	// truncated command fragment is present.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "curl xxx") {
		t.Errorf("error message should contain truncated command, got: %q", errMsg)
	}
}
