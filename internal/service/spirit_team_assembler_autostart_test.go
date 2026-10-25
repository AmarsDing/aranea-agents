package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// D8-P3（87-orchestration-chain-hardening Wave 2）回归守卫：
// SpiritTeamAssembler.AssembleTeam 已删除 AutoStart 死路径（原分支绕过
// BuildTeamTurnInput，会让 DAG 队首轮输入缺上游前缀与强制交付协议，叠 Fix-1
// 零交付门必然误判 failed）；组装 ≠ 启动——唯一启动入口 =
// RealTeamOrchestrator.Orchestrate → StartTeamTurn（完整 BuildTeamTurnInput
// 管道）或 lazy DAG 激活 biz.scheduleDependentTeams。

// autostartStubTeamUC 实现 biz.SpiritTeamAssembler 窄接口。
type autostartStubTeamUC struct {
	mu       atomic.Int32
	bySpirit map[string][]biz.Team
	byID     map[string]biz.Team
}

func newAutostartStubTeamUC() *autostartStubTeamUC {
	return &autostartStubTeamUC{bySpirit: map[string][]biz.Team{}, byID: map[string]biz.Team{}}
}

func (s *autostartStubTeamUC) Create(_ context.Context, in biz.Team) (biz.Team, error) {
	if in.ID == "" {
		in.ID = fmt.Sprintf("team-%d", s.mu.Add(1))
	}
	s.byID[in.ID] = in
	s.bySpirit[in.SpiritSessionID] = append(s.bySpirit[in.SpiritSessionID], in)
	return in, nil
}
func (s *autostartStubTeamUC) Get(_ context.Context, id string) (biz.Team, error) {
	t, ok := s.byID[id]
	if !ok {
		return biz.Team{}, fmt.Errorf("not found: %s", id)
	}
	return t, nil
}
func (s *autostartStubTeamUC) Update(_ context.Context, id string, patch biz.Team) (biz.Team, error) {
	if patch.ID == "" {
		patch.ID = id
	}
	s.byID[id] = patch
	return patch, nil
}
func (s *autostartStubTeamUC) TransitionStatus(_ context.Context, id, newStatus string) (biz.Team, error) {
	t := s.byID[id]
	t.Status = newStatus
	s.byID[id] = t
	return t, nil
}
func (s *autostartStubTeamUC) ListBySpiritSessionID(_ context.Context, spiritID string) ([]biz.Team, error) {
	return s.bySpirit[spiritID], nil
}
func (s *autostartStubTeamUC) BatchArchiveTeams(_ context.Context, _ []string) (int, error) {
	return 0, nil
}
func (s *autostartStubTeamUC) ListRuns(_ context.Context, _ string, _ int) ([]biz.TeamRunRecord, error) {
	return nil, nil
}

// autostartStubSessionUC 实现 biz.SpiritSessionAccessor 窄接口。
type autostartStubSessionUC struct {
	mu       atomic.Int32
	byID     map[string]biz.Session
	byParent map[string][]biz.Session
}

func newAutostartStubSessionUC() *autostartStubSessionUC {
	return &autostartStubSessionUC{byID: map[string]biz.Session{}, byParent: map[string][]biz.Session{}}
}

func (s *autostartStubSessionUC) Get(_ context.Context, id string) (biz.Session, error) {
	sess, ok := s.byID[id]
	if !ok {
		return biz.Session{}, fmt.Errorf("not found: %s", id)
	}
	return sess, nil
}
func (s *autostartStubSessionUC) Create(_ context.Context, in biz.Session) (biz.Session, error) {
	if in.ID == "" {
		in.ID = fmt.Sprintf("sess-%d", s.mu.Add(1))
	}
	s.byID[in.ID] = in
	if in.ParentSessionID != "" {
		s.byParent[in.ParentSessionID] = append(s.byParent[in.ParentSessionID], in)
	}
	return in, nil
}
func (s *autostartStubSessionUC) Search(_ context.Context, _ biz.SessionSearchQuery) (biz.SessionListResult, error) {
	return biz.SessionListResult{}, nil
}
func (s *autostartStubSessionUC) ListMessagesRecent(_ context.Context, _ string, _ int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (s *autostartStubSessionUC) ListChildSessions(_ context.Context, parentID string) ([]biz.Session, error) {
	return s.byParent[parentID], nil
}

// autostartStubAgentUC 实现 biz.SpiritAgentResolver 窄接口。
type autostartStubAgentUC struct{}

func (s *autostartStubAgentUC) List(_ context.Context, _ biz.AgentListQuery) (biz.AgentListResult, error) {
	return biz.AgentListResult{
		Items: []biz.Agent{{ID: "agent_agent-1", AgentKey: "agent-1"}},
		Total: 1,
	}, nil
}

// autostartStubTransactor 实现 biz.SpiritTransactor（直通调用 fn）。
type autostartStubTransactor struct{}

func (s *autostartStubTransactor) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// autostartSpyTeamStarter 实现 biz.TeamStarterPort，原子计数 StartTeamTurn 调用
// （删除 AutoStart 死路径后应恒为 0）。
type autostartSpyTeamStarter struct {
	startCalls  atomic.Int32
	resultCalls atomic.Int32
}

func (s *autostartSpyTeamStarter) StartTeamTurn(_ context.Context, _, _ string) error {
	s.startCalls.Add(1)
	return nil
}
func (s *autostartSpyTeamStarter) HandleTeamTurnResult(_ context.Context, _, _, _, _, _ string) {
	s.resultCalls.Add(1)
}

// 回归守卫：AutoStart=true 调用 AssembleTeam 后，500ms 有界轮询窗口内
// StartTeamTurn 调用数必须恒为 0——pre-fix 分支经 safego.Go 几乎立即触发
// 一次调用（RED）；D8-P3 删除后恒 GREEN。
func TestAssembleTeam_AutoStart_DoesNotStartTurn(t *testing.T) {
	teamUC := newAutostartStubTeamUC()
	sessionUC := newAutostartStubSessionUC()
	sessionUC.byID["spirit-1"] = biz.Session{ID: "spirit-1", OwnerType: "spirit"}
	transactor := &autostartStubTransactor{}

	uc := biz.NewSpiritTeamUsecase(teamUC, sessionUC, &autostartStubAgentUC{}, loggateway.NewNoop(), biz.WithSpiritTransactor(transactor))
	spy := &autostartSpyTeamStarter{}
	// eventBus/seq/orchCache 传 nil：v2EventReady()=false → publish 早退安全路径。
	a := NewSpiritTeamAssembler(uc, nil, nil, nil, spy, nil, loggateway.NewNoop(), nil, nil)

	_, _, _, err := a.AssembleTeam(context.Background(), biz.SpiritTeamParams{
		SpiritSessionID: "spirit-1",
		TaskDescription: "autostart regression probe",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		AutoStart:       true,
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}

	// 有界 500ms 轮询窗口：pre-fix 分支经 safego.Go 几乎立即置 1。
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := spy.startCalls.Load(); got > 0 {
			t.Fatalf("StartTeamTurn must not be invoked from AssembleTeam (D8-P3); got %d calls", got)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := spy.startCalls.Load(); got != 0 {
		t.Fatalf("StartTeamTurn call count after grace window = %d, want 0", got)
	}
}
