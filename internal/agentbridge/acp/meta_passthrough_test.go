package acp

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestCodebuddyMetaPassthrough M4 P0-5 fixture：codebuddy 厂商扩展 `_meta`
// 双位置（params 根 + update 内）透传不丢；未知 update kind/未知 meta key 容忍。
// fixture：../acpshim/testdata/codebuddy_meta_degradation.ndjson
func TestCodebuddyMetaPassthrough(t *testing.T) {
	f, err := os.Open("../acpshim/testdata/codebuddy_meta_degradation.ndjson")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	type wireFrame struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}

	var notes []SessionNotification
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var wf wireFrame
		if err := json.Unmarshal([]byte(line), &wf); err != nil {
			t.Fatalf("line not json-rpc: %v", err)
		}
		if wf.Method != MethodSessionUpdate {
			t.Fatalf("method = %q, want %s", wf.Method, MethodSessionUpdate)
		}
		var n SessionNotification
		if err := json.Unmarshal(wf.Params, &n); err != nil {
			t.Fatalf("params decode: %v", err)
		}
		notes = append(notes, n)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(notes) != 3 {
		t.Fatalf("frames = %d, want 3", len(notes))
	}

	// 帧 1：params 根 _meta（{"codebuddy":{"v":1}}）+ update 内 _meta（partial/seq/trace）。
	n1 := notes[0]
	if n1.SessionID != "s-cb1" {
		t.Fatalf("sessionId = %q", n1.SessionID)
	}
	assertMetaKey(t, n1.Meta, "codebuddy", `"v":1`)
	if n1.Update.Kind != "agent_message_chunk" || n1.Update.Content == nil || n1.Update.Content.Text != "部分输出" {
		t.Fatalf("frame1 update = %+v", n1.Update)
	}
	assertMetaKey(t, n1.Update.Meta, "codebuddy", `"seq":7`)

	// 帧 2：tool_call kind=edit（本系统未用的 kind 取值）——解析不报错、
	// Title/ToolCallID 照常，_meta 的 workspace_uri/diff_patch 原样在岗。
	n2 := notes[1]
	if n2.Update.Kind != "tool_call" || n2.Update.ToolCallID != "tc1" || n2.Update.Title != "fs_write" {
		t.Fatalf("frame2 update = %+v", n2.Update)
	}
	assertMetaKey(t, n2.Update.Meta, "codebuddy", `"diff_patch":true`)

	// 帧 3：final 序号透传。
	n3 := notes[2]
	assertMetaKey(t, n3.Update.Meta, "codebuddy", `"final":true`)
}

// assertMetaKey 断言 _meta 顶层 key 存在且其 JSON 文本包含 want。
func assertMetaKey(t *testing.T, raw json.RawMessage, key, want string) {
	t.Helper()
	if len(raw) == 0 {
		t.Fatalf("meta empty, want key %q", key)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("meta not object: %v", err)
	}
	v, ok := m[key]
	if !ok {
		t.Fatalf("meta keys = %v, want %q", keysOf(m), key)
	}
	if !strings.Contains(string(v), want) {
		t.Fatalf("meta[%q] = %s, want containing %s", key, string(v), want)
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
