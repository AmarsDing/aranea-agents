package computeruse

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/pkg/loggateway"
)

type fakeLLMCaller struct {
	resp   string
	err    error
	gotReq biz.LLMCallRequest
	called int
}

func (f *fakeLLMCaller) Call(ctx context.Context, req biz.LLMCallRequest) (string, int, error) {
	f.called++
	f.gotReq = req
	return f.resp, 0, f.err
}

type fakeVisionCatalog struct {
	models []biz.ProviderModel
	err    error
}

func (f fakeVisionCatalog) List(ctx context.Context) ([]biz.ProviderModel, error) {
	return f.models, f.err
}

type fakeVisionSys struct {
	provider, model string
}

func (f fakeVisionSys) Get(ctx context.Context) (biz.SystemSetting, error) {
	s := biz.SystemSetting{}
	s.DefaultRefineLLM.Provider = f.provider
	s.DefaultRefineLLM.Model = f.model
	return s, nil
}

func vlmTestImage(t *testing.T) bizcu.Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 80, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return bizcu.Image{PNG: buf.Bytes(), Width: 80, Height: 60, ScaleFactor: 1.0}
}

func vlmCandidates() []bizcu.UIElement {
	return []bizcu.UIElement{
		{Ref: "g1.e0", Name: "文件", Type: "menuitem", BBox: bizcu.Rect{X: 1, Y: 1, W: 20, H: 10}, Generation: 1},
		{Ref: "g1.v0", Name: "保存按钮", Type: "icon", BBox: bizcu.Rect{X: 30, Y: 30, W: 20, H: 20}, Generation: 1},
		{Ref: "g1.v1", Name: "搜索", Type: "icon", BBox: bizcu.Rect{X: 55, Y: 30, W: 20, H: 20}, Generation: 1},
	}
}

func visionCatalog() fakeVisionCatalog {
	return fakeVisionCatalog{models: []biz.ProviderModel{
		{Provider: "openai", Model: "gpt-4o", Enabled: true, Capabilities: biz.ModelCapabilities{Vision: true}},
	}}
}

// TestNewVLMGrounder_NilLogger 构造函数对 nil Logger 兜底 Noop（B2）。
func TestNewVLMGrounder_NilLogger(t *testing.T) {
	g := NewVLMGrounder(&fakeLLMCaller{}, nil, nil, nil)
	if g.lg == nil {
		t.Fatal("lg = nil, want Noop 兜底")
	}
}

func TestVLMGrounderPick_SelectsByNumber(t *testing.T) {
	caller := &fakeLLMCaller{resp: "2"}
	g := NewVLMGrounder(caller, nil, visionCatalog(), loggateway.NewNoop())
	ref, err := g.Pick(context.Background(), vlmTestImage(t), vlmCandidates(), "点击保存按钮")
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ref != "g1.v0" {
		t.Fatalf("ref=%q, want g1.v0（编号 2 → 第 2 个候选）", ref)
	}
	// 请求必须带 SoM 标注图且选中 vision 模型。
	if len(caller.gotReq.Images) != 1 || caller.gotReq.Images[0].Format != "png" {
		t.Fatalf("Images 缺失或格式错误: %+v", caller.gotReq.Images)
	}
	if caller.gotReq.Provider != "openai" || caller.gotReq.Model != "gpt-4o" {
		t.Fatalf("模型选择错误: %s/%s", caller.gotReq.Provider, caller.gotReq.Model)
	}
	if !strings.Contains(caller.gotReq.User, "点击保存按钮") {
		t.Fatalf("User prompt 应含 target")
	}
	if !strings.Contains(caller.gotReq.User, "保存按钮") || !strings.Contains(caller.gotReq.User, "搜索") {
		t.Fatalf("User prompt 应含候选名称列表: %q", caller.gotReq.User)
	}
}

func TestVLMGrounderPick_FallsBackToSysSetting(t *testing.T) {
	caller := &fakeLLMCaller{resp: "1"}
	// catalog 无 vision 模型 → 回退 DefaultRefineLLM。
	cat := fakeVisionCatalog{models: []biz.ProviderModel{
		{Provider: "openai", Model: "gpt-4o-mini", Enabled: true, Capabilities: biz.ModelCapabilities{Text: true}},
	}}
	g := NewVLMGrounder(caller, fakeVisionSys{provider: "anthropic", model: "claude-sonnet"}, cat, loggateway.NewNoop())
	ref, err := g.Pick(context.Background(), vlmTestImage(t), vlmCandidates(), "文件菜单")
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ref != "g1.e0" {
		t.Fatalf("ref=%q, want g1.e0", ref)
	}
	if caller.gotReq.Provider != "anthropic" || caller.gotReq.Model != "claude-sonnet" {
		t.Fatalf("应回退 sys 配置: %s/%s", caller.gotReq.Provider, caller.gotReq.Model)
	}
}

func TestVLMGrounderPick_NoVisionModel(t *testing.T) {
	caller := &fakeLLMCaller{resp: "1"}
	g := NewVLMGrounder(caller, fakeVisionSys{}, fakeVisionCatalog{}, loggateway.NewNoop())
	_, err := g.Pick(context.Background(), vlmTestImage(t), vlmCandidates(), "x")
	if err == nil {
		t.Fatalf("无可用视觉模型应报错")
	}
	if caller.called != 0 {
		t.Fatalf("无模型时不应调用 LLM")
	}
}

func TestVLMGrounderPick_EmptyCandidates(t *testing.T) {
	caller := &fakeLLMCaller{resp: "1"}
	g := NewVLMGrounder(caller, nil, visionCatalog(), loggateway.NewNoop())
	_, err := g.Pick(context.Background(), vlmTestImage(t), nil, "x")
	if !errors.Is(err, bizcu.ErrGroundingFailed) {
		t.Fatalf("空候选应 ErrGroundingFailed, got %v", err)
	}
	if caller.called != 0 {
		t.Fatalf("空候选不应调用 LLM")
	}
}

func TestVLMGrounderPick_InvalidResponse(t *testing.T) {
	caller := &fakeLLMCaller{resp: "我无法确定"}
	g := NewVLMGrounder(caller, nil, visionCatalog(), loggateway.NewNoop())
	_, err := g.Pick(context.Background(), vlmTestImage(t), vlmCandidates(), "x")
	if !errors.Is(err, bizcu.ErrGroundingFailed) {
		t.Fatalf("无编号响应应 ErrGroundingFailed, got %v", err)
	}
}

func TestVLMGrounderPick_OutOfRange(t *testing.T) {
	caller := &fakeLLMCaller{resp: "9"}
	g := NewVLMGrounder(caller, nil, visionCatalog(), loggateway.NewNoop())
	_, err := g.Pick(context.Background(), vlmTestImage(t), vlmCandidates(), "x")
	if !errors.Is(err, bizcu.ErrGroundingFailed) {
		t.Fatalf("越界编号应 ErrGroundingFailed, got %v", err)
	}
}

func TestVLMGrounderPick_ResponseWithExtraText(t *testing.T) {
	// VLM 啰嗦输出时提取首个整数。
	caller := &fakeLLMCaller{resp: "我认为是 3 号元素"}
	g := NewVLMGrounder(caller, nil, visionCatalog(), loggateway.NewNoop())
	ref, err := g.Pick(context.Background(), vlmTestImage(t), vlmCandidates(), "x")
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ref != "g1.v1" {
		t.Fatalf("ref=%q, want g1.v1", ref)
	}
}
