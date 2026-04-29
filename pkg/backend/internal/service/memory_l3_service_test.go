// memory_l3_service_test.go 覆盖 §12 对 L3 语义记忆服务的验收：upsert 与版本、去重与标签合并、
// recall 排序、反馈驱动置信度（含归档与自动冲突）、衰减批处理、PII 脱敏与作用域隔离。
package service

import (
	mem "arenea/backend/internal/memory/domain"

	"context"
	"path/filepath"
	"strings"
	"testing"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// newTestL3Service 搭建内存 L3 栈（repo + service）。返回的 repo 供测试在
// 不变量未从服务层暴露时直接查看底层状态。
func newTestL3Service(t *testing.T) (*MemoryL3Service, repository.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "l3.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return NewMemoryL3Service(repo), repo
}

func mustUpsert(t *testing.T, svc *MemoryL3Service, in mem.FactUpsertInput) mem.MemoryFact {
	t.Helper()
	fact, err := svc.UpsertFact(context.Background(), in)
	if err != nil {
		t.Fatalf("upsert fact: %v", err)
	}
	return fact
}

// §12 #1 – 创建事实并校验 memory_facts 与 memory_fact_versions 的 v1 均已写入。
func TestL3UpsertCreatesFactAndV1Version(t *testing.T) {
	svc, _ := newTestL3Service(t)
	fact := mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser,
		ScopeID:   "u_alice",
		UserID:    "u_alice",
		Statement: "Prefers React over Vue for new front-end projects.",
		Kind:      mem.FactPreference,
		Tags:      []string{"frontend", "react"},
	})
	if fact.ID == "" || fact.Version != 1 {
		t.Fatalf("expected new fact with version 1, got %#v", fact)
	}
	versions, err := svc.ListVersions(context.Background(), fact.ID, 10)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != 1 {
		t.Fatalf("expected single v1 version, got %#v", versions)
	}
}

// §12 #2 – 同作用域相同陈述 upsert 应升版本并合并标签，而非新插一行。
func TestL3UpsertDedupsByFingerprintAndMergesTags(t *testing.T) {
	svc, repo := newTestL3Service(t)
	first := mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser, ScopeID: "u_bob", UserID: "u_bob",
		Statement: "Run tests with Vitest.", Kind: mem.FactRule,
		Tags: []string{"testing"},
	})
	second := mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser, ScopeID: "u_bob", UserID: "u_bob",
		Statement: "Run tests with Vitest.", Kind: mem.FactRule,
		Tags: []string{"frontend"},
	})
	if second.ID != first.ID {
		t.Fatalf("expected same fact id on dedup, got %q vs %q", second.ID, first.ID)
	}
	if second.Version != 2 {
		t.Fatalf("expected version=2 after dedup, got %d", second.Version)
	}
	if !containsAll(second.Tags, []string{"testing", "frontend"}) {
		t.Fatalf("expected tags to be merged, got %v", second.Tags)
	}
	list, _, err := repo.ListFacts(repository.FactListQuery{ScopeType: mem.ScopeUser, ScopeID: "u_bob"})
	if err != nil {
		t.Fatalf("list facts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 fact after dedup, got %d", len(list))
	}
}

// §12 #3 – Recall 返回按 final_score 排序且数量 ≤ top_k，渲染后提示不超长 max_chars。
func TestL3RecallRespectsTopKAndRenderBudget(t *testing.T) {
	svc, _ := newTestL3Service(t)
	mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser, ScopeID: "u_eve", UserID: "u_eve",
		Statement: "Always use TypeScript for new code.", Kind: mem.FactRule,
	})
	mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser, ScopeID: "u_eve", UserID: "u_eve",
		Statement: "Code reviews require two approvals.", Kind: mem.FactRule,
	})
	mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser, ScopeID: "u_eve", UserID: "u_eve",
		Statement: "Use Tailwind for styling.", Kind: mem.FactPreference,
	})
	hits, err := svc.Recall(context.Background(), mem.FactRecallQuery{
		UserID: "u_eve",
		Query:  "TypeScript",
		TopK:   2,
	})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(hits) > 2 {
		t.Fatalf("expected ≤ 2 hits, got %d", len(hits))
	}
	for i := 1; i < len(hits); i++ {
		if hits[i-1].FinalScore < hits[i].FinalScore {
			t.Fatalf("expected hits sorted by final_score desc, got %v", hits)
		}
	}
	block, err := svc.RenderForPrompt(context.Background(), hits, 80)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(block.Content) > 80 {
		t.Fatalf("expected content <= 80 chars, got %d", len(block.Content))
	}
}

// §12 #5 – 确认 +0.10、拒绝 -0.20 调整置信度。
func TestL3FeedbackAdjustsConfidence(t *testing.T) {
	svc, repo := newTestL3Service(t)
	fact := mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser, ScopeID: "u_frank", UserID: "u_frank",
		Statement: "Prefer pnpm over npm.",
		Confidence: 0.5,
	})
	if err := svc.Feedback(context.Background(), mem.FactFeedback{
		FactID: fact.ID, Type: mem.FactFeedbackConfirm, Source: "user:u_frank",
	}); err != nil {
		t.Fatalf("confirm feedback: %v", err)
	}
	after, err := repo.GetFact(fact.ID)
	if err != nil {
		t.Fatalf("get fact: %v", err)
	}
	if !approxEqual(after.Confidence, 0.6, 0.001) {
		t.Fatalf("expected confidence=0.6 after confirm, got %.3f", after.Confidence)
	}
	if err = svc.Feedback(context.Background(), mem.FactFeedback{
		FactID: fact.ID, Type: mem.FactFeedbackReject, Source: "user:u_frank",
	}); err != nil {
		t.Fatalf("reject feedback: %v", err)
	}
	after, err = repo.GetFact(fact.ID)
	if err != nil {
		t.Fatalf("get fact: %v", err)
	}
	if !approxEqual(after.Confidence, 0.4, 0.001) {
		t.Fatalf("expected confidence=0.4 after reject, got %.3f", after.Confidence)
	}
}

// §12 #6 – 连续三次拒绝自动建冲突行。
func TestL3FeedbackAutoCreatesConflictAfterThreeRejects(t *testing.T) {
	svc, _ := newTestL3Service(t)
	fact := mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser, ScopeID: "u_gabi", UserID: "u_gabi",
		Statement:  "Always rebase, never merge.",
		Confidence: 0.9,
	})
	for i := 0; i < 3; i++ {
		if err := svc.Feedback(context.Background(), mem.FactFeedback{
			FactID: fact.ID, Type: mem.FactFeedbackReject, Source: "user:u_gabi",
		}); err != nil {
			t.Fatalf("reject feedback %d: %v", i, err)
		}
	}
	conflicts, err := svc.ListOpenConflicts(context.Background(), mem.ScopeUser, "u_gabi")
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) == 0 {
		t.Fatalf("expected at least one open conflict after 3 rejects")
	}
}

// §12 #7 – 衰减批处理降置信度，低于归档阈的事实归档。
func TestL3DecayBatchArchivesLowConfidenceFacts(t *testing.T) {
	svc, repo := newTestL3Service(t)
	low := mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser, ScopeID: "u_hank", UserID: "u_hank",
		Statement:  "Old preference about Bower.",
		Confidence: 0.15,
	})
	keep := mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser, ScopeID: "u_hank", UserID: "u_hank",
		Statement:  "Prefer pnpm over yarn.",
		Confidence: 0.9,
	})
	report, err := svc.RunDecayBatch(context.Background())
	if err != nil {
		t.Fatalf("decay batch: %v", err)
	}
	if report.Processed == 0 {
		t.Fatalf("expected decay to process at least one fact")
	}
	got, err := repo.GetFact(low.ID)
	if err != nil {
		t.Fatalf("get low: %v", err)
	}
	if got.Status != mem.FactStatusArchived {
		t.Fatalf("expected low-confidence fact to be archived, got status=%s", got.Status)
	}
	got, err = repo.GetFact(keep.ID)
	if err != nil {
		t.Fatalf("get keep: %v", err)
	}
	if got.Status != mem.FactStatusActive {
		t.Fatalf("expected high-confidence fact to stay active, got status=%s", got.Status)
	}
}

// §12 #8 – PII 检测强制 user 作用域，仅展示脱敏后陈述。
func TestL3UpsertRedactsPIIAndForcesUserScope(t *testing.T) {
	svc, _ := newTestL3Service(t)
	fact := mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeWorkspace, WorkspaceID: "ws_acme",
		UserID:    "u_iris",
		Statement: "Contact Iris at iris@example.com or +1 415 555 1234 for billing.",
	})
	if fact.ScopeType != mem.ScopeUser {
		t.Fatalf("expected scope forced to user, got %s", fact.ScopeType)
	}
	if !fact.PIIFlag {
		t.Fatalf("expected pii_flag=true on detected PII")
	}
	if !strings.Contains(fact.RedactedStatement, "[REDACTED_") {
		t.Fatalf("expected redacted token in statement, got %q", fact.RedactedStatement)
	}
	hits, err := svc.Recall(context.Background(), mem.FactRecallQuery{UserID: "u_iris", Query: "billing"})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	block, err := svc.RenderForPrompt(context.Background(), hits, 500)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(block.Content, "iris@example.com") {
		t.Fatalf("rendered prompt leaked email: %q", block.Content)
	}
}

// §12 #9 – scope=user 的事实不得出现在 scope=workspace 的 recall 结果中。
func TestL3RecallRespectsScopeIsolation(t *testing.T) {
	svc, _ := newTestL3Service(t)
	mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeUser, ScopeID: "u_jane", UserID: "u_jane",
		Statement: "User Jane prefers dark themes.", Kind: mem.FactPreference,
	})
	mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws_acme", WorkspaceID: "ws_acme",
		Statement: "All workspace UIs use the Acme palette.", Kind: mem.FactRule,
	})
	hits, err := svc.Recall(context.Background(), mem.FactRecallQuery{
		WorkspaceID:   "ws_acme",
		IncludeScopes: []mem.ScopeType{mem.ScopeWorkspace},
		Query:         "theme",
	})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	for _, h := range hits {
		if h.Fact.ScopeType == mem.ScopeUser {
			t.Fatalf("workspace recall leaked user-scoped fact: %#v", h.Fact)
		}
	}
}

// §12 #11 – 关闭 l3_enabled 不注入 L0 片段。
func TestL3RecallSegmentForL0RespectsDisabledFlag(t *testing.T) {
	svc, repo := newTestL3Service(t)
	mustUpsert(t, svc, mem.FactUpsertInput{
		ScopeType: mem.ScopeAgent, ScopeID: "agent-l3", AgentID: "agent-l3",
		Statement: "Default temperature is 0.7.",
	})
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:   "agent-l3",
		L3Enabled: false,
	}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	if _, ok := svc.RecallSegmentForL0(context.Background(), "sess", "agent-l3", "temperature"); ok {
		t.Fatalf("expected RecallSegmentForL0 to return ok=false when l3_enabled=false")
	}
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:            "agent-l3",
		L3Enabled:          true,
		L3RecallTopK:       3,
		L3RecallMinScore:   0.05,
		L3RecallScopesJSON: `["agent"]`,
	}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	if _, ok := svc.RecallSegmentForL0(context.Background(), "sess", "agent-l3", "temperature"); !ok {
		t.Fatalf("expected RecallSegmentForL0 to return ok=true when l3_enabled=true")
	}
}

// --- 辅助 ----------------------------------------------------------------

func containsAll(haystack, needles []string) bool {
	set := map[string]bool{}
	for _, h := range haystack {
		set[strings.ToLower(strings.TrimSpace(h))] = true
	}
	for _, n := range needles {
		if !set[strings.ToLower(strings.TrimSpace(n))] {
			return false
		}
	}
	return true
}

func approxEqual(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}
