package server

import (
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/conf"
)

// TestRedactGlobalMonitorFrame pins S-7 field-level degradation: content
// fields (user text, LLM output, tool args/results, errors) must be stripped
// for global monitor connections while id/status/seq metadata survives.
func TestRedactGlobalMonitorFrame(t *testing.T) {
	env := map[string]any{
		"type":       "v2_event",
		"kind":       "step.updated",
		"session_id": "sess-1",
		"event_id":   "ev-9",
		"payload": map[string]any{
			"Step": map[string]any{
				"ID":         "step-1",
				"Status":     "running",
				"Seq":        7,
				"Content":    "secret llm output",
				"Reasoning":  "secret chain",
				"ToolName":   "shell",
				"ToolArgs":   json.RawMessage(`{"cmd":"rm -rf"}`),
				"ToolResult": json.RawMessage(`{"out":"leak"}`),
			},
		},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	out := redactGlobalMonitorFrame(raw)
	if out == nil {
		t.Fatal("expected redacted frame, got nil")
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["kind"] != "step.updated" || decoded["session_id"] != "sess-1" {
		t.Fatalf("envelope metadata must survive: %v", decoded)
	}
	step := decoded["payload"].(map[string]any)["Step"].(map[string]any)
	for _, deny := range []string{"Content", "Reasoning", "ToolArgs", "ToolResult"} {
		if _, hit := step[deny]; hit {
			t.Fatalf("denylisted key %s must be stripped, frame=%s", deny, out)
		}
	}
	if step["ID"] != "step-1" || step["Status"] != "running" {
		t.Fatalf("id/status metadata must survive: %v", step)
	}
}

// TestRedactGlobalMonitorFrameNested pins recursive stripping inside nested
// member reports (plan step Result / FailedMember paths).
func TestRedactGlobalMonitorFrameNested(t *testing.T) {
	env := map[string]any{
		"type": "v2_event",
		"kind": "plan_step.completed",
		"payload": map[string]any{
			"PlanStep": map[string]any{
				"ID":     "ps-1",
				"Status": "completed",
				"Result": map[string]any{
					"Output": "full report text",
					"MemberReports": []any{
						map[string]any{"AgentKey": "a1", "Output": "member text", "Error": "boom with user text"},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(env)
	out := redactGlobalMonitorFrame(raw)
	if out == nil {
		t.Fatal("expected redacted frame")
	}
	s := string(out)
	for _, deny := range []string{"full report text", "member text", "boom with user text"} {
		if strings.Contains(s, deny) {
			t.Fatalf("nested content leaked: %s", s)
		}
	}
	if !strings.Contains(s, `"ps-1"`) {
		t.Fatalf("step id must survive: %s", s)
	}
}

// TestRedactGlobalMonitorFrameFailClosed pins that unparseable frames are
// dropped (nil) instead of fanned out raw to the global audience.
func TestRedactGlobalMonitorFrameFailClosed(t *testing.T) {
	if out := redactGlobalMonitorFrame([]byte("not-json")); out != nil {
		t.Fatalf("malformed frame must be dropped, got %s", out)
	}
	// Payload-free control frames pass through untouched (nothing to strip).
	plain := []byte(`{"type":"ping"}`)
	if out := redactGlobalMonitorFrame(plain); string(out) != string(plain) {
		t.Fatalf("payload-free frame must pass through, got %s", out)
	}
}

// TestBroadcastToSessionDegradesGlobalMonitor pins the S-7 fanout split: the
// dedicated session connection receives the full frame while the global
// (session_id=*) monitor connection receives the content-degraded copy.
func TestBroadcastToSessionDegradesGlobalMonitor(t *testing.T) {
	srv := newTestWSServer(nil, nil)
	mk := func() *wsConn {
		return &wsConn{
			channels: map[string]bool{"chat": true},
			send:     make(chan []byte, 4),
			queues:   newConnQueues(conf.RuntimeWSConfig{}),
		}
	}
	sessConn := mk()
	globalConn := mk()
	srv.store.add("sess-1", sessConn)
	srv.store.add("*", globalConn)

	frame := []byte(`{"type":"v2_event","kind":"task.updated","session_id":"sess-1","payload":{"Task":{"ID":"t1","Status":"running","UserMessage":"top secret user text"}}}`)
	srv.BroadcastToSession("sess-1", frame)

	select {
	case got := <-sessConn.queues.normal:
		if !strings.Contains(string(got), "top secret user text") {
			t.Fatalf("session conn must receive the full frame, got %s", got)
		}
	default:
		t.Fatal("session conn received nothing")
	}

	select {
	case got := <-globalConn.queues.normal:
		if strings.Contains(string(got), "top secret user text") {
			t.Fatalf("global conn must not receive user content, got %s", got)
		}
		if !strings.Contains(string(got), `"t1"`) || !strings.Contains(string(got), `"task.updated"`) {
			t.Fatalf("global conn must still receive metadata, got %s", got)
		}
	default:
		t.Fatal("global conn received nothing — degrade must not drop the frame")
	}
}
