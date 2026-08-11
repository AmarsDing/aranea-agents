package tools

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// --- stubs ---

type stubSpiritAgentLookup struct {
	agent biz.Agent
	err   error
}

func (s *stubSpiritAgentLookup) GetAgentByAgentKey(_ context.Context, key string) (biz.Agent, error) {
	if key != biz.SpiritAgentKey {
		return biz.Agent{}, errors.New("unexpected agent key: " + key)
	}
	return s.agent, s.err
}

type stubSpiritSessions struct {
	searchRes biz.SessionListResult
	searchErr error
	created   biz.Session
	createErr error

	gotQuery biz.SessionSearchQuery
}

func (s *stubSpiritSessions) Search(_ context.Context, q biz.SessionSearchQuery) (biz.SessionListResult, error) {
	s.gotQuery = q
	return s.searchRes, s.searchErr
}

func (s *stubSpiritSessions) Create(_ context.Context, in biz.Session) (biz.Session, error) {
	return s.created, s.createErr
}

type stubDelegationSubmitter struct {
	mu       sync.Mutex
	calls    []biz.TurnInput
	accepted bool
	err      error
}

func (s *stubDelegationSubmitter) SubmitDelegatedTurn(_ context.Context, in biz.TurnInput) (bool, error) {
	s.mu.Lock()
	s.calls = append(s.calls, in)
	s.mu.Unlock()
	return s.accepted, s.err
}

func (s *stubDelegationSubmitter) waitCall(t *testing.T) biz.TurnInput {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		if len(s.calls) > 0 {
			c := s.calls[0]
			s.mu.Unlock()
			return c
		}
		s.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("submitter not called within 2s")
	return biz.TurnInput{}
}

type stubDelegationRegistry struct {
	mu            sync.Mutex
	registrations [][3]string
	failures      map[int64]string
	nextID        int64
}

func (r *stubDelegationRegistry) Register(voiceSessionID, spiritSessionID, content string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	r.registrations = append(r.registrations, [3]string{voiceSessionID, spiritSessionID, content})
	return r.nextID
}

func (r *stubDelegationRegistry) MarkSubmitFailed(regID int64, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failures == nil {
		r.failures = map[int64]string{}
	}
	r.failures[regID] = message
}

func (r *stubDelegationRegistry) waitFailure(t *testing.T) (int64, string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for id, msg := range r.failures {
			r.mu.Unlock()
			return id, msg
		}
		r.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("MarkSubmitFailed not called within 2s")
	return 0, ""
}

// --- tests ---

func TestNewDelegateToSpiritTool_NilDeps_ReturnsNil(t *testing.T) {
	if got := NewDelegateToSpiritTool(DelegateToSpiritDeps{}); got != nil {
		t.Fatal("expected nil tool when all deps nil")
	}
	partial := DelegateToSpiritDeps{
		Agents:   &stubSpiritAgentLookup{agent: biz.Agent{ID: "a-spirit"}},
		Sessions: &stubSpiritSessions{},
		// Submitter/Registry nil
	}
	if got := NewDelegateToSpiritTool(partial); got != nil {
		t.Fatal("expected nil tool when Submitter/Registry nil")
	}
}

func TestDelegateToSpiritTool_Declaration(t *testing.T) {
	tl := NewDelegateToSpiritTool(DelegateToSpiritDeps{
		Agents:    &stubSpiritAgentLookup{agent: biz.Agent{ID: "a-spirit"}},
		Sessions:  &stubSpiritSessions{},
		Submitter: &stubDelegationSubmitter{accepted: true},
		Registry:  &stubDelegationRegistry{},
	})
	decl := tl.Declaration()
	if decl.Name != "delegate_to_spirit" {
		t.Fatalf("name=%q want delegate_to_spirit", decl.Name)
	}
	if !strings.Contains(decl.Description, "精灵助手") {
		t.Fatalf("description should mention 精灵助手, got %q", decl.Description)
	}
}

func TestDelegateToSpiritTool_Call_ExistingSession(t *testing.T) {
	sessions := &stubSpiritSessions{searchRes: biz.SessionListResult{Items: []biz.Session{
		{ID: "sess-old", LastMessageAt: "2026-08-10T01:00:00Z"},
		{ID: "sess-recent", LastMessageAt: "2026-08-11T01:00:00Z"},
	}}}
	submitter := &stubDelegationSubmitter{accepted: true}
	registry := &stubDelegationRegistry{}
	tl := NewDelegateToSpiritTool(DelegateToSpiritDeps{
		Agents:    &stubSpiritAgentLookup{agent: biz.Agent{ID: "a-spirit"}},
		Sessions:  sessions,
		Submitter: submitter,
		Registry:  registry,
	})

	res, err := tl.Call(spiritInvocationCtx("sess-voice"), []byte(`{"task":"帮我安装 xlsx skill"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	out := res.(DelegateToSpiritOutput)
	if !strings.Contains(out.Content, "精灵助手") {
		t.Fatalf("output should confirm delegation, got %q", out.Content)
	}
	// 最近活跃会话被选中
	call := submitter.waitCall(t)
	if call.SessionID != "sess-recent" {
		t.Fatalf("submitted session=%q want sess-recent (most recent)", call.SessionID)
	}
	if call.Content != "帮我安装 xlsx skill" {
		t.Fatalf("submitted content=%q", call.Content)
	}
	// 注册先于提交，内容精确匹配（R10 内容绑定依据）
	if len(registry.registrations) != 1 {
		t.Fatalf("registrations=%d want 1", len(registry.registrations))
	}
	reg := registry.registrations[0]
	if reg[0] != "sess-voice" || reg[1] != "sess-recent" || reg[2] != "帮我安装 xlsx skill" {
		t.Fatalf("registration=%v", reg)
	}
	// 查询限定：spirit agent + 根会话
	if sessions.gotQuery.AgentID != "a-spirit" || !sessions.gotQuery.RootOnly {
		t.Fatalf("query=%+v", sessions.gotQuery)
	}
}

func TestDelegateToSpiritTool_Call_NoSession_Creates(t *testing.T) {
	sessions := &stubSpiritSessions{created: biz.Session{ID: "sess-new"}}
	submitter := &stubDelegationSubmitter{accepted: true}
	registry := &stubDelegationRegistry{}
	tl := NewDelegateToSpiritTool(DelegateToSpiritDeps{
		Agents:    &stubSpiritAgentLookup{agent: biz.Agent{ID: "a-spirit"}},
		Sessions:  sessions,
		Submitter: submitter,
		Registry:  registry,
	})

	if _, err := tl.Call(spiritInvocationCtx("sess-voice"), []byte(`{"task":"写一份周报"}`)); err != nil {
		t.Fatalf("Call: %v", err)
	}
	call := submitter.waitCall(t)
	if call.SessionID != "sess-new" {
		t.Fatalf("submitted session=%q want sess-new (created)", call.SessionID)
	}
}

func TestDelegateToSpiritTool_Call_SubmitSyncFailed_MarksRegistry(t *testing.T) {
	sessions := &stubSpiritSessions{searchRes: biz.SessionListResult{Items: []biz.Session{{ID: "s1", LastMessageAt: "2026-08-11T00:00:00Z"}}}}
	submitter := &stubDelegationSubmitter{accepted: false, err: errors.New("admission rejected")}
	registry := &stubDelegationRegistry{}
	tl := NewDelegateToSpiritTool(DelegateToSpiritDeps{
		Agents:    &stubSpiritAgentLookup{agent: biz.Agent{ID: "a-spirit"}},
		Sessions:  sessions,
		Submitter: submitter,
		Registry:  registry,
	})

	if _, err := tl.Call(spiritInvocationCtx("sess-voice"), []byte(`{"task":"task-x"}`)); err != nil {
		t.Fatalf("Call: %v", err)
	}
	regID, msg := registry.waitFailure(t)
	if regID != 1 {
		t.Fatalf("failed regID=%d want 1", regID)
	}
	if !strings.Contains(msg, "失败") {
		t.Fatalf("failure message should be speakable, got %q", msg)
	}
}

func TestDelegateToSpiritTool_Call_Queued_IsAccepted(t *testing.T) {
	sessions := &stubSpiritSessions{searchRes: biz.SessionListResult{Items: []biz.Session{{ID: "s1", LastMessageAt: "2026-08-11T00:00:00Z"}}}}
	// accepted=true + err=nil 模拟排队受理（service 适配器把 ErrTurnMessageQueued 归一）
	submitter := &stubDelegationSubmitter{accepted: true}
	registry := &stubDelegationRegistry{}
	tl := NewDelegateToSpiritTool(DelegateToSpiritDeps{
		Agents:    &stubSpiritAgentLookup{agent: biz.Agent{ID: "a-spirit"}},
		Sessions:  sessions,
		Submitter: submitter,
		Registry:  registry,
	})

	if _, err := tl.Call(spiritInvocationCtx("sess-voice"), []byte(`{"task":"task-y"}`)); err != nil {
		t.Fatalf("Call: %v", err)
	}
	submitter.waitCall(t)
	time.Sleep(20 * time.Millisecond) // 给失败路径留窗口（不应发生）
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if len(registry.failures) != 0 {
		t.Fatalf("queued submission must not mark failure, got %v", registry.failures)
	}
}

func TestDelegateToSpiritTool_Call_EmptyTask(t *testing.T) {
	tl := NewDelegateToSpiritTool(DelegateToSpiritDeps{
		Agents:    &stubSpiritAgentLookup{agent: biz.Agent{ID: "a-spirit"}},
		Sessions:  &stubSpiritSessions{},
		Submitter: &stubDelegationSubmitter{accepted: true},
		Registry:  &stubDelegationRegistry{},
	})
	if _, err := tl.Call(spiritInvocationCtx("sess-voice"), []byte(`{"task":"  "}`)); err == nil {
		t.Fatal("expected error for empty task")
	}
}

func TestDelegateToSpiritTool_Call_NoInvocationSession(t *testing.T) {
	tl := NewDelegateToSpiritTool(DelegateToSpiritDeps{
		Agents:    &stubSpiritAgentLookup{agent: biz.Agent{ID: "a-spirit"}},
		Sessions:  &stubSpiritSessions{},
		Submitter: &stubDelegationSubmitter{accepted: true},
		Registry:  &stubDelegationRegistry{},
	})
	if _, err := tl.Call(context.Background(), []byte(`{"task":"task-z"}`)); err == nil {
		t.Fatal("expected error when invocation session missing")
	}
}
