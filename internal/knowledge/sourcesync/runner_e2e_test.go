// runner_e2e_test.go —— G-38 W7 墓碑级联 e2e 钉阵（真 walker↔runner↔DocApplier↔
// 内存仓全链）。与 W3 TestE2E_SharedDirThreeRounds 同面：前者钉 shared_dir 建→
// 不变→更新→级删四轮，本组钉 notion 全链三轮（build→no-op→外部删除级联）
// ＋释放态闸阵（disabled/墓碑行/失联三轮不动写面不入退避）。
//
// notion e2e 用 httptest 假 Notion 服务器（search/pages.retrieve/blocks 三端点
// 状态机）＋真 NotionWalker ＋真 Runner 八步序＋真 DocApplier——与单元钉的分工：
// 单元钉锁每步语义，本钉锁「步骤间接线」无回归（walker 契约/runner 归一/
// applier 直读通道三层真实组合）。

package sourcesync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/pkg/loggateway"
)

// TestE2E_NotionCascadeRounds notion 三轮全链：R1 search 出席 2 页建文档 →
// R2 指纹不变全 unchanged → R3 page2 外部归档（检索面消失＋retrieve 报
// archived）走「walk 缺席→missing→ConfirmDeleted 三态→own_lifecycle 级联」
// 正道；page1 文档不动、runs 审计三轮齐落。
func TestE2E_NotionCascadeRounds(t *testing.T) {
	const page1, page2 = notionTestPageID1, notionTestPageID2
	archived2 := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/search":
			page2row := ""
			if !archived2 {
				page2row = "," + notionPageJSON(page2, "2026-10-27T10:02:00Z", "Beta", false)
			}
			fmt.Fprintf(w, `{"results":[%s%s],"has_more":false,"next_cursor":null}`,
				notionPageJSON(page1, "2026-10-27T10:00:00Z", "Alpha", false), page2row)
		case "/v1/pages/" + page1:
			fmt.Fprint(w, notionPageJSON(page1, "2026-10-27T10:00:00Z", "Alpha", false))
		case "/v1/pages/" + page2:
			fmt.Fprint(w, notionPageJSON(page2, "2026-10-27T10:02:00Z", "Beta", archived2))
		case "/v1/blocks/" + page1 + "/children":
			fmt.Fprint(w, `{"results":[{"type":"paragraph","paragraph":{"rich_text":[{"plain_text":"alpha body"}]}}],"has_more":false,"next_cursor":null}`)
		case "/v1/blocks/" + page2 + "/children":
			fmt.Fprint(w, `{"results":[{"type":"paragraph","paragraph":{"rich_text":[{"plain_text":"beta body"}]}}],"has_more":false,"next_cursor":null}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	repo := newDocMemRepo()
	uc := bizknowledge.NewUsecaseFromRepo(repo)
	conns := newMemConnectorRepo()
	items := newMemItemRepo()
	runs := &memRunRepo{}
	creds := newMemCredentialRepo()
	appl, err := NewDocApplier(DocApplierDeps{UC: uc, Items: items, Log: loggateway.NewNoop()})
	if err != nil {
		t.Fatal(err)
	}
	walker, err := NewNotionWalker(NotionWalkerDeps{
		Log:         loggateway.NewNoop(),
		Credentials: creds,
		Crypto:      biz.NewCredentialCrypto(nil, loggateway.NewNoop()),
		HTTP:        srv.Client(),
		BaseURL:     srv.URL,
		Sleep:       func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	conn := bizsync.Connector{
		ID: "src:conn:notion-e2e", Kind: bizsync.KindNotion, Name: "ne2e", CollectionID: "col1",
		ConfigJSON:          `{}`,
		IntervalSeconds:     900,
		Enabled:             true,
		SyncState:           bizsync.SyncStateIdle,
		MaxItemsPerRun:      500,
		MaxDeletesPerRun:    200,
		MaxFetchBytesPerRun: 512 << 20,
		CreatedAt:           fixedClock,
		UpdatedAt:           fixedClock,
	}
	conns.seed(conn)
	seedNotionEnvToken(t, creds, conn.ID, "")
	r := NewRunner(Deps{
		Connectors: conns, Items: items, Runs: runs, Locker: &fakeLocker{ok: true},
		Walkers:  map[bizsync.Kind]Walker{bizsync.KindNotion: walker},
		Appliers: map[bizsync.Kind]Applier{bizsync.KindNotion: appl},
		Now:      func() time.Time { return fixedClock },
	})
	ctx := context.Background()

	// R1：出席建 2（page2 全量取体）
	out, err := r.SyncOnce(ctx, conn.ID)
	if err != nil || out.Status != bizsync.RunStatusSuccess {
		t.Fatalf("R1: err=%v status=%s code=%s", err, out.Status, out.ErrorCode)
	}
	if out.Stats.Created != 2 {
		t.Fatalf("R1 created = %+v", out.Stats)
	}
	if len(repo.docs) != 2 {
		t.Fatalf("R1 docs = %+v", repo.docs)
	}
	doc2ID := ""
	for id := range repo.docs {
		if it, ok := items.get(conn.ID, page2); ok && it.DocumentID == id {
			doc2ID = id
		}
	}
	if doc2ID == "" {
		t.Fatalf("page2 document binding missing")
	}

	// R2：指纹不变 → 全 unchanged（不动文档不写新行）
	out, err = r.SyncOnce(ctx, conn.ID)
	if err != nil || out.Status != bizsync.RunStatusSuccess || out.Stats.Unchanged != 2 {
		t.Fatalf("R2: %v %s %+v", err, out.Status, out.Stats)
	}
	if out.Stats.Updated != 0 || out.Stats.Created != 0 {
		t.Fatalf("R2 must be pure no-op: %+v", out.Stats)
	}

	// R3：page2 外部归档 → walk 缺席 → ConfirmDeleted(archived)=Confirmed → 级联
	archived2 = true
	out, err = r.SyncOnce(ctx, conn.ID)
	if err != nil || out.Status != bizsync.RunStatusSuccess || out.Stats.Deleted != 1 {
		t.Fatalf("R3: %v %s %+v", err, out.Status, out.Stats)
	}
	gone, _ := items.get(conn.ID, page2)
	if gone.Status != bizsync.ItemStatusDeleted {
		t.Fatalf("page2 item = %+v", gone)
	}
	if _, ok := repo.docs[doc2ID]; ok {
		t.Fatalf("page2 document must be cascade-deleted")
	}
	keep, _ := items.get(conn.ID, page1)
	if keep.Status != bizsync.ItemStatusActive {
		t.Fatalf("page1 must stay active: %+v", keep)
	}
	if len(repo.docs) != 1 {
		t.Fatalf("only alpha doc survives: %+v", repo.docs)
	}
	if runs.count() != 3 {
		t.Fatalf("audit rows = %d, want 3", runs.count())
	}
}

// TestE2E_ReleaseGates 释放态闸阵：disabled / 墓碑行（DeleteConnector 软删后
// 行对 runner 不可见）/ 失联三轮一律 Skipped——零 walker 调用、零 run 审计行、
// 连接器面零写动（游标/退避/next_run_at 原样）。
func TestE2E_ReleaseGates(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		f := newRunnerFixture(t, func(c *bizsync.Connector) { c.Enabled = false })
		f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "v1"}}
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !out.Skipped || out.SkipReason != "disabled" {
			t.Fatalf("outcome = %+v", out)
		}
		assertNoRunSideEffects(t, f)
	})

	t.Run("tombstoned_row_invisible", func(t *testing.T) {
		// DeleteConnector 释放态全链钉：真软删路径（PG GetConnector WHERE
		// deleted_at 恒空串守卫）后 runner 闸 1 即 not_found 释放——调度侧
		// 与「全新未知 ID」同面；软删同时清 run_token（释放租约语义）。
		f := newRunnerFixture(t, nil)
		ok, err := f.conns.SoftDeleteConnector(context.Background(), f.conn.ID)
		if err != nil || !ok {
			t.Fatalf("soft delete: ok=%v err=%v", ok, err)
		}
		f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "v1"}}
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !out.Skipped || out.SkipReason != "not_found" {
			t.Fatalf("outcome = %+v", out)
		}
		assertNoRunSideEffects(t, f)
	})

	t.Run("claim_lost", func(t *testing.T) {
		// 轮次已被他主认领（SyncState=running ∧ 心跳鲜活）→ claim_lost 释放（闸 3）。
		f := newRunnerFixture(t, func(c *bizsync.Connector) {
			c.RunToken = "tok:other-owner"
			c.SyncState = bizsync.SyncStateRunning
			c.UpdatedAt = fixedClock // 心跳鲜活（租约窗内）——TryClaimRun 拒窃取
		})
		f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "v1"}}
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !out.Skipped || out.SkipReason != "claim_lost" {
			t.Fatalf("outcome = %+v", out)
		}
		assertNoRunSideEffects(t, f)
	})
}

// assertNoRunSideEffects 释放态三零钉：walker 零调用、审计零行、连接器零漂移。
func assertNoRunSideEffects(t *testing.T, f *runnerFixture) {
	t.Helper()
	if f.walker.walkCalls != 0 {
		t.Fatalf("walker must not run on skipped round: %d calls", f.walker.walkCalls)
	}
	if f.runs.count() != 0 {
		t.Fatalf("skipped round must write no run row: %d", f.runs.count())
	}
	// 行在闸 1 前即被拒（connector 面零读即返）的情形仅 disabled/tombstone；
	// claim_lost 已读到 running 态属主行——零漂移断言对基线态逐字复核。
	if _, ok := f.conns.rows[f.conn.ID]; !ok {
		return // 行已被物理移除（本钉阵不走此径——软删保留行）
	}
	conn := f.conns.get(f.conn.ID)
	base := f.conn
	if base.Enabled && conn.SyncState != bizsync.SyncStateIdle {
		// claim_lost 属主行：running 态基线即非 idle——跳过状态断言，只核漂移面。
		if conn.Cursor != base.Cursor || conn.ConsecutiveFailures != base.ConsecutiveFailures ||
			conn.RunToken != base.RunToken || conn.SyncState != base.SyncState {
			t.Fatalf("claimed row must not drift: %+v vs baseline %+v", conn, base)
		}
		return
	}
	if conn.Cursor != "" || conn.ConsecutiveFailures != 0 || conn.SyncState != bizsync.SyncStateIdle {
		t.Fatalf("connector must not drift: %+v", conn)
	}
	if !conn.NextRunAt.Equal(f.conn.NextRunAt) {
		t.Fatalf("next_run_at must not move: %v", conn.NextRunAt)
	}
}
