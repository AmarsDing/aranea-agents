package tools

import (
	"context"
	"fmt"
	"testing"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestParseDurationSec(t *testing.T) {
	tests := []struct {
		sec  int
		want time.Duration
	}{
		{0, 0},
		{1, time.Second},
		{30, 30 * time.Second},
		{60, time.Minute},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.sec), func(t *testing.T) {
			got := ParseDurationSec(tt.sec)
			if got != tt.want {
				t.Errorf("parseDurationSec(%d) = %v, want %v", tt.sec, got, tt.want)
			}
		})
	}
}

func TestMcpTimeoutDuration_default(t *testing.T) {
	got := McpTimeoutDuration(0)
	if got != time.Duration(DefaultMCPServerTimeoutSec)*time.Second {
		t.Fatalf("mcpTimeoutDuration(0) = %v, want default", got)
	}
}

func TestMcpTimeoutDuration_custom(t *testing.T) {
	got := McpTimeoutDuration(30)
	if got != 30*time.Second {
		t.Fatalf("mcpTimeoutDuration(30) = %v, want 30s", got)
	}
}

func TestMcpTimeoutDuration_negative(t *testing.T) {
	got := McpTimeoutDuration(-5)
	if got != time.Duration(DefaultMCPServerTimeoutSec)*time.Second {
		t.Fatalf("mcpTimeoutDuration(-5) = %v, want default for negative", got)
	}
}

func TestMCPServerConfig_ToConnectionConfig_defaultTransport(t *testing.T) {
	cfg := MCPServerConfig{
		Name:    "test",
		Command: "echo",
	}
	cc := cfg.ToConnectionConfig()
	if cc.Transport != "stdio" {
		t.Fatalf("Transport = %q, want stdio as default", cc.Transport)
	}
	if cc.Command != "echo" {
		t.Fatalf("Command = %q, want echo", cc.Command)
	}
}

func TestMCPServerConfig_ToConnectionConfig_sseTransport(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "test",
		Transport: "sse",
		ServerURL: "http://localhost:8080",
	}
	cc := cfg.ToConnectionConfig()
	if cc.Transport != "sse" {
		t.Fatalf("Transport = %q, want sse", cc.Transport)
	}
	if cc.ServerURL != "http://localhost:8080" {
		t.Fatalf("ServerURL = %q", cc.ServerURL)
	}
}

func TestMCPServerConfig_ToConnectionConfig_streamableAlias(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "test",
		Transport: "streamable_http",
		ServerURL: "http://localhost:9090",
	}
	cc := cfg.ToConnectionConfig()
	if cc.Transport != "streamable" {
		t.Fatalf("Transport = %q, want streamable (normalized)", cc.Transport)
	}
}

func TestMCPServerConfig_ToConnectionConfig_customTimeout(t *testing.T) {
	cfg := MCPServerConfig{
		Name:       "test",
		Transport:  "stdio",
		Command:    "echo",
		TimeoutSec: 60,
	}
	cc := cfg.ToConnectionConfig()
	if cc.Timeout != 60*time.Second {
		t.Fatalf("Timeout = %v, want 60s", cc.Timeout)
	}
}

func TestMCPServerConfig_ToConnectionConfig_headers(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "test",
		Headers:   map[string]string{"Authorization": "Bearer token"},
		Transport: "sse",
		ServerURL: "http://localhost:8080",
	}
	cc := cfg.ToConnectionConfig()
	if cc.Headers["Authorization"] != "Bearer token" {
		t.Fatalf("Headers = %v", cc.Headers)
	}
}

func TestAliasNameOrUnknown_nil(t *testing.T) {
	got := AliasNameOrUnknown(nil)
	if got != "<unknown>" {
		t.Fatalf("got %q, want <unknown>", got)
	}
}

func TestAliasNameOrUnknown_emptyName(t *testing.T) {
	a := &aliasTool{name: "", inner: nil}
	got := AliasNameOrUnknown(a)
	if got != "<unknown>" {
		t.Fatalf("got %q, want <unknown>", got)
	}
}

func TestAliasNameOrUnknown_withName(t *testing.T) {
	a := &aliasTool{name: "shell", inner: nil}
	got := AliasNameOrUnknown(a)
	if got != "shell" {
		t.Fatalf("got %q, want shell", got)
	}
}

func TestAliasTool_Declaration_nilInner(t *testing.T) {
	a := NewAliasTool("shell", nil)
	d := a.Declaration()
	if d != nil {
		t.Fatalf("expected nil declaration for nil inner, got %v", d)
	}
}

func TestAliasTool_Declaration_nilInnerDecl(t *testing.T) {
	inner := &mockToolForAlias{decl: nil}
	a := NewAliasTool("shell", inner)
	d := a.Declaration()
	if d == nil {
		t.Fatal("expected non-nil declaration with alias name fallback")
	}
	if d.Name != "shell" {
		t.Fatalf("Name = %q, want shell", d.Name)
	}
}

func TestAliasTool_Declaration_renamesAlias(t *testing.T) {
	inner := &mockToolForAlias{decl: &Declaration{Name: "exec_command", Description: "run command"}}
	a := NewAliasTool("shell", inner)
	d := a.Declaration()
	if d.Name != "shell" {
		t.Fatalf("Name = %q, want shell", d.Name)
	}
	if d.Description != "run command" {
		t.Fatalf("Description = %q, want run command", d.Description)
	}
}

func TestAliasTool_Declaration_doesNotMutateOriginal(t *testing.T) {
	inner := &mockToolForAlias{decl: &Declaration{Name: "exec_command", Description: "run"}}
	a := NewAliasTool("shell", inner)
	_ = a.Declaration()
	orig := inner.Declaration()
	if orig.Name != "exec_command" {
		t.Fatalf("original Name mutated to %q", orig.Name)
	}
}

func TestAliasTool_Call_nilInner(t *testing.T) {
	a := NewAliasTool("shell", nil)
	_, err := a.Call(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil inner")
	}
}

func TestAliasTool_Call_nonCallable(t *testing.T) {
	inner := &mockToolForAlias{decl: &Declaration{Name: "x"}}
	a := NewAliasTool("shell", inner)
	_, err := a.Call(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for non-callable inner")
	}
}

func TestAliasTool_Call_callableInner(t *testing.T) {
	inner := &mockCallableForAlias{decl: &Declaration{Name: "exec_command"}}
	a := NewAliasTool("shell", inner)
	out, err := a.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "called" {
		t.Fatalf("out = %v, want called", out)
	}
}

func TestAliasTool_SkipSummarization_false(t *testing.T) {
	inner := &mockToolForAlias{decl: &Declaration{Name: "x"}}
	a := NewAliasTool("shell", inner)
	if a.SkipSummarization() {
		t.Fatal("expected false for non-skipper inner")
	}
}

func TestAliasTool_LongRunning_false(t *testing.T) {
	inner := &mockToolForAlias{decl: &Declaration{Name: "x"}}
	a := NewAliasTool("shell", inner)
	if a.LongRunning() {
		t.Fatal("expected false for non-longrunner inner")
	}
}

func TestValidateRuntimeAliasesAgainstPolicy(t *testing.T) {
	err := ValidateRuntimeAliasesAgainstPolicy()
	if err != nil {
		t.Fatalf("aliases should be aligned with policy: %v", err)
	}
}

func TestApplyRuntimeNameAliases_nil(t *testing.T) {
	ApplyRuntimeNameAliases(context.Background(), nil)
}

func TestApplyRuntimeNameAliases_empty(t *testing.T) {
	out := &AssembledToolsets{}
	ApplyRuntimeNameAliases(context.Background(), out)
	if len(out.Tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(out.Tools))
	}
}

func TestApplyRuntimeNameAliases_addsAliases(t *testing.T) {
	inner := &mockToolForAlias{decl: &Declaration{Name: "exec_command"}}
	out := &AssembledToolsets{
		Tools: []Tool{inner},
	}
	ApplyRuntimeNameAliases(context.Background(), out)
	byName := map[string]bool{}
	for _, t := range out.Tools {
		if d := t.Declaration(); d != nil {
			byName[d.Name] = true
		}
	}
	if !byName["exec_command"] {
		t.Fatal("expected original tool name")
	}
	if !byName["shell_exec"] {
		t.Fatal("expected shell_exec alias for exec_command")
	}
}

func TestApplyRuntimeNameAliases_noDuplicateAlias(t *testing.T) {
	inner := &mockToolForAlias{decl: &Declaration{Name: "todo_write"}}
	out := &AssembledToolsets{
		Tools: []Tool{inner},
	}
	ApplyRuntimeNameAliases(context.Background(), out)
	count := 0
	for _, t := range out.Tools {
		if d := t.Declaration(); d != nil && d.Name == "todo_write" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("todo_write count = %d, want 1 (no duplicate)", count)
	}
}

func TestApplyRuntimeNameAliases_skipsNilTool(t *testing.T) {
	out := &AssembledToolsets{
		Tools: []Tool{nil},
	}
	ApplyRuntimeNameAliases(context.Background(), out)
	if len(out.Tools) != 1 {
		t.Fatalf("expected 1 tool (nil preserved), got %d", len(out.Tools))
	}
}

func TestApplyRuntimeNameAliases_skipsNilDeclaration(t *testing.T) {
	inner := &mockToolForAlias{decl: nil}
	out := &AssembledToolsets{
		Tools: []Tool{inner},
	}
	ApplyRuntimeNameAliases(context.Background(), out)
	if len(out.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(out.Tools))
	}
}

func TestApplyRuntimeNameAliases_toolSetTools(t *testing.T) {
	innerTool := &mockToolForAlias{decl: &Declaration{Name: "save_file"}}
	ts := &mockToolSetForAlias{
		name:  "file",
		tools: []Tool{innerTool},
	}
	out := &AssembledToolsets{
		ToolSets: []ToolSet{ts},
	}
	ApplyRuntimeNameAliases(context.Background(), out)
	byName := map[string]bool{}
	for _, t := range out.Tools {
		if d := t.Declaration(); d != nil {
			byName[d.Name] = true
		}
	}
	if !byName["write_file"] {
		names := make([]string, 0, len(byName))
		for n := range byName {
			names = append(names, n)
		}
		t.Fatalf("expected write_file alias for save_file, got names: %v", names)
	}
}

type mockToolForAlias struct {
	decl *Declaration
}

func (m *mockToolForAlias) Declaration() *Declaration {
	return m.decl
}

type mockCallableForAlias struct {
	decl *Declaration
}

func (m *mockCallableForAlias) Declaration() *Declaration {
	return m.decl
}

func (m *mockCallableForAlias) Call(_ context.Context, _ []byte) (any, error) {
	return "called", nil
}

type mockToolSetForAlias struct {
	name  string
	tools []Tool
}

func (m *mockToolSetForAlias) Name() string { return m.name }
func (m *mockToolSetForAlias) Close() error { return nil }
func (m *mockToolSetForAlias) Tools(_ context.Context) []trpctool.Tool {
	out := make([]trpctool.Tool, len(m.tools))
	for i, t := range m.tools {
		out[i] = t
	}
	return out
}

// mockStreamableForAlias is both callable and streamable.
type mockStreamableForAlias struct {
	decl *Declaration
}

func (m *mockStreamableForAlias) Declaration() *Declaration { return m.decl }
func (m *mockStreamableForAlias) Call(_ context.Context, _ []byte) (any, error) {
	return "called", nil
}
func (m *mockStreamableForAlias) StreamableCall(_ context.Context, _ []byte) (*trpctool.StreamReader, error) {
	s := trpctool.NewStream(1)
	go func() {
		s.Writer.Send(trpctool.StreamChunk{Content: "chunk"}, nil)
		s.Writer.Close()
	}()
	return s.Reader, nil
}

// Regression (2026-07-18): an alias wrapping a non-streamable inner tool must
// NOT satisfy trpctool.StreamableTool. Otherwise the framework's
// executeTool/isStreamable classification routes the call to StreamableCall,
// which fails with "inner tool is not streamable" — breaking every aliased
// non-streaming tool (todo, list_files, ...) after the P2-02 stream decorator
// started preserving the StreamableTool interface.
func TestApplyRuntimeNameAliases_nonStreamableInner_notMisclassified(t *testing.T) {
	inner := &mockCallableForAlias{decl: &Declaration{Name: "todo_write"}}
	out := &AssembledToolsets{Tools: []Tool{inner}}
	ApplyRuntimeNameAliases(context.Background(), out)
	for _, tool := range out.Tools {
		d := tool.Declaration()
		if d == nil || d.Name != "todo" {
			continue
		}
		if _, ok := tool.(trpctool.StreamableTool); ok {
			t.Fatal("alias of non-streamable inner must not satisfy StreamableTool")
		}
		return
	}
	t.Fatal("expected todo alias to be registered")
}

func TestApplyRuntimeNameAliases_streamableInner_staysStreamable(t *testing.T) {
	inner := &mockStreamableForAlias{decl: &Declaration{Name: "todo_write"}}
	out := &AssembledToolsets{Tools: []Tool{inner}}
	ApplyRuntimeNameAliases(context.Background(), out)
	for _, tool := range out.Tools {
		d := tool.Declaration()
		if d == nil || d.Name != "todo" {
			continue
		}
		st, ok := tool.(trpctool.StreamableTool)
		if !ok {
			t.Fatal("alias of streamable inner must satisfy StreamableTool")
		}
		r, err := st.StreamableCall(context.Background(), []byte(`{}`))
		if err != nil || r == nil {
			t.Fatalf("StreamableCall should delegate to inner: err=%v reader=%v", err, r)
		}
		return
	}
	t.Fatal("expected todo alias to be registered")
}
