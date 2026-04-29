package repository

import (
	"path/filepath"
	"testing"

	"arenea/backend/internal/domain"
)

func newTestRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repo
}

func TestSQLiteRepositoryChatOptionsSeeded(t *testing.T) {
	repo := newTestRepo(t)
	opts, err := repo.ListChatOptions("dialog_mode")
	if err != nil {
		t.Fatalf("list chat options: %v", err)
	}
	if len(opts) == 0 {
		t.Fatal("expected seeded dialog options")
	}
}

func TestSQLiteRepositorySearchAgentsFiltersAndCounts(t *testing.T) {
	repo := newTestRepo(t)
	agents := []domain.Agent{
		{ID: "a1", AgentKey: "coding-one", DisplayName: "Coding One", Provider: "openrouter", Model: "gpt", CategoryPositionID: "cat_coding"},
		{ID: "a2", AgentKey: "ops-one", DisplayName: "Ops One", Provider: "anthropic", Model: "claude", CategoryPositionID: "cat_ops"},
	}
	for _, agent := range agents {
		if _, err := repo.CreateAgent(agent); err != nil {
			t.Fatalf("create agent %s: %v", agent.ID, err)
		}
	}

	result, err := repo.SearchAgents(domain.AgentListQuery{Keyword: "coding", Provider: "openrouter", Limit: 10})
	if err != nil {
		t.Fatalf("search agents: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != "a1" {
		t.Fatalf("unexpected filtered result: %#v", result)
	}

	// Migrate() 会植入内置 __system_admin__（见 catalog/adapters/sqlite.SeedSystemAdminAgent），
	// 故未过滤总数为两条测试智能体加一条内置管理员。此处按分类过滤，使分页断言只针对本测试插入的智能体。
	paged, err := repo.SearchAgents(domain.AgentListQuery{CategoryID: "cat_ops", Limit: 1, Offset: 0})
	if err != nil {
		t.Fatalf("search paged agents: %v", err)
	}
	if paged.Total != 1 || len(paged.Items) != 1 || paged.Items[0].ID != "a2" {
		t.Fatalf("unexpected paged result: %#v", paged)
	}
}

func TestSQLiteRepositorySessionMessageOrderingAndDelete(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.CreateAgent(domain.Agent{
		ID:          "a1",
		AgentKey:    "default",
		DisplayName: "Default",
		Provider:    "openai",
		Model:       "gpt",
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := repo.CreateSession(domain.Session{ID: "s1", AgentID: "a1", Title: "Session"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	first, err := repo.AddMessage(domain.Message{ID: "m1", SessionID: "s1", Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("add first message: %v", err)
	}
	second, err := repo.AddMessage(domain.Message{ID: "m2", SessionID: "s1", Role: "assistant", Content: "hi"})
	if err != nil {
		t.Fatalf("add second message: %v", err)
	}
	if first.TurnIndex >= second.TurnIndex {
		t.Fatalf("expected increasing turn index, got %d then %d", first.TurnIndex, second.TurnIndex)
	}
	items, err := repo.ListMessages("s1")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(items) != 2 || items[0].ID != "m1" || items[1].ID != "m2" {
		t.Fatalf("unexpected message order: %#v", items)
	}
	if err = repo.DeleteSession("s1"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	sessions, err := repo.ListSessions("a1")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected deleted session hidden, got %d", len(sessions))
	}
}

func TestSQLiteRepositoryModelUsageEventAggregates(t *testing.T) {
	repo := newTestRepo(t)
	event := domain.ModelTokenUsageEvent{
		ID:                  "u1",
		OccurredAt:          "2026-04-25T07:00:00Z",
		DateKey:             "2026-04-25",
		HourKey:             "2026-04-25T07:00",
		AgentID:             "a1",
		AgentKey:            "default",
		SessionID:           "s1",
		MessageID:           "m1",
		ProviderCode:        "openrouter",
		ProviderType:        "OpenAI Compatible",
		ProviderDisplayName: "OpenRouter",
		ModelAPIID:          "gpt-4.1-mini",
		ModelDisplayName:    "GPT 4.1 Mini",
		UsageKind:           "chat",
		CallCount:           1,
		InputTokens:         100,
		OutputTokens:        50,
		TotalTokens:         150,
		TotalCostMicroUSD:   2000,
		LatencyMS:           1200,
		TokensPerSecond:     41.6,
		Status:              "success",
		ModelCategoryJSON:   "[]",
		MetadataJSON:        "{}",
		CreatedAt:           "2026-04-25T07:00:00Z",
	}
	created, err := repo.AddModelTokenUsageEvent(event)
	if err != nil {
		t.Fatalf("add usage event: %v", err)
	}
	if err = repo.UpsertModelTokenUsageDaily(created); err != nil {
		t.Fatalf("upsert daily usage: %v", err)
	}

	query := domain.ModelUsageQuery{StartDate: "2026-04-25", EndDate: "2026-04-25"}
	summary, err := repo.GetModelUsageSummary(query)
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}
	if summary.CallCount != 1 || summary.TotalTokens != 150 || summary.TotalCostMicroUSD != 2000 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	trends, err := repo.ListModelUsageTrends(query)
	if err != nil {
		t.Fatalf("usage trends: %v", err)
	}
	if len(trends) != 1 || trends[0].DateKey != "2026-04-25" {
		t.Fatalf("unexpected trends: %#v", trends)
	}
}
