package browser

import (
	"context"
	"sync/atomic"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestWrapSession_StampsNextToolAfterNavigate(t *testing.T) {
	inner := &stubBrowserSet{tools: []trpctool.Tool{
		&stubBrowserTool{name: "browser_navigate", result: "ok"},
		&stubBrowserTool{name: "browser_snapshot", result: "tree"},
	}}
	ts := WrapSession(inner)
	var nav trpctool.CallableTool
	for _, t0 := range ts.Tools(context.Background()) {
		if t0.Declaration().Name == "browser_navigate" {
			nav = t0.(trpctool.CallableTool)
		}
	}
	out, err := nav.Call(context.Background(), []byte(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("type %T", out)
	}
	if m["next_tool"] != "browser_snapshot" {
		t.Fatalf("%v", m)
	}
}

func TestWrapSession_SnapshotHasNoNextTool(t *testing.T) {
	inner := &stubBrowserSet{tools: []trpctool.Tool{
		&stubBrowserTool{name: "browser_snapshot", result: "tree"},
	}}
	ts := WrapSession(inner)
	ct := ts.Tools(context.Background())[0].(trpctool.CallableTool)
	out, err := ct.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out.(string); !ok {
		t.Fatalf("snapshot must stay string, got %T", out)
	}
}

func TestWrapSession_SerializesCalls(t *testing.T) {
	var concurrent int32
	var max int32
	inner := &stubBrowserSet{tools: []trpctool.Tool{
		&stubBrowserTool{
			name:   "browser_click",
			result: "clicked",
			during: func() {
				n := atomic.AddInt32(&concurrent, 1)
				for {
					old := atomic.LoadInt32(&max)
					if n <= old || atomic.CompareAndSwapInt32(&max, old, n) {
						break
					}
				}
				atomic.AddInt32(&concurrent, -1)
			},
		},
	}}
	ts := WrapSession(inner)
	ct := ts.Tools(context.Background())[0].(trpctool.CallableTool)
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, _ = ct.Call(context.Background(), []byte(`{}`))
			done <- struct{}{}
		}()
	}
	<-done
	<-done
	if atomic.LoadInt32(&max) != 1 {
		t.Fatalf("max concurrent=%d", max)
	}
}

type stubBrowserSet struct{ tools []trpctool.Tool }

func (s *stubBrowserSet) Tools(context.Context) []trpctool.Tool { return s.tools }
func (s *stubBrowserSet) Close() error                          { return nil }
func (s *stubBrowserSet) Name() string                          { return "browser" }

type stubBrowserTool struct {
	name   string
	result any
	during func()
}

func (s *stubBrowserTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: s.name, Description: s.name}
}
func (s *stubBrowserTool) Call(context.Context, []byte) (any, error) {
	if s.during != nil {
		s.during()
	}
	return s.result, nil
}
