package computeruse

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// --- fakes ---

type fakeGateway struct {
	snap       Snapshot
	info       DeviceInfo
	windows    []WindowInfo
	invokedRef string
	clicked    *Point
	typed      string
	keyed      string
	launched   string
	err        error

	// S3/S4 扩展：覆盖默认快照/截图行为（verify 前后对比、zoom 精化断言）。
	snapFn     func(SnapshotOpts) (Snapshot, error)
	shotFn     func(*Rect, float64) (Image, error)
	shotCalls  int
	lastRegion *Rect
	lastZoom   float64
}

func (f *fakeGateway) Info(context.Context) (DeviceInfo, error) {
	if f.info.Platform == "" {
		return DeviceInfo{Platform: "windows", ScreenW: 1920, ScreenH: 1080, ScaleFactor: 1.0}, nil
	}
	return f.info, nil
}

func (f *fakeGateway) Snapshot(_ context.Context, opts SnapshotOpts) (Snapshot, error) {
	if f.snapFn != nil {
		return f.snapFn(opts)
	}
	if f.err != nil {
		return Snapshot{}, f.err
	}
	return f.snap, nil
}

func (f *fakeGateway) Screenshot(_ context.Context, r *Rect, zoom float64) (Image, error) {
	f.shotCalls++
	f.lastRegion = r
	f.lastZoom = zoom
	if f.shotFn != nil {
		return f.shotFn(r, zoom)
	}
	return Image{PNG: []byte("png"), Width: 100, Height: 100, ScaleFactor: 1}, nil
}

func (f *fakeGateway) Invoke(_ context.Context, ref string, _ int) error {
	f.invokedRef = ref
	return f.err
}

func (f *fakeGateway) Click(_ context.Context, p Point, _ string, _ int) error {
	f.clicked = &p
	return f.err
}

func (f *fakeGateway) TypeText(_ context.Context, text string) error {
	f.typed = text
	return f.err
}

func (f *fakeGateway) Key(_ context.Context, combo string) error {
	f.keyed = combo
	return f.err
}

func (f *fakeGateway) FocusWindow(context.Context, string) error { return f.err }

func (f *fakeGateway) Launch(_ context.Context, target, _, _ string) (int, error) {
	f.launched = target
	return 1234, f.err
}

func (f *fakeGateway) ListWindows(context.Context) ([]WindowInfo, error) { return f.windows, nil }

type fakeMatcher struct{ hit *UIElement }

func (m fakeMatcher) Match(_ []UIElement, _ string) *UIElement { return m.hit }

// mutableMatcher 可在测试中途切换命中结果（B2 回归：先失败后成功）。
type mutableMatcher struct{ hit *UIElement }

func (m *mutableMatcher) Match(_ []UIElement, _ string) *UIElement { return m.hit }

type fakeAudit struct{ entries []AuditEntry }

func (a *fakeAudit) RecordStep(_ context.Context, e AuditEntry) error {
	a.entries = append(a.entries, e)
	return nil
}

func (a *fakeAudit) ListSteps(_ context.Context, sessionID string) ([]AuditEntry, error) {
	var out []AuditEntry
	for _, e := range a.entries {
		if e.SessionID == sessionID {
			out = append(out, e)
		}
	}
	return out, nil
}

type fakeEvents struct{ steps []Step }

func (p *fakeEvents) PublishStep(_ context.Context, s Step) { p.steps = append(p.steps, s) }

// fakeFlowLog 捕获流程日志调用级别（F2：降级必须 warn 而非 error）。
type fakeFlowLog struct{ warnSteps, errorSteps []string }

func (f *fakeFlowLog) LogFlowStart(_ context.Context, _, stepID, _ string, _ ...biz.LogPair) {}
func (f *fakeFlowLog) LogFlowDone(_ context.Context, _, stepID, _ string, _ ...biz.LogPair)  {}
func (f *fakeFlowLog) LogFlowError(_ context.Context, _, stepID, _ string, _ ...biz.LogPair) {
	f.errorSteps = append(f.errorSteps, stepID)
}
func (f *fakeFlowLog) LogFlowWarn(_ context.Context, _, stepID, _ string, _ ...biz.LogPair) {
	f.warnSteps = append(f.warnSteps, stepID)
}

func newTestUsecase(gw *fakeGateway, m ElementMatcher) (*ComputerUseUsecase, *fakeAudit, *fakeEvents) {
	audit := &fakeAudit{}
	events := &fakeEvents{}
	now := time.Now()
	u := NewComputerUseUsecase(Deps{
		Gateway: gw,
		Match:   m,
		Audit:   audit,
		Events:  events,
		Now:     func() time.Time { return now },
		Settle:  func(time.Duration) {}, // 测试跳过 verify settle 等待
	})
	return u, audit, events
}

// newVisionUsecase 带视觉组件的用例（S3 grounding 链测试）。
func newVisionUsecase(gw *fakeGateway, m ElementMatcher, v VisionParser, gr VisionGrounder) *ComputerUseUsecase {
	return NewComputerUseUsecase(Deps{
		Gateway:  gw,
		Match:    m,
		Vision:   v,
		Grounder: gr,
		Settle:   func(time.Duration) {},
	})
}

// fakeVisionParser 实现 VisionParser（OmniParser 桩）。
type fakeVisionParser struct {
	available bool
	els       []UIElement
	err       error
}

func (p *fakeVisionParser) Available(context.Context) bool { return p.available }
func (p *fakeVisionParser) Parse(context.Context, Image) ([]UIElement, error) {
	return p.els, p.err
}

// fakeGrounder 实现 VisionGrounder；coordFn 可按图像尺寸区分全屏/zoom 调用。
type fakeGrounder struct {
	pickRef    string
	pickErr    error
	coordFn    func(Image) (Point, error)
	coordCalls int
}

func (g *fakeGrounder) Pick(context.Context, Image, []UIElement, string) (string, error) {
	return g.pickRef, g.pickErr
}

func (g *fakeGrounder) PickCoordinate(_ context.Context, img Image, _ string) (Point, error) {
	g.coordCalls++
	if g.coordFn == nil {
		return Point{}, ErrGroundingFailed
	}
	return g.coordFn(img)
}

var saveButton = UIElement{
	Ref: "g1.e3", Type: "button", Name: "保存",
	BBox:          Rect{X: 100, Y: 200, W: 80, H: 28},
	Interactivity: true, Source: "uia", Enabled: true, Generation: 1,
}

// dispatchMatcher 按 target 分派命中元素（批量动作测试）。
type dispatchMatcher struct{ byTarget map[string]*UIElement }

func (m dispatchMatcher) Match(_ []UIElement, target string) *UIElement { return m.byTarget[target] }

// --- tests ---

func TestAct_A11yHitInvoke(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, audit, events := newTestUsecase(gw, fakeMatcher{hit: &saveButton})

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "保存", Action: ActionInvoke,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if gw.invokedRef != "g1.e3" {
		t.Errorf("invokedRef = %q, want g1.e3", gw.invokedRef)
	}
	if res.Step.Path != PathA11y || res.Step.Result != StepOK {
		t.Errorf("step path/result = %s/%s", res.Step.Path, res.Step.Result)
	}
	if len(audit.entries) != 1 || audit.entries[0].Target != "保存" {
		t.Errorf("audit entries = %+v", audit.entries)
	}
	if len(events.steps) != 1 {
		t.Errorf("events steps = %d", len(events.steps))
	}
	// 会话自动创建且回到 idle
	s, _ := u.GetSession(context.Background(), res.Step.SessionID)
	if s.Status != SessionIdle || s.StepsUsed != 1 {
		t.Errorf("session status=%s steps=%d", s.Status, s.StepsUsed)
	}
}

func TestAct_MissNoVisionFails(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, _, _ := newTestUsecase(gw, fakeMatcher{hit: nil}) // Vision/Grounder 未接线

	_, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "不存在的元素", Action: ActionInvoke,
	})
	if !errors.Is(err, ErrGroundingFailed) {
		t.Errorf("err = %v, want ErrGroundingFailed", err)
	}
}

func TestAct_DryRunNoInjection(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, _, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "保存", Action: ActionInvoke, DryRun: true,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if gw.invokedRef != "" {
		t.Error("dry-run must not inject")
	}
	if res.Plan == nil || res.Plan.ResolvedRef != "g1.e3" || res.Step.Result != StepDryRun {
		t.Errorf("plan = %+v result=%s", res.Plan, res.Step.Result)
	}
}

func TestAct_BudgetExceeded(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, _, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})
	ctx := context.Background()

	s, _ := u.StartSession(ctx, "agent1", Budget{MaxSteps: 1, Deadline: time.Now().Add(time.Hour)})
	if _, err := u.Act(ctx, ActRequest{AgentKey: "agent1", SessionID: s.ID, Target: "保存", Action: ActionInvoke}); err != nil {
		t.Fatalf("first act err: %v", err)
	}
	_, err := u.Act(ctx, ActRequest{AgentKey: "agent1", SessionID: s.ID, Target: "保存", Action: ActionInvoke})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("err = %v, want ErrBudgetExceeded", err)
	}
}

func TestAct_BlockedProcessRejected(t *testing.T) {
	gw := &fakeGateway{
		snap:    Snapshot{Elements: []UIElement{saveButton}, Generation: 1},
		windows: []WindowInfo{{ProcessName: "keepass.exe", IsForeground: true}},
	}
	u, _, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})

	_, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "保存", Action: ActionInvoke,
	})
	if !errors.Is(err, ErrBlockedProcess) {
		t.Errorf("err = %v, want ErrBlockedProcess", err)
	}
	if gw.invokedRef != "" {
		t.Error("blocked process must not inject")
	}
}

func TestAct_DangerFlagged(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, audit, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})

	_, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "点击删除按钮", Action: ActionInvoke, ConfirmedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if !audit.entries[0].Danger {
		t.Error("danger word should mark step.danger")
	}
	if audit.entries[0].ConfirmedBy != "user-1" {
		t.Errorf("confirmed_by = %q", audit.entries[0].ConfirmedBy)
	}
}

func TestAct_KeyDirect(t *testing.T) {
	gw := &fakeGateway{}
	u, _, _ := newTestUsecase(gw, fakeMatcher{})

	_, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Action: ActionKey, Args: map[string]any{"combo": "ctrl+s"},
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if gw.keyed != "ctrl+s" {
		t.Errorf("keyed = %q", gw.keyed)
	}
}

func TestKillSwitch_CancelsSession(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, _, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})
	ctx := context.Background()

	s, _ := u.StartSession(ctx, "agent1", Budget{})
	if err := u.KillSwitch(ctx, s.ID); err != nil {
		t.Fatalf("KillSwitch err: %v", err)
	}
	got, _ := u.GetSession(ctx, s.ID)
	if got.Status != SessionCancelled {
		t.Errorf("status = %s, want cancelled", got.Status)
	}
	// 急停后 act 拒绝
	_, err := u.Act(ctx, ActRequest{AgentKey: "agent1", SessionID: s.ID, Target: "保存", Action: ActionInvoke})
	if !errors.Is(err, ErrSessionCancelled) {
		t.Errorf("err = %v, want ErrSessionCancelled", err)
	}
}

func TestLaunch_ChargesBudgetAndAudits(t *testing.T) {
	gw := &fakeGateway{}
	u, audit, _ := newTestUsecase(gw, fakeMatcher{})

	step, err := u.Launch(context.Background(), "agent1", "notepad.exe", "", "", "")
	if err != nil {
		t.Fatalf("Launch err: %v", err)
	}
	if gw.launched != "notepad.exe" {
		t.Errorf("launched = %q", gw.launched)
	}
	if step.Result != StepOK || step.Action != ActionLaunch {
		t.Errorf("step = %+v", step)
	}
	if len(audit.entries) != 1 {
		t.Errorf("audit entries = %d", len(audit.entries))
	}
}

func TestObserve_SummaryContainsElements(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 7}}
	u, _, _ := newTestUsecase(gw, fakeMatcher{})

	res, err := u.Observe(context.Background(), ObserveRequest{AgentKey: "agent1"})
	if err != nil {
		t.Fatalf("Observe err: %v", err)
	}
	if res.Generation != 7 || len(res.Elements) != 1 {
		t.Errorf("observe result = %+v", res)
	}
	if res.Summary == "" {
		t.Error("summary should not be empty")
	}
}

// --- 75 review 修复回归（B1/B2/B3）---

// B2：动作失败把会话置 failed 后，agent 不得被永久阻塞——下一次 Act 自动重建会话。
func TestAct_FailedSessionDoesNotBlockAgent(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	m := &mutableMatcher{} // 首轮必失败（a11y 未命中且无视觉兜底）
	u, _, _ := newTestUsecase(gw, m)
	ctx := context.Background()

	if _, err := u.Act(ctx, ActRequest{AgentKey: "agent1", Target: "不存在", Action: ActionInvoke}); !errors.Is(err, ErrGroundingFailed) {
		t.Fatalf("first act err = %v, want ErrGroundingFailed", err)
	}

	m.hit = &saveButton // 修复条件后重试
	res, err := u.Act(ctx, ActRequest{AgentKey: "agent1", Target: "保存", Action: ActionInvoke})
	if err != nil {
		t.Fatalf("second act should auto-recreate session, got err: %v", err)
	}
	if res.Step.Result != StepOK {
		t.Errorf("second act result = %s", res.Step.Result)
	}
	// 显式引用已失败会话仍拒绝（终态语义不变）。
	s1, _ := u.GetSession(ctx, res.Step.SessionID)
	_ = s1
	failed := ""
	u.mu.Lock()
	for id, s := range u.sessions {
		if s.Status == SessionFailed {
			failed = id
		}
	}
	u.mu.Unlock()
	if failed == "" {
		t.Fatal("expected a failed session recorded")
	}
	if _, err := u.Act(ctx, ActRequest{AgentKey: "agent1", SessionID: failed, Target: "保存", Action: ActionInvoke}); !errors.Is(err, ErrSessionTerminal) {
		t.Errorf("act on failed session err = %v, want ErrSessionTerminal", err)
	}
}

// B2：预算耗尽置 failed 后，agent 下一次 Act 自动换新会话，不再报 ErrSessionTerminal。
func TestAct_BudgetExhaustedSessionDoesNotBlockAgent(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, _, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})
	ctx := context.Background()

	s, _ := u.StartSession(ctx, "agent1", Budget{MaxSteps: 1, Deadline: time.Now().Add(time.Hour)})
	if _, err := u.Act(ctx, ActRequest{AgentKey: "agent1", SessionID: s.ID, Target: "保存", Action: ActionInvoke}); err != nil {
		t.Fatalf("first act err: %v", err)
	}
	if _, err := u.Act(ctx, ActRequest{AgentKey: "agent1", SessionID: s.ID, Target: "保存", Action: ActionInvoke}); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("second act err = %v, want ErrBudgetExceeded", err)
	}
	// 不显式指定会话：应自动创建新会话并成功。
	res, err := u.Act(ctx, ActRequest{AgentKey: "agent1", Target: "保存", Action: ActionInvoke})
	if err != nil {
		t.Fatalf("auto-recreate after budget exhaustion err: %v", err)
	}
	if res.Step.SessionID == s.ID {
		t.Error("expected a fresh session id, got the exhausted one")
	}
}

// F3：beginStep 原子性——同一会话并发开始步骤，恰好一个成功、其余忙拒绝，
// StepsUsed 严格等于成功数（旧 chargeBudget+分离 transit 序列在并发下双计费泄漏：
// 两者都过 idle 检查与计费，后到者 transit 失败直接返回，预算白扣且无审计）。
func TestBeginStep_ConcurrentNoDoubleCharge(t *testing.T) {
	u, _, _ := newTestUsecase(&fakeGateway{}, fakeMatcher{})
	if _, err := u.StartSession(context.Background(), "agent1", Budget{}); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	s, err := u.resolveSession(ActRequest{AgentKey: "agent1"})
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = u.beginStep(s, EvGround)
		}(i)
	}
	wg.Wait()

	okCount, busyCount := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			okCount++
		case strings.Contains(err.Error(), "会话忙"):
			busyCount++
		default:
			t.Errorf("beginStep err = %v, want nil 或会话忙", err)
		}
	}
	if okCount != 1 {
		t.Errorf("成功数 = %d, want 1", okCount)
	}
	if busyCount != goroutines-1 {
		t.Errorf("忙拒绝数 = %d, want %d", busyCount, goroutines-1)
	}
	if s.StepsUsed != 1 {
		t.Errorf("StepsUsed = %d, want 1（无预算泄漏）", s.StepsUsed)
	}
}

// F3 关联：预算耗尽的被拒步审计索引 = stepsUsed+1，单调不重复
// （旧实现回填 s.StepsUsed，与最后成功步索引撞号）。
func TestAct_BudgetExceededRejectedStepIndex(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, audit, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})
	ctx := context.Background()

	s, _ := u.StartSession(ctx, "agent1", Budget{MaxSteps: 1, Deadline: time.Now().Add(time.Hour)})
	if _, err := u.Act(ctx, ActRequest{AgentKey: "agent1", SessionID: s.ID, Target: "保存", Action: ActionInvoke}); err != nil {
		t.Fatalf("first act err: %v", err)
	}
	if _, err := u.Act(ctx, ActRequest{AgentKey: "agent1", SessionID: s.ID, Target: "保存", Action: ActionInvoke}); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("second act err = %v, want ErrBudgetExceeded", err)
	}
	entries, err := audit.ListSteps(ctx, s.ID)
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("audit entries = %d, want 2（成功 1 + 被拒 1）", len(entries))
	}
	if entries[0].Index != 1 || entries[1].Index != 2 {
		t.Errorf("audit index = [%d, %d], want [1, 2]（被拒步索引单调不重复）", entries[0].Index, entries[1].Index)
	}
}

// B1/B3：KillSwitch 与进行中 Act 并发——状态经状态机转换，字段访问全部持锁。
// go test -race 下验证无数据竞态；功能上动作要么完成要么被取消，不得死锁。
func TestAct_ConcurrentKillSwitch(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, _, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})
	ctx := context.Background()

	s, _ := u.StartSession(ctx, "agent1", Budget{})
	done := make(chan error, 1)
	go func() {
		_, err := u.Act(ctx, ActRequest{AgentKey: "agent1", SessionID: s.ID, Target: "保存", Action: ActionInvoke})
		done <- err
	}()
	for i := 0; i < 10; i++ {
		_ = u.KillSwitch(ctx, s.ID)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("act did not return after kill switch")
	}
	got, _ := u.GetSession(ctx, s.ID)
	if got.Status != SessionCancelled {
		t.Errorf("status = %s, want cancelled", got.Status)
	}
}

// ---------------------------------------------------------------------------
// S3：grounding fallback 链（a11y → OmniParser SoM+VLM → VLM 坐标直判 + zoom 精化）
// ---------------------------------------------------------------------------

// SoM 路径正常命中：OmniParser 可用 + Pick 返回 vision 元素 ref → PathVision，点击元素中心。
func TestAct_VisionSoMHit(t *testing.T) {
	visEl := UIElement{
		Ref: "g1.v0", Type: "icon", Name: "保存图标",
		BBox:          Rect{X: 300, Y: 100, W: 40, H: 40},
		Interactivity: true, Source: "vision", Enabled: true, Generation: 1,
	}
	gw := &fakeGateway{snap: Snapshot{Generation: 1}}
	v := &fakeVisionParser{available: true, els: []UIElement{visEl}}
	gr := &fakeGrounder{pickRef: "g1.v0"}
	u := newVisionUsecase(gw, fakeMatcher{hit: nil}, v, gr)

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "保存图标", Action: ActionClick,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if res.Step.Path != PathVision {
		t.Errorf("path = %s, want vision", res.Step.Path)
	}
	want := visEl.BBox.Center()
	if gw.clicked == nil || *gw.clicked != want {
		t.Errorf("clicked = %v, want %v", gw.clicked, want)
	}
}

// SoM 解析失败（OmniParser /parse 报错）→ 降级 VLM 坐标直判。
func TestAct_VisionSoMFailureFallsBackToDirect(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Generation: 1}}
	v := &fakeVisionParser{available: true, err: errors.New("omniparser 500")}
	gr := &fakeGrounder{coordFn: func(img Image) (Point, error) {
		return Point{X: img.Width / 2, Y: img.Height / 2}, nil
	}}
	u := newVisionUsecase(gw, fakeMatcher{hit: nil}, v, gr)

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "某图标", Action: ActionClick,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if res.Step.Path != PathVLMDirect {
		t.Errorf("path = %s, want vlm_direct", res.Step.Path)
	}
	if gw.clicked == nil {
		t.Fatal("expected coordinate click")
	}
}

// VLM 直判 zoom 精化：全屏粗判 → 以粗判点为中心 480x360 裁剪 2x 精判 → 映射回物理坐标。
// 数学：全屏 1000x800 粗判 (500,400) → region(260,220,480,360) zoom2 → zimg 960x720
// 精判 (480,360) → 物理 (260+480*480/960, 220+360*360/720) = (500,400)。
func TestAct_VLMDirectZoomRefinement(t *testing.T) {
	gw := &fakeGateway{
		snap: Snapshot{Generation: 1},
		shotFn: func(r *Rect, zoom float64) (Image, error) {
			if r == nil {
				return Image{PNG: []byte("full"), Width: 1000, Height: 800, ScaleFactor: 1}, nil
			}
			return Image{PNG: []byte("zoom"), Width: int(float64(r.W) * zoom), Height: int(float64(r.H) * zoom), ScaleFactor: 1}, nil
		},
	}
	gr := &fakeGrounder{coordFn: func(img Image) (Point, error) {
		if img.Width == 1000 { // 全屏粗判
			return Point{X: 500, Y: 400}, nil
		}
		return Point{X: 480, Y: 360}, nil // zoom 图精判（中心）
	}}
	u := newVisionUsecase(gw, fakeMatcher{hit: nil}, nil, gr)

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "某图标", Action: ActionClick,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if res.Step.Path != PathVLMDirect {
		t.Errorf("path = %s, want vlm_direct", res.Step.Path)
	}
	if gr.coordCalls != 2 {
		t.Errorf("coordCalls = %d, want 2（粗判+精判）", gr.coordCalls)
	}
	if gw.shotCalls != 2 || gw.lastZoom != 2.0 {
		t.Errorf("shotCalls=%d lastZoom=%v, want 2 次且第二次 zoom=2", gw.shotCalls, gw.lastZoom)
	}
	if gw.lastRegion == nil || *gw.lastRegion != (Rect{X: 260, Y: 220, W: 480, H: 360}) {
		t.Errorf("lastRegion = %+v, want {260 220 480 360}", gw.lastRegion)
	}
	if gw.clicked == nil || *gw.clicked != (Point{X: 500, Y: 400}) {
		t.Errorf("clicked = %v, want {500 400}", gw.clicked)
	}
}

// F2：a11y 未命中降级 SoM 是设计内降级，流程日志必须 warn 级（K3），
// 不得用 error 级把正常 fallback 渲染成故障。
func TestAct_GroundingFallbackLogsWarn(t *testing.T) {
	visEl := UIElement{
		Ref: "g1.v0", Type: "icon", Name: "保存图标",
		BBox:          Rect{X: 300, Y: 100, W: 40, H: 40},
		Interactivity: true, Source: "vision", Enabled: true, Generation: 1,
	}
	gw := &fakeGateway{snap: Snapshot{Generation: 1}}
	flow := &fakeFlowLog{}
	u := NewComputerUseUsecase(Deps{
		Gateway:  gw,
		Match:    fakeMatcher{hit: nil},
		Vision:   &fakeVisionParser{available: true, els: []UIElement{visEl}},
		Grounder: &fakeGrounder{pickRef: "g1.v0"},
		FlowLog:  flow,
		Settle:   func(time.Duration) {},
	})

	if _, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "保存图标", Action: ActionClick,
	}); err != nil {
		t.Fatalf("Act err: %v", err)
	}
	for _, id := range flow.warnSteps {
		if id == StepGroundFall {
			return // 命中：warn 级降级日志
		}
	}
	t.Errorf("StepGroundFall 应以 warn 级记录，warnSteps=%v errorSteps=%v", flow.warnSteps, flow.errorSteps)
}

// F1 回归：DPI 缩放显示器（ScaleFactor=1.5）下 vlm_direct 不得二次换算。
// sidecar 为 PerMonitorV2 DPI aware（app.manifest），截图图像素==物理像素，
// ScaleFactor 仅信息元数据；再除一次会把粗判点缩到 2/3 处导致误点。
// 数学：全屏 1500x1000 粗判 (750,500) → region(510,320,480,360) zoom2 → zimg 960x720
// 精判 (480,360) → 物理 (510+480*480/960, 320+360*360/720) = (750,500)。
func TestAct_VLMDirectScaledDisplay(t *testing.T) {
	gw := &fakeGateway{
		snap: Snapshot{Generation: 1},
		shotFn: func(r *Rect, zoom float64) (Image, error) {
			if r == nil {
				return Image{PNG: []byte("full"), Width: 1500, Height: 1000, ScaleFactor: 1.5}, nil
			}
			return Image{PNG: []byte("zoom"), Width: int(float64(r.W) * zoom), Height: int(float64(r.H) * zoom), ScaleFactor: 1.5}, nil
		},
	}
	gr := &fakeGrounder{coordFn: func(img Image) (Point, error) {
		if img.Width == 1500 { // 全屏粗判
			return Point{X: 750, Y: 500}, nil
		}
		return Point{X: 480, Y: 360}, nil // zoom 图精判（中心）
	}}
	u := newVisionUsecase(gw, fakeMatcher{hit: nil}, nil, gr)

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "某图标", Action: ActionClick,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if res.Step.Path != PathVLMDirect {
		t.Errorf("path = %s, want vlm_direct", res.Step.Path)
	}
	if gw.lastRegion == nil || *gw.lastRegion != (Rect{X: 510, Y: 320, W: 480, H: 360}) {
		t.Errorf("lastRegion = %+v, want {510 320 480 360}（图像素即物理像素）", gw.lastRegion)
	}
	if gw.clicked == nil || *gw.clicked != (Point{X: 750, Y: 500}) {
		t.Errorf("clicked = %v, want {750 500}（禁止再除 ScaleFactor）", gw.clicked)
	}
}

// zoom 精化失败（截图或精判报错）→ 降级用粗判坐标，不算失败。
func TestAct_VLMDirectCoarseFallbackWhenZoomFails(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Generation: 1}} // 默认截图 100x100
	calls := 0
	gr := &fakeGrounder{coordFn: func(img Image) (Point, error) {
		calls++
		if calls == 1 {
			return Point{X: 50, Y: 50}, nil // 粗判
		}
		return Point{}, errors.New("vlm zoom parse failed") // 精判失败
	}}
	u := newVisionUsecase(gw, fakeMatcher{hit: nil}, nil, gr)

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "某图标", Action: ActionClick,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if gw.clicked == nil || *gw.clicked != (Point{X: 50, Y: 50}) {
		t.Errorf("clicked = %v, want 粗判点 {50 50}", gw.clicked)
	}
	if res.Step.Result != StepOK {
		t.Errorf("result = %s, want ok（精化失败不阻断）", res.Step.Result)
	}
}

// vlm_direct 路径 invoke 动作无元素 ref → 降级为坐标 click。
func TestAct_VLMDirectInvokeDegradesToClick(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Generation: 1}}
	gr := &fakeGrounder{coordFn: func(img Image) (Point, error) {
		return Point{X: 10, Y: 20}, nil
	}}
	u := newVisionUsecase(gw, fakeMatcher{hit: nil}, nil, gr)

	_, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "某按钮", Action: ActionInvoke,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if gw.clicked == nil {
		t.Error("vlm_direct + invoke 应降级为坐标 click")
	}
	if gw.invokedRef != "" {
		t.Errorf("vlm_direct 无 ref 不应走 invoke，got %q", gw.invokedRef)
	}
}

// 全部视觉路径失败 → ErrGroundingFailed。
func TestAct_GroundingFailsWhenAllVisionFails(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Generation: 1}}
	gr := &fakeGrounder{coordFn: func(Image) (Point, error) { return Point{}, errors.New("vlm down") }}
	u := newVisionUsecase(gw, fakeMatcher{hit: nil}, nil, gr)

	_, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "某图标", Action: ActionClick,
	})
	if !errors.Is(err, ErrGroundingFailed) {
		t.Errorf("err = %v, want ErrGroundingFailed", err)
	}
}

// ---------------------------------------------------------------------------
// S4：执行后验证闭环（settle → post-snapshot 树哈希对比 + 前台窗口）
// ---------------------------------------------------------------------------

// 动作后元素树变化 → Verify.Changed=true，无 no_effect 提示。
func TestAct_VerifyChanged(t *testing.T) {
	pre := Snapshot{Elements: []UIElement{saveButton}, Generation: 1}
	post := Snapshot{Elements: []UIElement{saveButton, {
		Ref: "g2.e0", Type: "dialog", Name: "另存为",
		BBox:          Rect{X: 400, Y: 300, W: 300, H: 200},
		Interactivity: true, Source: "uia", Enabled: true, Generation: 2,
	}}, Generation: 2}
	calls := 0
	gw := &fakeGateway{
		snapFn: func(SnapshotOpts) (Snapshot, error) {
			calls++
			if calls == 1 {
				return pre, nil
			}
			return post, nil
		},
	}
	u, _, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "保存", Action: ActionInvoke,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if res.Verify == nil {
		t.Fatal("Verify = nil, want 非空")
	}
	if !res.Verify.HasBaseline || !res.Verify.Changed {
		t.Errorf("Verify = %+v, want HasBaseline=true Changed=true", res.Verify)
	}
	if res.Verify.Hint != "" {
		t.Errorf("Hint = %q, want 空", res.Verify.Hint)
	}
}

// 动作后元素树与前台窗口均无变化且为 click/invoke → hint=no_observable_effect。
func TestAct_VerifyNoObservableEffect(t *testing.T) {
	gw := &fakeGateway{
		snap:    Snapshot{Elements: []UIElement{saveButton}, Generation: 1},
		windows: []WindowInfo{{Title: "记事本", ProcessName: "notepad.exe", IsForeground: true}},
	}
	u, _, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "保存", Action: ActionClick,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if res.Verify == nil || res.Verify.Hint != "no_observable_effect" {
		t.Errorf("Verify = %+v, want hint=no_observable_effect", res.Verify)
	}
	if res.Verify.ForegroundAfter != "记事本" {
		t.Errorf("ForegroundAfter = %q, want 记事本", res.Verify.ForegroundAfter)
	}
}

// dry-run 不做动作后验证。
func TestAct_DryRunSkipsVerify(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, _, _ := newTestUsecase(gw, fakeMatcher{hit: &saveButton})

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Target: "保存", Action: ActionInvoke, DryRun: true,
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if res.Verify != nil {
		t.Errorf("dry-run Verify 应为 nil，got %+v", res.Verify)
	}
}

// key 直行动作无 grounding 基线 → HasBaseline=false，但仍带前台窗口信息。
func TestAct_KeyActionVerifyNoBaseline(t *testing.T) {
	gw := &fakeGateway{
		windows: []WindowInfo{{Title: "记事本", ProcessName: "notepad.exe", IsForeground: true}},
	}
	u, _, _ := newTestUsecase(gw, fakeMatcher{})

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", Action: ActionKey, Args: map[string]any{"combo": "ctrl+s"},
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if res.Verify == nil {
		t.Fatal("Verify = nil")
	}
	if res.Verify.HasBaseline {
		t.Error("key 直行动作无 grounding 快照，HasBaseline 应为 false")
	}
	if res.Verify.ForegroundAfter != "记事本" {
		t.Errorf("ForegroundAfter = %q, want 记事本", res.Verify.ForegroundAfter)
	}
	if res.Verify.Hint != "" {
		t.Errorf("key 动作不应产生 no_effect 提示，got %q", res.Verify.Hint)
	}
}

// ---------------------------------------------------------------------------
// S5：批量动作 actions[]（一次调用多步执行，fail-fast，按步计费/审计）
// ---------------------------------------------------------------------------

var editBox = UIElement{
	Ref: "g1.e9", Type: "edit", Name: "编辑框",
	BBox:          Rect{X: 10, Y: 10, W: 200, H: 24},
	Interactivity: true, Source: "uia", Enabled: true, Generation: 1,
}

// 批量 click+type：两步按序执行、各计一步预算、各落一条审计。
func TestAct_BatchClickThenType(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton, editBox}, Generation: 1}}
	u, audit, _ := newTestUsecase(gw, dispatchMatcher{byTarget: map[string]*UIElement{
		"保存": &saveButton, "编辑框": &editBox,
	}})

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1",
		Actions: []SubAction{
			{Target: "保存", Action: ActionClick},
			{Target: "编辑框", Action: ActionTypeText, Args: map[string]any{"text": "hello"}},
		},
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if len(res.Batch) != 2 {
		t.Fatalf("batch len = %d, want 2", len(res.Batch))
	}
	if res.Batch[0].Step.Action != ActionClick || res.Batch[1].Step.Action != ActionTypeText {
		t.Errorf("batch actions = %s/%s", res.Batch[0].Step.Action, res.Batch[1].Step.Action)
	}
	if res.Step.Action != ActionTypeText {
		t.Errorf("res.Step 应为最后一步，got %s", res.Step.Action)
	}
	if gw.clicked == nil || gw.typed != "hello" {
		t.Errorf("clicked=%v typed=%q, want 均执行", gw.clicked, gw.typed)
	}
	if len(audit.entries) != 2 {
		t.Errorf("audit entries = %d, want 2（每步一条）", len(audit.entries))
	}
}

// 批量 fail-fast：第二步 grounding 失败即停，第三步不执行；已完成步骤保留在 Batch。
func TestAct_BatchFailFastOnGrounding(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, _, _ := newTestUsecase(gw, dispatchMatcher{byTarget: map[string]*UIElement{
		"保存": &saveButton, // "不存在" 无命中；无视觉组件 → ErrGroundingFailed
	}})

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1",
		Actions: []SubAction{
			{Target: "保存", Action: ActionClick},
			{Target: "不存在", Action: ActionClick},
			{Target: "保存", Action: ActionClick},
		},
	})
	if !errors.Is(err, ErrGroundingFailed) {
		t.Fatalf("err = %v, want ErrGroundingFailed", err)
	}
	if len(res.Batch) != 1 {
		t.Errorf("batch len = %d, want 1（仅保留失败前完成的步骤）", len(res.Batch))
	}
}

// 批量 dry-run：全部产出计划，零执行。
func TestAct_BatchDryRun(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton, editBox}, Generation: 1}}
	u, _, _ := newTestUsecase(gw, dispatchMatcher{byTarget: map[string]*UIElement{
		"保存": &saveButton, "编辑框": &editBox,
	}})

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1", DryRun: true,
		Actions: []SubAction{
			{Target: "保存", Action: ActionClick},
			{Target: "编辑框", Action: ActionTypeText, Args: map[string]any{"text": "hello"}},
		},
	})
	if err != nil {
		t.Fatalf("Act err: %v", err)
	}
	if len(res.Batch) != 2 || res.Batch[0].Plan == nil || res.Batch[1].Plan == nil {
		t.Fatalf("dry-run batch 应全部带 plan: %+v", res.Batch)
	}
	if gw.clicked != nil || gw.typed != "" {
		t.Errorf("dry-run 不应执行：clicked=%v typed=%q", gw.clicked, gw.typed)
	}
}

// 批量按步计费：MaxSteps=1 时第二步 ErrBudgetExceeded。
func TestAct_BatchBudgetExceeded(t *testing.T) {
	gw := &fakeGateway{snap: Snapshot{Elements: []UIElement{saveButton}, Generation: 1}}
	u, _, _ := newTestUsecase(gw, dispatchMatcher{byTarget: map[string]*UIElement{"保存": &saveButton}})
	if _, err := u.StartSession(context.Background(), "agent1", Budget{MaxSteps: 1}); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	res, err := u.Act(context.Background(), ActRequest{
		AgentKey: "agent1",
		Actions: []SubAction{
			{Target: "保存", Action: ActionClick},
			{Target: "保存", Action: ActionClick},
		},
	})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("err = %v, want ErrBudgetExceeded", err)
	}
	if len(res.Batch) != 1 {
		t.Errorf("batch len = %d, want 1", len(res.Batch))
	}
}
