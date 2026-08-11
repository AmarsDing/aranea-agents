package trpc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizskill "aranea-agents/internal/biz/skill"
	"aranea-agents/pkg/loggateway"
)

// stubSkillRepo 嵌入 bizskill.Repo（nil），仅覆盖本测试触达的方法；
// 其余方法被调用即 panic（暴露测试路径偏差）。
type stubSkillRepo struct {
	bizskill.Repo
	mu         sync.Mutex
	candidates []bizskill.RuntimeCandidate
	err        error
	blockCh    chan struct{} // 非 nil 时 List 阻塞直至 channel 关闭
	calls      atomic.Int32
}

func (s *stubSkillRepo) ListEnabledPublishedSkillCandidates(_ context.Context) ([]bizskill.RuntimeCandidate, error) {
	s.calls.Add(1)
	s.mu.Lock()
	ch := s.blockCh
	s.mu.Unlock()
	if ch != nil {
		<-ch
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	out := make([]bizskill.RuntimeCandidate, len(s.candidates))
	copy(out, s.candidates)
	return out, nil
}

func (s *stubSkillRepo) setCandidates(cs ...bizskill.RuntimeCandidate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.candidates = cs
}

func (s *stubSkillRepo) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *stubSkillRepo) setBlock(ch chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockCh = ch
}

func (s *stubSkillRepo) callCount() int32 {
	return s.calls.Load()
}

// loadBody 链路（Get 的慢路径）桩：返回固定 skill + 非空 markdown，
// 使结果进入 skillCache，避免对刷新逻辑的断言被 body 加载干扰。
func (s *stubSkillRepo) GetSkillBySkillKey(_ context.Context, skillKey string) (bizskill.Skill, error) {
	return bizskill.Skill{ID: "id-" + skillKey, Slug: skillKey}, nil
}

func (s *stubSkillRepo) GetLatestSkillMarkdown(_ context.Context, _ string) (string, error) {
	return "# body", nil
}

func newStubUsecase(repo bizskill.Repo) *biz.SkillUsecase {
	return biz.NewSkillUsecase(repo, nil)
}

func cand(slug string) bizskill.RuntimeCandidate {
	return bizskill.RuntimeCandidate{Slug: slug, Name: slug, Description: "desc " + slug}
}

// TestDBRepositoryAdapter_ColdStartCacheEffective：回归测试 —— 冷启动首次加载后
// loaded 必须正确设置（此前 loaded 被错误保持零值，TTL 永不生效，每次读都同步
// 全量拉 DB，实测每轮 ~280ms）。TTL 窗口内的重复读不得触发任何 DB 调用。
func TestDBRepositoryAdapter_ColdStartCacheEffective(t *testing.T) {
	repo := &stubSkillRepo{candidates: []bizskill.RuntimeCandidate{cand("skill-a")}}
	adapter := NewDBRepositoryAdapter(newStubUsecase(repo), time.Hour, loggateway.NewNoop())

	if sums := adapter.Summaries(); len(sums) != 1 {
		t.Fatalf("cold start summaries = %+v", sums)
	}
	if got := repo.callCount(); got != 1 {
		t.Fatalf("cold start calls = %d, want 1", got)
	}
	for i := 0; i < 5; i++ {
		_ = adapter.Summaries()
		_, _ = adapter.Get("skill-a")
	}
	if got := repo.callCount(); got != 1 {
		t.Fatalf("calls within TTL = %d, want 1 (cache must be effective after cold start)", got)
	}
}

// TestDBRepositoryAdapter_StaleWhileRevalidate：TTL 自然过期后读路径不得阻塞
// 等待 DB reload —— 应立即返回旧快照，由后台单飞刷新，完成后新数据可见。
func TestDBRepositoryAdapter_StaleWhileRevalidate(t *testing.T) {
	repo := &stubSkillRepo{candidates: []bizskill.RuntimeCandidate{cand("skill-a")}}
	adapter := NewDBRepositoryAdapter(newStubUsecase(repo), 50*time.Millisecond, loggateway.NewNoop())

	// 冷启动同步加载。
	sums := adapter.Summaries()
	if len(sums) != 1 || sums[0].Name != "skill-a" {
		t.Fatalf("cold start summaries = %+v, want [skill-a]", sums)
	}
	if got := repo.callCount(); got != 1 {
		t.Fatalf("cold start calls = %d, want 1", got)
	}

	// 数据变更 + TTL 过期；mock 阻塞模拟慢 DB。
	repo.setCandidates(cand("skill-a"), cand("skill-b"))
	block := make(chan struct{})
	repo.setBlock(block)
	time.Sleep(60 * time.Millisecond) // 等 TTL 过期

	// stale 读必须立即返回旧快照（非阻塞）。
	done := make(chan []string, 1)
	go func() {
		var names []string
		for _, s := range adapter.Summaries() {
			names = append(names, s.Name)
		}
		done <- names
	}()
	select {
	case names := <-done:
		if len(names) != 1 || names[0] != "skill-a" {
			t.Fatalf("stale read summaries = %v, want old snapshot [skill-a]", names)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("stale read blocked on DB reload; want stale-while-revalidate")
	}

	// 后台 reload 已触发（single-flight）。
	close(block)
	deadline := time.Now().Add(2 * time.Second)
	for {
		var names []string
		for _, s := range adapter.Summaries() {
			names = append(names, s.Name)
		}
		if len(names) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background reload did not publish new snapshot; last = %v", names)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestDBRepositoryAdapter_InvalidateSyncsNextRead：主动失效（skill 变更）后
// 首个读必须同步拿到新数据 —— 失效语义是「立即生效」，不能落回旧快照。
func TestDBRepositoryAdapter_InvalidateSyncsNextRead(t *testing.T) {
	repo := &stubSkillRepo{candidates: []bizskill.RuntimeCandidate{cand("skill-a")}}
	adapter := NewDBRepositoryAdapter(newStubUsecase(repo), time.Hour, loggateway.NewNoop())

	if sums := adapter.Summaries(); len(sums) != 1 {
		t.Fatalf("cold start summaries = %+v", sums)
	}

	repo.setCandidates(cand("skill-b"))
	adapter.Invalidate()

	sums := adapter.Summaries()
	if len(sums) != 1 || sums[0].Name != "skill-b" {
		t.Fatalf("post-invalidate summaries = %+v, want [skill-b] (sync reload)", sums)
	}
}

// TestDBRepositoryAdapter_ReloadFailureBacksOff：后台 reload 失败后保留旧快照，
// 且在退避窗口内不再触发 DB 调用（避免 DB 故障期间每请求一次后台重试）。
func TestDBRepositoryAdapter_ReloadFailureBacksOff(t *testing.T) {
	prev := reloadFailureBackoff
	reloadFailureBackoff = 200 * time.Millisecond
	t.Cleanup(func() { reloadFailureBackoff = prev })

	repo := &stubSkillRepo{candidates: []bizskill.RuntimeCandidate{cand("skill-a")}}
	adapter := NewDBRepositoryAdapter(newStubUsecase(repo), 50*time.Millisecond, loggateway.NewNoop())
	if sums := adapter.Summaries(); len(sums) != 1 {
		t.Fatalf("cold start summaries = %+v", sums)
	}

	repo.setErr(errors.New("db down"))
	time.Sleep(60 * time.Millisecond) // TTL 过期

	// stale 读立即返回旧快照，后台 reload 失败。
	if sums := adapter.Summaries(); len(sums) != 1 || sums[0].Name != "skill-a" {
		t.Fatalf("stale summaries = %+v, want old [skill-a]", sums)
	}
	deadline := time.Now().Add(2 * time.Second)
	for repo.callCount() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("background reload not triggered; calls = %d", repo.callCount())
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 退避窗口内：重复 stale 读不再触发 DB 调用。
	for i := 0; i < 3; i++ {
		_ = adapter.Summaries()
	}
	if got := repo.callCount(); got != 2 {
		t.Fatalf("calls within backoff window = %d, want 2 (no retry storm)", got)
	}

	// 退避过后 + 故障恢复：后台 reload 成功，新数据可见。
	repo.setErr(nil)
	repo.setCandidates(cand("skill-c"))
	time.Sleep(200 * time.Millisecond) // 等退避窗口结束
	_ = adapter.Summaries()            // 触发后台 reload
	deadline = time.Now().Add(2 * time.Second)
	for {
		sums := adapter.Summaries()
		if len(sums) == 1 && sums[0].Name == "skill-c" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("post-backoff reload did not publish; last = %+v", sums)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
