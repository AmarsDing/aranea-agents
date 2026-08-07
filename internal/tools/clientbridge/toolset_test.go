package clientbridge

import (
	"context"
	"errors"
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// fakeInvoker captures the request the toolset hands to the bridge.
type fakeInvoker struct {
	req InvokeRequest
	res InvokeResult
	err error
}

func (f *fakeInvoker) Invoke(_ context.Context, req InvokeRequest) (InvokeResult, error) {
	f.req = req
	return f.res, f.err
}

func invocationCtx(userID, sessionID string) context.Context {
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{AppName: "app", UserID: userID, ID: sessionID},
	}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

func TestToolSet_NameAndMembers(t *testing.T) {
	ts := NewToolSet(&fakeInvoker{})
	if ts.Name() != "client" {
		t.Fatalf("toolset name = %q, want client", ts.Name())
	}
	tools := ts.Tools(context.Background())
	if len(tools) != 2 {
		t.Fatalf("tools = %d, want 2 (open_app/open_url)", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Declaration().Name] = true
	}
	if !names["open_app"] || !names["open_url"] {
		t.Fatalf("member names = %v, want open_app + open_url", names)
	}
}

func TestClientTool_Call_NoInvocation(t *testing.T) {
	ts := NewToolSet(&fakeInvoker{})
	tl := ts.Tools(context.Background())[0]
	if _, err := tl.(interface {
		Call(context.Context, []byte) (any, error)
	}).Call(context.Background(), []byte(`{"target":"wechat"}`)); err == nil {
		t.Fatal("expected error when invocation context is absent")
	}
}

func TestClientTool_Call_SuccessRoutesToBridge(t *testing.T) {
	inv := &fakeInvoker{res: InvokeResult{OK: true, Output: "opened"}}
	ts := NewToolSet(inv)
	var callable interface {
		Call(context.Context, []byte) (any, error)
	}
	for _, tl := range ts.Tools(context.Background()) {
		if tl.Declaration().Name == "open_app" {
			callable = tl.(interface {
				Call(context.Context, []byte) (any, error)
			})
		}
	}
	if callable == nil {
		t.Fatal("open_app tool not found")
	}
	res, err := callable.Call(invocationCtx("u1", "s1"), []byte(`{"target":"wechat"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if inv.req.SessionID != "s1" || inv.req.UserID != "u1" {
		t.Fatalf("req session/user = %q/%q, want s1/u1", inv.req.SessionID, inv.req.UserID)
	}
	if inv.req.Tool != ToolOpenApp {
		t.Fatalf("req tool = %q, want %q", inv.req.Tool, ToolOpenApp)
	}
	if string(inv.req.Args) != `{"target":"wechat"}` {
		t.Fatalf("req args = %s", inv.req.Args)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map envelope", res)
	}
	if okFlag, _ := m["ok"].(bool); !okFlag {
		t.Fatalf("result = %v, want ok=true", m)
	}
	if out, _ := m["output"].(string); out != "opened" {
		t.Fatalf("output = %v, want opened", m["output"])
	}
}

func TestClientTool_Call_BridgeErrorBecomesStructuredEnvelope(t *testing.T) {
	inv := &fakeInvoker{err: &Error{Code: ErrCodeOffline, Message: "no eligible desktop companion connection for session"}}
	ts := NewToolSet(inv)
	var callable interface {
		Call(context.Context, []byte) (any, error)
	}
	for _, tl := range ts.Tools(context.Background()) {
		if tl.Declaration().Name == "open_url" {
			callable = tl.(interface {
				Call(context.Context, []byte) (any, error)
			})
		}
	}
	// Bridge-level failures must surface as a structured envelope (not a Go
	// error) so the Agent can paraphrase the machine-readable code.
	res, err := callable.Call(invocationCtx("u1", "s1"), []byte(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatalf("Call err = %v, want structured envelope instead", err)
	}
	m := res.(map[string]any)
	if okFlag, _ := m["ok"].(bool); okFlag {
		t.Fatalf("result = %v, want ok=false", m)
	}
	if code, _ := m["error_code"].(string); code != ErrCodeOffline {
		t.Fatalf("error_code = %v, want %q", m["error_code"], ErrCodeOffline)
	}
}

func TestClientTool_Call_ClientFailurePropagates(t *testing.T) {
	inv := &fakeInvoker{res: InvokeResult{OK: false, Error: "whitelist rejected"}}
	ts := NewToolSet(inv)
	callable := ts.Tools(context.Background())[0].(interface {
		Call(context.Context, []byte) (any, error)
	})
	res, err := callable.Call(invocationCtx("u1", "s1"), []byte(`{"target":"x"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	m := res.(map[string]any)
	if okFlag, _ := m["ok"].(bool); okFlag {
		t.Fatalf("result = %v, want ok=false", m)
	}
	if e, _ := m["error"].(string); e != "whitelist rejected" {
		t.Fatalf("error = %v, want whitelist rejected", m["error"])
	}
}

func TestClientTool_Call_NonBridgeErrorPropagates(t *testing.T) {
	inv := &fakeInvoker{err: errors.New("boom")}
	ts := NewToolSet(inv)
	callable := ts.Tools(context.Background())[0].(interface {
		Call(context.Context, []byte) (any, error)
	})
	if _, err := callable.Call(invocationCtx("u1", "s1"), []byte(`{}`)); err == nil {
		t.Fatal("non-bridge errors must propagate as Go errors")
	}
}
