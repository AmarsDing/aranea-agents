package hostexec

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestNormalizeUTF8(t *testing.T) {
	gbk := string([]byte{0xC4, 0xE3, 0xBA, 0xC3})                 // GBK "你好"
	utf16le := string([]byte{0xFF, 0xFE, 0x60, 0x4F, 0x7D, 0x59}) // UTF-16LE BOM + "你好"

	tests := []struct {
		name  string
		in    string
		want  string
		valid bool // only assert utf8 validity, not exact content
	}{
		{name: "empty", in: "", want: ""},
		{name: "valid utf8 passthrough", in: "hello 你好 ✨", want: "hello 你好 ✨"},
		{name: "ascii passthrough", in: "plain ascii\n", want: "plain ascii\n"},
		{name: "gbk decoded", in: gbk, want: "你好"},
		{name: "utf16le bom decoded", in: utf16le, want: "你好"},
		{name: "invalid non-gbk replaced", in: string([]byte{0xFF, 0xFF, 0xFF, 0xFF}), valid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeUTF8(tt.in)
			if !utf8.ValidString(got) {
				t.Fatalf("NormalizeUTF8(%q) produced invalid UTF-8: %q", tt.in, got)
			}
			if tt.want != "" && got != tt.want {
				t.Fatalf("NormalizeUTF8(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

type utf8MockToolSet struct {
	inner trpctool.ToolSet
	tools []trpctool.Tool
}

func (m *utf8MockToolSet) Name() string { return "hostexec" }
func (m *utf8MockToolSet) Close() error { return nil }
func (m *utf8MockToolSet) Tools(context.Context) []trpctool.Tool {
	return m.tools
}

type utf8MockCallable struct {
	name   string
	result any
}

func (m *utf8MockCallable) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: m.name}
}
func (m *utf8MockCallable) Call(_ context.Context, _ []byte) (any, error) {
	return m.result, nil
}

func TestUTF8NormToolSetSanitizesStringFields(t *testing.T) {
	gbkOut := map[string]any{
		"status":    "exited",
		"output":    string([]byte{0xC4, 0xE3, 0xBA, 0xC3, '\n'}), // GBK 你好
		"exit_code": 0,
	}
	inner := &utf8MockToolSet{tools: []trpctool.Tool{
		&utf8MockCallable{name: "exec_command", result: gbkOut},
	}}
	wrapped := WrapUTF8Norm(inner)
	tools := wrapped.Tools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	ct, ok := tools[0].(trpctool.CallableTool)
	if !ok {
		t.Fatal("expected CallableTool")
	}
	res, err := ct.Call(context.Background(), []byte(`{"command":"x"}`))
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", res)
	}
	out, _ := m["output"].(string)
	if !utf8.ValidString(out) {
		t.Fatalf("output still invalid UTF-8: %q", out)
	}
	if !strings.Contains(out, "你好") {
		t.Fatalf("expected GBK decoded to 你好, got %q", out)
	}
	// non-string fields untouched
	if m["exit_code"] != 0 {
		t.Fatalf("exit_code mutated: %v", m["exit_code"])
	}
	// declaration delegated
	if ct.Declaration().Name != "exec_command" {
		t.Fatalf("declaration name = %q", ct.Declaration().Name)
	}
}

func TestUTF8NormToolSetJSONCoercionSafe(t *testing.T) {
	// 验证修复后的结果经 encoding/json 序列化不再产生 U+FFFD（模型可见路径）。
	gbkOut := map[string]any{"output": string([]byte{0xC4, 0xE3, 0xBA, 0xC3})}
	inner := &utf8MockToolSet{tools: []trpctool.Tool{
		&utf8MockCallable{name: "exec_command", result: gbkOut},
	}}
	wrapped := WrapUTF8Norm(inner)
	ct := wrapped.Tools(context.Background())[0].(trpctool.CallableTool)
	res, err := ct.Call(context.Background(), []byte(`{"command":"x"}`))
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if strings.Contains(string(b), "�") {
		t.Fatalf("json still contains replacement char: %s", b)
	}
}

func TestUTF8NormToolSetNilAndPassthrough(t *testing.T) {
	if WrapUTF8Norm(nil) != nil {
		t.Fatal("expected nil for nil toolset")
	}
	// 非 map 结果原样透传
	inner := &utf8MockToolSet{tools: []trpctool.Tool{
		&utf8MockCallable{name: "exec_command", result: "plain"},
	}}
	wrapped := WrapUTF8Norm(inner)
	ct := wrapped.Tools(context.Background())[0].(trpctool.CallableTool)
	res, err := ct.Call(context.Background(), []byte(`{"command":"x"}`))
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if res != "plain" {
		t.Fatalf("expected passthrough, got %v", res)
	}
}
