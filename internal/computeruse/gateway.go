// gateway.go 实现 biz 层 DeviceGateway 端口：把 biz 调用翻译为 CDP 方法，
// 经 Manager 懒拉起 sidecar 后由 Client 完成 RPC，并做 JSON ↔ biz 模型映射。
package computeruse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	bizcomputeruse "aranea-agents/internal/biz/computeruse"
	"aranea-agents/pkg/loggateway"
)

// Gateway biz DeviceGateway 的 sidecar 实现。组合 Manager 懒拉起进程。
type Gateway struct {
	mgr *Manager
	lg  loggateway.Logger
}

// NewGateway 构造 Gateway；lg 为 nil 时使用 Noop。
func NewGateway(mgr *Manager, lg loggateway.Logger) *Gateway {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &Gateway{mgr: mgr, lg: lg.With(loggateway.Domain("computeruse"))}
}

var _ bizcomputeruse.DeviceGateway = (*Gateway)(nil)

// call 懒拉起 sidecar 并发起 RPC，result 反序列化进 out（out 可 nil）。
func (g *Gateway) call(ctx context.Context, method string, params any, out any) error {
	if err := g.mgr.EnsureRunning(ctx); err != nil {
		return err
	}
	raw, err := g.mgr.Client().Call(ctx, method, params)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("computeruse: %s 响应解析失败: %w", method, err)
	}
	return nil
}

// ---- DevicePerceiver ----

// Info 设备信息（device.info）：screen 嵌套结构映射为扁平 DeviceInfo。
func (g *Gateway) Info(ctx context.Context) (bizcomputeruse.DeviceInfo, error) {
	var res struct {
		Platform string `json:"platform"`
		Screen   struct {
			Width       int     `json:"width"`
			Height      int     `json:"height"`
			ScaleFactor float64 `json:"scaleFactor"`
		} `json:"screen"`
	}
	if err := g.call(ctx, "device.info", nil, &res); err != nil {
		return bizcomputeruse.DeviceInfo{}, err
	}
	return bizcomputeruse.DeviceInfo{
		Platform:    res.Platform,
		ScreenW:     res.Screen.Width,
		ScreenH:     res.Screen.Height,
		ScaleFactor: res.Screen.ScaleFactor,
	}, nil
}

// Snapshot a11y 快照（perception.snapshot）；generation 回填到每个元素。
func (g *Gateway) Snapshot(ctx context.Context, opts bizcomputeruse.SnapshotOpts) (bizcomputeruse.Snapshot, error) {
	params := map[string]any{}
	if opts.WindowTitle != "" {
		params["windowTitle"] = opts.WindowTitle
	}
	if opts.IncludeScreenshot {
		params["includeScreenshot"] = true
	}
	if opts.MaxElements > 0 {
		params["maxElements"] = opts.MaxElements
	}

	var res struct {
		Elements   []elementDTO `json:"elements"`
		Generation int          `json:"generation"`
		Screenshot *struct {
			PNGBase64   string  `json:"pngBase64"`
			Width       int     `json:"width"`
			Height      int     `json:"height"`
			ScaleFactor float64 `json:"scaleFactor"`
		} `json:"screenshot"`
	}
	if err := g.call(ctx, "perception.snapshot", params, &res); err != nil {
		return bizcomputeruse.Snapshot{}, err
	}

	snap := bizcomputeruse.Snapshot{
		Generation: res.Generation,
		Elements:   make([]bizcomputeruse.UIElement, 0, len(res.Elements)),
	}
	for _, dto := range res.Elements {
		snap.Elements = append(snap.Elements, dto.toBiz(res.Generation))
	}
	// F2：includeScreenshot=true 时 sidecar 内联返回截图子对象。
	if res.Screenshot != nil {
		png, err := base64.StdEncoding.DecodeString(res.Screenshot.PNGBase64)
		if err != nil {
			return bizcomputeruse.Snapshot{}, fmt.Errorf("computeruse: 快照截图 base64 解码失败: %w", err)
		}
		snap.Screenshot = &bizcomputeruse.Image{
			PNG: png, Width: res.Screenshot.Width, Height: res.Screenshot.Height, ScaleFactor: res.Screenshot.ScaleFactor,
		}
	}
	return snap, nil
}

// Screenshot 截图（perception.screenshot）；PNG 以 base64 内联返回，这里解码。
func (g *Gateway) Screenshot(ctx context.Context, region *bizcomputeruse.Rect, zoom float64) (bizcomputeruse.Image, error) {
	params := map[string]any{}
	if region != nil {
		params["region"] = region
	}
	if zoom > 0 {
		params["zoom"] = zoom
	}

	var res struct {
		PNGBase64   string  `json:"pngBase64"`
		Width       int     `json:"width"`
		Height      int     `json:"height"`
		ScaleFactor float64 `json:"scaleFactor"`
	}
	if err := g.call(ctx, "perception.screenshot", params, &res); err != nil {
		return bizcomputeruse.Image{}, err
	}
	png, err := base64.StdEncoding.DecodeString(res.PNGBase64)
	if err != nil {
		return bizcomputeruse.Image{}, fmt.Errorf("computeruse: 截图 base64 解码失败: %w", err)
	}
	return bizcomputeruse.Image{PNG: png, Width: res.Width, Height: res.Height, ScaleFactor: res.ScaleFactor}, nil
}

// ---- DeviceActor ----

// Invoke 元素级直调（action.invoke）。跨代 ref 由 sidecar 返回 -32001，
// 经 Client 映射为 ErrElementNotFound，这里补充上下文。
func (g *Gateway) Invoke(ctx context.Context, ref string, generation int) error {
	params := map[string]any{"ref": ref, "generation": generation}
	if err := g.call(ctx, "action.invoke", params, nil); err != nil {
		return fmt.Errorf("computeruse: invoke ref=%q generation=%d: %w", ref, generation, err)
	}
	return nil
}

// Click 坐标级点击（action.click），物理像素。
func (g *Gateway) Click(ctx context.Context, p bizcomputeruse.Point, button string, clickCount int) error {
	params := map[string]any{"x": p.X, "y": p.Y, "button": button, "clickCount": clickCount}
	return g.call(ctx, "action.click", params, nil)
}

// TypeText 文本注入（action.type）。
func (g *Gateway) TypeText(ctx context.Context, text string) error {
	return g.call(ctx, "action.type", map[string]any{"text": text}, nil)
}

// Key 组合键（action.key），如 "ctrl+s"。
func (g *Gateway) Key(ctx context.Context, combo string) error {
	return g.call(ctx, "action.key", map[string]any{"combo": combo}, nil)
}

// Wheel 滚轮（action.wheel），delta 正上负下，120 为一格。
func (g *Gateway) Wheel(ctx context.Context, p bizcomputeruse.Point, delta int) error {
	return g.call(ctx, "action.wheel", map[string]any{"x": p.X, "y": p.Y, "delta": delta}, nil)
}

// Drag 拖拽（action.drag），物理像素 from/to。
func (g *Gateway) Drag(ctx context.Context, from, to bizcomputeruse.Point, durationMs int) error {
	if durationMs <= 0 {
		durationMs = 300
	}
	return g.call(ctx, "action.drag", map[string]any{
		"from":       map[string]int{"x": from.X, "y": from.Y},
		"to":         map[string]int{"x": to.X, "y": to.Y},
		"durationMs": durationMs,
	}, nil)
}

// ---- DeviceController ----

// FocusWindow 按标题正则聚焦窗口（window.focus）。
func (g *Gateway) FocusWindow(ctx context.Context, titleRegex string) error {
	return g.call(ctx, "window.focus", map[string]any{"titleRegex": titleRegex}, nil)
}

// Launch 拉起应用（app.launch），返回 pid。
func (g *Gateway) Launch(ctx context.Context, target string, args string, workDir string) (int, error) {
	params := map[string]any{"target": target}
	if args != "" {
		params["args"] = args
	}
	if workDir != "" {
		params["workDir"] = workDir
	}
	var res struct {
		PID int `json:"pid"`
	}
	if err := g.call(ctx, "app.launch", params, &res); err != nil {
		return 0, err
	}
	return res.PID, nil
}

// ListWindows 窗口列表（window.list）。
func (g *Gateway) ListWindows(ctx context.Context) ([]bizcomputeruse.WindowInfo, error) {
	var res struct {
		Windows []struct {
			Hwnd         int64  `json:"hwnd"`
			Title        string `json:"title"`
			ProcessName  string `json:"processName"`
			IsForeground bool   `json:"isForeground"`
		} `json:"windows"`
	}
	if err := g.call(ctx, "window.list", nil, &res); err != nil {
		return nil, err
	}
	wins := make([]bizcomputeruse.WindowInfo, 0, len(res.Windows))
	for _, w := range res.Windows {
		wins = append(wins, bizcomputeruse.WindowInfo{
			Hwnd:         w.Hwnd,
			Title:        w.Title,
			ProcessName:  w.ProcessName,
			IsForeground: w.IsForeground,
		})
	}
	return wins, nil
}

// elementDTO sidecar 线格式（camelCase，CDP §2.3）→ biz UIElement。
type elementDTO struct {
	Ref  string `json:"ref"`
	Type string `json:"type"`
	Name string `json:"name"`
	BBox struct {
		X int `json:"x"`
		Y int `json:"y"`
		W int `json:"w"`
		H int `json:"h"`
	} `json:"bbox"`
	Interactivity bool   `json:"interactivity"`
	Source        string `json:"source"`
	AppName       string `json:"appName"`
	Enabled       bool   `json:"enabled"`
}

// toBiz 转换为 biz 模型并回填所属 snapshot 代。
func (d elementDTO) toBiz(generation int) bizcomputeruse.UIElement {
	return bizcomputeruse.UIElement{
		Ref:           d.Ref,
		Type:          d.Type,
		Name:          d.Name,
		BBox:          bizcomputeruse.Rect{X: d.BBox.X, Y: d.BBox.Y, W: d.BBox.W, H: d.BBox.H},
		Interactivity: d.Interactivity,
		Source:        d.Source,
		AppName:       d.AppName,
		Enabled:       d.Enabled,
		Generation:    generation,
	}
}
