package data

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/toolspill"
	"aranea-agents/pkg/apierror"
)

// tool_spill_pg_store_test.go —— C-5 PG SpillStore 语义钉死套件
//（91-multi-replica-spill-store）。
//
// 调度纪律（共享 PG 库，勿破坏）：
//   - GC 三件套（TestToolSpillPGStore_GC*）**不加 t.Parallel** +
//     用例首句清表——Go 语义保证非并行用例先于并行队列跑完，
//     GC 的全表计数断言因此 hermetic；
//   - 其余纯 Put/Get 用例并行无碍：读写一律按自有 session_hash 过滤，
//     互不触碰，也不做全表统计。

// ensureToolSpillPGTable 逐句镜像迁移 20261296_tool_result_spills.sql
// （raw-SQL 表不经 ent Schema.Create；迁移文件改动必须同步本镜像，
// 列序钉死测试依赖不漂移）。
func ensureToolSpillPGTable(t *testing.T, d *Data) {
	t.Helper()
	ctx := context.Background()
	db := d.RWDB().WriteDB(ctx)
	if db == nil {
		t.Fatal("test data rwdb unavailable")
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS tool_result_spills (
  id           TEXT PRIMARY KEY,
  session_hash TEXT NOT NULL DEFAULT '',
  tool         TEXT NOT NULL DEFAULT '',
  content      BYTEA NOT NULL,
  byte_size    BIGINT NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_tool_result_spills_session_created
  ON tool_result_spills(session_hash, created_at);
`)
	if err != nil {
		t.Fatalf("create tool_result_spills mirror DDL: %v", err)
	}
}

// cleanToolSpillPGTable GC 用例前置清表（仅非并行用例可调用——见文件头调度纪律）。
func cleanToolSpillPGTable(t *testing.T, d *Data) {
	t.Helper()
	ctx := context.Background()
	if _, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, `DELETE FROM tool_result_spills`); err != nil {
		t.Fatalf("clean tool_result_spills: %v", err)
	}
}

// openTestSpillPGData C-f：Put/GC 的 advisory-lock 事务走 d.Postgres()
// （PostgresExecInTx 对 pg==nil 收敛 Internal），而 newTestDataPG 只落
// rwDB/rawDB 不落 pg/pgRead——同款 knowledge 测试先例
// （knowledge_blocks_test.go:31 结构字面量）在包内补齐 priv 字段；句柄复用
// SetupTestPG 同一 *sql.DB，随机 schema 隔离语义不变。
func openTestSpillPGData(t *testing.T) *Data {
	t.Helper()
	d := openTestData(t)
	if d.rawDB == nil {
		t.Fatal("test data rawdb unavailable")
	}
	d.pg = d.rawDB
	d.pgRead = d.rawDB
	return d
}

func ageToolSpillPGRow(t *testing.T, d *Data, id string, createdAt time.Time) {
	t.Helper()
	ctx := context.Background()
	if _, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(
		`UPDATE tool_result_spills SET created_at = ? WHERE id = ?`),
		formatBoardRunLeaseTime(createdAt), id); err != nil {
		t.Fatalf("age spill row %s: %v", id, err)
	}
}

// TestToolSpillPGStore_RoundtripPutGetStat 二进制原样往返 + locator 形态
// + Stat 元数据 parity + CreatedAt 定宽往返无损 + 空内容容错 + 常规 not-found。
func TestToolSpillPGStore_RoundtripPutGetStat(t *testing.T) {
	t.Parallel()
	d := openTestSpillPGData(t)
	ensureToolSpillPGTable(t, d)
	fixedNow := time.Unix(1_800_000_000, 123456789).UTC()
	st := NewToolSpillPGStore(d, PGSpillOptions{Now: func() time.Time { return fixedNow }})
	ctx := context.Background()

	content := []byte{0x00, 0xFF, 0x80, 0xFE, 0x22, 0x27, 0x0A, 0x5A, 'g', 'o'} // 含 NUL/高位字节/引号字节
	loc, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-rt", Tool: "bash", Content: content})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if loc.Store != toolspill.StorePG {
		t.Fatalf("locator store = %q, want %q", loc.Store, toolspill.StorePG)
	}
	if len(loc.ID) != 32 {
		t.Fatalf("locator id len = %d, want 32 (%q)", len(loc.ID), loc.ID)
	}
	if loc.Bytes != int64(len(content)) {
		t.Fatalf("locator bytes = %d, want %d", loc.Bytes, len(content))
	}
	if !strings.HasPrefix(loc.String(), "spill://pg/") {
		t.Fatalf("locator string %q missing spill://pg/ prefix", loc.String())
	}
	parsed, err := toolspill.ParseLocator(loc.String())
	if err != nil || parsed.Store != toolspill.StorePG || parsed.ID != loc.ID {
		t.Fatalf("locator parse roundtrip = (%+v,%v), want id %q", parsed, err, loc.ID)
	}

	rec, err := st.Get(ctx, "sess-pg-rt", loc.String())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(rec.Content, content) {
		t.Fatalf("content roundtrip mismatch: got %d bytes %x, want %x", len(rec.Content), rec.Content, content)
	}
	if rec.ID != loc.ID || rec.SessionID != "sess-pg-rt" || rec.Tool != "bash" || rec.ByteSize != int64(len(content)) {
		t.Fatalf("record metadata = %+v", rec)
	}
	if !rec.CreatedAt.Equal(fixedNow) {
		t.Fatalf("created_at roundtrip = %v, want %v", rec.CreatedAt, fixedNow)
	}

	info, err := st.Stat(ctx, "sess-pg-rt", loc.String())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Tool != "bash" || info.ByteSize != int64(len(content)) || info.Locator.ID != loc.ID || !info.CreatedAt.Equal(fixedNow) {
		t.Fatalf("stat parity broken: %+v", info)
	}

	// 空内容（nil）归一为空 BYTEA，往返 0 字节不炸 NOT NULL。
	locEmpty, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-rt", Tool: "noop"})
	if err != nil {
		t.Fatalf("put empty: %v", err)
	}
	recEmpty, err := st.Get(ctx, "sess-pg-rt", locEmpty.String())
	if err != nil || len(recEmpty.Content) != 0 || recEmpty.ByteSize != 0 {
		t.Fatalf("empty content roundtrip = (%d,%d,%v)", len(recEmpty.Content), recEmpty.ByteSize, err)
	}

	// 常规 not-found：形态合法但行不存在。
	missing := "spill://pg/" + strings.Repeat("f", 32)
	if _, err := st.Get(ctx, "sess-pg-rt", missing); !errors.Is(err, toolspill.ErrNotFound) {
		t.Fatalf("get missing = %v, want ErrNotFound", err)
	}
	if _, err := st.Stat(ctx, "sess-pg-rt", missing); !errors.Is(err, toolspill.ErrNotFound) {
		t.Fatalf("stat missing = %v, want ErrNotFound", err)
	}

	// 空 session Put 必须失败（同 FS 侧姿态）。
	if _, err := st.Put(ctx, toolspill.Record{SessionID: "  ", Content: []byte("x")}); err == nil {
		t.Fatal("empty-session put must fail")
	}
}

// TestToolSpillPGStore_CrossSessionNotFound 跨会话读一律 ErrNotFound，
// 永不说明理由（FS resolveOwned 同姿态）；空 session 读同样收敛 not-found。
func TestToolSpillPGStore_CrossSessionNotFound(t *testing.T) {
	t.Parallel()
	d := openTestSpillPGData(t)
	ensureToolSpillPGTable(t, d)
	st := NewToolSpillPGStore(d, PGSpillOptions{})
	ctx := context.Background()

	loc, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-a", Tool: "t", Content: []byte("payload-a")})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := st.Get(ctx, "sess-pg-b", loc.String()); !errors.Is(err, toolspill.ErrNotFound) {
		t.Fatalf("cross-session get = %v, want ErrNotFound", err)
	}
	if _, err := st.Stat(ctx, "sess-pg-b", loc.String()); !errors.Is(err, toolspill.ErrNotFound) {
		t.Fatalf("cross-session stat = %v, want ErrNotFound", err)
	}
	if _, err := st.Get(ctx, "", loc.String()); !errors.Is(err, toolspill.ErrNotFound) {
		t.Fatalf("empty-session get = %v, want ErrNotFound", err)
	}
	if _, err := st.Stat(ctx, "  ", loc.String()); !errors.Is(err, toolspill.ErrNotFound) {
		t.Fatalf("blank-session stat = %v, want ErrNotFound", err)
	}
	// 属主会话照常可读（防 over-block 回归）。
	if _, err := st.Get(ctx, "sess-pg-a", loc.String()); err != nil {
		t.Fatalf("owner-session get must succeed: %v", err)
	}
}

// TestToolSpillPGStore_FSLocatorUnsupported fs scheme locator 一律
// ErrUnsupportedStore + 迁移提示（措辞钉死 spill://pg/ 与 migration），
// 畸形 locator 收敛 ErrInvalidLocator。
func TestToolSpillPGStore_FSLocatorUnsupported(t *testing.T) {
	t.Parallel()
	d := openTestSpillPGData(t)
	ensureToolSpillPGTable(t, d)
	st := NewToolSpillPGStore(d, PGSpillOptions{})
	ctx := context.Background()

	fsLoc := "spill://fs/" + toolspill.SessionDirName("sess-pg-fs") + "/" + strings.Repeat("a", 32) + ".spill.json"
	_, err := st.Get(ctx, "sess-pg-fs", fsLoc)
	if !errors.Is(err, toolspill.ErrUnsupportedStore) {
		t.Fatalf("fs locator get = %v, want ErrUnsupportedStore", err)
	}
	if !strings.Contains(err.Error(), "spill://pg/") || !strings.Contains(err.Error(), "migration") {
		t.Fatalf("unsupported error must carry migration hint: %q", err.Error())
	}
	if _, err := st.Stat(ctx, "sess-pg-fs", fsLoc); !errors.Is(err, toolspill.ErrUnsupportedStore) {
		t.Fatalf("fs locator stat = %v, want ErrUnsupportedStore", err)
	}

	if _, err := st.Get(ctx, "sess-pg-fs", "spill://pg"); !errors.Is(err, toolspill.ErrInvalidLocator) {
		t.Fatalf("malformed locator = %v, want ErrInvalidLocator", err)
	}
}

// TestToolSpillPGStore_FileTooLargeAndSessionQuota 配额哨兵与 FS 同型。
func TestToolSpillPGStore_FileTooLargeAndSessionQuota(t *testing.T) {
	t.Parallel()
	d := openTestSpillPGData(t)
	ensureToolSpillPGTable(t, d)
	ctx := context.Background()

	big := NewToolSpillPGStore(d, PGSpillOptions{MaxFileBytes: 4})
	if _, err := big.Put(ctx, toolspill.Record{SessionID: "sess-pg-quota", Tool: "t", Content: []byte("12345")}); !errors.Is(err, toolspill.ErrFileTooLarge) {
		t.Fatalf("oversize put = %v, want ErrFileTooLarge", err)
	}

	quota := NewToolSpillPGStore(d, PGSpillOptions{MaxSessionBytes: 10})
	if _, err := quota.Put(ctx, toolspill.Record{SessionID: "sess-pg-quota", Tool: "t", Content: []byte("123456")}); err != nil {
		t.Fatalf("first put: %v", err)
	}
	if _, err := quota.Put(ctx, toolspill.Record{SessionID: "sess-pg-quota", Tool: "t", Content: []byte("123456")}); !errors.Is(err, toolspill.ErrSessionQuota) {
		t.Fatalf("quota-breaching put = %v, want ErrSessionQuota", err)
	}
	// 邻会话不受该配额消耗影响（session_hash 行隔离）。
	if _, err := quota.Put(ctx, toolspill.Record{SessionID: "sess-pg-quota-2", Tool: "t", Content: []byte("123456")}); err != nil {
		t.Fatalf("neighbour session must have own quota: %v", err)
	}
}

// TestToolSpillPGStore_IDCollisionRetry 设计 FR-3：主键冲突重生成重试一次
// 成功；两次皆撞收敛 apierror Internal（128bit 碰撞视为异常信号）。
func TestToolSpillPGStore_IDCollisionRetry(t *testing.T) {
	t.Parallel()
	d := openTestSpillPGData(t)
	ensureToolSpillPGTable(t, d)
	st := NewToolSpillPGStore(d, PGSpillOptions{}).(*toolSpillPGStore)
	ctx := context.Background()
	const (
		idX = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		idY = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	drain := errors.New("id sequence exhausted")
	seq := func(ids ...string) func() (string, error) {
		i := 0
		return func() (string, error) {
			if i >= len(ids) {
				return "", drain
			}
			id := ids[i]
			i++
			return id, nil
		}
	}

	st.newID = seq(idX)
	if _, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-coll", Tool: "t", Content: []byte("first")}); err != nil {
		t.Fatalf("seed put: %v", err)
	}
	// 首试撞 idX → 重试 idY 成功且 locator 落新 id。
	st.newID = seq(idX, idY)
	loc, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-coll", Tool: "t", Content: []byte("second")})
	if err != nil || loc.ID != idY {
		t.Fatalf("collision-retry put = (%q,%v), want id %q", loc.ID, err, idY)
	}
	// 两次皆撞 → ErrInternal。
	st.newID = seq(idX, idX)
	if _, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-coll", Tool: "t", Content: []byte("third")}); err == nil {
		t.Fatal("persistent collision must fail")
	} else if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeInternal {
		t.Fatalf("persistent collision error = %v, want apierror INTERNAL", err)
	}
	// id 生成器故障直接透传（非冲突、非 Internal 包装）。
	st.newID = seq()
	if _, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-coll", Tool: "t", Content: []byte("x")}); !errors.Is(err, drain) {
		t.Fatalf("id-generation failure must propagate: %v", err)
	}
}

// TestToolSpillPGStore_NilDataSafe nil Data 安全壳：四方法一律
// apierror Internal（装配层 OFF 之外的直接构造兜底）。C-f F2：补畸形
// locator 输入——钉死 nil-check 先于 ParseLocator/语法校验的顺序。
func TestToolSpillPGStore_NilDataSafe(t *testing.T) {
	t.Parallel()
	st := NewToolSpillPGStore(nil, PGSpillOptions{})
	ctx := context.Background()
	assertInternal := func(name string, err error) {
		t.Helper()
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeInternal {
			t.Fatalf("nil-data %s = %v, want apierror INTERNAL", name, err)
		}
	}
	_, err := st.Put(ctx, toolspill.Record{SessionID: "s", Content: []byte("x")})
	assertInternal("Put", err)
	_, err = st.Get(ctx, "s", "spill://pg/x")
	assertInternal("Get", err)
	_, err = st.Stat(ctx, "s", "spill://pg/x")
	assertInternal("Stat", err)
	_, err = st.GC(ctx, time.Now())
	assertInternal("GC", err)
	// 畸形 locator 同样收敛 Internal：nil-check 不得被语法校验超车。
	_, err = st.Get(ctx, "s", "not-a-locator")
	assertInternal("Get/malformed-locator", err)
	_, err = st.Stat(ctx, "s", "not-a-locator")
	// C-g 复审 C-f-TF-1：Stat 侧必须有 ParseLocator **失败型**探针——
	// 32×z 能通过 ParseLocator 只败在 IsSpillID，钉不住 nil-check 被换位到
	// (ParseLocator, IsSpillID) 开区间的回归；not-a-locator 在 ParseLocator
	// 即败，与 Get 侧同款锚点对齐。
	assertInternal("Stat/malformed-locator", err)
	_, err = st.Stat(ctx, "s", "spill://pg/"+strings.Repeat("z", 32))
	assertInternal("Stat/malformed-id", err)
}

// TestToolSpillPGStore_InvalidIDGrammarRejected C-f C5-PARITY-1：解析
// 通过但 id 形态非法（长度/字符集不符 32 小写 hex）的 locator 报
// ErrInvalidLocator 而非 ErrNotFound，与 FS splitFSID 哨兵分类对齐；
// 形态合法但不存在的 id 仍归 ErrNotFound。
func TestToolSpillPGStore_InvalidIDGrammarRejected(t *testing.T) {
	t.Parallel()
	d := openTestSpillPGData(t)
	ensureToolSpillPGTable(t, d)
	st := NewToolSpillPGStore(d, PGSpillOptions{})
	ctx := context.Background()
	bad := []string{
		"spill://pg/short",
		"spill://pg/" + strings.Repeat("z", 32), // 非 hex
		"spill://pg/" + strings.Repeat("a", 33), // 长度不符
		"spill://pg/" + strings.Repeat("A", 32), // 大写 hex 不收
	}
	for _, loc := range bad {
		if _, err := st.Get(ctx, "sess-pg-idgram", loc); !errors.Is(err, toolspill.ErrInvalidLocator) {
			t.Fatalf("Get %q = %v, want ErrInvalidLocator", loc, err)
		}
		if _, err := st.Stat(ctx, "sess-pg-idgram", loc); !errors.Is(err, toolspill.ErrInvalidLocator) {
			t.Fatalf("Stat %q = %v, want ErrInvalidLocator", loc, err)
		}
	}
	unknown := "spill://pg/" + strings.Repeat("c", 32)
	if _, err := st.Get(ctx, "sess-pg-idgram", unknown); !errors.Is(err, toolspill.ErrNotFound) {
		t.Fatalf("Get %q = %v, want ErrNotFound", unknown, err)
	}
}

// TestToolSpillPGStore_GC_TTL TTL 裁汰 + 幂等重跑。非并行 + 首句清表
// （文件头调度纪律）。
func TestToolSpillPGStore_GC_TTL(t *testing.T) {
	d := openTestSpillPGData(t)
	ensureToolSpillPGTable(t, d)
	cleanToolSpillPGTable(t, d)
	st := NewToolSpillPGStore(d, PGSpillOptions{})
	ctx := context.Background()

	old, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-gc-ttl", Tool: "t", Content: bytes.Repeat([]byte{0x5A}, 11)})
	if err != nil {
		t.Fatalf("put old: %v", err)
	}
	ageToolSpillPGRow(t, d, old.ID, time.Now().UTC().Add(-48*time.Hour))
	fresh, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-gc-ttl", Tool: "t", Content: []byte("fresh")})
	if err != nil {
		t.Fatalf("put fresh: %v", err)
	}

	now := time.Now()
	stats, err := st.GC(ctx, now)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if stats.Scanned != 2 || stats.Removed != 1 || stats.ReclaimedBytes != 11 {
		t.Fatalf("gc stats = %+v, want {Scanned:2 Removed:1 ReclaimedBytes:11}", stats)
	}
	if _, err := st.Get(ctx, "sess-pg-gc-ttl", old.String()); !errors.Is(err, toolspill.ErrNotFound) {
		t.Fatalf("expired row survived gc: %v", err)
	}
	if _, err := st.Get(ctx, "sess-pg-gc-ttl", fresh.String()); err != nil {
		t.Fatalf("fresh row must survive gc: %v", err)
	}
	stats, err = st.GC(ctx, now)
	if err != nil || stats.Removed != 0 || stats.ReclaimedBytes != 0 {
		t.Fatalf("gc rerun must be idempotent: %+v, %v", stats, err)
	}
}

// TestToolSpillPGStore_GC_WholeSecondBoundary 对抗评审 #0/#11 在 spill 侧的
// 端到端对抗锚：过期瞬时恰落整秒（零小数）的行必须被 TTL 裁掉。变宽
// RFC3339Nano 下该行写作 "…:SSZ"、cutoff 写作 "…:SS.5Z"，字典序倒置导致
// 字符串比较漏裁——本用例在旧布局下确定性失败。
func TestToolSpillPGStore_GC_WholeSecondBoundary(t *testing.T) {
	d := openTestSpillPGData(t)
	ensureToolSpillPGTable(t, d)
	cleanToolSpillPGTable(t, d)
	const ttl = time.Hour
	st := NewToolSpillPGStore(d, PGSpillOptions{TTL: ttl})
	ctx := context.Background()

	wholeSec := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	expired, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-gc-ws", Tool: "t", Content: []byte("old")})
	if err != nil {
		t.Fatalf("put expired: %v", err)
	}
	ageToolSpillPGRow(t, d, expired.ID, wholeSec)
	// gcNow 距 wholeSec 恰 ttl 余 500ms：cutoff 落 wholeSec+500ms（带小数），
	// 整秒过期行 cutoff 之下必裁（定宽）；变宽布局下字典序倒置逃裁。
	gcNow := wholeSec.Add(ttl).Add(500 * time.Millisecond)
	fresh, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-gc-ws", Tool: "t", Content: []byte("new")})
	if err != nil {
		t.Fatalf("put fresh: %v", err)
	}
	ageToolSpillPGRow(t, d, fresh.ID, gcNow)

	stats, err := st.GC(ctx, gcNow)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if stats.Removed != 1 {
		t.Fatalf("whole-second expired row escaped gc: %+v", stats)
	}
	if _, err := st.Get(ctx, "sess-pg-gc-ws", expired.String()); !errors.Is(err, toolspill.ErrNotFound) {
		t.Fatalf("whole-second expired row must be gone: %v", err)
	}
	if _, err := st.Get(ctx, "sess-pg-gc-ws", fresh.String()); err != nil {
		t.Fatalf("row at gcNow must survive: %v", err)
	}
}

// TestToolSpillPGStore_GC_TotalCapOldestFirst 总量裁汰：oldest-first 裁至
// 归位 + 幂等。非并行 + 首句清表。
func TestToolSpillPGStore_GC_TotalCapOldestFirst(t *testing.T) {
	d := openTestSpillPGData(t)
	ensureToolSpillPGTable(t, d)
	cleanToolSpillPGTable(t, d)
	st := NewToolSpillPGStore(d, PGSpillOptions{MaxTotalBytes: 100})
	ctx := context.Background()
	base := time.Now().UTC()

	puts := make([]toolspill.Locator, 0, 3)
	for i := 0; i < 3; i++ {
		loc, err := st.Put(ctx, toolspill.Record{SessionID: "sess-pg-gc-cap", Tool: "t", Content: bytes.Repeat([]byte{0x5A}, 40)})
		if err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
		puts = append(puts, loc)
		ageToolSpillPGRow(t, d, loc.ID, base.Add(time.Duration(i-3)*time.Hour))
	}

	// total=120 > cap=100 → excess=20 → 裁最旧一行（40B）归位。
	stats, err := st.GC(ctx, base)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if stats.Scanned != 3 || stats.Removed != 1 || stats.ReclaimedBytes != 40 {
		t.Fatalf("gc stats = %+v, want {Scanned:3 Removed:1 ReclaimedBytes:40}", stats)
	}
	if _, err := st.Get(ctx, "sess-pg-gc-cap", puts[0].String()); !errors.Is(err, toolspill.ErrNotFound) {
		t.Fatalf("oldest row must be evicted first: %v", err)
	}
	for i := 1; i < 3; i++ {
		if _, err := st.Get(ctx, "sess-pg-gc-cap", puts[i].String()); err != nil {
			t.Fatalf("newer row %d must survive: %v", i, err)
		}
	}
	stats, err = st.GC(ctx, base)
	if err != nil || stats.Removed != 0 {
		t.Fatalf("cap gc rerun must be idempotent: %+v, %v", stats, err)
	}
}
