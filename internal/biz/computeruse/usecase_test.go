package computeruse

import (
	"context"
	"errors"
	"testing"
	"time"
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
}

func (f *fakeGateway) Info(context.Context) (DeviceInfo, error) {
	if f.info.Platform == "" {
		return DeviceInfo{Platform: "windows", ScreenW: 1920, ScreenH: 1080, ScaleFactor: 1.0}, nil
	}
	return f.info, nil
}

func (f *fakeGateway) Snapshot(context.Context, SnapshotOpts) (Snapshot, error) {
	if f.err != nil {
		return Snapshot{}, f.err
	}
	return f.snap, nil
}

func (f *fakeGateway) Screenshot(context.Context, *Rect, float64) (Image, error) {
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
	})
	return u, audit, events
}

var saveButton = UIElement{
	Ref: "g1.e3", Type: "button", Name: "保存",
	BBox:          Rect{X: 100, Y: 200, W: 80, H: 28},
	Interactivity: true, Source: "uia", Enabled: true, Generation: 1,
}

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
