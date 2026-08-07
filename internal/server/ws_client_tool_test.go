package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/tools/clientbridge"
	"aranea-agents/pkg/loggateway"
)

func newClientToolTestConn(sessionID string, caps []string) *wsConn {
	wc := &wsConn{
		sessionID: sessionID,
		userID:    "u1",
		channels:  map[string]bool{"system": true},
		send:      make(chan []byte, 4),
		queues:    newConnQueues(conf.RuntimeWSConfig{}),
	}
	if caps != nil {
		wc.setCapabilities(caps)
	}
	return wc
}

func TestWSRegisterCapabilities(t *testing.T) {
	srv := newTestWSServer(nil, nil)
	wc := newClientToolTestConn("sess-1", nil)

	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "register_capabilities",
		Payload:   map[string]any{"capabilities": []any{"desktop_companion"}},
	})
	srv.handleUpstream(wc, raw)

	if !wc.hasCapability(CapabilityDesktopCompanion) {
		t.Fatal("expected desktop_companion capability registered")
	}
	if wc.hasCapability("something_else") {
		t.Fatal("unexpected capability present")
	}
}

func TestWSRouteClientToolInvokeCapabilityFilter(t *testing.T) {
	srv := newTestWSServer(nil, nil)
	capable := newClientToolTestConn("sess-1", []string{CapabilityDesktopCompanion})
	plain := newClientToolTestConn("sess-1", nil)
	srv.store.add("sess-1", capable)
	srv.store.add("sess-1", plain)

	delivered := srv.RouteClientToolInvoke("sess-1", clientbridge.InvokeMessage{
		InvocationID: "inv-1",
		SessionID:    "sess-1",
		UserID:       "u1",
		Tool:         clientbridge.ToolOpenURL,
		Args:         []byte(`{"url":"https://example.com"}`),
	})
	if !delivered {
		t.Fatal("expected delivery to the capable connection")
	}

	select {
	case rawOut := <-capable.queues.normal:
		var down wsDownstream
		if err := json.Unmarshal(rawOut, &down); err != nil {
			t.Fatal(err)
		}
		if down.Type != "client_tool.invoke" || down.Direction != "server_to_client" {
			t.Fatalf("unexpected downstream: %+v", down)
		}
		payload, ok := down.Payload.(map[string]any)
		if !ok {
			t.Fatalf("payload is not an object: %T", down.Payload)
		}
		if payload["invocation_id"] != "inv-1" {
			t.Fatalf("unexpected invocation_id: %v", payload["invocation_id"])
		}
		if payload["tool"] != clientbridge.ToolOpenURL {
			t.Fatalf("unexpected tool: %v", payload["tool"])
		}
		if payload["session_id"] != "sess-1" {
			t.Fatalf("unexpected session_id: %v", payload["session_id"])
		}
		args, ok := payload["args"].(map[string]any)
		if !ok || args["url"] != "https://example.com" {
			t.Fatalf("args not forwarded verbatim: %v", payload["args"])
		}
	case <-time.After(time.Second):
		t.Fatal("expected invoke frame on capable connection")
	}

	select {
	case rawOut := <-plain.queues.normal:
		t.Fatalf("non-capable connection must not receive invoke, got %s", string(rawOut))
	default:
	}
}

func TestWSRouteClientToolInvokeNoEligibleConn(t *testing.T) {
	srv := newTestWSServer(nil, nil)
	plain := newClientToolTestConn("sess-1", nil)
	srv.store.add("sess-1", plain)

	if srv.RouteClientToolInvoke("sess-1", clientbridge.InvokeMessage{
		InvocationID: "inv-x",
		SessionID:    "sess-1",
		Tool:         clientbridge.ToolOpenApp,
	}) {
		t.Fatal("expected false when no capable connection exists")
	}
}

func TestWSClientToolResultRoundTrip(t *testing.T) {
	bridge := clientbridge.NewBridge(clientbridge.Deps{
		Timeout: 2 * time.Second,
		LG:      loggateway.NewNoop(),
	})
	srv := newTestWSServer(nil, nil)
	srv.SetClientToolBridge(bridge)

	wc := newClientToolTestConn("sess-1", []string{CapabilityDesktopCompanion})
	srv.store.add("sess-1", wc)

	type outcome struct {
		res clientbridge.InvokeResult
		err error
	}
	outCh := make(chan outcome, 1)
	go func() {
		res, err := bridge.Invoke(context.Background(), clientbridge.InvokeRequest{
			SessionID: "sess-1",
			UserID:    "u1",
			Tool:      clientbridge.ToolOpenURL,
			Args:      []byte(`{"url":"https://example.com"}`),
		})
		outCh <- outcome{res, err}
	}()

	var invocationID string
	select {
	case rawOut := <-wc.queues.normal:
		var down wsDownstream
		if err := json.Unmarshal(rawOut, &down); err != nil {
			t.Fatal(err)
		}
		payload, _ := down.Payload.(map[string]any)
		invocationID, _ = payload["invocation_id"].(string)
	case <-time.After(time.Second):
		t.Fatal("expected invoke frame")
	}
	if invocationID == "" {
		t.Fatal("invoke frame missing invocation_id")
	}

	resultRaw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "client_tool.result",
		Payload: map[string]any{
			"invocation_id": invocationID,
			"ok":            true,
			"output":        "opened",
		},
	})
	srv.handleUpstream(wc, resultRaw)

	select {
	case out := <-outCh:
		if out.err != nil {
			t.Fatalf("invoke returned error: %v", out.err)
		}
		if !out.res.OK || out.res.Output != "opened" {
			t.Fatalf("unexpected result: %+v", out.res)
		}
	case <-time.After(time.Second):
		t.Fatal("invoke did not resolve after client_tool.result")
	}
}

func TestWSClientToolResultUnknownInvocationDropped(t *testing.T) {
	bridge := clientbridge.NewBridge(clientbridge.Deps{LG: loggateway.NewNoop()})
	srv := newTestWSServer(nil, nil)
	srv.SetClientToolBridge(bridge)
	wc := newClientToolTestConn("sess-1", []string{CapabilityDesktopCompanion})

	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "client_tool.result",
		Payload:   map[string]any{"invocation_id": "no-such-inv", "ok": true},
	})
	// Must not panic; the frame is silently dropped (unknown/stale invocation).
	srv.handleUpstream(wc, raw)

	if bridge.PendingCount() != 0 {
		t.Fatalf("expected 0 pending invocations, got %d", bridge.PendingCount())
	}
}
