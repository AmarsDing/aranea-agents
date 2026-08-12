// Package computeruse 提供桌面 GUI 自动化工具集（computer_use_observe/screenshot/act/launch/session）。
// 工具为薄壳层：参数解析 + 委托 biz/computeruse Usecase，无业务逻辑。
package computeruse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/pkg/apierror"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Tool 名常量（与种子 tool_key 一致）。
const (
	ToolObserve    = "computer_use_observe"
	ToolScreenshot = "computer_use_screenshot"
	ToolAct        = "computer_use_act"
	ToolLaunch     = "computer_use_launch"
	ToolSession    = "computer_use_session"
)

// NewToolset 构建 5 个 computer-use 工具；uc 为 nil 返回空（装配层裁剪）。
func NewToolset(uc *bizcu.ComputerUseUsecase) []trpctool.CallableTool {
	if uc == nil {
		return nil
	}
	return []trpctool.CallableTool{
		&tool{name: ToolObserve, desc: "感知当前桌面：返回前台窗口的可访问元素清单（ref/名称/类型/位置），供后续 computer_use_act 引用。",
			uc: uc, fn: observeFn, schema: schemaOf(`{"type":"object","properties":{
				"window_title":{"type":"string","description":"可选，限定窗口标题（正则）"},
				"include_screenshot":{"type":"boolean","description":"是否同时截图"},
				"max_elements":{"type":"integer","description":"最大元素数，默认 500"}}}`)},
		&tool{name: ToolScreenshot, desc: "截取当前桌面截图，返回 base64 PNG 与尺寸元数据。",
			uc: uc, fn: screenshotFn, schema: schemaOf(`{"type":"object","properties":{
				"region":{"type":"object","properties":{"x":{"type":"integer"},"y":{"type":"integer"},"w":{"type":"integer"},"h":{"type":"integer"}},"description":"可选裁剪区域（物理像素）"},
				"zoom":{"type":"number","description":"缩放倍率，默认 1.0"}}}`)},
		&tool{name: ToolAct, desc: "在桌面执行语义动作：给出目标元素的自然语言描述（如“保存菜单项”），系统自动定位并操作。action: invoke(默认，元素直调)|click|type|key。type 需 text 参数；key 需 combo 参数（如 ctrl+s）；dry_run=true 时只返回定位计划不实际操作。高危动作（删除/支付等）会被标记并需人工确认。",
			uc: uc, fn: actFn, schema: schemaOf(`{"type":"object","properties":{
				"target":{"type":"string","description":"目标元素的自然语言描述；坐标点击时可省略"},
				"action":{"type":"string","enum":["invoke","click","type","key"],"description":"动作类型，默认 invoke"},
				"text":{"type":"string","description":"action=type 时要输入的文本"},
				"combo":{"type":"string","description":"action=key 时组合键，如 ctrl+s"},
				"x":{"type":"integer","description":"action=click 且无 target 时的物理像素 X"},
				"y":{"type":"integer","description":"action=click 且无 target 时的物理像素 Y"},
				"button":{"type":"string","enum":["left","right","middle"],"description":"点击按键，默认 left"},
				"click_count":{"type":"integer","description":"点击次数，默认 1"},
				"dry_run":{"type":"boolean","description":"干跑模式：只定位并返回计划，不实际操作"},
				"session_id":{"type":"string","description":"可选，绑定既有会话；省略时自动复用/创建"},
				"confirmed_by":{"type":"string","description":"确认门通过后的确认人标识（审计）"}}}`)},
		&tool{name: ToolLaunch, desc: "启动桌面应用：target 为应用名或完整路径（如 notepad.exe）。",
			uc: uc, fn: launchFn, schema: schemaOf(`{"type":"object","properties":{
				"target":{"type":"string","description":"应用名或可执行文件路径"},
				"args":{"type":"string","description":"可选命令行参数"},
				"work_dir":{"type":"string","description":"可选工作目录"},
				"confirmed_by":{"type":"string","description":"确认人标识（审计）"}},
				"required":["target"]}`)},
		&tool{name: ToolSession, desc: "管理桌面自动化会话：action=start(可选预算)|stop|status|kill(急停)。会话累计步数预算，超限自动拒绝动作。",
			uc: uc, fn: sessionFn, schema: schemaOf(`{"type":"object","properties":{
				"action":{"type":"string","enum":["start","stop","status","kill"]},
				"session_id":{"type":"string","description":"stop/kill 时必填"},
				"max_steps":{"type":"integer","description":"start 时步数预算，默认 50"},
				"duration_minutes":{"type":"integer","description":"start 时会话时长预算（分钟），默认 30"}},
				"required":["action"]}`)},
	}
}

type tool struct {
	name   string
	desc   string
	uc     *bizcu.ComputerUseUsecase
	fn     func(context.Context, *bizcu.ComputerUseUsecase, []byte) (any, error)
	schema *trpctool.Schema
}

func (t *tool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: t.name, Description: t.desc, InputSchema: t.schema}
}

func (t *tool) Call(ctx context.Context, args []byte) (any, error) {
	if t.uc == nil {
		return nil, apierror.Internal(apierror.DomainTool, t.name+": computer-use usecase not configured")
	}
	return t.fn(ctx, t.uc, args)
}

func schemaOf(raw string) *trpctool.Schema {
	var s trpctool.Schema
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		panic("computeruse tool schema: " + err.Error())
	}
	return &s
}

// agentKeyFromCtx 从调用上下文取 AgentKey（构建期 agent name = AgentKey）。
// 无调用上下文（测试/直连）回退 "default"。
func agentKeyFromCtx(ctx context.Context) string {
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.AgentName != "" {
		return inv.AgentName
	}
	return "default"
}

func decode(args []byte, v any) error {
	if err := json.Unmarshal(args, v); err != nil {
		return apierror.BadRequest(apierror.DomainTool, "invalid args: "+err.Error())
	}
	return nil
}

func observeFn(ctx context.Context, uc *bizcu.ComputerUseUsecase, args []byte) (any, error) {
	var in struct {
		WindowTitle       string `json:"window_title"`
		IncludeScreenshot bool   `json:"include_screenshot"`
		MaxElements       int    `json:"max_elements"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	res, err := uc.Observe(ctx, bizcu.ObserveRequest{
		AgentKey:          agentKeyFromCtx(ctx),
		WindowTitle:       in.WindowTitle,
		IncludeScreenshot: in.IncludeScreenshot,
		MaxElements:       in.MaxElements,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"summary":       res.Summary,
		"element_count": len(res.Elements),
		"generation":    res.Generation,
		"screen":        res.Info,
	}, nil
}

func screenshotFn(ctx context.Context, uc *bizcu.ComputerUseUsecase, args []byte) (any, error) {
	var in struct {
		Region *struct {
			X int `json:"x"`
			Y int `json:"y"`
			W int `json:"w"`
			H int `json:"h"`
		} `json:"region"`
		Zoom float64 `json:"zoom"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	var region *bizcu.Rect
	if in.Region != nil {
		region = &bizcu.Rect{X: in.Region.X, Y: in.Region.Y, W: in.Region.W, H: in.Region.H}
	}
	if in.Zoom <= 0 {
		in.Zoom = 1.0
	}
	img, err := uc.Screenshot(ctx, region, in.Zoom)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"png_base64":   base64.StdEncoding.EncodeToString(img.PNG),
		"width":        img.Width,
		"height":       img.Height,
		"scale_factor": img.ScaleFactor,
		"note":         "base64 PNG，物理像素；P1 阶段图像不直接注入模型上下文",
	}, nil
}

func actFn(ctx context.Context, uc *bizcu.ComputerUseUsecase, args []byte) (any, error) {
	var in struct {
		Target      string `json:"target"`
		Action      string `json:"action"`
		Text        string `json:"text"`
		Combo       string `json:"combo"`
		X           *int   `json:"x"`
		Y           *int   `json:"y"`
		Button      string `json:"button"`
		ClickCount  int    `json:"click_count"`
		DryRun      bool   `json:"dry_run"`
		SessionID   string `json:"session_id"`
		ConfirmedBy string `json:"confirmed_by"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	action := bizcu.ActionType(strings.TrimSpace(in.Action))
	if action == "" {
		action = bizcu.ActionInvoke
	}
	actArgs := map[string]any{}
	if in.Text != "" {
		actArgs["text"] = in.Text
	}
	if in.Combo != "" {
		actArgs["combo"] = in.Combo
	}
	if in.X != nil {
		actArgs["x"] = *in.X
	}
	if in.Y != nil {
		actArgs["y"] = *in.Y
	}
	if in.Button != "" {
		actArgs["button"] = in.Button
	}
	if in.ClickCount > 0 {
		actArgs["click_count"] = in.ClickCount
	}
	res, err := uc.Act(ctx, bizcu.ActRequest{
		AgentKey:    agentKeyFromCtx(ctx),
		SessionID:   in.SessionID,
		Target:      in.Target,
		Action:      action,
		Args:        actArgs,
		DryRun:      in.DryRun,
		ConfirmedBy: in.ConfirmedBy,
	})
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"session_id":  res.Step.SessionID,
		"step_index":  res.Step.Index,
		"path":        res.Step.Path,
		"result":      res.Step.Result,
		"duration_ms": res.Step.DurationMs,
		"danger":      res.Step.Danger,
	}
	if res.Element != nil {
		out["resolved"] = map[string]any{"ref": res.Element.Ref, "name": res.Element.Name, "type": res.Element.Type}
	}
	if res.Plan != nil {
		out["plan"] = res.Plan
	}
	return out, nil
}

func launchFn(ctx context.Context, uc *bizcu.ComputerUseUsecase, args []byte) (any, error) {
	var in struct {
		Target      string `json:"target"`
		Args        string `json:"args"`
		WorkDir     string `json:"work_dir"`
		ConfirmedBy string `json:"confirmed_by"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Target) == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "computer_use_launch: target 必填")
	}
	step, err := uc.Launch(ctx, agentKeyFromCtx(ctx), in.Target, in.Args, in.WorkDir, in.ConfirmedBy)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"session_id": step.SessionID,
		"result":     step.Result,
		"pid":        step.Params["pid"],
	}, nil
}

func sessionFn(ctx context.Context, uc *bizcu.ComputerUseUsecase, args []byte) (any, error) {
	var in struct {
		Action          string `json:"action"`
		SessionID       string `json:"session_id"`
		MaxSteps        int    `json:"max_steps"`
		DurationMinutes int    `json:"duration_minutes"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	switch strings.TrimSpace(in.Action) {
	case "start":
		budget := bizcu.Budget{MaxSteps: in.MaxSteps}
		if in.DurationMinutes > 0 {
			budget.Deadline = time.Now().Add(time.Duration(in.DurationMinutes) * time.Minute)
		}
		s, err := uc.StartSession(ctx, agentKeyFromCtx(ctx), budget)
		if err != nil {
			return nil, err
		}
		return map[string]any{"session_id": s.ID, "status": s.Status, "max_steps": s.Budget.MaxSteps, "deadline": s.Budget.Deadline}, nil
	case "stop":
		if strings.TrimSpace(in.SessionID) == "" {
			return nil, apierror.BadRequest(apierror.DomainTool, "session stop: session_id 必填")
		}
		if err := uc.StopSession(ctx, in.SessionID); err != nil {
			return nil, err
		}
		return map[string]any{"session_id": in.SessionID, "status": bizcu.SessionDone}, nil
	case "kill":
		if strings.TrimSpace(in.SessionID) == "" {
			return nil, apierror.BadRequest(apierror.DomainTool, "session kill: session_id 必填")
		}
		if err := uc.KillSwitch(ctx, in.SessionID); err != nil {
			return nil, err
		}
		return map[string]any{"session_id": in.SessionID, "status": bizcu.SessionCancelled}, nil
	case "status":
		if strings.TrimSpace(in.SessionID) != "" {
			s, err := uc.GetSession(ctx, in.SessionID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"session_id": s.ID, "status": s.Status, "steps_used": s.StepsUsed, "max_steps": s.Budget.MaxSteps}, nil
		}
		return uc.Status(ctx), nil
	default:
		return nil, apierror.BadRequest(apierror.DomainTool, fmt.Sprintf("unknown session action: %q", in.Action))
	}
}
