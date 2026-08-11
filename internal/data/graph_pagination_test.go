package data_test

// 分页游标回归测试：graph 域所有列表的 ID 是随机 UUID，与排序字段
// （sort_order/created_at/started_at）无序关系。游标必须基于排序键做
// keyset 复合分页，否则翻页丢数据/空列表（2026-08-11 线上复现：
// pageSize=5 时第二页命中 13/25 行）。
//
// 每个用例都故意让 ID 字典序与排序序相反，旧实现（id 游标）必挂。

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func newTestData(t *testing.T) *data.Data {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	d := &data.Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	return d
}

// collectAllPages 翻完全部页，返回 ID 序列与页数。
func collectAllPages(t *testing.T, pageSize int, fetch func(pageToken string) ([]string, string, error)) ([]string, int) {
	t.Helper()
	var all []string
	token := ""
	pages := 0
	for {
		ids, next, err := fetch(token)
		if err != nil {
			t.Fatalf("page %d fetch: %v", pages+1, err)
		}
		pages++
		all = append(all, ids...)
		if pages > 20 {
			t.Fatal("pagination did not terminate (possible cursor loop)")
		}
		if next == "" {
			break
		}
		token = next
	}
	return all, pages
}

func assertNoDupComplete(t *testing.T, got []string, wantCount int) {
	t.Helper()
	if len(got) != wantCount {
		t.Fatalf("total rows across pages = %d, want %d (rows=%v)", len(got), wantCount, got)
	}
	seen := map[string]int{}
	for _, id := range got {
		seen[id]++
		if seen[id] > 1 {
			t.Fatalf("duplicate row across pages: %s (rows=%v)", id, got)
		}
	}
}

func TestGraphRepo_ListDefinitionsByWorkspace_PaginationContinuity(t *testing.T) {
	d := newTestData(t)
	repo := data.NewGraphRepo(d)
	ctx := context.Background()

	// 7 行：sort_order 全 0，created_at 递增；ID 字典序与创建序相反。
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	const n = 7
	for i := 0; i < n; i++ {
		_, err := repo.SaveDefinition(ctx, &biz.GraphDefinition{
			ID:          fmt.Sprintf("def-%03d", n-i), // def-007 最先创建，字典序最大
			Name:        fmt.Sprintf("g%d", i),
			EntryPoint:  "start",
			SortOrder:   0,
			WorkspaceID: "ws-page",
			CreatedAt:   base.Add(time.Duration(i) * time.Minute),
			UpdatedAt:   base.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("SaveDefinition %d: %v", i, err)
		}
	}

	got, pages := collectAllPages(t, 3, func(token string) ([]string, string, error) {
		defs, next, err := repo.ListDefinitionsByWorkspace(ctx, 3, token, "ws-page")
		if err != nil {
			return nil, "", err
		}
		ids := make([]string, len(defs))
		for i, d := range defs {
			ids[i] = d.ID
		}
		return ids, next, nil
	})
	assertNoDupComplete(t, got, n)
	if pages != 3 { // 3+3+1，末页无 token 即终止
		t.Fatalf("pages = %d, want 3", pages)
	}
	// 顺序必须与 created_at 一致（def-007 最先创建排第一）。
	for i, id := range got {
		want := fmt.Sprintf("def-%03d", n-i)
		if id != want {
			t.Fatalf("rows[%d] = %s, want %s (full order=%v)", i, id, want, got)
		}
	}
}

func TestGraphRunRepo_ListRunsByGraph_PaginationContinuity(t *testing.T) {
	d := newTestData(t)
	repo := data.NewGraphRunRepo(d)
	ctx := context.Background()

	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	const n = 7
	for i := 0; i < n; i++ {
		err := repo.SaveRun(ctx, &biz.GraphExecution{
			ID:        fmt.Sprintf("run-%03d", n-i), // 最晚开始的 ID 字典序最小
			GraphID:   "g-page",
			SessionID: "s1",
			Status:    string(biz.GraphExecCompleted),
			StartedAt: base.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("SaveRun %d: %v", i, err)
		}
	}

	// started_at DESC：最新（run-001）排第一。
	got, _ := collectAllPages(t, 3, func(token string) ([]string, string, error) {
		runs, next, err := repo.ListRunsByGraph(ctx, "g-page", 3, token)
		if err != nil {
			return nil, "", err
		}
		ids := make([]string, len(runs))
		for i, r := range runs {
			ids[i] = r.ID
		}
		return ids, next, nil
	})
	assertNoDupComplete(t, got, n)
	for i, id := range got {
		want := fmt.Sprintf("run-%03d", i+1)
		if id != want {
			t.Fatalf("rows[%d] = %s, want %s (full order=%v)", i, id, want, got)
		}
	}
}

func TestGraphRunRepo_ListRunsByGraph_CombinedFilters(t *testing.T) {
	d := newTestData(t)
	repo := data.NewGraphRunRepo(d)
	ctx := context.Background()

	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	seed := []struct {
		id      string
		status  string
		started time.Time
	}{
		{"run-f1", string(biz.GraphExecCompleted), base},
		{"run-f2", string(biz.GraphExecFailed), base.Add(time.Hour)},
		{"run-f3", string(biz.GraphExecCompleted), base.Add(2 * time.Hour)},
		{"run-f4", string(biz.GraphExecCompleted), base.Add(3 * time.Hour)},
	}
	for _, s := range seed {
		if err := repo.SaveRun(ctx, &biz.GraphExecution{
			ID: s.id, GraphID: "g-filter", Status: s.status, StartedAt: s.started,
		}); err != nil {
			t.Fatalf("SaveRun %s: %v", s.id, err)
		}
	}

	// status=completed 且 startedAfter=base+30m → 应只命中 run-f3 / run-f4。
	// 旧实现只取 opts[0]，startedAfter 被静默丢弃（会多返回 run-f1）。
	after := base.Add(30 * time.Minute)
	runs, _, err := repo.ListRunsByGraph(ctx, "g-filter", 10, "",
		biz.GraphRunListOption{Status: string(biz.GraphExecCompleted)},
		biz.GraphRunListOption{StartedAfter: &after},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		ids := make([]string, len(runs))
		for i, r := range runs {
			ids[i] = r.ID
		}
		t.Fatalf("combined filter rows = %v, want [run-f4 run-f3]", ids)
	}
}

func TestTaskRepo_ListTasksByExecution_PaginationContinuity(t *testing.T) {
	d := newTestData(t)
	repo := data.NewTaskRepo(d)
	ctx := context.Background()

	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	const n = 7
	for i := 0; i < n; i++ {
		err := repo.SaveTask(ctx, &biz.GraphTask{
			TaskID:      fmt.Sprintf("task-%03d", n-i), // 最先创建的 ID 字典序最大
			NodeID:      "node-1",
			ExecutionID: "exec-page",
			Status:      biz.GraphTaskStatusPending,
			CreatedAt:   base.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("SaveTask %d: %v", i, err)
		}
	}

	got, _ := collectAllPages(t, 3, func(token string) ([]string, string, error) {
		tasks, next, err := repo.ListTasksByExecution(ctx, "exec-page", "", 3, token)
		if err != nil {
			return nil, "", err
		}
		ids := make([]string, len(tasks))
		for i, tk := range tasks {
			ids[i] = tk.TaskID
		}
		return ids, next, nil
	})
	assertNoDupComplete(t, got, n)
	for i, id := range got {
		want := fmt.Sprintf("task-%03d", n-i)
		if id != want {
			t.Fatalf("rows[%d] = %s, want %s (full order=%v)", i, id, want, got)
		}
	}
}
