package computeruse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	bizcomputeruse "aranea-agents/internal/biz/computeruse"
	"aranea-agents/pkg/loggateway"
)

// newTestGateway fake sidecar 上的 Gateway；received 收集所有请求帧供断言。
func newTestGateway(t *testing.T, handler sidecarHandler) (*Gateway, chan rpcRequest) {
	t.Helper()
	received := make(chan rpcRequest, 64)
	st, _ := fakeStarter(func(req rpcRequest) (any, *rpcError) {
		received <- req
		return handler(req)
	})
	m := NewManager("fake-sidecar.exe", loggateway.NewNoop())
	m.starter = st
	t.Cleanup(func() { _ = m.Stop(context.Background()) })
	return NewGateway(m, loggateway.NewNoop()), received
}

// TestGatewayInfo device.info 字段映射（screen 嵌套 → 扁平 DeviceInfo）。
func TestGatewayInfo(t *testing.T) {
	g, _ := newTestGateway(t, func(req rpcRequest) (any, *rpcError) {
		if req.Method != "device.info" {
			return nil, &rpcError{Code: -32601, Message: "bad method"}
		}
		return json.RawMessage(`{"platform":"windows","screen":{"width":2560,"height":1440,"scaleFactor":1.5},"virtualScreen":{"x":-1920,"y":0,"width":3840,"height":1440,"scaleFactor":1.5}}`), nil
	})

	info, err := g.Info(context.Background())
	if err != nil {
		t.Fatalf("Info err = %v", err)
	}
	want := bizcomputeruse.DeviceInfo{
		Platform: "windows", ScreenW: 2560, ScreenH: 1440, ScaleFactor: 1.5,
		VirtualX: -1920, VirtualY: 0, VirtualW: 3840, VirtualH: 1440,
	}
	if info != want {
		t.Errorf("Info = %+v, want %+v", info, want)
	}
}

// TestGatewaySnapshot 元素映射 + generation 回填到每个元素。
func TestGatewaySnapshot(t *testing.T) {
	g, received := newTestGateway(t, func(req rpcRequest) (any, *rpcError) {
		return json.RawMessage(`{
			"generation": 12,
			"elements": [{
				"ref": "g12.e42", "type": "button", "name": "保存(S)",
				"bbox": {"x":100,"y":200,"w":80,"h":28},
				"interactivity": true, "source": "uia",
				"appName": "notepad.exe", "enabled": true
			}]
		}`), nil
	})

	snap, err := g.Snapshot(context.Background(), bizcomputeruse.SnapshotOpts{WindowTitle: "记事本", MaxElements: 100})
	if err != nil {
		t.Fatalf("Snapshot err = %v", err)
	}
	if snap.Generation != 12 || len(snap.Elements) != 1 {
		t.Fatalf("Snapshot = %+v", snap)
	}
	e := snap.Elements[0]
	if e.Ref != "g12.e42" || e.Type != "button" || e.Name != "保存(S)" || e.AppName != "notepad.exe" {
		t.Errorf("element = %+v", e)
	}
	if e.BBox != (bizcomputeruse.Rect{X: 100, Y: 200, W: 80, H: 28}) {
		t.Errorf("bbox = %+v", e.BBox)
	}
	if !e.Interactivity || !e.Enabled || e.Source != "uia" {
		t.Errorf("element flags = %+v", e)
	}
	if e.Generation != 12 {
		t.Errorf("element.Generation = %d, want 12（回填）", e.Generation)
	}

	// 请求参数透传
	req := <-received
	if req.Method != "perception.snapshot" {
		t.Errorf("method = %q", req.Method)
	}
	var params map[string]any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("params unmarshal: %v", err)
	}
	if params["windowTitle"] != "记事本" {
		t.Errorf("params = %v", params)
	}
}

// TestGatewayScreenshot PNG base64 解码 + region 参数。
func TestGatewayScreenshot(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	g, received := newTestGateway(t, func(req rpcRequest) (any, *rpcError) {
		return map[string]any{
			"pngBase64":   base64.StdEncoding.EncodeToString(png),
			"width":       800,
			"height":      600,
			"scaleFactor": 1.0,
		}, nil
	})

	img, err := g.Screenshot(context.Background(), &bizcomputeruse.Rect{X: 1, Y: 2, W: 3, H: 4}, 2.0)
	if err != nil {
		t.Fatalf("Screenshot err = %v", err)
	}
	if string(img.PNG) != string(png) {
		t.Errorf("PNG = %v", img.PNG)
	}
	if img.Width != 800 || img.Height != 600 {
		t.Errorf("Image = %+v", img)
	}

	req := <-received
	if req.Method != "perception.screenshot" {
		t.Errorf("method = %q", req.Method)
	}
	var params struct {
		Region *bizcomputeruse.Rect `json:"region"`
		Zoom   float64              `json:"zoom"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("params unmarshal: %v", err)
	}
	if params.Region == nil || *params.Region != (bizcomputeruse.Rect{X: 1, Y: 2, W: 3, H: 4}) {
		t.Errorf("region = %+v", params.Region)
	}
	if params.Zoom != 2.0 {
		t.Errorf("zoom = %v", params.Zoom)
	}
}

// TestGatewaySnapshotWithScreenshot 75 review F2：includeScreenshot 响应映射——
// sidecar 内联返回 screenshot 子对象，gateway 解码进 Snapshot.Screenshot。
func TestGatewaySnapshotWithScreenshot(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G'}
	g, received := newTestGateway(t, func(req rpcRequest) (any, *rpcError) {
		return json.RawMessage(`{
			"generation": 3, "elements": [],
			"screenshot": {"pngBase64": "` + base64.StdEncoding.EncodeToString(png) + `", "width": 640, "height": 480, "scaleFactor": 1.25}
		}`), nil
	})

	snap, err := g.Snapshot(context.Background(), bizcomputeruse.SnapshotOpts{IncludeScreenshot: true})
	if err != nil {
		t.Fatalf("Snapshot err = %v", err)
	}
	if snap.Screenshot == nil {
		t.Fatal("Snapshot.Screenshot should be mapped when sidecar returns it")
	}
	if snap.Screenshot.Width != 640 || snap.Screenshot.Height != 480 || snap.Screenshot.ScaleFactor != 1.25 {
		t.Errorf("screenshot meta = %+v", snap.Screenshot)
	}
	if string(snap.Screenshot.PNG) != string(png) {
		t.Errorf("screenshot PNG bytes mismatch")
	}

	req := <-received
	var params map[string]any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("params unmarshal: %v", err)
	}
	if params["includeScreenshot"] != true {
		t.Errorf("params = %v, want includeScreenshot=true", params)
	}
}

// TestGatewayInvoke 成功与跨代 ref 错误映射。
func TestGatewayInvoke(t *testing.T) {
	stale := false
	g, received := newTestGateway(t, func(req rpcRequest) (any, *rpcError) {
		if stale {
			return nil, &rpcError{Code: -32001, Message: "ref 已过期"}
		}
		return map[string]any{"ok": true, "via": "invoke"}, nil
	})

	if err := g.Invoke(context.Background(), "g12.e42", 12); err != nil {
		t.Fatalf("Invoke err = %v", err)
	}
	req := <-received
	if req.Method != "action.invoke" {
		t.Errorf("method = %q", req.Method)
	}
	var params map[string]any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("params unmarshal: %v", err)
	}
	if params["ref"] != "g12.e42" {
		t.Errorf("params = %v", params)
	}
	if gen, ok := params["generation"].(float64); !ok || gen != 12 {
		t.Errorf("generation param = %v, want 12", params["generation"])
	}

	stale = true
	err := g.Invoke(context.Background(), "g11.e3", 12)
	if !errors.Is(err, ErrElementNotFound) {
		t.Errorf("Invoke err = %v, want ErrElementNotFound（跨代 ref）", err)
	}
}

// TestGatewayActions 坐标/文本/组合键动作的方法名与参数。
func TestGatewayActions(t *testing.T) {
	g, received := newTestGateway(t, func(req rpcRequest) (any, *rpcError) {
		return map[string]any{"ok": true}, nil
	})
	ctx := context.Background()

	if err := g.Click(ctx, bizcomputeruse.Point{X: 10, Y: 20}, "right", 2); err != nil {
		t.Fatalf("Click err = %v", err)
	}
	req := <-received
	var clickParams struct {
		X          int    `json:"x"`
		Y          int    `json:"y"`
		Button     string `json:"button"`
		ClickCount int    `json:"clickCount"`
	}
	if req.Method != "action.click" {
		t.Errorf("method = %q", req.Method)
	}
	if err := json.Unmarshal(req.Params, &clickParams); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if clickParams.X != 10 || clickParams.Y != 20 || clickParams.Button != "right" || clickParams.ClickCount != 2 {
		t.Errorf("click params = %+v", clickParams)
	}

	if err := g.TypeText(ctx, "你好"); err != nil {
		t.Fatalf("TypeText err = %v", err)
	}
	req = <-received
	if req.Method != "action.type" {
		t.Errorf("method = %q", req.Method)
	}

	if err := g.Key(ctx, "ctrl+s"); err != nil {
		t.Fatalf("Key err = %v", err)
	}
	req = <-received
	if req.Method != "action.key" {
		t.Errorf("method = %q", req.Method)
	}
	var keyParams map[string]string
	if err := json.Unmarshal(req.Params, &keyParams); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if keyParams["combo"] != "ctrl+s" {
		t.Errorf("key params = %v", keyParams)
	}

	if err := g.Wheel(ctx, bizcomputeruse.Point{X: 40, Y: 50}, -120); err != nil {
		t.Fatalf("Wheel err = %v", err)
	}
	req = <-received
	if req.Method != "action.wheel" {
		t.Errorf("wheel method = %q", req.Method)
	}
	var wheelParams struct {
		X     int `json:"x"`
		Y     int `json:"y"`
		Delta int `json:"delta"`
	}
	if err := json.Unmarshal(req.Params, &wheelParams); err != nil {
		t.Fatalf("wheel unmarshal: %v", err)
	}
	if wheelParams.X != 40 || wheelParams.Y != 50 || wheelParams.Delta != -120 {
		t.Errorf("wheel params = %+v", wheelParams)
	}

	if err := g.Drag(ctx, bizcomputeruse.Point{X: 1, Y: 2}, bizcomputeruse.Point{X: 8, Y: 9}, 250); err != nil {
		t.Fatalf("Drag err = %v", err)
	}
	req = <-received
	if req.Method != "action.drag" {
		t.Errorf("drag method = %q", req.Method)
	}
	var dragParams struct {
		From       map[string]int `json:"from"`
		To         map[string]int `json:"to"`
		DurationMs int            `json:"durationMs"`
	}
	if err := json.Unmarshal(req.Params, &dragParams); err != nil {
		t.Fatalf("drag unmarshal: %v", err)
	}
	if dragParams.From["x"] != 1 || dragParams.To["x"] != 8 || dragParams.DurationMs != 250 {
		t.Errorf("drag params = %+v", dragParams)
	}
}

// TestGatewayController 窗口/应用控制。
func TestGatewayController(t *testing.T) {
	g, received := newTestGateway(t, func(req rpcRequest) (any, *rpcError) {
		switch req.Method {
		case "window.focus":
			return map[string]any{"ok": true, "hwnd": 1234}, nil
		case "app.launch":
			return map[string]any{"ok": true, "pid": 5678}, nil
		case "window.list":
			return json.RawMessage(`{"windows":[
				{"hwnd":1234,"title":"记事本","processName":"notepad.exe","isForeground":true},
				{"hwnd":2222,"title":"浏览器","processName":"chrome.exe","isForeground":false}
			]}`), nil
		}
		return nil, &rpcError{Code: -32601, Message: "bad method"}
	})
	ctx := context.Background()

	if err := g.FocusWindow(ctx, "记事.*"); err != nil {
		t.Fatalf("FocusWindow err = %v", err)
	}
	req := <-received
	var focusParams map[string]string
	if err := json.Unmarshal(req.Params, &focusParams); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if focusParams["titleRegex"] != "记事.*" {
		t.Errorf("focus params = %v", focusParams)
	}

	pid, err := g.Launch(ctx, "notepad.exe", "", `C:\Windows`)
	if err != nil {
		t.Fatalf("Launch err = %v", err)
	}
	if pid != 5678 {
		t.Errorf("pid = %d, want 5678", pid)
	}
	req = <-received
	var launchParams map[string]string
	if err := json.Unmarshal(req.Params, &launchParams); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if launchParams["target"] != "notepad.exe" || launchParams["workDir"] != `C:\Windows` {
		t.Errorf("launch params = %v", launchParams)
	}

	wins, err := g.ListWindows(ctx)
	if err != nil {
		t.Fatalf("ListWindows err = %v", err)
	}
	if len(wins) != 2 {
		t.Fatalf("ListWindows = %+v", wins)
	}
	if wins[0].Hwnd != 1234 || wins[0].Title != "记事本" || wins[0].ProcessName != "notepad.exe" || !wins[0].IsForeground {
		t.Errorf("windows[0] = %+v", wins[0])
	}
	if wins[1].IsForeground {
		t.Errorf("windows[1] = %+v", wins[1])
	}
}
