package openaicompat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/agentbridge/acpshim"
)

// fixturePath 解析 P0-5 fixture（acpshim/testdata/）。
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "testdata", name)
}

// serveFixture 起一个假 vendor：流式与非流式按请求 stream 位分别供 fixture。
type fakeVendor struct {
	t          *testing.T
	streamBody string
	nonStream  string
	streamCode int // 非 200 时流式请求直接失败（触发回退/报错路径）
	lastAuth   string
	lastBody   []byte
}

func (v *fakeVendor) handler(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	v.lastAuth = r.Header.Get("Authorization")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	v.lastBody = body
	var probe struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &probe)
	if probe.Stream {
		if v.streamCode != 0 && v.streamCode != http.StatusOK {
			http.Error(w, "stream unsupported", v.streamCode)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, v.streamBody)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, v.nonStream)
}

// collect 驱动 Chat 并收齐全量增量。
func collect(t *testing.T, c *Client, req acpshim.ChatRequest) []acpshim.ChatDelta {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := c.Chat(ctx, req)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	var out []acpshim.ChatDelta
	for d := range ch {
		out = append(out, d)
	}
	return out
}

func baseRequest() acpshim.ChatRequest {
	return acpshim.ChatRequest{
		Model:    "m-test",
		Messages: []acpshim.ChatMessage{{Role: "user", Content: "看下 README"}},
		Tools: []acpshim.ToolDef{{
			Name:           "fs_read",
			Description:    "读文件",
			ParametersJSON: json.RawMessage(`{"type":"object"}`),
		}},
	}
}

// finalOf 抽取 Done 帧。
func finalOf(t *testing.T, ds []acpshim.ChatDelta) acpshim.ChatDelta {
	t.Helper()
	for _, d := range ds {
		if d.Err != nil {
			t.Fatalf("unexpected stream error: %v", d.Err)
		}
		if d.Done {
			return d
		}
	}
	t.Fatal("no Done delta")
	return acpshim.ChatDelta{}
}

func textsOf(ds []acpshim.ChatDelta) (texts, reasonings []string) {
	for _, d := range ds {
		if d.Text != "" {
			texts = append(texts, d.Text)
		}
		if d.Reasoning != "" {
			reasonings = append(reasonings, d.Reasoning)
		}
	}
	return texts, reasonings
}

// TestGoldenDeepseekStream：流式 tool_calls 按 index 增量聚合（fixture 假设 1/2/3）。
func TestGoldenDeepseekStream(t *testing.T) {
	body, err := os.ReadFile(fixturePath(t, "deepseek_toolcall_stream.sse"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	v := &fakeVendor{t: t, streamBody: string(body)}
	srv := httptest.NewServer(http.HandlerFunc(v.handler))
	defer srv.Close()

	c := New(srv.URL, "k-test", ProviderDeepseek, srv.Client(), nil)
	ds := collect(t, c, baseRequest())

	texts, reasonings := textsOf(ds)
	if got := strings.Join(texts, ""); got != "我先读取" {
		t.Fatalf("text = %q, want %q", got, "我先读取")
	}
	// reasoning 独立成帧，绝不混入 arguments（假设 3）。
	if got := strings.Join(reasonings, ""); got != "先看下文件结构" {
		t.Fatalf("reasoning = %q, want %q", got, "先看下文件结构")
	}

	final := finalOf(t, ds)
	if final.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason = %q, want tool_calls", final.FinishReason)
	}
	if len(final.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(final.ToolCalls))
	}
	tc := final.ToolCalls[0]
	if tc.ID != "call_abc123" {
		t.Fatalf("id = %q, want call_abc123（首帧落、后续帧不覆盖）", tc.ID)
	}
	if tc.Name != "fs_read" {
		t.Fatalf("name = %q, want fs_read", tc.Name)
	}
	wantArgs := `{"path":"README.md"}`
	if tc.Arguments != wantArgs {
		t.Fatalf("arguments = %q, want %q（分片拼接）", tc.Arguments, wantArgs)
	}

	// 请求形状断言：鉴权头 + model 透传 + tools 线型。
	if v.lastAuth != "Bearer k-test" {
		t.Fatalf("Authorization = %q", v.lastAuth)
	}
	var sent map[string]any
	if err := json.Unmarshal(v.lastBody, &sent); err != nil {
		t.Fatalf("request body not json: %v", err)
	}
	if sent["model"] != "m-test" {
		t.Fatalf("model = %v, want m-test", sent["model"])
	}
	if sent["stream"] != true {
		t.Fatalf("stream = %v, want true", sent["stream"])
	}
	tools, ok := sent["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v, want 1 entry", sent["tools"])
	}
	first, _ := tools[0].(map[string]any)
	if first["type"] != "function" {
		t.Fatalf("tool type = %v, want function", first["type"])
	}
}

// TestGoldenGrokStream：xAI 方言——role+tool_calls 同帧、arguments 整段单帧、
// 终帧 content 与 finish_reason 同帧。
func TestGoldenGrokStream(t *testing.T) {
	body, err := os.ReadFile(fixturePath(t, "grok_toolcall_stream.sse"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	v := &fakeVendor{t: t, streamBody: string(body)}
	srv := httptest.NewServer(http.HandlerFunc(v.handler))
	defer srv.Close()

	c := New(srv.URL, "k-xai", ProviderGrok, srv.Client(), nil)
	ds := collect(t, c, baseRequest())

	texts, _ := textsOf(ds)
	if got := strings.Join(texts, ""); got != "已写入" {
		t.Fatalf("text = %q, want %q（终帧 content 同帧）", got, "已写入")
	}
	final := finalOf(t, ds)
	if len(final.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(final.ToolCalls))
	}
	tc := final.ToolCalls[0]
	if tc.ID != "call_grok1" || tc.Name != "fs_write" {
		t.Fatalf("tool_call = %s/%s, want call_grok1/fs_write", tc.ID, tc.Name)
	}
	wantArgs := `{"path":"a.go","content":"package a"}`
	if tc.Arguments != wantArgs {
		t.Fatalf("arguments = %q, want %q（整段单帧亦须正确聚合）", tc.Arguments, wantArgs)
	}
	if final.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason = %q", final.FinishReason)
	}
}

// TestGoldenDeepseekNonStreamFallback：流式失败（HTTP 500）→ 自动回退非流式档，
// 一遍拿全 message.tool_calls + usage 透传。
func TestGoldenDeepseekNonStreamFallback(t *testing.T) {
	body, err := os.ReadFile(fixturePath(t, "deepseek_nonstream_toolcall.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	v := &fakeVendor{t: t, streamCode: http.StatusInternalServerError, nonStream: string(body)}
	srv := httptest.NewServer(http.HandlerFunc(v.handler))
	defer srv.Close()

	c := New(srv.URL, "k-test", ProviderDeepseek, srv.Client(), nil)
	ds := collect(t, c, baseRequest())

	texts, _ := textsOf(ds)
	if got := strings.Join(texts, ""); got != "好的" {
		t.Fatalf("text = %q, want %q", got, "好的")
	}
	final := finalOf(t, ds)
	if len(final.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(final.ToolCalls))
	}
	tc := final.ToolCalls[0]
	if tc.ID != "call_def456" || tc.Name != "shell_exec" {
		t.Fatalf("tool_call = %s/%s", tc.ID, tc.Name)
	}
	if tc.Arguments != `{"command":"go test ./..."}` {
		t.Fatalf("arguments = %q", tc.Arguments)
	}
	if final.Usage == nil || final.Usage.PromptTokens != 12 || final.Usage.CompletionTokens != 34 {
		t.Fatalf("usage = %+v, want {12 34}（透传）", final.Usage)
	}
}

// TestGrokNoFallbackOnStreamFailure：Grok 无回退档——流式 HTTP 失败直接出 Err delta。
func TestGrokNoFallbackOnStreamFailure(t *testing.T) {
	v := &fakeVendor{t: t, streamCode: http.StatusInternalServerError, nonStream: `{"choices":[]}`}
	srv := httptest.NewServer(http.HandlerFunc(v.handler))
	defer srv.Close()

	c := New(srv.URL, "k-xai", ProviderGrok, srv.Client(), nil)
	ds := collect(t, c, baseRequest())
	var sawErr bool
	for _, d := range ds {
		if d.Err != nil {
			sawErr = true
			if !strings.Contains(d.Err.Error(), "500") {
				t.Fatalf("error = %v, want http 500 mention", d.Err)
			}
		}
	}
	if !sawErr {
		t.Fatal("expected error delta, got none")
	}
}

// TestNonStreamFallbackNotDoubleDelivered：流式已交付增量后再失败 → 不回退
// （防重复文本），直接报 Err。
func TestNonStreamFallbackNotDoubleDelivered(t *testing.T) {
	// 流式 fixture：先给一条文本增量，然后给一个非法 JSON chunk 触发解码错。
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"半句\"},\"finish_reason\":null}]}\n\ndata: {broken\n\n"
	v := &fakeVendor{t: t, streamBody: stream, nonStream: `{"choices":[{"message":{"content":"全集"},"role":"assistant"}],"finish_reason":"stop"}`}
	srv := httptest.NewServer(http.HandlerFunc(v.handler))
	defer srv.Close()

	c := New(srv.URL, "k-test", ProviderDeepseek, srv.Client(), nil)
	ds := collect(t, c, baseRequest())

	texts, _ := textsOf(ds)
	joined := strings.Join(texts, "")
	if strings.Contains(joined, "全集") {
		t.Fatalf("fallback fired after partial delivery: %q", joined)
	}
	if joined != "半句" {
		t.Fatalf("text = %q, want 半句", joined)
	}
	var sawErr bool
	for _, d := range ds {
		if d.Err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("expected error delta after mid-stream decode failure")
	}
}
