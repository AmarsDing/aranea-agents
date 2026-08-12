package computeruse

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	bizcu "aranea-agents/internal/biz/computeruse"
)

// --- fake gateway（实现 biz DeviceGateway 组合接口） ---

type fakeGW struct {
	snap    bizcu.Snapshot
	invoked string
}

func (f *fakeGW) Info(context.Context) (bizcu.DeviceInfo, error) {
	return bizcu.DeviceInfo{Platform: "windows", ScreenW: 1920, ScreenH: 1080, ScaleFactor: 1}, nil
}

func (f *fakeGW) Snapshot(context.Context, bizcu.SnapshotOpts) (bizcu.Snapshot, error) { return f.snap, nil }

func (f *fakeGW) Screenshot(context.Context, *bizcu.Rect, float64) (bizcu.Image, error) {
	return bizcu.Image{PNG: []byte("png-bytes"), Width: 800, Height: 600, ScaleFactor: 1.5}, nil
}

func (f *fakeGW) Invoke(_ context.Context, ref string, _ int) error { f.invoked = ref; return nil }
func (f *fakeGW) Click(context.Context, bizcu.Point, string, int) error { return nil }
func (f *fakeGW) TypeText(context.Context, string) error                 { return nil }
func (f *fakeGW) Key(context.Context, string) error                      { return nil }
func (f *fakeGW) FocusWindow(context.Context, string) error              { return nil }
func (f *fakeGW) Launch(context.Context, string, string, string) (int, error) {
	return 4321, nil
}
func (f *fakeGW) ListWindows(context.Context) ([]bizcu.WindowInfo, error) { return nil, nil }

type hitMatcher struct{ el *bizcu.UIElement }

func (m hitMatcher) Match([]bizcu.UIElement, string) *bizcu.UIElement { return m.el }

func newTestToolset(t *testing.T) (*fakeGW, []interface {
	Declaration() interface{ Name() }
}) {
	t.Helper()
	return nil, nil
}

func buildUC(gw *fakeGW, hit *bizcu.UIElement) *bizcu.ComputerUseUsecase {
	return bizcu.NewComputerUseUsecase(bizcu.Deps{
		Gateway: gw,
		Match:   hitMatcher{el: hit},
		Now:     time.Now,
	})
}

func findTool(tools []interface {
	Call(context.Context, []byte) (any, error)
	Decl() string
}, name string) int {
	for i, tl := range tools {
		if tl.Decl() == name {
			return i
		}
	}
	return -1
}

func TestNewToolset_NilUsecaseReturnsNil(t *testing.T) {
	if got := NewToolset(nil); got != nil {
		t.Errorf("NewToolset(nil) = %v, want nil", got)
	}
}

func TestToolset_Declarations(t *testing.T) {
	saveBtn := &bizcu.UIElement{Ref: "g1.e1", Name: "保存", Interactivity: true, Enabled: true}
	uc := buildUC(&fakeGW{}, saveBtn)
	tools := NewToolset(uc)
	if len(tools) != 5 {
		t.Fatalf("tools count = %d, want 5", len(tools))
	}
	want := map[string]bool{
		ToolObserve: false, ToolScreenshot: false, ToolAct: false, ToolLaunch: false, ToolSession: false,
	}
	for _, tl := range tools {
		d := tl.Declaration()
		if d == nil || d.InputSchema == nil {
			t.Errorf("tool missing declaration/schema")
			continue
		}
		if _, ok := want[d.Name]; !ok {
			t.Errorf("unexpected tool %q", d.Name)
			continue
		}
		want[d.Name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestActTool_EndToEnd(t *testing.T) {
	saveBtn := &bizcu.UIElement{
		Ref: "g1.e3", Name: "保存", Type: "button",
		BBox: bizcu.Rect{X: 10, Y: 20, W: 50, H: 20},
		Interactivity: true, Enabled: true,
	}
	gw := &fakeGW{snap: bizcu.Snapshot{Elements: []bizcu.UIElement{*saveBtn}, Generation: 1}}
	uc := buildUC(gw, saveBtn)
	tools := NewToolset(uc)

	var act interface {
		Call(context.Context, []byte) (any, error)
	}
	for _, tl := range tools {
		if tl.Declaration().Name == ToolAct {
			act = tl
		}
	}
	if act == nil {
		t.Fatal("act tool not found")
	}
	out, err := act.Call(context.Background(), []byte(`{"target":"保存","action":"invoke"}`))
	if err != nil {
		t.Fatalf("call err: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("result type %T", out)
	}
	if m["result"] != bizcu.StepOK {
		t.Errorf("result = %v", m["result"])
	}
	if gw.invoked != "g1.e3" {
		t.Errorf("invoked = %q", gw.invoked)
	}
	if m["session_id"] == "" {
		t.Error("session_id should be auto-created")
	}
}

func TestSessionTool_StartStop(t *testing.T) {
	uc := buildUC(&fakeGW{}, nil)
	tools := NewToolset(uc)
	var sess interface {
		Call(context.Context, []byte) (any, error)
	}
	for _, tl := range tools {
		if tl.Declaration().Name == ToolSession {
			sess = tl
		}
	}
	out, err := sess.Call(context.Background(), []byte(`{"action":"start","max_steps":5,"duration_minutes":10}`))
	if err != nil {
		t.Fatalf("start err: %v", err)
	}
	m := out.(map[string]any)
	sid, _ := m["session_id"].(string)
	if sid == "" {
		t.Fatal("session_id empty")
	}
	if m["max_steps"] != 5 {
		t.Errorf("max_steps = %v", m["max_steps"])
	}

	out, err = sess.Call(context.Background(), []byte(`{"action":"stop","session_id":"`+sid+`"}`))
	if err != nil {
		t.Fatalf("stop err: %v", err)
	}
	if out.(map[string]any)["status"] != bizcu.SessionDone {
		t.Errorf("status = %v", out.(map[string]any)["status"])
	}
}

func TestObserveTool_ReturnsSummary(t *testing.T) {
	saveBtn := bizcu.UIElement{Ref: "g1.e1", Name: "文件", Type: "menuitem", Interactivity: true, Enabled: true}
	gw := &fakeGW{snap: bizcu.Snapshot{Elements: []bizcu.UIElement{saveBtn}, Generation: 3}}
	uc := buildUC(gw, nil)
	tools := NewToolset(uc)
	var obs interface {
		Call(context.Context, []byte) (any, error)
	}
	for _, tl := range tools {
		if tl.Declaration().Name == ToolObserve {
			obs = tl
		}
	}
	out, err := obs.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("observe err: %v", err)
	}
	m := out.(map[string]any)
	if m["element_count"] != 1 || m["generation"] != 3 {
		t.Errorf("out = %+v", m)
	}
	if !strings.Contains(m["summary"].(string), "文件") {
		t.Errorf("summary missing element name: %v", m["summary"])
	}
}

func TestScreenshotTool_Base64(t *testing.T) {
	uc := buildUC(&fakeGW{}, nil)
	tools := NewToolset(uc)
	var shot interface {
		Call(context.Context, []byte) (any, error)
	}
	for _, tl := range tools {
		if tl.Declaration().Name == ToolScreenshot {
			shot = tl
		}
	}
	out, err := shot.Call(context.Background(), []byte(`{"zoom":2}`))
	if err != nil {
		t.Fatalf("screenshot err: %v", err)
	}
	m := out.(map[string]any)
	if m["width"] != 800 || m["scale_factor"] != 1.5 {
		t.Errorf("out = %+v", m)
	}
	if b64, _ := m["png_base64"].(string); b64 == "" {
		t.Error("png_base64 empty")
	} else {
		var raw []byte
		if err := json.Unmarshal([]byte(`"`+b64+`"`), &raw); err == nil {
			_ = raw
		}
	}
}

func TestLaunchTool_RequiresTarget(t *testing.T) {
	uc := buildUC(&fakeGW{}, nil)
	tools := NewToolset(uc)
	var launch interface {
		Call(context.Context, []byte) (any, error)
	}
	for _, tl := range tools {
		if tl.Declaration().Name == ToolLaunch {
			launch = tl
		}
	}
	if _, err := launch.Call(context.Background(), []byte(`{"target":""}`)); err == nil {
		t.Error("empty target should fail")
	}
	out, err := launch.Call(context.Background(), []byte(`{"target":"notepad.exe"}`))
	if err != nil {
		t.Fatalf("launch err: %v", err)
	}
	if out.(map[string]any)["pid"] != 4321 {
		t.Errorf("pid = %v", out.(map[string]any)["pid"])
	}
}
