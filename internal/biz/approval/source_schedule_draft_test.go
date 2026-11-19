package approval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/office"
)

// stubScheduleDraftReader 记录调用并返回固定行（验证「无权不触库」）。
type stubScheduleDraftReader struct {
	listCalls  int
	countCalls int
	rows       []*office.ScheduleDraft
}

func (s *stubScheduleDraftReader) ListPendingDrafts(_ context.Context, _ string, _ int) ([]*office.ScheduleDraft, error) {
	s.listCalls++
	return s.rows, nil
}

func (s *stubScheduleDraftReader) CountPendingDrafts(_ context.Context, _ string) (int64, error) {
	s.countCalls++
	return int64(len(s.rows)), nil
}

func viewerWithPerms(perms ...string) Viewer {
	set := map[string]bool{}
	for _, p := range perms {
		set[p] = true
	}
	return Viewer{UserID: 7, WorkspaceID: "ws-1", has: func(code string) bool { return set[code] }}
}

// TestScheduleDraftSource_Gating 钉死「不返回无权对象」：无 office.schedule
// 权限的 viewer——List 空、Count 0、且读口一次都不得被调用（元数据存在性
// 也不泄漏）；有权 viewer 可见且条目映射完整（kind/title/risk/context_json
// 携带完整 items 供审批人审阅）。
func TestScheduleDraftSource_Gating(t *testing.T) {
	reader := &stubScheduleDraftReader{rows: []*office.ScheduleDraft{{
		ID: "sd_1", WorkspaceID: "ws-1", UserID: "u-1", AgentID: "ag-1", SessionID: "sess-1",
		Status:    office.DraftStatusPending,
		CreatedAt: time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC),
		Items: []office.DraftItem{
			{Title: "排期A", StartAt: time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)},
			{Title: "排期B", StartAt: time.Date(2026, 11, 10, 11, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)},
		},
	}}}
	src := NewScheduleDraftSource(reader)
	if src == nil || src.Kind() != KindScheduleDraft {
		t.Fatalf("source kind = %v", src)
	}
	ctx := context.Background()

	// 无权：空集 + 0，读口零调用。
	noPerm := viewerWithPerms()
	items, err := src.List(ctx, noPerm, 50)
	if err != nil || items != nil {
		t.Fatalf("no-perm list = (%v,%v), want (nil,nil)", items, err)
	}
	n, err := src.Count(ctx, noPerm)
	if err != nil || n != 0 {
		t.Fatalf("no-perm count = (%d,%v), want 0", n, err)
	}
	if reader.listCalls != 0 || reader.countCalls != 0 {
		t.Fatalf("unauthorized viewer must not touch reader (list=%d count=%d)", reader.listCalls, reader.countCalls)
	}

	// 有权：条目映射。
	items, err = src.List(ctx, viewerWithPerms(PermOfficeSchedule), 50)
	if err != nil || len(items) != 1 {
		t.Fatalf("permitted list = (%v,%v)", len(items), err)
	}
	it := items[0]
	if it.Kind != KindScheduleDraft || it.ID != "sd_1" || it.RiskLevel != RiskMedium || it.Requester != "ag-1" {
		t.Fatalf("item shape mismatch: %+v", it)
	}
	if !strings.Contains(it.Title, "排期A") || !strings.Contains(it.Title, "2 项") {
		t.Fatalf("title mismatch: %q", it.Title)
	}
	var ctxMap map[string]any
	if err := json.Unmarshal([]byte(it.ContextJSON), &ctxMap); err != nil {
		t.Fatalf("context_json not an object: %v", err)
	}
	if ctxMap["agent_id"] != "ag-1" || ctxMap["session_id"] != "sess-1" || ctxMap["item_count"].(float64) != 2 {
		t.Fatalf("context_json mismatch: %v", ctxMap)
	}
	if _, ok := ctxMap["items"].([]any); !ok {
		t.Fatal("context_json must carry full items for approver review")
	}
	n, err = src.Count(ctx, viewerWithPerms(PermOfficeSchedule))
	if err != nil || n != 1 {
		t.Fatalf("permitted count = (%d,%v)", n, err)
	}
}

// TestScheduleDraftSource_NilReader nil 读口不注册（聚合器空 lane 语义）。
func TestScheduleDraftSource_NilReader(t *testing.T) {
	if NewScheduleDraftSource(nil) != nil {
		t.Fatal("nil reader must yield nil source")
	}
}
