package acpshim

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// sentinel_test.go — 协议演进哨兵（P1-8，设计 §E-guard）：
// 对 ACP v1 线型做 golden 比对。ACP 协议/我方线型一有漂移，
// 本测试即红——合并门禁据此拦截「静默偏离协议」的改动。
// golden 变更必须伴随设计文档 §D 不变量修订+评审，禁止随手更新基准。

// TestSentinel_InitializeResponseGolden initialize 响应线型钉版：
// protocolVersion=1 + agentCapabilities 空对象（不虚报 loadSession/fs/terminal——§D-6）。
func TestSentinel_InitializeResponseGolden(t *testing.T) {
	llm := &fakeLLM{script: func(int, []ChatMessage) []ChatDelta { return nil }}
	h := newHostHarness(t, mustServer(t, llm, &fakeTools{}, &fakeApprover{}))

	h.sendRaw(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":false,"writeTextFile":false},"terminal":false}}}`)
	resp := h.readFrame()

	got := normalizeJSON(t, resp.Result)
	want := normalizeJSONStr(t, `{"protocolVersion":1,"agentCapabilities":{}}`)
	if got != want {
		t.Fatalf("initialize result drift:\n got = %s\nwant = %s", got, want)
	}
}

// TestSentinel_PermissionRequestGolden request_permission 线型钉版：
// title 逐字参数、kind、options 四元全集顺序、rawInput 原样。
func TestSentinel_PermissionRequestGolden(t *testing.T) {
	fr := &fakeRouter{}
	c, _, hostR := testPipe(t, fr)
	defer c.Close()

	broker := NewBroker(nil, time.Minute, nil)
	broker.BindConn(c)

	go func() {
		_ = broker.Request(context.Background(), "s1", ToolCallRequest{ID: "tc-golden", Name: "fs_write", Arguments: `{"path":"a.go","content":"x"}`})
	}()
	req := readFrame(t, hostR)

	got := normalizeJSON(t, req.Params)
	want := normalizeJSONStr(t, `{
		"sessionId": "s1",
		"toolCall": {
			"toolCallId": "tc-golden",
			"title": "fs_write {\"path\":\"a.go\",\"content\":\"x\"}",
			"kind": "edit",
			"rawInput": {"path":"a.go","content":"x"}
		},
		"options": [
			{"optionId":"allow_once","name":"允许一次","kind":"allow_once"},
			{"optionId":"allow_always","name":"总是允许","kind":"allow_always"},
			{"optionId":"reject_once","name":"拒绝一次","kind":"reject_once"},
			{"optionId":"reject_always","name":"总是拒绝","kind":"reject_always"}
		]
	}`)
	if got != want {
		t.Fatalf("request_permission params drift:\n got = %s\nwant = %s", got, want)
	}
}

// TestSentinel_SessionUpdateGolden agent_message_chunk 更新线型钉版。
func TestSentinel_SessionUpdateGolden(t *testing.T) {
	llm := &fakeLLM{script: func(call int, msgs []ChatMessage) []ChatDelta {
		return []ChatDelta{{Text: "钉版"}, {Done: true}}
	}}
	h := newHostHarness(t, mustServer(t, llm, &fakeTools{}, &fakeApprover{}))
	sid := newSession(t, h)

	h.sendRaw(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sid + `","prompt":[{"type":"text","text":"hi"}]}}`)
	_, notes := h.readUntilResponse("3")

	var chunkParams json.RawMessage
	for _, n := range notes {
		if n.Method != "session/update" {
			continue
		}
		var probe struct {
			Update struct {
				Kind string `json:"sessionUpdate"`
			} `json:"update"`
		}
		if json.Unmarshal(n.Params, &probe) == nil && probe.Update.Kind == "agent_message_chunk" {
			chunkParams = n.Params
			break
		}
	}
	if chunkParams == nil {
		t.Fatal("no agent_message_chunk update captured")
	}
	got := normalizeJSON(t, chunkParams)
	want := normalizeJSONStr(t, `{
		"sessionId": "`+sid+`",
		"update": {"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"钉版"}}
	}`)
	if got != want {
		t.Fatalf("session/update drift:\n got = %s\nwant = %s", got, want)
	}
}

// normalizeJSON 语义归一（键序/空白非契约）后比较——线型契约是 JSON 语义级，
// 不是字节级。中文等非 ASCII 两侧同经 marshal 转义，比较一致。
func normalizeJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("remarshal: %v", err)
	}
	return string(data)
}

func normalizeJSONStr(t *testing.T, s string) string {
	t.Helper()
	return normalizeJSON(t, json.RawMessage(s))
}
