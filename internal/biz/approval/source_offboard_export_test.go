package approval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/auth"
)

type fakeOffboardReader struct {
	rows  []*biz.SecopsOffboardRequest
	calls int
}

func (f *fakeOffboardReader) ListPending(_ context.Context, _ int) ([]*biz.SecopsOffboardRequest, error) {
	f.calls++
	return f.rows, nil
}

func offboardViewer(t *testing.T, uid int64, roles ...string) Viewer {
	t.Helper()
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: uid, Roles: roles})
	return ViewerFromContext(ctx)
}

func pendingOffboardRow(id string, target, requester int64) *biz.SecopsOffboardRequest {
	return &biz.SecopsOffboardRequest{
		ID:           id,
		TargetUserID: target,
		RequesterID:  requester,
		Status:       biz.OffboardStatusRequested,
		CreatedAt:    time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
	}
}

// TestOffboardExportSource_Visibility 钉死「离职导出（仅 sysadmin 且非自提）」
// 可见性（wb approvals.go 对板；需求 §5.5 第三类）：
//   - sysadmin 可见他人申请、不可见自己的（不可自批展示兜底）；
//   - 无 user.offboard 码位的主体（auditor/secadmin）→ 空集且下游 reader
//     零调用（「不返回无权对象」）；
//   - Count 与 List 口径一致。
func TestOffboardExportSource_Visibility(t *testing.T) {
	reader := &fakeOffboardReader{rows: []*biz.SecopsOffboardRequest{
		pendingOffboardRow("obr-1", 501, 71), // 71 提的
		pendingOffboardRow("obr-2", 502, 73), // 73 提的
	}}
	src := NewOffboardExportSource(reader)
	if src == nil {
		t.Fatal("source must construct with non-nil reader")
	}
	if src.Kind() != KindOffboardExport {
		t.Fatalf("kind=%q", src.Kind())
	}
	ctx := context.Background()

	// sysadmin（user.offboard 持有者）视角：看到 obr-2，看不到自己的 obr-1。
	vSys := offboardViewer(t, 71, "sysadmin")
	items, err := src.List(ctx, vSys, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || items[0].ID != "obr-2" {
		t.Fatalf("sysadmin sees non-self only: %+v", items)
	}
	it := items[0]
	if it.Kind != KindOffboardExport || it.RiskLevel != RiskHigh {
		t.Fatalf("kind/risk: %+v", it)
	}
	if !strings.Contains(it.Title, "#502") {
		t.Fatalf("title: %q", it.Title)
	}
	var ctxObj map[string]any
	if err := json.Unmarshal([]byte(it.ContextJSON), &ctxObj); err != nil {
		t.Fatalf("context_json: %v", err)
	}
	if ctxObj["request_id"] != "obr-2" || ctxObj["user_id"] != float64(502) {
		t.Fatalf("context_json: %v", ctxObj)
	}
	n, err := src.Count(ctx, vSys)
	if err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}

	// 另一位 sysadmin（非任一单申请人）：两单全可见。
	vSys2 := offboardViewer(t, 72, "sysadmin")
	items, err = src.List(ctx, vSys2, 50)
	if err != nil || len(items) != 2 {
		t.Fatalf("sysadmin-72 sees all: %v %v", items, err)
	}

	// 无码位主体：空集 + reader 不再被调用。
	callsBefore := reader.calls
	for _, roles := range [][]string{{"auditor"}, {"secadmin"}, {}} {
		v := offboardViewer(t, 91, roles...)
		items, err := src.List(ctx, v, 50)
		if err != nil || len(items) != 0 {
			t.Fatalf("roles %v must see empty: %v %v", roles, items, err)
		}
		n, err := src.Count(ctx, v)
		if err != nil || n != 0 {
			t.Fatalf("roles %v count: %d %v", roles, n, err)
		}
	}
	if reader.calls != callsBefore {
		t.Fatalf("gated viewer must not reach reader: calls %d→%d", callsBefore, reader.calls)
	}

	// superadmin 经 * 通配可见。
	vSuper := offboardViewer(t, 99, "superadmin")
	items, err = src.List(ctx, vSuper, 50)
	if err != nil || len(items) != 2 {
		t.Fatalf("superadmin wildcard: %v %v", items, err)
	}
}

// TestOffboardExportSource_NilReader 装配约定：nil reader → nil Source
// （聚合器注册时跳过，与 schedule_draft 源同约定）。
func TestOffboardExportSource_NilReader(t *testing.T) {
	if src := NewOffboardExportSource(nil); src != nil {
		t.Fatalf("nil reader must yield nil source, got %T", src)
	}
}
