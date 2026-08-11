package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	sessionsess "aranea-agents/internal/biz/session"
	"aranea-agents/internal/conf"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/pkg/loggateway"
)

// ── P3 M2: AutoMemoryWorker Agent Case 提取分支 ─────────────────────────

type fakeCaseStore struct {
	existing   *biz.AgentCase
	upserts    []biz.AgentCase
	upsertErr  error
	readCalled bool
}

func (f *fakeCaseStore) GetAgentCaseBySession(_ context.Context, _, _ string) (*biz.AgentCase, error) {
	f.readCalled = true
	return f.existing, nil
}

func (f *fakeCaseStore) UpsertAgentCase(_ context.Context, c biz.AgentCase) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.upserts = append(f.upserts, c)
	return nil
}

type stubCaseExtractor struct {
	c     *biz.AgentCase
	err   error
	calls int
}

func (s *stubCaseExtractor) ExtractCase(_ context.Context, _ biz.ConsolidateInput) (*biz.AgentCase, error) {
	s.calls++
	return s.c, s.err
}

// newCaseTestWorker 构建带 Case 分支依赖的 Worker：会话含 2 条长 user 消息
// （过预过滤门槛），agent 使用默认运行时设置（L2EpisodeEnabled=true）。
func newCaseTestWorker(t *testing.T, store *fakeCaseStore, extractor biz.AgentCaseExtractor) *AutoMemoryWorker {
	t.Helper()
	const (
		sessID  = "sess-case-1"
		agentID = "agent-case-1"
		userID  = "user-case-1"
	)
	body := strings.Repeat("帮我分析生产环境接口超时原因并给出修复方案。", 8)
	repo := fixedSessionRepo{
		sess: sessionsess.Session{ID: sessID, AgentID: agentID, UserID: userID},
		msgs: []sessionsess.ChatMessage{
			{ID: "m1", SessionID: sessID, Role: "user", ContentMarkdown: body},
			{ID: "m2", SessionID: sessID, Role: "assistant", ContentMarkdown: "我先查看慢查询日志。"},
			{ID: "m3", SessionID: sessID, Role: "tool", ContentMarkdown: "3 slow queries", OptionsJSON: `{"tool_name":"query_db"}`},
			{ID: "m4", SessionID: sessID, Role: "user", ContentMarkdown: body},
			{ID: "m5", SessionID: sessID, Role: "assistant", ContentMarkdown: "已定位到缺失索引，建议加复合索引。"},
		},
	}
	sessionsUC := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, repo, loggateway.NewNoop(), nil)
	agentsUC := newMemoryEnabledAgentsUC(agentID)
	q := memtrpc.NewMemoryJobQueue(&conf.Runtime{}, 4, 0, loggateway.NewNoop())
	w, err := NewAutoMemoryWorker(AutoMemoryWorkerConfig{
		RuntimeConf:   &conf.Runtime{},
		Sessions:      sessionsUC,
		Agents:        agentsUC,
		Writer:        &fakeConsolidationWriter{},
		Consolidator:  &stubConsolidator{},
		Queue:         q,
		Logger:        loggateway.NewNoop(),
		CaseExtractor: extractor,
		CaseReader:    store,
		CaseWriter:    store,
	})
	if err != nil {
		t.Fatalf("NewAutoMemoryWorker: %v", err)
	}
	return w
}

func caseTestRequest() memtrpc.AutoMemoryJobRequest {
	return memtrpc.AutoMemoryJobRequest{SessionID: "sess-case-1", UserID: "user-case-1", AppName: "agent-case-1"}
}

// 正常路径：LLM 提取成功 → Case 落库，agent/session/user 由 Worker 补齐。
func TestAutoMemoryWorker_ExtractsAgentCase(t *testing.T) {
	store := &fakeCaseStore{}
	ext := &stubCaseExtractor{c: &biz.AgentCase{
		Goal: "定位接口超时", Approach: "查慢查询日志", Outcome: biz.AgentCaseOutcomeSuccess,
		ToolsUsed: []string{"query_db"}, Quality: 0.9,
	}}
	w := newCaseTestWorker(t, store, ext)

	if err := w.extract(context.Background(), caseTestRequest()); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if ext.calls != 1 {
		t.Fatalf("extractor calls=%d want 1", ext.calls)
	}
	if len(store.upserts) != 1 {
		t.Fatalf("expected 1 case upsert, got %d", len(store.upserts))
	}
	c := store.upserts[0]
	if c.AgentID != "agent-case-1" || c.SourceSessionID != "sess-case-1" || c.UserID != "user-case-1" {
		t.Fatalf("worker must fill agent/session/user, got %+v", c)
	}
	if c.Outcome != biz.AgentCaseOutcomeSuccess {
		t.Fatalf("outcome=%q", c.Outcome)
	}
}

// LLM 失败 → 启发式保底 Case（goal=首条 user 消息，tools 从 OptionsJSON 收集）。
func TestAutoMemoryWorker_AgentCaseLLMFailFallsBackHeuristic(t *testing.T) {
	store := &fakeCaseStore{}
	ext := &stubCaseExtractor{err: biz.ErrLLMExtractionFailed}
	w := newCaseTestWorker(t, store, ext)

	if err := w.extract(context.Background(), caseTestRequest()); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(store.upserts) != 1 {
		t.Fatalf("expected heuristic case, got %d upserts", len(store.upserts))
	}
	c := store.upserts[0]
	if !strings.Contains(c.Goal, "接口超时") {
		t.Fatalf("heuristic goal from first user message, got %q", c.Goal)
	}
	if len(c.ToolsUsed) != 1 || c.ToolsUsed[0] != "query_db" {
		t.Fatalf("heuristic tools from OptionsJSON, got %v", c.ToolsUsed)
	}
	if c.Outcome != biz.AgentCaseOutcomeSuccess {
		t.Fatalf("outcome=%q want success (ends with assistant)", c.Outcome)
	}
}

// LLM 判定 skip（无实质任务）→ 整条跳过，不落启发式。
func TestAutoMemoryWorker_AgentCaseSkipSignal(t *testing.T) {
	store := &fakeCaseStore{}
	ext := &stubCaseExtractor{err: biz.ErrAgentCaseSkip}
	w := newCaseTestWorker(t, store, ext)

	if err := w.extract(context.Background(), caseTestRequest()); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(store.upserts) != 0 {
		t.Fatalf("skip signal must not write, got %d", len(store.upserts))
	}
}

// 预过滤：单条 user 消息 → 不调 LLM，不写库。
func TestAutoMemoryWorker_AgentCasePrefilterSkips(t *testing.T) {
	store := &fakeCaseStore{}
	ext := &stubCaseExtractor{c: &biz.AgentCase{Goal: "g", Outcome: "success"}}
	w := newCaseTestWorker(t, store, ext)
	// 覆盖会话消息为单轮短对话（过不了预过滤）。
	repo := fixedSessionRepo{
		sess: sessionsess.Session{ID: "sess-case-1", AgentID: "agent-case-1", UserID: "user-case-1"},
		msgs: []sessionsess.ChatMessage{
			{ID: "m1", SessionID: "sess-case-1", Role: "user", ContentMarkdown: "你好"},
			{ID: "m2", SessionID: "sess-case-1", Role: "assistant", ContentMarkdown: "你好"},
		},
	}
	w.sessions = biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, repo, loggateway.NewNoop(), nil)

	if err := w.extract(context.Background(), caseTestRequest()); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if ext.calls != 0 {
		t.Fatalf("prefilter must skip LLM call, got %d", ext.calls)
	}
	if len(store.upserts) != 0 {
		t.Fatalf("prefilter must skip write, got %d", len(store.upserts))
	}
}

// 幂等：会话已有 Case → 不再提取（重试/重复入队安全）。
func TestAutoMemoryWorker_AgentCaseIdempotent(t *testing.T) {
	store := &fakeCaseStore{existing: &biz.AgentCase{ID: "c1", Goal: "已有"}}
	ext := &stubCaseExtractor{c: &biz.AgentCase{Goal: "g", Outcome: "success"}}
	w := newCaseTestWorker(t, store, ext)

	if err := w.extract(context.Background(), caseTestRequest()); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !store.readCalled {
		t.Fatal("expected idempotency read")
	}
	if ext.calls != 0 || len(store.upserts) != 0 {
		t.Fatalf("existing case must skip extraction, calls=%d upserts=%d", ext.calls, len(store.upserts))
	}
}

// Case 写入失败只 Warn，主提取流程（facts/episode）不受影响、job 不重试。
func TestAutoMemoryWorker_AgentCaseWriteFailureDoesNotFailJob(t *testing.T) {
	store := &fakeCaseStore{upsertErr: errors.New("db down")}
	ext := &stubCaseExtractor{c: &biz.AgentCase{Goal: "g", Outcome: "success", Quality: 0.9}}
	w := newCaseTestWorker(t, store, ext)

	if err := w.extract(context.Background(), caseTestRequest()); err != nil {
		t.Fatalf("case write failure must not fail the job, got %v", err)
	}
}
