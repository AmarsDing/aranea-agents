package computeruse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 流程日志 step_id（已登记 internal/event/flow_log.go stepTitleRegistry）。
const (
	StepSessionStart    = "computeruse.session.start"
	StepSessionDone     = "computeruse.session.done"
	StepAct             = "computeruse.act"
	StepActDone         = "computeruse.act.done"
	StepActError        = "computeruse.act.error"
	StepGroundFall      = "computeruse.grounding.fallback"
	StepBudgetExceeded  = "computeruse.budget.exceeded"
	StepKillSwitch      = "computeruse.killswitch"
)

// 业务错误（供 service/工具层判定）。
var (
	ErrBudgetExceeded   = errors.New("computeruse: 会话步数预算已耗尽或已超时")
	ErrSessionCancelled = errors.New("computeruse: 会话已急停取消")
	ErrSessionTerminal  = errors.New("computeruse: 会话已终止")
	ErrGroundingFailed  = errors.New("computeruse: 目标元素定位失败")
	ErrBlockedProcess   = errors.New("computeruse: 前台进程在禁区内，拒绝动作")
	ErrSessionNotFound  = errors.New("computeruse: 会话不存在")
)

// 默认会话预算：50 步 / 30 分钟。
const (
	defaultMaxSteps = 50
	defaultDeadline = 30 * time.Minute
)

// ElementMatcher a11y 元素模糊匹配器（internal/computeruse.Matcher 实现）。
// Stability:evolving
type ElementMatcher interface {
	// Match 在元素表中为 target 找最佳可交互元素；未命中返回 nil。
	Match(elements []UIElement, target string) *UIElement
}

// Deps Usecase 依赖聚合（构造注入；可 nil 的依赖已在字段注明）。
type Deps struct {
	Gateway  DeviceGateway      // sidecar 网关（必需）
	Match    ElementMatcher     // a11y 匹配器（必需）
	Vision   VisionParser       // 可 nil：nil 时跳过视觉兜底
	Grounder VisionGrounder     // 可 nil：nil 时跳过视觉兜底
	Audit    AuditStore         // 可 nil：跳过落库（M1.4 接线）
	Events   StepEventPublisher // 可 nil：跳过实时事件
	FlowLog  biz.FlowLogWriter  // 可 nil：跳过流程日志
	Policy   Policy             // 值类型，零值=安全默认
	Lg       loggateway.Logger  // 可 nil：noop
	Now      func() time.Time   // 可 nil：time.Now（测试注入）
	NewID    func() string      // 可 nil：内部自增（测试注入）
}

// ComputerUseUsecase 桌面 GUI 自动化用例编排。
type ComputerUseUsecase struct {
	d Deps

	mu             sync.Mutex
	sessions       map[string]*Session       // sessionID → session
	activeByAgent  map[string]string         // agentKey → 活跃 sessionID
	sessionCancels map[string]context.CancelFunc
	idSeq          int
}

// NewComputerUseUsecase 构造。
func NewComputerUseUsecase(d Deps) *ComputerUseUsecase {
	if d.Lg == nil {
		d.Lg = loggateway.NewNoop()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	u := &ComputerUseUsecase{
		d:              d,
		sessions:       map[string]*Session{},
		activeByAgent:  map[string]string{},
		sessionCancels: map[string]context.CancelFunc{},
	}
	if d.NewID == nil {
		d.NewID = u.nextID
	}
	u.d = d
	return u
}

func (u *ComputerUseUsecase) nextID() string {
	u.idSeq++
	return fmt.Sprintf("cu-%d-%d", u.d.Now().UnixNano(), u.idSeq)
}

// StartSession 显式创建会话。
func (u *ComputerUseUsecase) StartSession(_ context.Context, agentKey string, budget Budget) (Session, error) {
	if strings.TrimSpace(agentKey) == "" {
		return Session{}, fmt.Errorf("computeruse: agent_key 必填")
	}
	if budget.MaxSteps <= 0 {
		budget.MaxSteps = defaultMaxSteps
	}
	if budget.Deadline.IsZero() {
		budget.Deadline = u.d.Now().Add(defaultDeadline)
	}
	now := u.d.Now()
	u.mu.Lock()
	s := &Session{
		ID:        u.d.NewID(),
		AgentKey:  agentKey,
		Status:    SessionIdle,
		Budget:    budget,
		CreatedAt: now,
		UpdatedAt: now,
	}
	u.sessions[s.ID] = s
	u.activeByAgent[agentKey] = s.ID
	u.mu.Unlock()
	u.d.Lg.Info("computer-use 会话已创建",
		loggateway.StepID(StepSessionStart),
		loggateway.Str("session_id", s.ID),
		loggateway.Str("agent_key", agentKey),
		loggateway.Int("max_steps", budget.MaxSteps))
	return *s, nil
}

// StopSession 正常结束会话。
func (u *ComputerUseUsecase) StopSession(ctx context.Context, sessionID string) error {
	u.mu.Lock()
	s, ok := u.sessions[sessionID]
	if !ok {
		u.mu.Unlock()
		return ErrSessionNotFound
	}
	if IsTerminal(s.Status) {
		u.mu.Unlock()
		return nil
	}
	s.Status = SessionDone
	s.UpdatedAt = u.d.Now()
	if u.activeByAgent[s.AgentKey] == sessionID {
		delete(u.activeByAgent, s.AgentKey)
	}
	u.mu.Unlock()
	u.d.Lg.Info("computer-use 会话已结束",
		loggateway.StepID(StepSessionDone),
		loggateway.Str("session_id", sessionID),
		loggateway.Int("steps_used", s.StepsUsed))
	if u.d.FlowLog != nil {
		u.d.FlowLog.LogFlowDone(ctx, sessionID, StepSessionDone, "桌面会话已结束",
			biz.LogPair{Key: "steps_used", Value: s.StepsUsed})
	}
	return nil
}

// KillSwitch 急停：取消进行中动作 + 会话置 cancelled。
func (u *ComputerUseUsecase) KillSwitch(ctx context.Context, sessionID string) error {
	u.mu.Lock()
	s, ok := u.sessions[sessionID]
	if !ok {
		u.mu.Unlock()
		return ErrSessionNotFound
	}
	cancel := u.sessionCancels[sessionID]
	if !IsTerminal(s.Status) {
		s.Status = SessionCancelled
		s.UpdatedAt = u.d.Now()
	}
	if u.activeByAgent[s.AgentKey] == sessionID {
		delete(u.activeByAgent, s.AgentKey)
	}
	u.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	u.d.Lg.Warn("computer-use 急停触发",
		loggateway.StepID(StepKillSwitch),
		loggateway.Str("session_id", sessionID),
		loggateway.Str("agent_key", s.AgentKey),
		loggateway.Int("steps_used", s.StepsUsed))
	if u.d.FlowLog != nil {
		u.d.FlowLog.LogFlowError(ctx, sessionID, StepKillSwitch, "会话已急停",
			biz.LogPair{Key: "agent_key", Value: s.AgentKey})
	}
	return nil
}

// GetSession 查询会话。
func (u *ComputerUseUsecase) GetSession(_ context.Context, sessionID string) (Session, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	s, ok := u.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return *s, nil
}

// ListSteps 审计步骤查询。
func (u *ComputerUseUsecase) ListSteps(ctx context.Context, sessionID string) ([]AuditEntry, error) {
	if u.d.Audit == nil {
		return nil, nil
	}
	return u.d.Audit.ListSteps(ctx, sessionID)
}

// Status 健康状态：sidecar（gateway.Info 探测）+ 视觉组件可用性。
func (u *ComputerUseUsecase) Status(ctx context.Context) map[string]any {
	out := map[string]any{"sidecar": "down", "vision": false}
	if u.d.Gateway != nil {
		if info, err := u.d.Gateway.Info(ctx); err == nil {
			out["sidecar"] = "up"
			out["platform"] = info.Platform
			out["scale_factor"] = info.ScaleFactor
		}
	}
	if u.d.Vision != nil {
		out["vision"] = u.d.Vision.Available(ctx)
	}
	return out
}

// Observe 桌面感知（免确认）。
func (u *ComputerUseUsecase) Observe(ctx context.Context, req ObserveRequest) (ObserveResult, error) {
	if u.d.Gateway == nil {
		return ObserveResult{}, fmt.Errorf("computeruse: device gateway 未配置")
	}
	snap, err := u.d.Gateway.Snapshot(ctx, SnapshotOpts{
		WindowTitle:       req.WindowTitle,
		IncludeScreenshot: req.IncludeScreenshot,
		MaxElements:       req.MaxElements,
	})
	if err != nil {
		return ObserveResult{}, err
	}
	info, _ := u.d.Gateway.Info(ctx)
	return ObserveResult{
		Summary:    summarizeElements(snap.Elements),
		Elements:   snap.Elements,
		Generation: snap.Generation,
		Info:       info,
	}, nil
}

// Screenshot 桌面截图（免确认）。
func (u *ComputerUseUsecase) Screenshot(ctx context.Context, region *Rect, zoom float64) (Image, error) {
	if u.d.Gateway == nil {
		return Image{}, fmt.Errorf("computeruse: device gateway 未配置")
	}
	if zoom <= 0 {
		zoom = 1.0
	}
	return u.d.Gateway.Screenshot(ctx, region, zoom)
}

// Act 语义动作：grounding 编排（a11y 优先，视觉兜底）+ 安全策略 + 预算 + 审计。
func (u *ComputerUseUsecase) Act(ctx context.Context, req ActRequest) (ActResult, error) {
	if u.d.Gateway == nil || u.d.Match == nil {
		return ActResult{}, fmt.Errorf("computeruse: gateway/matcher 未配置")
	}
	started := u.d.Now()
	s, err := u.resolveSession(req)
	if err != nil {
		return ActResult{}, err
	}

	// 预算守卫（原子占用一步）。
	if err := u.chargeBudget(s); err != nil {
		if u.d.FlowLog != nil {
			u.d.FlowLog.LogFlowError(ctx, s.ID, StepBudgetExceeded, "会话预算耗尽",
				biz.LogPair{Key: "steps_used", Value: s.StepsUsed})
		}
		u.finishStep(ctx, s, req, Step{Result: StepFailed, Error: err.Error(), Danger: u.d.Policy.IsDanger(req.Target, req.Args)}, started)
		return ActResult{}, err
	}

	// 急停取消注册（进行中 Act 可被 KillSwitch 中断）。
	actCtx, cancel := context.WithCancel(ctx)
	u.mu.Lock()
	u.sessionCancels[s.ID] = cancel
	u.mu.Unlock()
	defer func() {
		cancel()
		u.mu.Lock()
		delete(u.sessionCancels, s.ID)
		u.mu.Unlock()
	}()

	danger := u.d.Policy.IsDanger(req.Target, req.Args)
	step := Step{
		SessionID:   s.ID,
		AgentKey:    s.AgentKey,
		Index:       s.StepsUsed,
		Target:      req.Target,
		Action:      req.Action,
		Params:      req.Args,
		Danger:      danger,
		ConfirmedBy: req.ConfirmedBy,
		CreatedAt:   started,
	}

	if u.d.FlowLog != nil {
		u.d.FlowLog.LogFlowStart(ctx, s.ID, StepAct, "执行桌面动作",
			biz.LogPair{Key: "target", Value: req.Target},
			biz.LogPair{Key: "action", Value: string(req.Action)},
			biz.LogPair{Key: "danger", Value: danger})
	}

	if !u.transit(s, EvGround) {
		return ActResult{}, fmt.Errorf("computeruse: 会话状态 %s 不允许动作", s.Status)
	}

	// 禁区检查：前台进程命中黑名单直接拒绝。
	if err := u.checkBlockedProcess(actCtx); err != nil {
		u.transit(s, EvFail)
		u.finishStep(ctx, s, req, withResult(step, StepFailed, err), started)
		return ActResult{}, err
	}

	result, gerr := u.groundAndExecute(actCtx, s, req, &step)
	u.finishStep(ctx, s, req, step, started)
	if gerr != nil {
		u.transit(s, EvFail)
		if u.d.FlowLog != nil {
			u.d.FlowLog.LogFlowError(ctx, s.ID, StepActError, "桌面动作失败",
				biz.LogPair{Key: "target", Value: req.Target},
				biz.LogPair{Key: "error", Value: gerr.Error()})
		}
		return result, gerr
	}
	u.transit(s, EvStepDone)
	if u.d.FlowLog != nil {
		u.d.FlowLog.LogFlowDone(ctx, s.ID, StepActDone, "桌面动作完成",
			biz.LogPair{Key: "target", Value: req.Target},
			biz.LogPair{Key: "path", Value: string(step.Path)})
	}
	return result, nil
}

// resolveSession 解析会话：显式 ID > agent 活跃会话 > 自动创建（默认预算）。
// 自动创建让 act/launch 工具可独立使用，无需先调 session start。
func (u *ComputerUseUsecase) resolveSession(req ActRequest) (*Session, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	var s *Session
	if id := strings.TrimSpace(req.SessionID); id != "" {
		var ok bool
		s, ok = u.sessions[id]
		if !ok {
			return nil, ErrSessionNotFound
		}
	} else if id := u.activeByAgent[req.AgentKey]; id != "" {
		s = u.sessions[id]
	}
	if s == nil {
		if strings.TrimSpace(req.AgentKey) == "" {
			return nil, fmt.Errorf("computeruse: agent_key 必填")
		}
		now := u.d.Now()
		s = &Session{
			ID:       u.d.NewID(),
			AgentKey: req.AgentKey,
			Status:   SessionIdle,
			Budget: Budget{
				MaxSteps: defaultMaxSteps,
				Deadline: now.Add(defaultDeadline),
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		u.sessions[s.ID] = s
		u.activeByAgent[req.AgentKey] = s.ID
		u.d.Lg.Info("computer-use 会话自动创建",
			loggateway.StepID(StepSessionStart),
			loggateway.Str("session_id", s.ID),
			loggateway.Str("agent_key", req.AgentKey))
		return s, nil
	}
	if s.Status == SessionCancelled {
		return nil, ErrSessionCancelled
	}
	if IsTerminal(s.Status) {
		return nil, ErrSessionTerminal
	}
	if s.Status != SessionIdle {
		return nil, fmt.Errorf("computeruse: 会话忙（%s）", s.Status)
	}
	return s, nil
}

// chargeBudget 原子占用一步预算；超限把会话置 failed/cancelled 并返回错误。
func (u *ComputerUseUsecase) chargeBudget(s *Session) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if s.Budget.MaxSteps > 0 && s.StepsUsed >= s.Budget.MaxSteps {
		s.Status = SessionFailed
		s.UpdatedAt = u.d.Now()
		return ErrBudgetExceeded
	}
	if !s.Budget.Deadline.IsZero() && u.d.Now().After(s.Budget.Deadline) {
		s.Status = SessionFailed
		s.UpdatedAt = u.d.Now()
		return ErrBudgetExceeded
	}
	s.StepsUsed++
	s.UpdatedAt = u.d.Now()
	return nil
}

// transit 状态机转换（非法转换记 warn 不阻断，防状态卡死）。
func (u *ComputerUseUsecase) transit(s *Session, ev SessionEvent) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	to, err := Transition(s.Status, ev)
	if err != nil {
		u.d.Lg.Warn("computer-use 非法状态转换",
			loggateway.StepID(StepAct),
			loggateway.Str("session_id", s.ID),
			loggateway.Str("from", string(s.Status)),
			loggateway.Str("event", string(ev)))
		return false
	}
	s.Status = to
	s.UpdatedAt = u.d.Now()
	return true
}

// checkBlockedProcess 禁区检查。
func (u *ComputerUseUsecase) checkBlockedProcess(ctx context.Context) error {
	wins, err := u.d.Gateway.ListWindows(ctx)
	if err != nil {
		// 窗口枚举失败不阻断（降级），仅告警。
		u.d.Lg.Warn("computer-use 窗口枚举失败，禁区检查跳过",
			loggateway.StepID(StepAct), loggateway.Err(err))
		return nil
	}
	for _, w := range wins {
		if w.IsForeground && u.d.Policy.IsBlockedProcess(w.ProcessName) {
			return fmt.Errorf("%w: %s", ErrBlockedProcess, w.ProcessName)
		}
	}
	return nil
}

// groundAndExecute grounding 决策流（设计 §3.3）。
func (u *ComputerUseUsecase) groundAndExecute(ctx context.Context, s *Session, req ActRequest, step *Step) (ActResult, error) {
	// 坐标类直行动作（key/坐标 click）无需 grounding。
	if run, el, direct := u.directAction(ctx, req, step); direct {
		return u.finishAction(req, step, el, run)
	}

	// 1. 感知
	snap, err := u.d.Gateway.Snapshot(ctx, SnapshotOpts{MaxElements: 500})
	if err != nil {
		*step = withResult(*step, StepFailed, err)
		return ActResult{Step: *step}, err
	}

	// 2. a11y 快路径
	hit := u.d.Match.Match(snap.Elements, req.Target)
	if hit != nil {
		step.Path = PathA11y
		return u.executeOnElement(ctx, req, step, *hit, snap.Generation)
	}

	// 3. 视觉兜底（M1.3 接线；组件缺失时记录降级并失败）
	if u.d.Vision == nil || u.d.Grounder == nil || !u.d.Vision.Available(ctx) {
		if u.d.FlowLog != nil {
			u.d.FlowLog.LogFlowError(ctx, s.ID, StepGroundFall, "a11y 未命中且视觉组件不可用",
				biz.LogPair{Key: "target", Value: req.Target})
		}
		*step = withResult(*step, StepFailed, ErrGroundingFailed)
		return ActResult{Step: *step}, fmt.Errorf("%w: %q（a11y 未命中，视觉兜底不可用）", ErrGroundingFailed, req.Target)
	}
	if u.d.FlowLog != nil {
		u.d.FlowLog.LogFlowError(ctx, s.ID, StepGroundFall, "a11y 未命中，降级视觉兜底",
			biz.LogPair{Key: "target", Value: req.Target})
	}
	el, verr := u.visionGround(ctx, snap, req.Target)
	if verr != nil {
		*step = withResult(*step, StepFailed, verr)
		return ActResult{Step: *step}, verr
	}
	step.Path = PathVision
	return u.executeOnElement(ctx, req, step, el, snap.Generation)
}

// directAction 无需 grounding 的动作：key 组合键、纯坐标 click。
// 返回 direct=true 表示本函数处理该动作；run 延迟到 finishAction 判定干跑后执行。
// wheel/drag 不在 P1 暴露（biz port 未定义，CDP 能力保留）。
func (u *ComputerUseUsecase) directAction(ctx context.Context, req ActRequest, step *Step) (run func() error, el *UIElement, direct bool) {
	switch req.Action {
	case ActionKey:
		combo, _ := req.Args["combo"].(string)
		if strings.TrimSpace(combo) == "" {
			return func() error { return fmt.Errorf("computeruse: key 动作缺少 combo 参数") }, nil, true
		}
		step.Path = PathA11y // 无 grounding，路径记 a11y（直发）
		return func() error { return u.d.Gateway.Key(ctx, combo) }, nil, true
	case ActionClick:
		x, xok := asInt(req.Args["x"])
		y, yok := asInt(req.Args["y"])
		if xok && yok && strings.TrimSpace(req.Target) == "" {
			return func() error {
				return u.d.Gateway.Click(ctx, Point{X: x, Y: y}, strArg(req.Args, "button", "left"), intArg(req.Args, "click_count", 1))
			}, nil, true
		}
		return nil, nil, false // 有 target：走 grounding
	}
	return nil, nil, false
}

// executeOnElement 在命中元素上执行动作（含干跑短路）。
func (u *ComputerUseUsecase) executeOnElement(ctx context.Context, req ActRequest, step *Step, el UIElement, generation int) (ActResult, error) {
	return u.finishAction(req, step, &el, func() error {
		switch req.Action {
		case ActionInvoke, "":
			return u.d.Gateway.Invoke(ctx, el.Ref, generation)
		case ActionClick:
			return u.d.Gateway.Click(ctx, el.BBox.Center(), strArg(req.Args, "button", "left"), intArg(req.Args, "click_count", 1))
		case ActionTypeText:
			text, _ := req.Args["text"].(string)
			if err := u.d.Gateway.Click(ctx, el.BBox.Center(), "left", 1); err != nil {
				return err
			}
			return u.d.Gateway.TypeText(ctx, text)
		default:
			return fmt.Errorf("computeruse: 动作 %s 不支持元素级执行", req.Action)
		}
	})
}

// finishAction 统一处理干跑短路/结果包装；run 仅在非干跑时执行。
func (u *ComputerUseUsecase) finishAction(req ActRequest, step *Step, el *UIElement, run func() error) (ActResult, error) {
	if req.DryRun {
		step.Result = StepDryRun
		plan := &DryRunPlan{Path: step.Path, WillDo: describeAction(req, el)}
		if el != nil {
			plan.ResolvedRef = el.Ref
			plan.ResolvedName = el.Name
		}
		return ActResult{Step: *step, Plan: plan, Element: el}, nil
	}
	if err := run(); err != nil {
		*step = withResult(*step, StepFailed, err)
		return ActResult{Step: *step, Element: el}, err
	}
	step.Result = StepOK
	return ActResult{Step: *step, Element: el}, nil
}

// visionGround 视觉兜底：截图 → OmniParser 解析 → VLM 选取。
func (u *ComputerUseUsecase) visionGround(ctx context.Context, snap Snapshot, target string) (UIElement, error) {
	img, err := u.d.Gateway.Screenshot(ctx, nil, 1.0)
	if err != nil {
		return UIElement{}, err
	}
	visionEls, err := u.d.Vision.Parse(ctx, img)
	if err != nil {
		return UIElement{}, err
	}
	// IoU 融合（a11y 主表优先，vision 补充去重，ref 重编 v 前缀）。
	merged := MergeA11yVision(snap.Elements, visionEls, snap.Generation)
	ref, err := u.d.Grounder.Pick(ctx, img, merged, target)
	if err != nil {
		return UIElement{}, err
	}
	for _, el := range merged {
		if el.Ref == ref {
			return el, nil
		}
	}
	return UIElement{}, fmt.Errorf("%w: VLM 返回未知 ref %q", ErrGroundingFailed, ref)
}

// finishStep 审计落库 + 实时事件（尽力而为，不阻断主流程）。
func (u *ComputerUseUsecase) finishStep(ctx context.Context, s *Session, req ActRequest, step Step, started time.Time) {
	step.SessionID = s.ID
	step.AgentKey = s.AgentKey
	if step.Index == 0 {
		step.Index = s.StepsUsed
	}
	if step.Target == "" {
		step.Target = req.Target
	}
	if step.Action == "" {
		step.Action = req.Action
	}
	step.DurationMs = u.d.Now().Sub(started).Milliseconds()
	if step.Result == "" {
		step.Result = StepFailed
	}
	if u.d.Audit != nil {
		if err := u.d.Audit.RecordStep(ctx, step); err != nil {
			u.d.Lg.Warn("computer-use 审计落库失败",
				loggateway.StepID(StepAct),
				loggateway.Str("session_id", s.ID),
				loggateway.Err(err))
		}
	}
	if u.d.Events != nil {
		u.d.Events.PublishStep(ctx, step)
	}
}

// Launch 启动应用（确认门在工具层）。
func (u *ComputerUseUsecase) Launch(ctx context.Context, agentKey, target, args, workDir, confirmedBy string) (Step, error) {
	if u.d.Gateway == nil {
		return Step{}, fmt.Errorf("computeruse: device gateway 未配置")
	}
	started := u.d.Now()
	s, err := u.resolveSession(ActRequest{AgentKey: agentKey})
	if err != nil {
		return Step{}, err
	}
	if err := u.chargeBudget(s); err != nil {
		return Step{}, err
	}
	step := Step{
		SessionID: s.ID, AgentKey: agentKey, Index: s.StepsUsed,
		Target: target, Action: ActionLaunch, Path: PathA11y,
		Params:      map[string]any{"target": target, "args": args},
		Danger:      u.d.Policy.IsDanger(target, map[string]any{"target": target}),
		ConfirmedBy: confirmedBy, CreatedAt: started,
	}
	pid, lerr := u.d.Gateway.Launch(ctx, target, args, workDir)
	if lerr != nil {
		step = withResult(step, StepFailed, lerr)
	} else {
		step.Result = StepOK
		step.Params["pid"] = pid
	}
	u.finishStep(ctx, s, ActRequest{AgentKey: agentKey, Target: target, Action: ActionLaunch}, step, started)
	return step, lerr
}

// --- helpers ---

func withResult(s Step, r StepResult, err error) Step {
	s.Result = r
	if err != nil {
		s.Error = err.Error()
	}
	return s
}

func summarizeElements(els []UIElement) string {
	const maxLines = 40
	var b strings.Builder
	fmt.Fprintf(&b, "共 %d 个可访问元素（仅列前 %d 条可交互项）：\n", len(els), maxLines)
	n := 0
	for _, el := range els {
		if !el.Interactivity || !el.Enabled {
			continue
		}
		fmt.Fprintf(&b, "- [%s] %s (%s) @(%d,%d %dx%d)\n", el.Ref, el.Name, el.Type, el.BBox.X, el.BBox.Y, el.BBox.W, el.BBox.H)
		if n++; n >= maxLines {
			break
		}
	}
	return b.String()
}

func describeAction(req ActRequest, el *UIElement) string {
	target := req.Target
	if el != nil {
		target = fmt.Sprintf("%s（%s @%d,%d）", el.Name, el.Ref, el.BBox.Center().X, el.BBox.Center().Y)
	}
	return fmt.Sprintf("%s → %s", req.Action, target)
}

func strArg(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return def
}

func intArg(args map[string]any, key string, def int) int {
	if v, ok := asInt(args[key]); ok {
		return v
	}
	return def
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}
