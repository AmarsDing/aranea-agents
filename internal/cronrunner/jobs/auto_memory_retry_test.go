package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	sessionsess "aranea-agents/internal/biz/session"
	"aranea-agents/internal/conf"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/pkg/loggateway"
)

// flakyEpisodeWriter 首 N 次带 episode 的写入报错（模拟 episode 写入瞬时
// 失败），此后成功；记录每次 episode 写入的 ID 供断言。
type flakyEpisodeWriter struct {
	mu           sync.Mutex
	episodeIDs   []string
	epFailBudget int
	epCallCh     chan string
}

func (f *flakyEpisodeWriter) UpsertFactsAndEpisodeBatch(_ context.Context, _ []biz.MemoryFactWrite, ep *biz.EpisodeWrite) (*biz.ConsolidationResult, error) {
	if ep == nil {
		return &biz.ConsolidationResult{}, nil
	}
	f.mu.Lock()
	f.episodeIDs = append(f.episodeIDs, ep.ID)
	fail := f.epFailBudget > 0
	if fail {
		f.epFailBudget--
	}
	f.mu.Unlock()
	if f.epCallCh != nil {
		f.epCallCh <- ep.ID
	}
	if fail {
		return nil, errors.New("simulated episode write failure")
	}
	return &biz.ConsolidationResult{}, nil
}

func (f *flakyEpisodeWriter) Episodes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.episodeIDs))
	copy(cp, f.episodeIDs)
	return cp
}

// newRetryTestWorker 构造一个 facts 走真实 FactWritePipeline（fake L3 writer
// 记录每次候选的 SourceEpisodeID）、episode 走 flakyEpisodeWriter 的 worker。
func newRetryTestWorker(t *testing.T, writer biz.MemoryConsolidationWriter, factWriter *fakePipelineFactWriter, maxRetries int32) *AutoMemoryWorker {
	t.Helper()
	const (
		sessID  = "sess-retry-ep"
		agentID = "agent-retry-ep"
		userID  = "user-retry-ep"
	)
	repo := fixedSessionRepo{
		sess: sessionsess.Session{ID: sessID, AgentID: agentID, UserID: userID},
		msgs: []sessionsess.ChatMessage{{
			ID: "m-retry-1", SessionID: sessID, Role: "user", ContentMarkdown: "我喜欢深色模式",
		}},
	}
	sessionsUC := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, repo, loggateway.NewNoop(), nil)
	agentsUC := newMemoryEnabledAgentsUC(agentID)
	q := memtrpc.NewMemoryJobQueue(&conf.Runtime{}, 4, time.Millisecond, loggateway.NewNoop())
	t.Cleanup(q.Close)
	pipeline := biz.NewFactWritePipeline(biz.FactWritePipelineDeps{Writer: factWriter})
	w, err := NewAutoMemoryWorker(AutoMemoryWorkerConfig{
		RuntimeConf: &conf.Runtime{AutoMemory: &conf.Runtime_AutoMemory{MaxRetries: maxRetries}},
		Sessions:    sessionsUC,
		Agents:      agentsUC,
		Writer:      writer,
		Consolidator: &stubConsolidator{proposals: []biz.MemoryProposal{{
			Layer:       biz.MemoryLayerL3,
			Statement:   "用户偏好深色模式",
			SubjectType: "preference",
			Scope:       "user",
			Confidence:  0.9,
		}}},
		Queue:        q,
		FactPipeline: pipeline,
		Logger:       loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewAutoMemoryWorker: %v", err)
	}
	return w
}

// P1-2（2026-10-18 审计）：重试必须贯穿同一个 job 级 episodeID——attempt 0
// 已写出的 facts 引用该 id，若 attempt 1 换 id，此前 facts 的
// source_episode_id 悬空指向永不存在的事件行。
func TestAutoMemoryWorker_RetryReusesEpisodeIDAcrossAttempts(t *testing.T) {
	factWriter := &fakePipelineFactWriter{}
	epWriter := &flakyEpisodeWriter{epFailBudget: 1, epCallCh: make(chan string, 4)}
	w := newRetryTestWorker(t, epWriter, factWriter, 3)
	// 注入毫秒级退避，测试等不起 30s/2m/10m。
	w.retryBackoff = []time.Duration{5 * time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond}

	w.processWithRetry(context.Background(), memtrpc.AutoMemoryJobRequest{
		SessionID: "sess-retry-ep",
		UserID:    "user-retry-ep",
		AppName:   "agent-retry-ep",
		Priority:  memtrpc.MemoryJobPriorityHigh,
	})

	// 同步等待整条重试链完成（第二次 episode 写入成功为止）。
	deadline := time.After(3 * time.Second)
	for calls := 0; calls < 2; calls++ {
		select {
		case <-epWriter.epCallCh:
		case <-deadline:
			t.Fatal("timed out waiting for retry episode writes")
		}
	}

	episodes := epWriter.Episodes()
	if len(episodes) != 2 {
		t.Fatalf("episode write calls: got %d, want 2", len(episodes))
	}
	if episodes[0] != episodes[1] {
		t.Errorf("episode id changed across attempts: %s vs %s (facts now dangle)", episodes[0], episodes[1])
	}
	var srcIDs []string
	for _, u := range factWriter.upserts {
		srcIDs = append(srcIDs, u.SourceEpisodeID)
	}
	if len(srcIDs) < 2 {
		t.Fatalf("expected facts written in both attempts, got %d upserts", len(srcIDs))
	}
	for i, id := range srcIDs {
		if id != episodes[0] {
			t.Errorf("attempt %d fact source_episode_id=%s, want job-level %s", i, id, episodes[0])
		}
	}
}

// P1-3（2026-10-18 审计）：worker 主循环不得在退避 sleep 里阻塞——持续失败的
// job 必须立即让出主循环（后台异步重试），否则队头阻塞使队列 3 档优先级语义
// 失效（高优先级反馈被低优先级失败任务堵死最长 ~15 分钟）。
func TestAutoMemoryWorker_PersistentlyFailingJobDoesNotBlockLoop(t *testing.T) {
	rc := &conf.Runtime{
		AutoMemory: &conf.Runtime_AutoMemory{MaxRetries: 3},
	}
	sink := &fakeDeadLetterSinkP203{}
	w := buildRetryExhaustedWorker(t, rc, sink)
	w.retryBackoff = []time.Duration{500 * time.Millisecond, 500 * time.Millisecond, 500 * time.Millisecond}

	req := memtrpc.AutoMemoryJobRequest{
		SessionID: "sess-retry-exhaust",
		AppName:   "agent-retry",
		Priority:  memtrpc.MemoryJobPriorityHigh,
	}
	t0 := time.Now()
	w.processWithRetry(context.Background(), req)
	elapsed := time.Since(t0)
	if elapsed >= 400*time.Millisecond {
		t.Fatalf("processWithRetry blocked the worker loop for %v >= first backoff (head-of-line blocking)", elapsed)
	}

	// 后台重试链仍须跑完：死信最终到达。
	deadline := time.Now().Add(5 * time.Second)
	for {
		if len(sink.Entries()) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background retry chain never reached dead-letter")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
