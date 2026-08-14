package computeruse

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 流程日志 step_id（已登记 internal/event/flow_log.go stepTitleRegistry）。
const (
	StepSessionStart   = "computeruse.session.start"
	StepSessionDone    = "computeruse.session.done"
	StepAct            = "computeruse.act"
	StepActDone        = "computeruse.act.done"
	StepActError       = "computeruse.act.error"
	StepGroundFall     = "computeruse.grounding.fallback"
	StepBudgetExceeded = "computeruse.budget.exceeded"
	StepKillSwitch     = "computeruse.killswitch"
	// StepInjectionDetected 屏幕内容注入检出（M77）：warn 级（设计内安全防线触发，非系统故障）。
	StepInjectionDetected = "computeruse.injection.detected"
)

// 业务错误（供 service/工具层判定）。
var (
	ErrBudgetExceeded   = errors.New("computeruse: 会话步数预算已耗尽或已超时")
	ErrSessionCancelled = errors.New("computeruse: 会话已急停取消")
	ErrSessionTerminal  = errors.New("computeruse: 会话已终止")
	ErrGroundingFailed  = errors.New("computeruse: 目标元素定位失败")
	ErrBlockedProcess   = errors.New("computeruse: 前台进程在禁区内，拒绝动作")
	ErrSessionNotFound  = errors.New("computeruse: 会话不存在")
	ErrMustReobserve    = errors.New("computeruse: 上一步无可见效果，请先 observe/screenshot/wait 再执行写动作")
)

// 默认会话预算：50 步 / 30 分钟。
const (
	defaultMaxSteps = 50
	defaultDeadline = 30 * time.Minute
	maxWait         = 10 * time.Second
	askUserAfter    = 2
	maxVerifyRetry  = 2
)

// ElementMatcher a11y 元素模糊匹配器（internal/computeruse.Matcher 实现）。
// Stability:evolving
type ElementMatcher interface {
	// Match 在元素表中为 target 找最佳可交互元素；未命中返回 nil。
	Match(elements []UIElement, target string) *UIElement
}

// Deps Usecase 依赖聚合（构造注入；可 nil 的依赖已在字段注明）。
type Deps struct {
	Gateway      DeviceGateway       // sidecar 网关（必需）
	Match        ElementMatcher      // a11y 匹配器（必需）
	Vision       VisionParser        // 可 nil：nil 时跳过 SoM 视觉兜底
	Grounder     VisionGrounder      // 可 nil：nil 时跳过视觉兜底
	Specialist   VisionGrounder      // 可 nil：专用 GUI grounding（M3.2），插在 SoM 与 vlm_direct 之间
	Audit        AuditStore          // 可 nil：跳过落库（M1.4 接线）
	Events       StepEventPublisher  // 可 nil：跳过实时事件
	FlowLog      biz.FlowLogWriter   // 可 nil：跳过流程日志
	Policy       Policy              // 值类型，零值=安全默认
	Guard        InjectionGuard      // 值类型，零值=安全默认（M77 屏幕内容注入检测）
	Lg           loggateway.Logger   // 可 nil：noop
	Now          func() time.Time    // 可 nil：time.Now（测试注入）
	NewID        func() string       // 可 nil：内部自增（测试注入）
	Settle       func(time.Duration) // 可 nil：time.Sleep（动作后验证等待，测试注入 no-op）
	AuditShotDir string              // 可空：空则不落审计截图文件
}

// ComputerUseUsecase 桌面 GUI 自动化用例编排。
type ComputerUseUsecase struct {
	d Deps

	mu             sync.Mutex
	sessions       map[string]*Session // sessionID → session
	activeByAgent  map[string]string   // agentKey → 活跃 sessionID
	sessionCancels map[string]context.CancelFunc
	// suspectedByAgent agentKey → 屏幕内容注入打标（M77；每次 Observe 全量刷新，
	// 命中后该 Agent 后续写动作强制 danger 升级；内存态不落库，ADR-77-02）。
	suspectedByAgent map[string]bool
	// groundFailsByAgent 跨会话累计 grounding 失败次数（失败会话进终态会重建，
	// 计数不能只挂在 Session 上，否则 ask_user 永远达不到阈值）。
	groundFailsByAgent map[string]int
	idSeq              int
}

// NewComputerUseUsecase 构造。
func NewComputerUseUsecase(d Deps) *ComputerUseUsecase {
	if d.Lg == nil {
		d.Lg = loggateway.NewNoop()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Settle == nil {
		d.Settle = time.Sleep
	}
	u := &ComputerUseUsecase{
		d:                  d,
		sessions:           map[string]*Session{},
		activeByAgent:      map[string]string{},
		sessionCancels:     map[string]context.CancelFunc{},
		suspectedByAgent:   map[string]bool{},
		groundFailsByAgent: map[string]int{},
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
	u.transitionLocked(s, EvFinish)
	stepsUsed := s.StepsUsed
	u.mu.Unlock()
	u.d.Lg.Info("computer-use 会话已结束",
		loggateway.StepID(StepSessionDone),
		loggateway.Str("session_id", sessionID),
		loggateway.Int("steps_used", stepsUsed))
	if u.d.FlowLog != nil {
		u.d.FlowLog.LogFlowDone(ctx, sessionID, StepSessionDone, "桌面会话已结束",
			biz.LogPair{Key: "steps_used", Value: stepsUsed})
	}
	return nil
}

// KillSwitch 急停：取消进行中动作 + 会话置 cancelled，保持 activeByAgent 禁止自动重建。
// 已发出的 sidecar SendInput 无法中途撤回，急停只阻断后续步骤与 settle 验证。
func (u *ComputerUseUsecase) KillSwitch(ctx context.Context, sessionID string) error {
	u.mu.Lock()
	s, ok := u.sessions[sessionID]
	if !ok {
		u.mu.Unlock()
		return ErrSessionNotFound
	}
	cancel := u.sessionCancels[sessionID]
	if !IsTerminal(s.Status) {
		u.transitionLocked(s, EvCancel)
	}
	agentKey, stepsUsed := s.AgentKey, s.StepsUsed
	u.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	u.d.Lg.Warn("computer-use 急停触发",
		loggateway.StepID(StepKillSwitch),
		loggateway.Str("session_id", sessionID),
		loggateway.Str("agent_key", agentKey),
		loggateway.Int("steps_used", stepsUsed))
	if u.d.FlowLog != nil {
		u.d.FlowLog.LogFlowError(ctx, sessionID, StepKillSwitch, "会话已急停",
			biz.LogPair{Key: "agent_key", Value: agentKey})
	}
	return nil
}

// FailActiveOnSidecarRestart sidecar 看门狗重启后取消所有非终态会话。
// 经 KillSwitch 保持 activeByAgent，禁止自动重建直至显式 session.start（A7/A8）。
func (u *ComputerUseUsecase) FailActiveOnSidecarRestart() {
	if u == nil {
		return
	}
	u.mu.Lock()
	ids := make([]string, 0, len(u.sessions))
	for id, s := range u.sessions {
		if s != nil && !IsTerminal(s.Status) {
			ids = append(ids, id)
		}
	}
	u.mu.Unlock()
	if len(ids) == 0 {
		return
	}
	u.d.Lg.Warn("sidecar 重启，取消进行中会话",
		loggateway.StepID("computeruse.sidecar.restart"),
		loggateway.Int("count", len(ids)))
	ctx := context.Background()
	for _, id := range ids {
		_ = u.KillSwitch(ctx, id)
	}
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

// BindGoal 将会话约束原文写入（仅当现 Goal 为空）。空 goal 为 no-op。
func (u *ComputerUseUsecase) BindGoal(sessionID, goal string) error {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return nil
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	s, ok := u.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	if s.Goal == "" {
		s.Goal = goal
	}
	return nil
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

	// M77 注入检测：扫描屏幕文本，命中即打标（每次 Observe 全量刷新该 Agent 标记）。
	// 只打标不篡改：元素清单原样透出（红线）。
	hits := u.d.Guard.Scan(snap.Elements)
	suspected := len(hits) > 0
	u.mu.Lock()
	u.suspectedByAgent[req.AgentKey] = suspected
	u.mu.Unlock()
	if suspected && u.d.FlowLog != nil {
		u.d.FlowLog.LogFlowWarn(ctx, "", StepInjectionDetected, "屏幕内容疑似注入，后续写动作将强制人工确认",
			biz.LogPair{Key: "agent_key", Value: req.AgentKey},
			biz.LogPair{Key: "hits", Value: len(hits)},
			biz.LogPair{Key: "first_pattern", Value: hits[0].Pattern},
			biz.LogPair{Key: "first_ref", Value: hits[0].Ref},
			biz.LogPair{Key: "first_snippet", Value: hits[0].Snippet})
	}

	constraints := u.touchObserve(req.AgentKey, req.Goal)
	return ObserveResult{
		Summary:            summarizeElements(snap.Elements),
		Elements:           snap.Elements,
		Generation:         snap.Generation,
		Info:               info,
		SessionID:          u.ActiveSessionID(req.AgentKey),
		InjectionSuspected: suspected,
		InjectionHits:      hits,
		Constraints:        constraints,
	}, nil
}

// InjectionSuspected 供确认门查询屏幕注入打标（按 AgentKey，与工具层一致）。
func (u *ComputerUseUsecase) InjectionSuspected(agentKey string) bool {
	return u.injectionSuspectedOf(agentKey)
}

// ActiveSessionID 返回该 Agent 当前活跃会话；无则空串。
func (u *ComputerUseUsecase) ActiveSessionID(agentKey string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.activeByAgent[agentKey]
}

// injectionSuspectedOf 锁内读注入打标（M1.5-B1 锁纪律：共享状态必须经 u.mu）。
func (u *ComputerUseUsecase) injectionSuspectedOf(agentKey string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.suspectedByAgent[agentKey]
}

func (u *ComputerUseUsecase) rememberGoal(s *Session, goal string) {
	goal = strings.TrimSpace(goal)
	if s == nil || goal == "" {
		return
	}
	u.mu.Lock()
	if s.Goal == "" {
		s.Goal = goal
	}
	u.mu.Unlock()
}

func (u *ComputerUseUsecase) constraintsOf(s *Session) []string {
	if s == nil {
		return nil
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if s.Goal == "" {
		return nil
	}
	return []string{s.Goal}
}

func (u *ComputerUseUsecase) mustReobserveOf(s *Session) bool {
	if s == nil {
		return false
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return s.MustReobserve
}

func (u *ComputerUseUsecase) setMustReobserve(s *Session, v bool) {
	if s == nil {
		return
	}
	u.mu.Lock()
	s.MustReobserve = v
	u.mu.Unlock()
}

func (u *ComputerUseUsecase) noteGroundFail(s *Session) (ask bool, suggestion string) {
	key := ""
	if s != nil {
		key = s.AgentKey
	}
	if key == "" {
		return false, ""
	}
	u.mu.Lock()
	s.GroundFails++
	u.groundFailsByAgent[key]++
	n := u.groundFailsByAgent[key]
	u.mu.Unlock()
	if n < askUserAfter {
		return false, ""
	}
	return true, "连续两次未能定位目标，请向用户确认目标控件/窗口是否可见，或改用 API/文件/CLI。"
}

func (u *ComputerUseUsecase) resetGroundFails(s *Session) {
	if s == nil {
		return
	}
	u.mu.Lock()
	s.GroundFails = 0
	delete(u.groundFailsByAgent, s.AgentKey)
	u.mu.Unlock()
}

// touchObserve 为 observe 绑定约束并清除 must_reobserve（会话空闲时）。
func (u *ComputerUseUsecase) touchObserve(agentKey, goal string) []string {
	goal = strings.TrimSpace(goal)
	u.mu.Lock()
	defer u.mu.Unlock()
	sid := u.activeByAgent[agentKey]
	var s *Session
	if sid != "" {
		s = u.sessions[sid]
	}
	if s == nil || IsTerminal(s.Status) {
		if agentKey == "" {
			return nil
		}
		now := u.d.Now()
		s = &Session{
			ID:        u.d.NewID(),
			AgentKey:  agentKey,
			Status:    SessionIdle,
			Budget:    Budget{MaxSteps: defaultMaxSteps, Deadline: now.Add(defaultDeadline)},
			CreatedAt: now,
			UpdatedAt: now,
			Goal:      goal,
		}
		u.sessions[s.ID] = s
		u.activeByAgent[agentKey] = s.ID
		if s.Goal == "" {
			return nil
		}
		return []string{s.Goal}
	}
	if goal != "" && s.Goal == "" {
		s.Goal = goal
	}
	if s.Status == SessionIdle {
		s.MustReobserve = false
	}
	if s.Goal == "" {
		return nil
	}
	return []string{s.Goal}
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

// MarkReobserved 截图/观察后允许再次写动作（清除 must_reobserve）。
func (u *ComputerUseUsecase) MarkReobserved(agentKey string) {
	if strings.TrimSpace(agentKey) == "" {
		return
	}
	_ = u.touchObserve(agentKey, "")
}

// Act 语义动作：grounding 编排（a11y 优先，视觉兜底）+ 安全策略 + 预算 + 审计。
// req.Actions 非空时走批量路径（按序 fail-fast，逐步计费/审计）。
func (u *ComputerUseUsecase) Act(ctx context.Context, req ActRequest) (ActResult, error) {
	if u.d.Gateway == nil || u.d.Match == nil {
		return ActResult{}, fmt.Errorf("computeruse: gateway/matcher 未配置")
	}
	if len(req.Actions) > 0 {
		return u.actBatch(ctx, req)
	}
	s, err := u.resolveSession(req)
	if err != nil {
		return ActResult{}, err
	}
	return u.actOne(ctx, s, req)
}

// actBatch 批量动作：会话解析一次，子动作按序执行；任一步失败即停（fail-fast），
// 已完成步骤保留在 Batch。每步独立计费/状态机/审计（与单步语义一致）。
func (u *ComputerUseUsecase) actBatch(ctx context.Context, req ActRequest) (ActResult, error) {
	s, err := u.resolveSession(req)
	if err != nil {
		return ActResult{}, err
	}
	batch := make([]ActResult, 0, len(req.Actions))
	for i, sub := range req.Actions {
		res, aerr := u.actOne(ctx, s, ActRequest{
			AgentKey:    s.AgentKey,
			SessionID:   s.ID,
			Target:      sub.Target,
			Action:      sub.Action,
			Args:        sub.Args,
			DryRun:      req.DryRun,
			ConfirmedBy: req.ConfirmedBy,
		})
		if aerr != nil {
			// fail-fast：失败步不进 Batch，仅保留失败前已完成的步骤（Step 仍透出失败步供排障）。
			// 错误携带步位/已完成数：已执行的步骤真实生效，整体重试会重复操作。
			return ActResult{Step: res.Step, Batch: batch}, fmt.Errorf(
				"computeruse: 批量动作第 %d/%d 步失败（前 %d 步已执行，请勿整体重试）: %w",
				i+1, len(req.Actions), len(batch), aerr)
		}
		batch = append(batch, res)
	}
	return ActResult{Step: batch[len(batch)-1].Step, Batch: batch}, nil
}

// actOne 单步执行：预算计费 → 状态机 → 禁区检查 → grounding 执行 → 执行后验证 → 审计。
func (u *ComputerUseUsecase) actOne(ctx context.Context, s *Session, req ActRequest) (ActResult, error) {
	started := u.d.Now()
	u.rememberGoal(s, req.Goal)

	if req.Action != ActionWait && u.mustReobserveOf(s) {
		return ActResult{Constraints: u.constraintsOf(s)}, ErrMustReobserve
	}
	if req.Action == ActionWait {
		u.setMustReobserve(s, false)
	}

	// 预算守卫 + 会话占用（原子：忙/终态检查 + 计费 + 状态转换，F3）。
	stepsUsed, err := u.beginStep(s, EvGround)
	if err != nil {
		if errors.Is(err, ErrBudgetExceeded) {
			u.rejectBudgetStep(ctx, s, req, stepsUsed, err, started)
		}
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

	// M77：注入命中会话的写动作无条件升级高危（与敏感词并联，逻辑或；ADR-77-04）。
	danger := u.d.Policy.IsDanger(req.Target, req.Args) || u.injectionSuspectedOf(s.AgentKey)
	step := Step{
		SessionID:   s.ID,
		AgentKey:    s.AgentKey,
		Index:       stepsUsed,
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

	// 禁区检查：前台进程命中黑名单或窗口枚举失败均拒绝（fail-closed）。
	if err := u.checkBlockedProcess(actCtx); err != nil {
		u.transit(s, EvStepDone)
		u.finishStep(ctx, s, req, withResult(step, StepFailed, err), started)
		return ActResult{}, err
	}

	preFG := u.foregroundTitle(actCtx)
	result, preSnap, gerr := u.groundAndExecute(actCtx, s, req, &step)
	if gerr == nil && !req.DryRun {
		result.Verify = u.verifyAfterAction(actCtx, req, preSnap, preFG)
		if result.Verify != nil && result.Verify.Hint != "" {
			if step.Params == nil {
				step.Params = map[string]any{}
			}
			step.Params["verify_hint"] = result.Verify.Hint
		}
		result.Step = step
		for retries := 0; result.Verify != nil && result.Verify.HasBaseline && !result.Verify.Changed && needsEffectHint(req.Action) && retries < maxVerifyRetry; retries++ {
			idx, rerr := u.chargeRetry(s)
			if rerr != nil {
				break
			}
			retryRec := step
			retryRec.Index = idx
			retryRec.Result = StepRetry
			retryRec.Error = result.Verify.Hint
			u.finishStep(ctx, s, req, retryRec, started)
			result, preSnap, gerr = u.groundAndExecute(actCtx, s, req, &step)
			if gerr != nil {
				break
			}
			result.Verify = u.verifyAfterAction(actCtx, req, preSnap, preFG)
			if result.Verify != nil && result.Verify.Hint != "" {
				if step.Params == nil {
					step.Params = map[string]any{}
				}
				step.Params["verify_hint"] = result.Verify.Hint
			}
			result.Step = step
		}
		if result.Verify != nil && result.Verify.HasBaseline && !result.Verify.Changed && needsEffectHint(req.Action) {
			u.setMustReobserve(s, true)
		}
	}
	if gerr != nil && errors.Is(gerr, ErrGroundingFailed) {
		result.AskUser, result.Suggestion = u.noteGroundFail(s)
	} else if gerr == nil {
		u.resetGroundFails(s)
	}
	result.Constraints = u.constraintsOf(s)
	step = u.finishStep(ctx, s, req, step, started)
	result.Step = step
	if gerr != nil {
		u.transit(s, EvStepDone)
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

// rejectBudgetStep 预算拒绝善后：流程日志 + 审计被拒尝试步
// （Index = stepsUsed+1 单调递增，不与已执行步撞号）。
func (u *ComputerUseUsecase) rejectBudgetStep(ctx context.Context, s *Session, req ActRequest, stepsUsed int, err error, started time.Time) {
	if u.d.FlowLog != nil {
		u.d.FlowLog.LogFlowError(ctx, s.ID, StepBudgetExceeded, "会话预算耗尽",
			biz.LogPair{Key: "steps_used", Value: stepsUsed})
	}
	u.finishStep(ctx, s, req, Step{Index: stepsUsed + 1, Result: StepFailed, Error: err.Error(), Danger: u.d.Policy.IsDanger(req.Target, req.Args)}, started)
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

// beginStep 原子开始一步：会话忙/终态检查 + 预算守卫 + StepsUsed++ + 可选状态转换，
// 全程同一把锁——杜绝并发 Act「双计费后一者 transit 失败」的预算泄漏（F3）。
// 预算超限时经状态机把会话置 failed，并保持 activeByAgent 映射以禁止自动重建（A7）。
// ev 为空串表示只占预算不占状态机（Launch 只计费不转换）。
func (u *ComputerUseUsecase) beginStep(s *Session, ev SessionEvent) (int, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if s.Status == SessionCancelled {
		return s.StepsUsed, ErrSessionCancelled
	}
	if IsTerminal(s.Status) {
		return s.StepsUsed, ErrSessionTerminal
	}
	if s.Status != SessionIdle {
		return s.StepsUsed, fmt.Errorf("computeruse: 会话忙（%s）", s.Status)
	}
	if s.Budget.MaxSteps > 0 && s.StepsUsed >= s.Budget.MaxSteps {
		u.transitionLocked(s, EvFail)
		return s.StepsUsed, ErrBudgetExceeded
	}
	if !s.Budget.Deadline.IsZero() && u.d.Now().After(s.Budget.Deadline) {
		u.transitionLocked(s, EvFail)
		return s.StepsUsed, ErrBudgetExceeded
	}
	s.StepsUsed++
	s.UpdatedAt = u.d.Now()
	if ev != "" {
		u.transitionLocked(s, ev) // 上方已保证 idle，转换必然合法
	}
	return s.StepsUsed, nil
}

// chargeRetry 验证失败自动重试计费（同会话，不改状态机；超预算则停止重试）。
func (u *ComputerUseUsecase) chargeRetry(s *Session) (int, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if s.Status == SessionCancelled {
		return s.StepsUsed, ErrSessionCancelled
	}
	if s.Budget.MaxSteps > 0 && s.StepsUsed >= s.Budget.MaxSteps {
		return s.StepsUsed, ErrBudgetExceeded
	}
	if !s.Budget.Deadline.IsZero() && u.d.Now().After(s.Budget.Deadline) {
		return s.StepsUsed, ErrBudgetExceeded
	}
	s.StepsUsed++
	s.UpdatedAt = u.d.Now()
	return s.StepsUsed, nil
}

// transit 状态机转换（非法转换记 warn 不阻断，防状态卡死）。
func (u *ComputerUseUsecase) transit(s *Session, ev SessionEvent) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.transitionLocked(s, ev)
}

// transitionLocked 状态机转换（调用方须持 u.mu）。正常结束解除 agent 活跃映射；
// 预算耗尽/急停保持映射，禁止自动重建会话（A7）。
func (u *ComputerUseUsecase) transitionLocked(s *Session, ev SessionEvent) bool {
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
	if IsTerminal(to) && u.activeByAgent[s.AgentKey] == s.ID {
		// 仅正常结束解除映射，允许下一任务自动建会话。
		// 预算耗尽/急停保持映射，禁止自动重建（A7）。
		if to == SessionDone {
			delete(u.activeByAgent, s.AgentKey)
		}
	}
	return true
}

// statusOf 锁内读取会话当前状态（供错误消息等锁外路径使用——B1）。
func (u *ComputerUseUsecase) statusOf(s *Session) SessionStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return s.Status
}

// checkBlockedProcess 禁区检查。
func (u *ComputerUseUsecase) checkBlockedProcess(ctx context.Context) error {
	wins, err := u.d.Gateway.ListWindows(ctx)
	if err != nil {
		return fmt.Errorf("%w: 窗口枚举失败，无法确认禁区: %v", ErrBlockedProcess, err)
	}
	for _, w := range wins {
		if w.IsForeground && u.d.Policy.IsBlockedProcess(w.ProcessName) {
			return fmt.Errorf("%w: %s", ErrBlockedProcess, w.ProcessName)
		}
	}
	return nil
}

// groundAndExecute grounding 决策流（设计 §3.3 + S3 fallback 链）。
// 返回动作前快照（可 nil：直行/坐标动作无），供执行后验证对比基线。
func (u *ComputerUseUsecase) groundAndExecute(ctx context.Context, s *Session, req ActRequest, step *Step) (ActResult, *Snapshot, error) {
	// 坐标类直行动作（key/坐标 click）无需 grounding。
	if run, el, direct := u.directAction(ctx, req, step); direct {
		res, err := u.finishAction(req, step, el, run)
		return res, nil, err
	}

	// 1. 感知
	snap, err := u.d.Gateway.Snapshot(ctx, SnapshotOpts{MaxElements: 500})
	if err != nil {
		*step = withResult(*step, StepFailed, err)
		return ActResult{Step: *step}, nil, err
	}

	// 2. a11y 快路径
	hit := u.d.Match.Match(snap.Elements, req.Target)
	if hit != nil {
		step.Path = PathA11y
		res, err := u.executeOnElement(ctx, req, step, *hit, snap.Generation)
		return res, &snap, err
	}

	// 3. SoM 视觉兜底（OmniParser 可用时）：截图 → 解析 → IoU 融合 → VLM 选编号
	if u.d.Vision != nil && u.d.Grounder != nil && u.d.Vision.Available(ctx) {
		if u.d.FlowLog != nil {
			u.d.FlowLog.LogFlowWarn(ctx, s.ID, StepGroundFall, "a11y 未命中，降级 SoM 视觉兜底",
				biz.LogPair{Key: "target", Value: req.Target})
		}
		el, verr := u.visionGround(ctx, snap, req.Target)
		if verr == nil {
			step.Path = PathVision
			res, err := u.executeOnElement(ctx, req, step, el, snap.Generation)
			return res, &snap, err
		}
		// SoM 失败不终局：继续降级 VLM 坐标直判（K3 降级日志）。
		u.d.Lg.Warn("SoM 视觉兜底失败，继续降级",
			loggateway.StepID(StepGroundFall),
			loggateway.Str("session_id", s.ID),
			loggateway.Err(verr))
	}

	// 3.5 专用 GUI grounding 模型（M3.2）
	if u.d.Specialist != nil {
		if u.d.FlowLog != nil {
			u.d.FlowLog.LogFlowWarn(ctx, s.ID, StepGroundFall, "a11y/SoM 未命中，降级专用 grounding 模型",
				biz.LogPair{Key: "target", Value: req.Target})
		}
		pt, serr := u.specialistGround(ctx, req.Target)
		if serr == nil {
			step.Path = PathGrounder
			res, err := u.executeAtPoint(ctx, req, step, pt)
			return res, &snap, err
		}
		u.d.Lg.Warn("专用 grounding 失败，降级 VLM 坐标直判",
			loggateway.StepID(StepGroundFall),
			loggateway.Str("session_id", s.ID),
			loggateway.Err(serr))
	}

	// 4. VLM 坐标直判（vlm_direct：免 OmniParser 的最低精度路径，含 zoom 精化）
	if u.d.Grounder != nil {
		if u.d.FlowLog != nil {
			u.d.FlowLog.LogFlowWarn(ctx, s.ID, StepGroundFall, "a11y 未命中，降级 VLM 坐标直判",
				biz.LogPair{Key: "target", Value: req.Target})
		}
		pt, derr := u.directGround(ctx, req.Target)
		if derr == nil {
			step.Path = PathVLMDirect
			res, err := u.executeAtPoint(ctx, req, step, pt)
			return res, &snap, err
		}
		werr := fmt.Errorf("%w: %q（%v）", ErrGroundingFailed, req.Target, derr)
		*step = withResult(*step, StepFailed, werr)
		return ActResult{Step: *step}, &snap, werr
	}

	// 5. 全部视觉组件缺失（最终失败由 actOne 的 StepActError 覆盖，此处 warn 记降级链断裂）
	if u.d.FlowLog != nil {
		u.d.FlowLog.LogFlowWarn(ctx, s.ID, StepGroundFall, "a11y 未命中且视觉组件不可用",
			biz.LogPair{Key: "target", Value: req.Target})
	}
	*step = withResult(*step, StepFailed, ErrGroundingFailed)
	return ActResult{Step: *step}, &snap, fmt.Errorf("%w: %q（a11y 未命中，视觉兜底不可用）", ErrGroundingFailed, req.Target)
}

// directAction 无需 grounding 的动作：key 组合键、纯坐标 click。
// 返回 direct=true 表示本函数处理该动作；run 延迟到 finishAction 判定干跑后执行。
// wheel/drag/wait 在 P1 由 DirectAction 或元素级执行；Wait 不进 sidecar。
func (u *ComputerUseUsecase) directAction(ctx context.Context, req ActRequest, step *Step) (run func() error, el *UIElement, direct bool) {
	switch req.Action {
	case ActionFocus:
		title := strArg(req.Args, "title_regex", "")
		if strings.TrimSpace(title) == "" {
			title = req.Target
		}
		if strings.TrimSpace(title) == "" {
			return func() error { return fmt.Errorf("computeruse: focus 需要 title_regex 或 target") }, nil, true
		}
		step.Path = PathA11y
		return func() error { return u.d.Gateway.FocusWindow(ctx, title) }, nil, true
	case ActionWait:
		ms := intArg(req.Args, "ms", 0)
		if ms <= 0 {
			return func() error { return fmt.Errorf("computeruse: wait 动作缺少正数 ms 参数") }, nil, true
		}
		d := time.Duration(ms) * time.Millisecond
		if d > maxWait {
			d = maxWait
		}
		step.Path = PathA11y
		return func() error { return waitCancelable(ctx, d) }, nil, true
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
	case ActionWheel:
		x, xok := asInt(req.Args["x"])
		y, yok := asInt(req.Args["y"])
		if xok && yok && strings.TrimSpace(req.Target) == "" {
			delta := intArg(req.Args, "delta", 120)
			step.Path = PathA11y
			return func() error { return u.d.Gateway.Wheel(ctx, Point{X: x, Y: y}, delta) }, nil, true
		}
		return nil, nil, false
	case ActionDrag:
		fx, fok := asInt(req.Args["from_x"])
		fy, yok := asInt(req.Args["from_y"])
		tx, tok := asInt(req.Args["to_x"])
		ty, tyok := asInt(req.Args["to_y"])
		if fok && yok && tok && tyok {
			dur := intArg(req.Args, "duration_ms", 300)
			step.Path = PathA11y
			return func() error {
				return u.d.Gateway.Drag(ctx, Point{X: fx, Y: fy}, Point{X: tx, Y: ty}, dur)
			}, nil, true
		}
		return nil, nil, false
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
		case ActionWheel:
			return u.d.Gateway.Wheel(ctx, el.BBox.Center(), intArg(req.Args, "delta", 120))
		case ActionDrag:
			tx, tok := asInt(req.Args["to_x"])
			ty, tyok := asInt(req.Args["to_y"])
			if !tok || !tyok {
				return fmt.Errorf("computeruse: drag 需要 to_x/to_y")
			}
			return u.d.Gateway.Drag(ctx, el.BBox.Center(), Point{X: tx, Y: ty}, intArg(req.Args, "duration_ms", 300))
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

// ---------------------------------------------------------------------------
// S3：vlm_direct 坐标直判（免 OmniParser 的最低精度路径，含 zoom 精化）
// ---------------------------------------------------------------------------

const (
	// zoom 精化：以粗判点为中心截取 480x360 物理像素区域 @2x 放大重判。
	zoomRegionW = 480
	zoomRegionH = 360
	zoomFactor  = 2.0
)

// directGround VLM 坐标直判：全屏截图粗判 → 以粗判点为中心 zoom 精判 → 映射回物理坐标。
// 精化失败（截图/VLM 解析）不阻断，降级返回粗判点。
func (u *ComputerUseUsecase) directGround(ctx context.Context, target string) (Point, error) {
	full, err := u.d.Gateway.Screenshot(ctx, nil, 1.0)
	if err != nil {
		return Point{}, err
	}
	coarse, err := u.d.Grounder.PickCoordinate(ctx, full, target)
	if err != nil {
		return Point{}, err
	}
	// 粗判点即物理坐标：sidecar 为 PerMonitorV2 DPI aware（app.manifest），
	// 截图图像素与物理像素 1:1；Image.ScaleFactor 仅信息元数据，禁止再换算
	// （F1 修复：DPI 150% 下二次换算会把坐标缩到 2/3 处导致误点）。
	phys := coarse

	region := Rect{X: phys.X - zoomRegionW/2, Y: phys.Y - zoomRegionH/2, W: zoomRegionW, H: zoomRegionH}
	if region.X < 0 {
		region.X = 0
	}
	if region.Y < 0 {
		region.Y = 0
	}
	zimg, err := u.d.Gateway.Screenshot(ctx, &region, zoomFactor)
	if err != nil || zimg.Width <= 0 || zimg.Height <= 0 {
		return phys, nil
	}
	fine, err := u.d.Grounder.PickCoordinate(ctx, zimg, target)
	if err != nil {
		return phys, nil
	}
	// 精判点（zoom 图像素）映射回物理坐标。
	return Point{
		X: region.X + fine.X*region.W/zimg.Width,
		Y: region.Y + fine.Y*region.H/zimg.Height,
	}, nil
}

// executeAtPoint 在物理坐标点上执行动作（vlm_direct 无元素 ref，invoke 降级为 click）。
func (u *ComputerUseUsecase) executeAtPoint(ctx context.Context, req ActRequest, step *Step, pt Point) (ActResult, error) {
	return u.finishAction(req, step, nil, func() error {
		switch req.Action {
		case ActionClick, ActionInvoke, "":
			return u.d.Gateway.Click(ctx, pt, strArg(req.Args, "button", "left"), intArg(req.Args, "click_count", 1))
		case ActionTypeText:
			text, _ := req.Args["text"].(string)
			if err := u.d.Gateway.Click(ctx, pt, "left", 1); err != nil {
				return err
			}
			return u.d.Gateway.TypeText(ctx, text)
		case ActionWheel:
			return u.d.Gateway.Wheel(ctx, pt, intArg(req.Args, "delta", 120))
		case ActionDrag:
			tx, tok := asInt(req.Args["to_x"])
			ty, tyok := asInt(req.Args["to_y"])
			if !tok || !tyok {
				return fmt.Errorf("computeruse: drag 需要 to_x/to_y")
			}
			return u.d.Gateway.Drag(ctx, pt, Point{X: tx, Y: ty}, intArg(req.Args, "duration_ms", 300))
		default:
			return fmt.Errorf("computeruse: 动作 %s 不支持坐标级执行", req.Action)
		}
	})
}

// ---------------------------------------------------------------------------
// S4：执行后验证闭环（settle → post-snapshot 树哈希对比 + 前台窗口）
// ---------------------------------------------------------------------------

// verifySettleDelay 动作后等待 UI 稳定的时间（测试注入 no-op 跳过）。
const verifySettleDelay = 400 * time.Millisecond

// verifyAfterAction 执行后验证：settle 等待 UI 稳定 → 重新快照 → 对比动作前基线。
// preSnap 为 nil（直行/坐标动作无 grounding 基线）时仅回报前台窗口。
// 验证失败（快照不可达）降级：仅记录告警，不影响已成功动作的结果。
func (u *ComputerUseUsecase) verifyAfterAction(ctx context.Context, req ActRequest, preSnap *Snapshot, preFG string) *ActionVerify {
	if ctx.Err() != nil {
		return &ActionVerify{ForegroundBefore: preFG}
	}
	u.d.Settle(verifySettleDelay)
	if ctx.Err() != nil {
		return &ActionVerify{ForegroundBefore: preFG}
	}
	v := &ActionVerify{ForegroundBefore: preFG}
	v.ForegroundAfter = u.foregroundTitle(ctx)
	if preSnap == nil || len(preSnap.Elements) == 0 {
		return v // HasBaseline=false：直行/坐标动作或空树，不触发重试
	}
	v.HasBaseline = true
	post, err := u.d.Gateway.Snapshot(ctx, SnapshotOpts{MaxElements: 500})
	if err != nil {
		u.d.Lg.Warn("computer-use 执行后验证快照失败，跳过对比",
			loggateway.StepID(StepAct), loggateway.Err(err))
		return v
	}
	v.Changed = elementTreeHash(preSnap.Elements) != elementTreeHash(post.Elements) ||
		v.ForegroundAfter != preFG
	if !v.Changed && needsEffectHint(req.Action) {
		v.Hint = "no_observable_effect"
	}
	return v
}

// needsEffectHint click/invoke/type 等元素级动作预期产生可见 UI 变化。
func needsEffectHint(a ActionType) bool {
	switch a {
	case ActionClick, ActionInvoke, ActionTypeText, ActionWheel, ActionDrag, "":
		return true
	}
	return false
}

// foregroundTitle 当前前台窗口标题（枚举失败/无前台返回 ""）。
func (u *ComputerUseUsecase) foregroundTitle(ctx context.Context) string {
	wins, err := u.d.Gateway.ListWindows(ctx)
	if err != nil {
		return ""
	}
	for _, w := range wins {
		if w.IsForeground {
			return w.Title
		}
	}
	return ""
}

// elementTreeHash 元素树内容哈希：排除 ref/generation（跨快照不稳定），
// 保留语义内容（名称/类型/位置/可用态），用于动作前后对比。
func elementTreeHash(elements []UIElement) uint64 {
	h := fnv.New64a()
	for _, el := range elements {
		fmt.Fprintf(h, "%s|%s|%d,%d,%d,%d|%t|%t\n",
			el.Name, el.Type, el.BBox.X, el.BBox.Y, el.BBox.W, el.BBox.H,
			el.Enabled, el.Interactivity)
	}
	return h.Sum64()
}

func (u *ComputerUseUsecase) persistAuditScreenshot(ctx context.Context, step Step) string {
	if u.d.AuditShotDir == "" || u.d.Gateway == nil || step.Result == StepDryRun {
		return ""
	}
	img, err := u.d.Gateway.Screenshot(ctx, nil, 1)
	if err != nil || len(img.PNG) == 0 {
		return ""
	}
	if err := os.MkdirAll(u.d.AuditShotDir, 0o755); err != nil {
		u.d.Lg.Warn("computer-use 审计截图目录创建失败", loggateway.StepID(StepAct), loggateway.Err(err))
		return ""
	}
	name := fmt.Sprintf("%s-%d.png", step.SessionID, step.Index)
	path := filepath.Join(u.d.AuditShotDir, name)
	if err := os.WriteFile(path, img.PNG, 0o644); err != nil {
		u.d.Lg.Warn("computer-use 审计截图写入失败", loggateway.StepID(StepAct), loggateway.Err(err))
		return ""
	}
	return path
}

// finishStep 审计落库 + 实时事件（尽力而为，不阻断主流程）。
func (u *ComputerUseUsecase) finishStep(ctx context.Context, s *Session, req ActRequest, step Step, started time.Time) Step {
	step.SessionID = s.ID
	step.AgentKey = s.AgentKey
	if step.Index == 0 {
		u.mu.Lock()
		step.Index = s.StepsUsed
		u.mu.Unlock()
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
	step.Degraded = step.Path == PathVLMDirect
	if step.ScreenshotRef == "" {
		step.ScreenshotRef = u.persistAuditScreenshot(ctx, step)
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
	return step
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
	if u.mustReobserveOf(s) {
		return Step{}, ErrMustReobserve
	}
	stepsUsed, err := u.beginStep(s, "") // Launch 只计费不占状态机
	if err != nil {
		return Step{}, err
	}
	step := Step{
		SessionID: s.ID, AgentKey: agentKey, Index: stepsUsed,
		Target: target, Action: ActionLaunch, Path: PathA11y,
		Params:      map[string]any{"target": target, "args": args},
		Danger:      u.d.Policy.IsDanger(target, map[string]any{"target": target}) || u.injectionSuspectedOf(agentKey),
		ConfirmedBy: confirmedBy, CreatedAt: started,
	}
	pid, lerr := u.d.Gateway.Launch(ctx, target, args, workDir)
	if lerr != nil {
		step = withResult(step, StepFailed, lerr)
	} else {
		step.Result = StepOK
		step.Params["pid"] = pid
	}
	step = u.finishStep(ctx, s, ActRequest{AgentKey: agentKey, Target: target, Action: ActionLaunch}, step, started)
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

func waitCancelable(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (u *ComputerUseUsecase) specialistGround(ctx context.Context, target string) (Point, error) {
	img, err := u.d.Gateway.Screenshot(ctx, nil, 1.0)
	if err != nil {
		return Point{}, err
	}
	return u.d.Specialist.PickCoordinate(ctx, img, target)
}
