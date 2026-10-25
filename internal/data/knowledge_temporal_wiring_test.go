package data

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"
)

// knowledge_temporal_wiring_test.go G-32 T4 接线钉阵（第二组）：
// tombstone 入口集成、决策 7 引用回落接线、时态守卫/RRF 同轨纯函数、
// 决策 12③ as-of ACL 复合谓词（T4 接线评审修正项——授权用当下、内容用历史：
// 活文档行级当下可见性；tombstone 文档行删后回落 collection 级当下口径）。
// 档案内核本身由 knowledge_chunk_archive_test.go 覆盖；十三步差分主账由
// knowledge_diff_commit_test.go 覆盖。

// ── T4 入口集成：DeleteDocument 全链（FOR UPDATE 锁 + 零账本 mint + 归档整
// 档 + producer=user-delete 登记值空间）────────────────────────────────────
func TestG32T4DeleteDocumentTombstoneIntegration(t *testing.T) {
	ctx, db, repo := t4Repo(t)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'c1', 'm0')`)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_documents (id, collection_id, source, content_text, created_at)
		VALUES ('ddel', 'c1', 's1', 'old body', $1)`, t3T0)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, chunk_index) VALUES
		('ddel-v1-ch-0', 'ddel', 'c1', 'alpha', 0),
		('ddel-v1-ch-1', 'ddel', 'c1', 'beta', 1)`)

	if err := repo.DeleteDocument(ctx, "ddel"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM knowledge_documents WHERE id='ddel'`); got != 0 {
		t.Fatalf("doc row survived: %d", got)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM knowledge_chunks WHERE doc_id='ddel'`); got != 0 {
		t.Fatalf("current chunks survived: %d", got)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_archive WHERE doc_id='ddel'`); got != 2 {
		t.Fatalf("archive rows=%d want 2", got)
	}
	// 零账本 mint：v1=seed（g32-tombstone-mint），v2=tombstone（user-delete）。
	vrows, err := db.QueryContext(ctx,
		`SELECT version_no, change_kind, producer FROM document_versions WHERE doc_id='ddel' ORDER BY version_no`)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	defer vrows.Close()
	type vrow struct {
		ver  int
		kind string
		prod string
	}
	got := []vrow{}
	for vrows.Next() {
		var r vrow
		if err := vrows.Scan(&r.ver, &r.kind, &r.prod); err != nil {
			t.Fatalf("scan ledger: %v", err)
		}
		got = append(got, r)
	}
	want := []vrow{{1, "seed", "g32-tombstone-mint"}, {2, "tombstone", "user-delete"}}
	if len(got) != len(want) {
		t.Fatalf("ledger rows=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ledger[%d]=%+v want %+v", i, got[i], want[i])
		}
	}
}

// ── 决策 7 引用回落接线集成：current 缺席移交档案最新死亡化身补内容 ────────
func TestG32T4CitationArchiveFallbackIntegration(t *testing.T) {
	ctx, db, repo := t4Repo(t)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'c1', 'm0')`)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_documents (id, collection_id, source, content_text, created_at)
		VALUES ('dc', 'c1', 's1', 'body', $1)`, t3T0)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, chunk_index)
		VALUES ('c-live', 'dc', 'c1', 'live body', 0)`)
	mustExecG32T3(t, ctx, db, `INSERT INTO chunk_archive (doc_id, chunk_id, revision_no, collection_id, content)
		VALUES ('dc', 'c-dead', 1, 'c1', 'dead body')`)

	trace := &knowledgeCitationTraceRepo{data: repo.data}
	out, err := trace.loadChunkContents(ctx, []string{"c-live", "c-dead", "c-void"})
	if err != nil {
		t.Fatalf("loadChunkContents: %v", err)
	}
	if out["c-live"] != "live body" {
		t.Fatalf("live content=%q", out["c-live"])
	}
	if out["c-dead"] != "dead body" {
		t.Fatalf("archive fallback content=%q", out["c-dead"])
	}
	if _, ok := out["c-void"]; ok {
		t.Fatalf("void id must stay dropped")
	}
	if len(out) != 2 {
		t.Fatalf("resolved entries=%v want 2", out)
	}
}

// ── 接线守卫纯函数钉（零 DB）───────────────────────────────────────────────

// resolveAsOfInstant：RecordedAt 非零结构性拒绝（errors.Is 精确可匹配）；
// 零 ValidAt 退化装配时钟现值（防御性兜底）；显式时点统一 UTC。
func TestG32T4ResolveAsOfInstantGuards(t *testing.T) {
	if _, err := resolveAsOfInstant(bizknowledge.TemporalSpan{RecordedAt: time.Now()}); !errors.Is(err, bizknowledge.ErrUnsupportedOperation) {
		t.Fatalf("recorded-axis guard: %v want ErrUnsupportedOperation", err)
	}
	before := time.Now().Add(-time.Second)
	at, err := resolveAsOfInstant(bizknowledge.TemporalSpan{})
	if err != nil {
		t.Fatalf("zero span: %v", err)
	}
	if at.Before(before) || at.After(time.Now().Add(time.Second)) {
		t.Fatalf("zero ValidAt did not degrade to live clock: %v", at)
	}
	zoned := time.Date(2026, 10, 1, 12, 0, 0, 0, time.FixedZone("X", 2*3600))
	at, err = resolveAsOfInstant(bizknowledge.TemporalSpan{ValidAt: zoned})
	if err != nil {
		t.Fatalf("explicit span: %v", err)
	}
	if !at.Equal(zoned.UTC()) || at.Location() != time.UTC {
		t.Fatalf("UTC normalization: %v", at)
	}
}

// mergeChunkAsOfRankings 与 mergeBM25Rankings 同轨钉：rrfK=60、分支内
// weight=score/branchMax、累加 weight/(rrfK+rank+1)、同分 chunk_id 升序。
func TestG32T4MergeChunkAsOfRankingsRRFParity(t *testing.T) {
	branch1 := []chunkAsOfItem{{ChunkID: "X", Score: 1.0}, {ChunkID: "Y", Score: 0.5}}
	branch2 := []chunkAsOfItem{{ChunkID: "Y", Score: 0.8}, {ChunkID: "Z", Score: 0.4}}
	merged := mergeChunkAsOfRankings([][]chunkAsOfItem{branch1, branch2}, 10)
	if len(merged) != 3 {
		t.Fatalf("merged=%d want 3", len(merged))
	}
	wantX := 1.0 / 61.0
	wantY := 1.0/61.0 + (0.5/1.0)/62.0
	wantZ := (0.4 / 0.8) / 62.0
	const eps = 1e-15
	scoreOf := map[string]float64{}
	for _, it := range merged {
		scoreOf[it.ChunkID] = it.Score
	}
	for id, want := range map[string]float64{"X": wantX, "Y": wantY, "Z": wantZ} {
		if diff := scoreOf[id] - want; diff > eps || diff < -eps {
			t.Fatalf("score[%s]=%v want %v (RRF parity)", id, scoreOf[id], want)
		}
	}
	if merged[0].ChunkID != "Y" || merged[1].ChunkID != "X" || merged[2].ChunkID != "Z" {
		t.Fatalf("order wrong: %v", merged)
	}
	cut := mergeChunkAsOfRankings([][]chunkAsOfItem{branch1, branch2}, 2)
	if len(cut) != 2 || cut[0].ChunkID != "Y" || cut[1].ChunkID != "X" {
		t.Fatalf("topK cut wrong: %v", cut)
	}
	tie := mergeChunkAsOfRankings([][]chunkAsOfItem{
		{{ChunkID: "B", Score: 1.0}},
		{{ChunkID: "A", Score: 1.0}},
	}, 10)
	if len(tie) != 2 || tie[0].ChunkID != "A" || tie[1].ChunkID != "B" {
		t.Fatalf("tie-break wrong: %v", tie)
	}
}

// ── 决策 12③ as-of ACL 复合谓词钉（T4 接线评审修正项）─────────────────────
func TestG32T4AsOfACLPrivateDocAndTombstoneReplay(t *testing.T) {
	ctx, db, _ := t4Repo(t)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'c1', 'm0')`)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_documents (id, collection_id, source, content_text, visibility, created_at)
		VALUES ('dpub', 'c1', 's1', 'pub body', 'collection', $1)`, t3T0)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_documents (id, collection_id, source, content_text, visibility, owner_user_id, created_at)
		VALUES ('dpriv', 'c1', 's1', 'priv body', 'private', '42', $1)`, t3T0)
	for _, doc := range []string{"dpub", "dpriv"} {
		mustExecG32T3(t, ctx, db, `INSERT INTO document_versions (doc_id, version_no, content_hash, change_kind, producer, created_at)
			VALUES ($1, 1, 'sha1:v1', 'seed', 'g32-seed', $2)`, doc, t3T0)
		mustExecG32T3(t, ctx, db, `INSERT INTO document_versions (doc_id, version_no, content_hash, change_kind, producer, created_at)
			VALUES ($1, 2, 'sha1:v2', 'edit', 'g32-seed', $2)`, doc, t3T1)
	}
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, chunk_index) VALUES
		('dpub-v1-ch-0', 'dpub', 'c1', 'pub live', 0),
		('dpriv-v1-ch-0', 'dpriv', 'c1', 'priv live', 0)`)
	mustExecG32T3(t, ctx, db, `INSERT INTO chunk_archive (doc_id, chunk_id, revision_no, collection_id, content) VALUES
		('dpub', 'dpub-v1-ch-1', 2, 'c1', 'pub dead'),
		('dpriv', 'dpriv-v1-ch-1', 2, 'c1', 'priv dead')`)

	mid := mustTime(t, "2026-10-01 12:00:00Z")
	ids := func(c context.Context) []string {
		t.Helper()
		items, err := listChunksAsOfCore(c, db, "c1", mid, "")
		if err != nil {
			t.Fatalf("asof list: %v", err)
		}
		out := []string{}
		for _, it := range items {
			out = append(out, it.ChunkID)
		}
		sort.Strings(out)
		return out
	}
	equal := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	// 无身份买家：活 private 文档双腿全部隐藏（current + archived）。
	if got := ids(ctx); !equal(got, []string{"dpub-v1-ch-0", "dpub-v1-ch-1"}) {
		t.Fatalf("anonymous buyer leaked or lost rows: %v", got)
	}
	// owner：行级当下可见性放行 private 全部双腿。
	owner := auth.NewContext(ctx, &auth.Auth{UserID: 42})
	if got := ids(owner); !equal(got, []string{"dpriv-v1-ch-0", "dpriv-v1-ch-1", "dpub-v1-ch-0", "dpub-v1-ch-1"}) {
		t.Fatalf("owner rows: %v", got)
	}
	// 系统身份：无过滤。
	sys := workspace.WithSystemWorkspace(ctx)
	if got := ids(sys); !equal(got, []string{"dpriv-v1-ch-0", "dpriv-v1-ch-1", "dpub-v1-ch-0", "dpub-v1-ch-1"}) {
		t.Fatalf("system rows: %v", got)
	}

	// tombstone 形态：文档行删（DeleteDocument 终态）——current 腿随之消失，
	// 归档腿按决策 12③ 回落 collection 级当下口径，无身份买家可回放。
	mustExecG32T3(t, ctx, db, `DELETE FROM knowledge_chunks WHERE doc_id='dpriv'`)
	mustExecG32T3(t, ctx, db, `DELETE FROM knowledge_documents WHERE id='dpriv'`)
	if got := ids(ctx); !equal(got, []string{"dpriv-v1-ch-1", "dpub-v1-ch-0", "dpub-v1-ch-1"}) {
		t.Fatalf("post-tombstone replay rows: %v", got)
	}
}
