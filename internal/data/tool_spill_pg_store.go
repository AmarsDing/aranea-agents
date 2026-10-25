package data

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/toolspill"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
)

// tool_spill_pg_store.go —— C-5 PG SpillStore（91-multi-replica-spill-store）：
// toolspill.Store 的 PG 行存第二实现，结构镜像 board_run_lease_repo.go 先例
//（RAW SQL + RenumberPlaceholders + apierror 稳定模块码 + nil-data 安全壳）。
//
// 语义与边界：
//   - content BYTEA 原字节承载（无 JSON 封套——行即记录）；conf 默认单文件
//     上界 10MiB 远低于 PG 行限。
//   - 会话所有权：locator 无会话段（spill://pg/<id32>，不同于 FS 的 sha16
//     前缀）——Get/Stat 以 session_hash = toolspill.SessionDirName(sessionID)
//     行过滤，跨会话一律 ErrNotFound（与 FS resolveOwned 的 never-the-reason
//     同姿态）。不落原 sessionID。
//   - created_at TEXT + 定宽布局（复用 boardRunLeaseTimeLayout /
//     formatBoardRunLeaseTime，同包）：批C C-e #0/#11 教训——RFC3339Nano
//     变宽小数在整秒瞬时字典序倒置，TEXT 时间列做范围比较（GC TTL）必须
//     定宽。字典序 == 时序。
//   - id = 32 hex chars（8B 毫秒大端 + 8B crypto/rand，镜像 toolspill
//     defaultID）；Put 主键冲突重生成重试一次，再败 ErrInternal（设计
//     FR-3 决策表钉死）。
//   - 配额哨兵与 FS 同型（ErrFileTooLarge / ErrSessionQuota），上层既有
//     映射不动；非 pg scheme 的 locator 一律 ErrUnsupportedStore + 迁移
//     提示（措辞镜像 FS resolveOwned）。
//   - 并发与哨兵（C-f 评审裁断落实）：Put 的配额判定与 INSERT 包在同一
//     事务，按 session_hash 取 pg advisory xact lock 序列化同会话多副本
//     并发写（SPILL-PG-02：SUM-then-INSERT 同窗竞态精确化）；Get/Stat
//     对解析通过但形态非法的 id 报 ErrInvalidLocator（toolspill.IsSpillID，
//     镜像 FS splitFSID 哨兵分类——C5-PARITY-1）。
//   - 字节口径（C5-PARITY-2 裁断记录，刻意不对齐）：PG 配额/GC 按裸
//     byte_size = len(content)；FS 按盘上 envelope 字节（≈4/3×raw+JSON
//     开销）。两侧均为 guardrail 语义，PG 口径更贴真实内容，不改 FS。
//   - GC：整函数包在单事务内并以固定 key 取 pg advisory xact lock
//     （C-f GC-1：跨副本 GC 互斥，后到者在锁后重读真实快照，杜绝双份
//     excess 超裁）；TTL 先裁（定宽 created_at 字符串比较 == 时序比较），
//     总量超 MaxTotalBytes（>0 启用）再按 created_at ASC oldest-first
//     裁至归位；两阶段删行均用 CTE RETURNING 精确计数（C-f GC-2）。
//     幂等，重跑零行变更。生产侧 GC 调度接线为 deferred（同 FS，Q7）。
//   - execer 面（tx.go:107）只有 ExecContext/QueryContext 两方法，单行
//     查询经 toolSpillPGQueryRow 适配（无 QueryRowContext 可用）。
//
// 测试须用 ensureToolSpillPGTable 镜像 DDL（raw-SQL 表不经 ent
// Schema.Create；迁移文件 20261296_tool_result_spills.sql 改动必须同步
// 镜像，列序钉死）。

const toolSpillPGModule = "TOOL_SPILL_PG"

// toolSpillGCAdvisoryLockKey GC 跨副本互斥的固定 advisory xact lock key
// （C-f GC-1）。命名族与 ddlMigrationAdvisoryLockKey（0x4152414E4D4947
// "ARANMIG" 截断）对齐，ASCII "ARANSPGC" 整 8B 不同值，避免同库键碰撞。
const toolSpillGCAdvisoryLockKey int64 = 0x4152414E_53504743

// PGSpillOptions 装配参数：数值项 <=0 回落 toolspill.Default* 默认值；
// Logger/Now 为 nil 时分别回落 noop 与 time.Now。newID 走默认值（测试可
// 经包内字段注入碰撞序列）。
type PGSpillOptions struct {
	MaxFileBytes    int64
	MaxSessionBytes int64
	MaxTotalBytes   int64
	TTL             time.Duration
	Logger          loggateway.Logger
	Now             func() time.Time
}

type toolSpillPGStore struct {
	data            *Data
	maxFileBytes    int64
	maxSessionBytes int64
	maxTotalBytes   int64
	ttl             time.Duration
	lg              loggateway.Logger
	now             func() time.Time
	newID           func() (string, error)
}

var _ toolspill.Store = (*toolSpillPGStore)(nil)

// NewToolSpillPGStore 构造 PG 行存 SpillStore。d 为 nil 时返回 nil-data
// 安全壳：四方法一律 apierror.Internal("repository unavailable")，装配层
// （provideToolSpillDeps）env=pg 且 d==nil 时按 Warn + OFF 处理。
func NewToolSpillPGStore(d *Data, opts PGSpillOptions) toolspill.Store {
	s := &toolSpillPGStore{
		data:            d,
		maxFileBytes:    opts.MaxFileBytes,
		maxSessionBytes: opts.MaxSessionBytes,
		maxTotalBytes:   opts.MaxTotalBytes,
		ttl:             opts.TTL,
		lg:              opts.Logger,
		now:             opts.Now,
		newID:           genToolSpillPGID,
	}
	if s.maxFileBytes <= 0 {
		s.maxFileBytes = toolspill.DefaultMaxFileBytes
	}
	if s.maxSessionBytes <= 0 {
		s.maxSessionBytes = toolspill.DefaultMaxSessionBytes
	}
	if s.ttl <= 0 {
		s.ttl = toolspill.DefaultTTL
	}
	if s.lg == nil {
		s.lg = loggateway.NewNoop()
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// genToolSpillPGID 默认 id 生成：8B 毫秒大端 + 8B crypto/rand → 32 hex
// chars，镜像 toolspill defaultID（同一格式写进迁移头注释钉死）。
func genToolSpillPGID() (string, error) {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixMilli()))
	if _, err := rand.Read(b[8:]); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// toolSpillPGQueryRow 单行查询适配器：execer 面没有 QueryRowContext，以
// QueryContext + 单次 Next 模拟；零行返回 sql.ErrNoRows 供调用方映射哨兵。
func toolSpillPGQueryRow(ctx context.Context, db execer, query string, args []any, dest ...any) error {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	return rows.Err()
}

// Put implements toolspill.Store. 配额 SUM 与 INSERT 包在同一事务并按
// session_hash 取 pg advisory xact lock（C-f SPILL-PG-02：同会话多副本
// 并发判定串行化，MaxSessionBytes 精确生效）。主键冲突经 ON CONFLICT
// DO NOTHING 探测后重生成 id 重试一次——错误驱动的重试在事务内会触发
// 25P02 aborted，故不走 isPgUniqueViolation 路径；ctx 取消不重试；重试
// 再撞按设计 FR-3 收敛为 ErrInternal。
func (s *toolSpillPGStore) Put(ctx context.Context, rec toolspill.Record) (toolspill.Locator, error) {
	if s == nil || s.data == nil {
		return toolspill.Locator{}, apierror.Internal(toolSpillPGModule, "repository unavailable")
	}
	if err := ctx.Err(); err != nil {
		return toolspill.Locator{}, err
	}
	if strings.TrimSpace(rec.SessionID) == "" {
		return toolspill.Locator{}, fmt.Errorf("toolspill: session id is empty")
	}
	content := rec.Content
	if content == nil {
		// content 列 NOT NULL：pq 对 nil []byte 写 NULL，归一为空数组
		//（knowledge_visibility.go 同款先例）。
		content = []byte{}
	}
	size := int64(len(content))
	if size > s.maxFileBytes {
		return toolspill.Locator{}, fmt.Errorf("%w (%d > %d)", toolspill.ErrFileTooLarge, size, s.maxFileBytes)
	}
	sessHash := toolspill.SessionDirName(rec.SessionID)
	nowS := formatBoardRunLeaseTime(s.now())
	renum := s.data.Dialect().RenumberPlaceholders
	var id string
	err := s.data.PostgresExecInTx(ctx, func(txCtx context.Context, tx *sql.Tx) error {
		// 锁内串行化同会话并发写；锁键按 session_hash 派生（hashtextextended
		// 给 64bit 空间，误碰撞仅损失并发度）。
		if _, err := tx.ExecContext(txCtx, renum(
			`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`), sessHash); err != nil {
			return err
		}
		var used int64
		// SUM(BIGINT) 在 PG 出 NUMERIC：::bigint 显式回收窄类型。
		if err := toolSpillPGQueryRow(txCtx, tx, renum(
			`SELECT COALESCE(SUM(byte_size), 0)::bigint FROM tool_result_spills WHERE session_hash = ?`),
			[]any{sessHash}, &used); err != nil {
			return err
		}
		if used+size > s.maxSessionBytes {
			return fmt.Errorf("%w (%d + %d > %d)", toolspill.ErrSessionQuota, used, size, s.maxSessionBytes)
		}
		const insertSQL = `INSERT INTO tool_result_spills (id, session_hash, tool, content, byte_size, created_at)
VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`
		for attempt := 0; ; attempt++ {
			var err error
			id, err = s.newID()
			if err != nil {
				return fmt.Errorf("toolspill: id generation: %w", err)
			}
			res, err := tx.ExecContext(txCtx, renum(insertSQL),
				id, sessHash, rec.Tool, content, size, nowS)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n == 1 {
				return nil
			}
			// n == 0：唯一冲突命中（ON CONFLICT 探测，事务保持活跃）。
			if txCtx.Err() != nil {
				return txCtx.Err()
			}
			if attempt > 0 {
				// 重试再撞：128bit 空间下视为异常信号，按设计 FR-3 钉 ErrInternal。
				return apierror.Internal(toolSpillPGModule, "spill id collision persisted after retry")
			}
		}
	})
	if err != nil {
		return toolspill.Locator{}, err
	}

	loc := toolspill.Locator{Store: toolspill.StorePG, ID: id, Bytes: size}
	// tool_result.spilled — Informational（AS-EVT-01）：高频，log-only，不上总线。
	s.lg.Info("tool result spilled",
		loggateway.StepID("tool_result.spilled"),
		loggateway.SessionID(rec.SessionID),
		loggateway.Str("tool", rec.Tool),
		loggateway.Int64("bytes", size),
		loggateway.Str("locator", loc.String()),
	)
	return loc, nil
}

// unsupportedStoreError 镜像 FS resolveOwned 的 ErrUnsupportedStore 措辞
// 形态，仅替换本侧 scheme。
func unsupportedStoreError(store string) error {
	return fmt.Errorf("%w %q: this deployment only resolves spill://%s/ locators; the locator scheme migration is pending",
		toolspill.ErrUnsupportedStore, store, toolspill.StorePG)
}

// Get implements toolspill.Store.
func (s *toolSpillPGStore) Get(ctx context.Context, sessionID, locator string) (toolspill.Record, error) {
	if s == nil || s.data == nil {
		return toolspill.Record{}, apierror.Internal(toolSpillPGModule, "repository unavailable")
	}
	loc, err := toolspill.ParseLocator(locator)
	if err != nil {
		return toolspill.Record{}, err
	}
	if loc.Store != toolspill.StorePG {
		return toolspill.Record{}, unsupportedStoreError(loc.Store)
	}
	if !toolspill.IsSpillID(loc.ID) {
		return toolspill.Record{}, fmt.Errorf("%w: bad id segment", toolspill.ErrInvalidLocator)
	}
	if strings.TrimSpace(sessionID) == "" {
		// 跨会话防读同 FS：报 not-found，永不说明理由。
		return toolspill.Record{}, toolspill.ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return toolspill.Record{}, err
	}
	db := s.data.RWDB().ReadDB(ctx)
	if db == nil {
		return toolspill.Record{}, apierror.Internal(toolSpillPGModule, "repository unavailable")
	}
	var (
		tool    string
		content []byte
		size    int64
		created string
	)
	err = toolSpillPGQueryRow(ctx, db, s.data.Dialect().RenumberPlaceholders(
		`SELECT tool, content, byte_size, created_at FROM tool_result_spills WHERE id = ? AND session_hash = ?`),
		[]any{loc.ID, toolspill.SessionDirName(sessionID)},
		&tool, &content, &size, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return toolspill.Record{}, toolspill.ErrNotFound
	}
	if err != nil {
		return toolspill.Record{}, err
	}
	createdAt, err := time.Parse(boardRunLeaseTimeLayout, created)
	if err != nil {
		return toolspill.Record{}, fmt.Errorf("toolspill: created_at decode: %w", err)
	}
	return toolspill.Record{
		ID:        loc.ID,
		SessionID: sessionID,
		Tool:      tool,
		Content:   content,
		ByteSize:  size,
		CreatedAt: createdAt,
	}, nil
}

// Stat implements toolspill.Store（与 Get 同所有权规则，不取 content）。
func (s *toolSpillPGStore) Stat(ctx context.Context, sessionID, locator string) (toolspill.FileInfo, error) {
	if s == nil || s.data == nil {
		return toolspill.FileInfo{}, apierror.Internal(toolSpillPGModule, "repository unavailable")
	}
	loc, err := toolspill.ParseLocator(locator)
	if err != nil {
		return toolspill.FileInfo{}, err
	}
	if loc.Store != toolspill.StorePG {
		return toolspill.FileInfo{}, unsupportedStoreError(loc.Store)
	}
	if !toolspill.IsSpillID(loc.ID) {
		return toolspill.FileInfo{}, fmt.Errorf("%w: bad id segment", toolspill.ErrInvalidLocator)
	}
	if strings.TrimSpace(sessionID) == "" {
		return toolspill.FileInfo{}, toolspill.ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return toolspill.FileInfo{}, err
	}
	db := s.data.RWDB().ReadDB(ctx)
	if db == nil {
		return toolspill.FileInfo{}, apierror.Internal(toolSpillPGModule, "repository unavailable")
	}
	var (
		tool    string
		size    int64
		created string
	)
	err = toolSpillPGQueryRow(ctx, db, s.data.Dialect().RenumberPlaceholders(
		`SELECT tool, byte_size, created_at FROM tool_result_spills WHERE id = ? AND session_hash = ?`),
		[]any{loc.ID, toolspill.SessionDirName(sessionID)},
		&tool, &size, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return toolspill.FileInfo{}, toolspill.ErrNotFound
	}
	if err != nil {
		return toolspill.FileInfo{}, err
	}
	createdAt, err := time.Parse(boardRunLeaseTimeLayout, created)
	if err != nil {
		return toolspill.FileInfo{}, fmt.Errorf("toolspill: created_at decode: %w", err)
	}
	return toolspill.FileInfo{Locator: loc, Tool: tool, ByteSize: size, CreatedAt: createdAt}, nil
}

// GC implements toolspill.Store：整函数包在单事务内并以固定 key 取
// pg advisory xact lock（C-f GC-1：跨副本 GC 互斥——后到者等待时仍能
// 看到先裁方提交，锁后重读的 COUNT/SUM 快照即真实状态，杜绝双份 excess
// 超裁）。TTL 先裁（定宽 created_at 字符串比较 == 时序比较），总量超
// MaxTotalBytes（>0 启用）再按 created_at ASC oldest-first 裁至归位；
// 两阶段删行走的均是 CTE RETURNING 精确计数（C-f GC-2），并发败方零行
// 不虚报。幂等；扫描循环内检查 ctx。
func (s *toolSpillPGStore) GC(ctx context.Context, now time.Time) (toolspill.GCStats, error) {
	var stats toolspill.GCStats
	if s == nil || s.data == nil {
		return stats, apierror.Internal(toolSpillPGModule, "repository unavailable")
	}
	renum := s.data.Dialect().RenumberPlaceholders
	err := s.data.PostgresExecInTx(ctx, func(txCtx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(txCtx, renum(`SELECT pg_advisory_xact_lock(?)`),
			toolSpillGCAdvisoryLockKey); err != nil {
			return err
		}

		// Scanned = 裁汰前全表行数（FS 版扫描全部 spill 文件的对应语义）。
		var total int64
		if err := toolSpillPGQueryRow(txCtx, tx, renum(
			`SELECT COUNT(*), COALESCE(SUM(byte_size), 0)::bigint FROM tool_result_spills`),
			nil, &stats.Scanned, &total); err != nil {
			return err
		}

		// 阶段 1：TTL 裁汰。CTE 一次拿回行数与回收字节（定宽布局字典序==时序，
		// created_at < cutoff 字符串比较成立——C-e #0/#11 教训）。
		cutoff := formatBoardRunLeaseTime(now.Add(-s.ttl))
		var ttlRemoved, ttlReclaimed int64
		if err := toolSpillPGQueryRow(txCtx, tx, renum(
			`WITH d AS (DELETE FROM tool_result_spills WHERE created_at < ? RETURNING byte_size)
SELECT COUNT(*), COALESCE(SUM(byte_size), 0)::bigint FROM d`),
			[]any{cutoff}, &ttlRemoved, &ttlReclaimed); err != nil {
			return err
		}
		stats.Removed += int(ttlRemoved)
		stats.ReclaimedBytes += ttlReclaimed
		total -= ttlReclaimed

		// 阶段 2：总量裁汰（oldest-first），仅当 MaxTotalBytes>0 且仍超额。
		if s.maxTotalBytes > 0 && total > s.maxTotalBytes {
			if err := txCtx.Err(); err != nil {
				return err
			}
			excess := total - s.maxTotalBytes
			rows, err := tx.QueryContext(txCtx, renum(
				`SELECT id, byte_size FROM tool_result_spills ORDER BY created_at ASC`))
			if err != nil {
				return err
			}
			var ids []string
			var freed int64
			for rows.Next() {
				if err := txCtx.Err(); err != nil {
					rows.Close()
					return err
				}
				var (
					id   string
					size int64
				)
				if err := rows.Scan(&id, &size); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, id)
				freed += size
				if freed >= excess {
					break
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
			if len(ids) > 0 {
				// C-f GC-2：与 TTL 阶段同型 CTE，stats 按实际删除计，不按
				// SELECT 快照虚报（并发败方在 advisory 锁下已不可能，此处
				// 口径仍与删除事实钉死）。
				var capRemoved, capReclaimed int64
				if err := toolSpillPGQueryRow(txCtx, tx, renum(
					`WITH d AS (DELETE FROM tool_result_spills WHERE id = ANY(?) RETURNING byte_size)
SELECT COUNT(*), COALESCE(SUM(byte_size), 0)::bigint FROM d`),
					[]any{pq.Array(ids)}, &capRemoved, &capReclaimed); err != nil {
					return err
				}
				stats.Removed += int(capRemoved)
				stats.ReclaimedBytes += capReclaimed
			}
		}
		return nil
	})
	if err != nil {
		// D2-f-3（评审确认）：事务整包回滚，累加 stats 从未生效——err 路径
		// 一律返回零值，与 Store.GC 契约（err 时 stats=已生效部分计数）钉死，
		// 防 GC 调度器的审计 Extra 虚报回滚计数。
		return toolspill.GCStats{}, err
	}
	return stats, nil
}
