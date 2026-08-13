package clientbridge

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// fakeRouter records routed invoke messages and reports deliverability.
type fakeRouter struct {
	mu        sync.Mutex
	deliver   bool
	delivered []InvokeMessage
}

func (f *fakeRouter) RouteClientToolInvoke(sessionID string, msg InvokeMessage) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delivered = append(f.delivered, msg)
	return f.deliver
}

func (f *fakeRouter) last() InvokeMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.delivered) == 0 {
		return InvokeMessage{}
	}
	return f.delivered[len(f.delivered)-1]
}

// fakeAudit records audit entries.
type fakeAudit struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (f *fakeAudit) RecordClientToolAudit(_ context.Context, e AuditEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
}

func (f *fakeAudit) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.entries))
	for _, e := range f.entries {
		out = append(out, e.Action)
	}
	return out
}

// fakeFlow records flow log emissions.
type fakeFlow struct {
	mu    sync.Mutex
	steps []string
}

func (f *fakeFlow) LogFlowStart(_ context.Context, _, stepID, _ string, _ ...LogPair) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, "start:"+stepID)
}

func (f *fakeFlow) LogFlowDone(_ context.Context, _, stepID, _ string, _ ...LogPair) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, "done:"+stepID)
}

func (f *fakeFlow) LogFlowWarn(_ context.Context, _, stepID, _ string, _ ...LogPair) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, "warn:"+stepID)
}

func (f *fakeFlow) LogFlowError(_ context.Context, _, stepID, _ string, _ ...LogPair) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, "error:"+stepID)
}

func (f *fakeFlow) has(step string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.steps {
		if s == step {
			return true
		}
	}
	return false
}

func newTestBridge(timeout time.Duration) (*Bridge, *fakeRouter, *fakeAudit, *fakeFlow) {
	r := &fakeRouter{deliver: true}
	a := &fakeAudit{}
	fl := &fakeFlow{}
	b := NewBridge(Deps{
		Timeout: timeout,
		Audit:   a,
		Flow:    fl,
		LG:      loggateway.NewNoop(),
	})
	b.SetRouter(r)
	return b, r, a, fl
}

func TestBridge_Invoke_Success(t *testing.T) {
	b, r, a, fl := newTestBridge(2 * time.Second)
	args := json.RawMessage(`{"target":"wechat"}`)
	done := make(chan struct {
		InvokeResult
		error
	}, 1)
	go func() {
		res, err := b.Invoke(context.Background(), InvokeRequest{
			SessionID: "s1", UserID: "u1", Tool: "client_open_app", Args: args,
		})
		done <- struct {
			InvokeResult
			error
		}{res, err}
	}()

	// Router must receive the invoke with a registered invocation id.
	deadline := time.After(2 * time.Second)
	var invID string
	for invID == "" {
		select {
		case <-deadline:
			t.Fatal("router was not called in time")
		default:
			if m := r.last(); m.InvocationID != "" {
				invID = m.InvocationID
				if m.Tool != "client_open_app" {
					t.Fatalf("router tool = %q, want client_open_app", m.Tool)
				}
				if m.SessionID != "s1" {
					t.Fatalf("router session = %q, want s1", m.SessionID)
				}
				if string(m.Args) != string(args) {
					t.Fatalf("router args = %s, want %s", m.Args, args)
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	if ok := b.ResolveResult(invID, true, "opened", ""); !ok {
		t.Fatal("ResolveResult returned false for live invocation")
	}
	out := <-done
	if out.error != nil {
		t.Fatalf("Invoke error: %v", out.error)
	}
	if !out.InvokeResult.OK || out.InvokeResult.Output != "opened" {
		t.Fatalf("Invoke result = %+v, want ok/opened", out.InvokeResult)
	}

	// Audit: invoke + result.
	actions := a.actions()
	if len(actions) != 2 || actions[0] != AuditActionInvoke || actions[1] != AuditActionResult {
		t.Fatalf("audit actions = %v, want [invoke result]", actions)
	}
	// Flow: invoke start + result done.
	if !fl.has("start:" + StepInvoke) {
		t.Fatal("missing flow start for client_tool.invoke")
	}
	if !fl.has("done:" + StepResult) {
		t.Fatal("missing flow done for client_tool.result")
	}
	if b.PendingCount() != 0 {
		t.Fatalf("pending = %d after resolve, want 0", b.PendingCount())
	}
}

func TestBridge_Invoke_Offline_NoRouter(t *testing.T) {
	b := NewBridge(Deps{Timeout: time.Second, LG: loggateway.NewNoop()})
	_, err := b.Invoke(context.Background(), InvokeRequest{
		SessionID: "s1", Tool: "client_open_url", Args: json.RawMessage(`{"url":"https://example.com"}`),
	})
	var berr *Error
	if !errors.As(err, &berr) {
		t.Fatalf("err type = %T, want *Error", err)
	}
	if berr.Code != ErrCodeOffline {
		t.Fatalf("err code = %q, want %q", berr.Code, ErrCodeOffline)
	}
	if b.PendingCount() != 0 {
		t.Fatalf("pending = %d after offline, want 0", b.PendingCount())
	}
}

func TestBridge_Invoke_Offline_NoEligibleConn(t *testing.T) {
	b, r, a, _ := newTestBridge(time.Second)
	r.deliver = false
	_, err := b.Invoke(context.Background(), InvokeRequest{
		SessionID: "s1", Tool: "client_open_app", Args: json.RawMessage(`{"target":"x"}`),
	})
	var berr *Error
	if !errors.As(err, &berr) || berr.Code != ErrCodeOffline {
		t.Fatalf("err = %v, want offline *Error", err)
	}
	if b.PendingCount() != 0 {
		t.Fatalf("pending = %d after offline, want 0", b.PendingCount())
	}
	// Offline path must audit an offline entry (K3 degradation).
	found := false
	for _, act := range a.actions() {
		if act == AuditActionOffline {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit actions = %v, want an offline entry", a.actions())
	}
}

func TestBridge_Invoke_Timeout(t *testing.T) {
	b, _, a, fl := newTestBridge(50 * time.Millisecond)
	_, err := b.Invoke(context.Background(), InvokeRequest{
		SessionID: "s1", Tool: "client_open_app", Args: json.RawMessage(`{"target":"x"}`),
	})
	var berr *Error
	if !errors.As(err, &berr) || berr.Code != ErrCodeTimeout {
		t.Fatalf("err = %v, want timeout *Error", err)
	}
	if b.PendingCount() != 0 {
		t.Fatalf("pending = %d after timeout, want 0", b.PendingCount())
	}
	found := false
	for _, act := range a.actions() {
		if act == AuditActionTimeout {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit actions = %v, want a timeout entry", a.actions())
	}
	if !fl.has("error:" + StepTimeout) {
		t.Fatal("missing flow error for client_tool.timeout")
	}
}

func TestBridge_Invoke_EmptySession(t *testing.T) {
	b, _, _, _ := newTestBridge(time.Second)
	_, err := b.Invoke(context.Background(), InvokeRequest{Tool: "client_open_app"})
	if err == nil {
		t.Fatal("expected error for empty session id")
	}
}

func TestBridge_Invoke_ResultError(t *testing.T) {
	b, r, _, _ := newTestBridge(2 * time.Second)
	done := make(chan InvokeResult, 1)
	go func() {
		res, _ := b.Invoke(context.Background(), InvokeRequest{
			SessionID: "s1", Tool: "client_open_app", Args: json.RawMessage(`{"target":"x"}`),
		})
		done <- res
	}()
	var invID string
	deadline := time.After(2 * time.Second)
	for invID == "" {
		select {
		case <-deadline:
			t.Fatal("router was not called in time")
		default:
			invID = r.last().InvocationID
			time.Sleep(5 * time.Millisecond)
		}
	}
	if ok := b.ResolveResult(invID, false, "", "whitelist rejected"); !ok {
		t.Fatal("ResolveResult returned false")
	}
	res := <-done
	if res.OK || res.Error != "whitelist rejected" {
		t.Fatalf("result = %+v, want ok=false with error text", res)
	}
}

func TestBridge_ResolveResult_Unknown(t *testing.T) {
	b, _, _, _ := newTestBridge(time.Second)
	if b.ResolveResult("no-such-id", true, "", "") {
		t.Fatal("ResolveResult should return false for unknown invocation")
	}
}

func TestBridge_ResolveResult_AfterTimeout(t *testing.T) {
	b, r, _, _ := newTestBridge(30 * time.Millisecond)
	done := make(chan error, 1)
	go func() {
		_, err := b.Invoke(context.Background(), InvokeRequest{
			SessionID: "s1", Tool: "client_open_app", Args: json.RawMessage(`{"target":"x"}`),
		})
		done <- err
	}()
	<-done // timed out
	if b.ResolveResult(r.last().InvocationID, true, "late", "") {
		t.Fatal("ResolveResult should return false after timeout removed pending")
	}
}

func TestBridge_Invoke_ConcurrentIndependent(t *testing.T) {
	b, r, _, _ := newTestBridge(2 * time.Second)
	const n = 8
	var wg sync.WaitGroup
	results := make(chan InvokeResult, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := b.Invoke(context.Background(), InvokeRequest{
				SessionID: "s1", Tool: "client_open_url", Args: json.RawMessage(`{"url":"https://example.com"}`),
			})
			if err != nil {
				t.Errorf("concurrent invoke err: %v", err)
				return
			}
			results <- res
		}()
	}
	// Resolve each pending invocation once its id appears at the router.
	seen := map[string]bool{}
	for len(seen) < n {
		r.mu.Lock()
		msgs := append([]InvokeMessage(nil), r.delivered...)
		r.mu.Unlock()
		for _, m := range msgs {
			if seen[m.InvocationID] {
				continue
			}
			seen[m.InvocationID] = true
			if !b.ResolveResult(m.InvocationID, true, "ok:"+m.InvocationID, "") {
				t.Errorf("resolve failed for %s", m.InvocationID)
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	wg.Wait()
	close(results)
	count := 0
	for res := range results {
		count++
		if !res.OK {
			t.Errorf("result not ok: %+v", res)
		}
	}
	if count != n {
		t.Fatalf("resolved %d/%d invocations", count, n)
	}
	if b.PendingCount() != 0 {
		t.Fatalf("pending = %d, want 0", b.PendingCount())
	}
}
