// knowledge_source_sync_repo.go —— G-38 SourceSync W1 四表 hand-repo
// （P6 拍板 a：裸 SQL，CRUD 扁平；先例镜像 board_run_lease_repo /
// tool_spill_pg_store 家族：TEXT 定宽时间列复用 boardRunLeaseTimeLayout、
// RenumberPlaceholders、apierror 模块码、ExecInTx+TxExecerFromCtx 同事务多写）。
//
// 纪律：
//   - 唯一写面在 knowledge 域 hand-authored 面内，凡 codegen/注册面一律不新占
//     （五簇贯穿原则 G-38①）；列序与迁移 20261298–20261301 钉死一致——
//     黄金表 sourceSyncTableColumnSet 的 information_schema 钉守卫。
//   - finalize 保留（P10）：InsertRunWithRetention 同事务 INSERT+双 DELETE
//     （runs 保 RunKeep 最近、items 超 ItemRetention 的 missing/deleted 硬清），
//     索引 idx_src_runs_conn (connector_id, started_at DESC) 为共用硬前置。
//   - 凭据 partial-unique 活性唯一（MCP 20261209 空串::text 谓词先例）冲突由
//     entErrToBizErr 扭 409；软删重建由墓碑行豁免天然放行。
package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/pkg/apierror"
)

const sourceSyncModule = "KNOWLEDGE_SOURCE_SYNC"

type sourceSyncRepo struct {
	data *Data
}

var (
	_ sourcesync.ConnectorRepo  = (*sourceSyncRepo)(nil)
	_ sourcesync.ItemRepo       = (*sourceSyncRepo)(nil)
	_ sourcesync.CredentialRepo = (*sourceSyncRepo)(nil)
	_ sourcesync.RunRepo        = (*sourceSyncRepo)(nil)
)

// NewSourceSyncRepo nil-data 安全壳：读写一律 apierror Internal（lease 先例）。
func NewSourceSyncRepo(d *Data) SourceSyncRepos {
	return &sourceSyncRepo{data: d}
}

// SourceSyncRepos 四仓口聚合面（W2/W4/W5 注入侧单点注入；各面仍是
// biz/sourcesync 接口，本聚合仅免四处装配）。
type SourceSyncRepos interface {
	sourcesync.ConnectorRepo
	sourcesync.ItemRepo
	sourcesync.CredentialRepo
	sourcesync.RunRepo
}

func (r *sourceSyncRepo) unavailable() error {
	return apierror.Internal(sourceSyncModule, "repository unavailable")
}

func (r *sourceSyncRepo) db() (*Data, bool) {
	if r == nil || r.data == nil {
		return nil, false
	}
	return r.data, true
}

// ── 时间编解码（TEXT 定宽：字典序==时序；空串↔零值时间） ──

func ssFormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatBoardRunLeaseTime(t)
}

func ssParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(boardRunLeaseTimeLayout, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// ──────────────────────────────── ConnectorRepo ────────────────────────────────

const ssConnectorCols = `id, kind, name, workspace_id, collection_id, config_json, interval_seconds, enabled, sync_state, cursor, next_run_at, last_run_at, last_success_at, consecutive_failures, error_code, error_message, max_items_per_run, max_deletes_per_run, max_fetch_bytes_per_run, run_token, created_at, updated_at`

func (r *sourceSyncRepo) CreateConnector(ctx context.Context, c sourcesync.Connector) error {
	d, ok := r.db()
	if !ok {
		return r.unavailable()
	}
	if c.ID == "" || c.Kind == "" || c.Name == "" || c.CollectionID == "" {
		return apierror.BadRequest(sourceSyncModule, "id, kind, name and collection_id are required")
	}
	nowS := formatBoardRunLeaseTime(time.Now())
	_, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
INSERT INTO knowledge_source_connectors (`+ssConnectorCols+`)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		c.ID, string(c.Kind), c.Name, c.WorkspaceID, c.CollectionID, c.ConfigJSON,
		c.IntervalSeconds, c.Enabled, string(c.SyncState), c.Cursor,
		ssFormatTime(c.NextRunAt), ssFormatTime(c.LastRunAt), ssFormatTime(c.LastSuccessAt),
		c.ConsecutiveFailures, string(c.ErrorCode), sourcesync.TruncateErrorMessage(c.ErrorMessage),
		c.MaxItemsPerRun, c.MaxDeletesPerRun, c.MaxFetchBytesPerRun,
		c.RunToken, nowS, nowS,
	)
	if err != nil {
		// partial-unique（同 collection 活性唯一）/CHECK 违例 → 409/400 分级。
		return entErrToBizErr(err, sourceSyncModule)
	}
	return nil
}

// scanConnectorRow 单行走查（列序与 ssConnectorCols 钉死）。
func scanConnectorRow(row interface{ Scan(...any) error }) (sourcesync.Connector, error) {
	var c sourcesync.Connector
	var kind, state, code, nextRun, lastRun, lastOK, createdAt, updatedAt string
	err := row.Scan(
		&c.ID, &kind, &c.Name, &c.WorkspaceID, &c.CollectionID, &c.ConfigJSON,
		&c.IntervalSeconds, &c.Enabled, &state, &c.Cursor,
		&nextRun, &lastRun, &lastOK,
		&c.ConsecutiveFailures, &code, &c.ErrorMessage,
		&c.MaxItemsPerRun, &c.MaxDeletesPerRun, &c.MaxFetchBytesPerRun,
		&c.RunToken, &createdAt, &updatedAt,
	)
	if err != nil {
		return c, err
	}
	c.Kind = sourcesync.Kind(kind)
	c.SyncState = sourcesync.SyncState(state)
	c.ErrorCode = sourcesync.ErrorCode(code)
	if c.NextRunAt, err = ssParseTime(nextRun); err != nil {
		return c, err
	}
	if c.LastRunAt, err = ssParseTime(lastRun); err != nil {
		return c, err
	}
	if c.LastSuccessAt, err = ssParseTime(lastOK); err != nil {
		return c, err
	}
	if c.CreatedAt, err = ssParseTime(createdAt); err != nil {
		return c, err
	}
	if c.UpdatedAt, err = ssParseTime(updatedAt); err != nil {
		return c, err
	}
	return c, nil
}

func (r *sourceSyncRepo) GetConnector(ctx context.Context, id string) (sourcesync.Connector, bool, error) {
	var c sourcesync.Connector
	d, ok := r.db()
	if !ok {
		return c, false, r.unavailable()
	}
	if id == "" {
		return c, false, nil
	}
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx, d.Dialect().RenumberPlaceholders(`
SELECT `+ssConnectorCols+` FROM knowledge_source_connectors WHERE id = ? AND deleted_at = ''`), id)
	if err != nil {
		return c, false, entErrToBizErr(err, sourceSyncModule)
	}
	defer rows.Close()
	if !rows.Next() {
		return c, false, rows.Err()
	}
	c, err = scanConnectorRow(rows)
	if err != nil {
		return c, false, entErrToBizErr(err, sourceSyncModule)
	}
	return c, true, rows.Err()
}

func (r *sourceSyncRepo) UpdateConnector(ctx context.Context, c sourcesync.Connector) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	if c.ID == "" {
		return false, apierror.BadRequest(sourceSyncModule, "id required")
	}
	// run_token='' 管理面 CAS 守卫（W5 评审防全行 replace 吞在途轮：谓词原子
	// 拒管理写撞 active claim——读改竞态窗口由本谓词兜底，在途即 found=false，
	// 管理编排侧据此分级 409 而非 404）。
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connectors SET
  kind = ?, name = ?, workspace_id = ?, collection_id = ?, config_json = ?,
  interval_seconds = ?, enabled = ?, sync_state = ?, cursor = ?,
  next_run_at = ?, last_run_at = ?, last_success_at = ?,
  consecutive_failures = ?, error_code = ?, error_message = ?,
  max_items_per_run = ?, max_deletes_per_run = ?, max_fetch_bytes_per_run = ?,
  run_token = ?, updated_at = ?
WHERE id = ? AND run_token = '' AND deleted_at = ''`),
		string(c.Kind), c.Name, c.WorkspaceID, c.CollectionID, c.ConfigJSON,
		c.IntervalSeconds, c.Enabled, string(c.SyncState), c.Cursor,
		ssFormatTime(c.NextRunAt), ssFormatTime(c.LastRunAt), ssFormatTime(c.LastSuccessAt),
		c.ConsecutiveFailures, string(c.ErrorCode), sourcesync.TruncateErrorMessage(c.ErrorMessage),
		c.MaxItemsPerRun, c.MaxDeletesPerRun, c.MaxFetchBytesPerRun,
		c.RunToken, formatBoardRunLeaseTime(time.Now()), c.ID,
	)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sourceSyncRepo) SoftDeleteConnector(ctx context.Context, id string) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	if id == "" {
		return false, apierror.BadRequest(sourceSyncModule, "id required")
	}
	deleted := false
	err := d.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, d.RWDB().WriteHandle())
		nowS := formatBoardRunLeaseTime(time.Now())
		res, err := e.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connectors
SET deleted_at = ?, updated_at = ?, run_token = ''
WHERE id = ? AND deleted_at = ''`), nowS, nowS, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			deleted = true
		}
		// ⑦ 级联软删凭据（同笔事务；幂等——即使 connector 重复软删也执行，
		// 覆盖「首轮部分级联失败后重试」残局）。
		if _, err := e.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connector_credential
SET deleted_at = ?, updated_at = ?
WHERE connector_id = ? AND deleted_at = ''`), nowS, nowS, id); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	return deleted, nil
}

func (r *sourceSyncRepo) ListConnectorsByWorkspace(ctx context.Context, workspaceID string) ([]sourcesync.Connector, error) {
	d, ok := r.db()
	if !ok {
		return nil, r.unavailable()
	}
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx, d.Dialect().RenumberPlaceholders(`
SELECT `+ssConnectorCols+` FROM knowledge_source_connectors
WHERE workspace_id = ? AND deleted_at = ''
ORDER BY created_at, id`), workspaceID)
	if err != nil {
		return nil, entErrToBizErr(err, sourceSyncModule)
	}
	return scanConnectors(rows)
}

func (r *sourceSyncRepo) ListDueConnectors(ctx context.Context, now time.Time, limit int) ([]sourcesync.Connector, error) {
	d, ok := r.db()
	if !ok {
		return nil, r.unavailable()
	}
	if limit <= 0 {
		limit = 8
	}
	// due 谓词与 idx_src_connectors_due 字节对齐（enabled ∧ 未软删）；
	// running/auth_paused 残余过滤（防双跑/防烧配额）。
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx, d.Dialect().RenumberPlaceholders(`
SELECT `+ssConnectorCols+` FROM knowledge_source_connectors
WHERE enabled AND deleted_at = '' AND next_run_at <> '' AND next_run_at <= ?
  AND sync_state NOT IN ('running', 'auth_paused')
ORDER BY next_run_at ASC
LIMIT ?`), ssFormatTime(now), limit)
	if err != nil {
		return nil, entErrToBizErr(err, sourceSyncModule)
	}
	return scanConnectors(rows)
}

func scanConnectors(rows *sql.Rows) ([]sourcesync.Connector, error) {
	defer rows.Close()
	out := []sourcesync.Connector{}
	for rows.Next() {
		c, err := scanConnectorRow(rows)
		if err != nil {
			return nil, entErrToBizErr(err, sourceSyncModule)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── W2 认领/收轮（claim CAS + finalize CAS guard + 崩溃回收） ──
//
// 认领时间戳纪律：表内无 run_claimed_at 列（迁移 20261298 定列冻结），
// updated_at 双重承载「最后写时刻」与「租约心跳」——TryClaimRun 落 now、
// Runner 长轮周期心跳续期、Finalize 收轮再落；僵死谓词一律
// sync_state='running' ∧ updated_at < now-leaseTTL（TEXT 定宽字典序==时序）。
// PATCH/PUT 管理面写入顺带续期属可接受副作用（延迟回收一个 TTL，无害）。

func (r *sourceSyncRepo) TryClaimRun(ctx context.Context, id, runToken string, leaseTTL time.Duration, now time.Time) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	if id == "" || runToken == "" {
		return false, apierror.BadRequest(sourceSyncModule, "id and runToken required")
	}
	if leaseTTL <= 0 {
		leaseTTL = sourcesync.DefaultRunLeaseTTL
	}
	nowS := formatBoardRunLeaseTime(now)
	staleBefore := ssFormatTime(now.Add(-leaseTTL))
	// 原子 CAS 一条 UPDATE 完成：新轮守旧轮（idle/error 直取）∨
	// 窘迫尸窃取（running 但心跳过期）；auth_paused 一律拒（凭据唤醒唯一出路）。
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connectors
SET run_token = ?, sync_state = 'running', updated_at = ?
WHERE id = ? AND deleted_at = '' AND enabled
  AND (
    sync_state IN ('idle', 'error')
    OR (sync_state = 'running' AND updated_at <> '' AND updated_at < ?)
  )`), runToken, nowS, id, staleBefore)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sourceSyncRepo) HeartbeatClaim(ctx context.Context, id, runToken string, now time.Time) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	if id == "" || runToken == "" {
		return false, apierror.BadRequest(sourceSyncModule, "id and runToken required")
	}
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connectors SET updated_at = ?
WHERE id = ? AND run_token = ? AND sync_state = 'running' AND deleted_at = ''`),
		formatBoardRunLeaseTime(now), id, runToken)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sourceSyncRepo) FinalizeRunWithClaim(ctx context.Context, c sourcesync.Connector, expectedRunToken string) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	if c.ID == "" || expectedRunToken == "" {
		return false, apierror.BadRequest(sourceSyncModule, "id and expectedRunToken required")
	}
	// 收轮只写运行生命周期列（管理面列归 UpdateConnector 全量替换，两写面
	// 不互踩）；CAS 守卫 run_token——失配即本轮已被软删/他人接管，found=false。
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connectors SET
  sync_state = ?, cursor = ?, next_run_at = ?, last_run_at = ?, last_success_at = ?,
  consecutive_failures = ?, error_code = ?, error_message = ?,
  run_token = '', updated_at = ?
WHERE id = ? AND run_token = ? AND deleted_at = ''`),
		string(c.SyncState), c.Cursor,
		ssFormatTime(c.NextRunAt), ssFormatTime(c.LastRunAt), ssFormatTime(c.LastSuccessAt),
		c.ConsecutiveFailures, string(c.ErrorCode), sourcesync.TruncateErrorMessage(c.ErrorMessage),
		formatBoardRunLeaseTime(time.Now()), c.ID, expectedRunToken,
	)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sourceSyncRepo) ReclaimZombieRuns(ctx context.Context, now time.Time, leaseTTL time.Duration) (int, error) {
	d, ok := r.db()
	if !ok {
		return 0, r.unavailable()
	}
	if leaseTTL <= 0 {
		leaseTTL = sourcesync.DefaultRunLeaseTTL
	}
	staleBefore := ssFormatTime(now.Add(-leaseTTL))
	// 崩溃回收：过期 running 重置 idle 让下轮 due 拾取可再认领；轻罚
	// consecutive_failures+1（僵死非瞬态）；不动 error/last_run_*（现场留痕）。
	// 与活跃 Runner 的 finalize 竞态由 run_token CAS 兜住——回收先清 token，
	// 后到的 FinalizeRunWithClaim 返回 found=false（Runner 记 claim_lost）。
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connectors
SET sync_state = 'idle', run_token = '', consecutive_failures = consecutive_failures + 1, updated_at = ?
WHERE deleted_at = '' AND sync_state = 'running'
  AND updated_at <> '' AND updated_at < ?`), formatBoardRunLeaseTime(now), staleBefore)
	if err != nil {
		return 0, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *sourceSyncRepo) TriggerConnectorSync(ctx context.Context, id string, now time.Time) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	if strings.TrimSpace(id) == "" {
		return false, apierror.BadRequest(sourceSyncModule, "id is required")
	}
	// 手动触发入队=next_run_at 置即时；在途/停轮/禁用的失败全收敛为
	// accepted=false（API 侧携状态分级 409；token 一律由 scheduler 认领时
	// 签发，本方法不碰）。sync_state<>auth_paused 与停轮语义钉对齐。
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connectors
SET next_run_at = ?, updated_at = ?
WHERE id = ? AND deleted_at = '' AND enabled = true
  AND run_token = '' AND sync_state <> 'auth_paused'`), formatBoardRunLeaseTime(now), formatBoardRunLeaseTime(now), id)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sourceSyncRepo) WakeConnectorFromAuthPaused(ctx context.Context, id string, now time.Time) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	if strings.TrimSpace(id) == "" {
		return false, apierror.BadRequest(sourceSyncModule, "id is required")
	}
	// PUT 凭据唤醒（④ 铁律链）：仅自 auth_paused 收；非停轮态 false=no-op
	// （重复 PUT/分类漂移不报错误读）。不动 next_run_at——auth 收轮未推进
	// 即已过期，下 sweep 自然拾取不硬顶波峰。
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connectors
SET sync_state = 'idle', error_code = '', consecutive_failures = 0, updated_at = ?
WHERE id = ? AND deleted_at = '' AND sync_state = 'auth_paused'`), formatBoardRunLeaseTime(now), id)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ──────────────────────────────── ItemRepo ────────────────────────────────

const ssItemCols = `connector_id, external_id, document_id, fingerprint, content_hash, own_lifecycle, status, miss_count, last_seen_run_id, error_message, first_seen_at, last_seen_at, deleted_at`

func (r *sourceSyncRepo) UpsertItemSeen(ctx context.Context, connectorID, externalID, documentID, fingerprint string, ownLifecycle bool, lastSeenRunID string, seenAt time.Time) error {
	d, ok := r.db()
	if !ok {
		return r.unavailable()
	}
	if connectorID == "" || externalID == "" {
		return apierror.BadRequest(sourceSyncModule, "connector_id and external_id are required")
	}
	seenS := ssFormatTime(seenAt)
	// Upsert 语义钉③-2：出席即 status=active、miss_count=0、error_message 清空
	//（flap 边界不触发删除）；deleted_at 一并清位——源复活=生命周期重开，
	// 文档重绑定走 SetItemDocument（W2/W3 Applier 消费）；document_id/
	// content_hash/own_lifecycle/first_seen_at 不被出席覆盖（绑定另走 Set* 方法）。
	_, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
INSERT INTO knowledge_source_connector_items (`+ssItemCols+`)
VALUES (?, ?, ?, ?, '', ?, 'active', 0, ?, '', ?, ?, '')
ON CONFLICT (connector_id, external_id) DO UPDATE SET
  fingerprint = excluded.fingerprint,
  status = 'active',
  miss_count = 0,
  error_message = '',
  deleted_at = '',
  last_seen_run_id = excluded.last_seen_run_id,
  last_seen_at = excluded.last_seen_at`),
		connectorID, externalID, documentID, fingerprint, ownLifecycle, lastSeenRunID, seenS, seenS,
	)
	return entErrToBizErr(err, sourceSyncModule)
}

func scanItemRow(row interface{ Scan(...any) error }) (sourcesync.Item, error) {
	var it sourcesync.Item
	var status, firstSeen, lastSeen, deletedAt string
	err := row.Scan(
		&it.ConnectorID, &it.ExternalID, &it.DocumentID, &it.Fingerprint,
		&it.ContentHash, &it.OwnLifecycle, &status, &it.MissCount,
		&it.LastSeenRunID, &it.ErrorMessage, &firstSeen, &lastSeen, &deletedAt,
	)
	if err != nil {
		return it, err
	}
	it.Status = sourcesync.ItemStatus(status)
	if it.FirstSeenAt, err = ssParseTime(firstSeen); err != nil {
		return it, err
	}
	if it.LastSeenAt, err = ssParseTime(lastSeen); err != nil {
		return it, err
	}
	if it.DeletedAt, err = ssParseTime(deletedAt); err != nil {
		return it, err
	}
	return it, nil
}

func (r *sourceSyncRepo) GetItem(ctx context.Context, connectorID, externalID string) (sourcesync.Item, bool, error) {
	var it sourcesync.Item
	d, ok := r.db()
	if !ok {
		return it, false, r.unavailable()
	}
	if connectorID == "" || externalID == "" {
		return it, false, nil
	}
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx, d.Dialect().RenumberPlaceholders(`
SELECT `+ssItemCols+` FROM knowledge_source_connector_items
WHERE connector_id = ? AND external_id = ?`), connectorID, externalID)
	if err != nil {
		return it, false, entErrToBizErr(err, sourceSyncModule)
	}
	defer rows.Close()
	if !rows.Next() {
		return it, false, rows.Err()
	}
	it, err = scanItemRow(rows)
	if err != nil {
		return it, false, entErrToBizErr(err, sourceSyncModule)
	}
	return it, true, rows.Err()
}

func (r *sourceSyncRepo) SetItemDocument(ctx context.Context, connectorID, externalID, documentID string, ownLifecycle bool) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connector_items
SET document_id = ?, own_lifecycle = ?
WHERE connector_id = ? AND external_id = ? AND status <> 'deleted'`),
		documentID, ownLifecycle, connectorID, externalID)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sourceSyncRepo) SetItemContentHash(ctx context.Context, connectorID, externalID, contentHash string) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	// 提交成功才回填（checkpoint 唯一推进点）。
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connector_items
SET content_hash = ?
WHERE connector_id = ? AND external_id = ? AND status = 'active'`),
		contentHash, connectorID, externalID)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sourceSyncRepo) SetItemError(ctx context.Context, connectorID, externalID, message string) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connector_items
SET error_message = ?
WHERE connector_id = ? AND external_id = ? AND status <> 'deleted'`),
		sourcesync.TruncateErrorMessage(message), connectorID, externalID)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sourceSyncRepo) ListItemsByStatus(ctx context.Context, connectorID string, status sourcesync.ItemStatus, limit int) ([]sourcesync.Item, error) {
	d, ok := r.db()
	if !ok {
		return nil, r.unavailable()
	}
	if limit <= 0 {
		limit = 500
	}
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx, d.Dialect().RenumberPlaceholders(`
SELECT `+ssItemCols+` FROM knowledge_source_connector_items
WHERE connector_id = ? AND status = ?
ORDER BY external_id
LIMIT ?`), connectorID, string(status), limit)
	if err != nil {
		return nil, entErrToBizErr(err, sourceSyncModule)
	}
	defer rows.Close()
	out := []sourcesync.Item{}
	for rows.Next() {
		it, err := scanItemRow(rows)
		if err != nil {
			return nil, entErrToBizErr(err, sourceSyncModule)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *sourceSyncRepo) CountActiveItems(ctx context.Context, connectorID string) (int, error) {
	d, ok := r.db()
	if !ok {
		return 0, r.unavailable()
	}
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx, d.Dialect().RenumberPlaceholders(`
SELECT COUNT(*) FROM knowledge_source_connector_items
WHERE connector_id = ? AND status = 'active'`), connectorID)
	if err != nil {
		return 0, entErrToBizErr(err, sourceSyncModule)
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, rows.Err()
	}
	var n int
	if err := rows.Scan(&n); err != nil {
		return 0, entErrToBizErr(err, sourceSyncModule)
	}
	return n, rows.Err()
}

func (r *sourceSyncRepo) MarkAbsentItemsMissing(ctx context.Context, connectorID, runToken string) (int, error) {
	d, ok := r.db()
	if !ok {
		return 0, r.unavailable()
	}
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connector_items
SET status = 'missing', miss_count = miss_count + 1
WHERE connector_id = ? AND status = 'active' AND last_seen_run_id <> ?`),
		connectorID, runToken)
	if err != nil {
		return 0, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *sourceSyncRepo) MarkItemDeleted(ctx context.Context, connectorID, externalID string, when time.Time) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connector_items
SET status = 'deleted', deleted_at = ?, error_message = ''
WHERE connector_id = ? AND external_id = ? AND status <> 'deleted'`),
		ssFormatTime(when), connectorID, externalID)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ──────────────────────────────── CredentialRepo ────────────────────────────────

const ssCredentialCols = `id, connector_id, credential_key, status, secret_ref, metadata_json, created_at, updated_at`

func (r *sourceSyncRepo) UpsertCredential(ctx context.Context, c sourcesync.Credential) error {
	d, ok := r.db()
	if !ok {
		return r.unavailable()
	}
	if c.ID == "" || c.ConnectorID == "" || c.CredentialKey == "" {
		return apierror.BadRequest(sourceSyncModule, "id, connector_id and credential_key are required")
	}
	if c.SecretRef == "" {
		return apierror.BadRequest(sourceSyncModule, "secret_ref required（RequireKeyForSecret 拒写闸对齐）")
	}
	nowS := formatBoardRunLeaseTime(time.Now())
	// partial-arbiter 谓词与 idx_src_credential_active 字节对齐：软删墓碑行不
	// 参与仲裁——软删后同 key 再创=新行插入；活跃行冲突=原位更新。
	_, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
INSERT INTO knowledge_source_connector_credential (`+ssCredentialCols+`)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (connector_id, credential_key) WHERE deleted_at = ''::text
DO UPDATE SET
  status = excluded.status,
  secret_ref = excluded.secret_ref,
  metadata_json = excluded.metadata_json,
  updated_at = excluded.updated_at`),
		c.ID, c.ConnectorID, c.CredentialKey, c.Status, c.SecretRef, c.MetadataJSON, nowS, nowS,
	)
	return entErrToBizErr(err, sourceSyncModule)
}

func scanCredentialRow(row interface{ Scan(...any) error }) (sourcesync.Credential, error) {
	var cr sourcesync.Credential
	var createdAt, updatedAt string
	err := row.Scan(&cr.ID, &cr.ConnectorID, &cr.CredentialKey, &cr.Status, &cr.SecretRef, &cr.MetadataJSON, &createdAt, &updatedAt)
	if err != nil {
		return cr, err
	}
	if cr.CreatedAt, err = ssParseTime(createdAt); err != nil {
		return cr, err
	}
	if cr.UpdatedAt, err = ssParseTime(updatedAt); err != nil {
		return cr, err
	}
	return cr, nil
}

func (r *sourceSyncRepo) GetCredential(ctx context.Context, connectorID, credentialKey string) (sourcesync.Credential, bool, error) {
	var cr sourcesync.Credential
	d, ok := r.db()
	if !ok {
		return cr, false, r.unavailable()
	}
	if connectorID == "" || credentialKey == "" {
		return cr, false, nil
	}
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx, d.Dialect().RenumberPlaceholders(`
SELECT id, connector_id, credential_key, status, secret_ref, metadata_json, created_at, updated_at
FROM knowledge_source_connector_credential
WHERE connector_id = ? AND credential_key = ? AND deleted_at = ''`), connectorID, credentialKey)
	if err != nil {
		return cr, false, entErrToBizErr(err, sourceSyncModule)
	}
	defer rows.Close()
	if !rows.Next() {
		return cr, false, rows.Err()
	}
	cr, err = scanCredentialRow(rows)
	if err != nil {
		return cr, false, entErrToBizErr(err, sourceSyncModule)
	}
	return cr, true, rows.Err()
}

func (r *sourceSyncRepo) SoftDeleteCredential(ctx context.Context, connectorID, credentialKey string) (bool, error) {
	d, ok := r.db()
	if !ok {
		return false, r.unavailable()
	}
	res, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, d.Dialect().RenumberPlaceholders(`
UPDATE knowledge_source_connector_credential
SET deleted_at = ?, updated_at = ?
WHERE connector_id = ? AND credential_key = ? AND deleted_at = ''`),
		formatBoardRunLeaseTime(time.Now()), formatBoardRunLeaseTime(time.Now()), connectorID, credentialKey)
	if err != nil {
		return false, entErrToBizErr(err, sourceSyncModule)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sourceSyncRepo) ListCredentialsByConnector(ctx context.Context, connectorID string) ([]sourcesync.Credential, error) {
	d, ok := r.db()
	if !ok {
		return nil, r.unavailable()
	}
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx, d.Dialect().RenumberPlaceholders(`
SELECT id, connector_id, credential_key, status, secret_ref, metadata_json, created_at, updated_at
FROM knowledge_source_connector_credential
WHERE connector_id = ? AND deleted_at = ''
ORDER BY credential_key`), connectorID)
	if err != nil {
		return nil, entErrToBizErr(err, sourceSyncModule)
	}
	defer rows.Close()
	out := []sourcesync.Credential{}
	for rows.Next() {
		cr, err := scanCredentialRow(rows)
		if err != nil {
			return nil, entErrToBizErr(err, sourceSyncModule)
		}
		out = append(out, cr)
	}
	return out, rows.Err()
}

// ──────────────────────────────── RunRepo ────────────────────────────────

const ssRunCols = `id, connector_id, started_at, finished_at, status, created, updated, unchanged, fetch_failed, deleted, miss_unknown, bytes_pulled, error_code, error_message`

func (r *sourceSyncRepo) InsertRunWithRetention(ctx context.Context, run sourcesync.Run) error {
	d, ok := r.db()
	if !ok {
		return r.unavailable()
	}
	if run.ID == "" || run.ConnectorID == "" || run.StartedAt.IsZero() || !run.Status.Valid() {
		return apierror.BadRequest(sourceSyncModule, "id, connector_id, started_at and terminal status are required")
	}
	return entErrToBizErr(d.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, d.RWDB().WriteHandle())
		if _, err := e.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`
INSERT INTO knowledge_source_connector_runs (`+ssRunCols+`)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			run.ID, run.ConnectorID, ssFormatTime(run.StartedAt), ssFormatTime(run.FinishedAt),
			string(run.Status), run.Created, run.Updated, run.Unchanged,
			run.FetchFailed, run.Deleted, run.MissUnknown, run.BytesPulled,
			string(run.ErrorCode), sourcesync.TruncateErrorMessage(run.ErrorMessage),
		); err != nil {
			return err
		}
		// P10 硬条文①：同事务 DELETE 保每 connector 最近 RunKeep 轮
		//（idx_src_runs_conn 支撑 ORDER BY started_at DESC 裁剪子查询；
		// 同比 tie 兜底 id DESC 定序——定宽布局同秒多轮可重）。
		if _, err := e.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`
DELETE FROM knowledge_source_connector_runs
WHERE connector_id = ? AND id NOT IN (
  SELECT id FROM knowledge_source_connector_runs
  WHERE connector_id = ?
  ORDER BY started_at DESC, id DESC
  LIMIT ?)`),
			run.ConnectorID, run.ConnectorID, sourcesync.RunKeep,
		); err != nil {
			return err
		}
		// P10 姊妹条款②：同事务第二 DELETE——超 ItemRetention 的
		// missing（last_seen_at 准）/deleted（deleted_at 准）指纹行硬清；
		// adopt（own_lifecycle=false）active 行与 missing 未到窗一律保留。
		cutoff := ssFormatTime(time.Now().Add(-sourcesync.ItemRetention))
		if _, err := e.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`
DELETE FROM knowledge_source_connector_items
WHERE connector_id = ? AND (
  (status = 'missing' AND last_seen_at <> '' AND last_seen_at < ?)
  OR (status = 'deleted' AND deleted_at <> '' AND deleted_at < ?)
)`),
			run.ConnectorID, cutoff, cutoff,
		); err != nil {
			return err
		}
		return nil
	}), sourceSyncModule)
}

func scanRunRow(row interface{ Scan(...any) error }) (sourcesync.Run, error) {
	var run sourcesync.Run
	var started, finished, status, code string
	err := row.Scan(
		&run.ID, &run.ConnectorID, &started, &finished, &status,
		&run.Created, &run.Updated, &run.Unchanged, &run.FetchFailed,
		&run.Deleted, &run.MissUnknown, &run.BytesPulled, &code, &run.ErrorMessage,
	)
	if err != nil {
		return run, err
	}
	run.Status = sourcesync.RunStatus(status)
	run.ErrorCode = sourcesync.ErrorCode(code)
	if run.StartedAt, err = ssParseTime(started); err != nil {
		return run, err
	}
	if run.FinishedAt, err = ssParseTime(finished); err != nil {
		return run, err
	}
	return run, nil
}

func (r *sourceSyncRepo) ListRuns(ctx context.Context, connectorID string, limit int) ([]sourcesync.Run, error) {
	d, ok := r.db()
	if !ok {
		return nil, r.unavailable()
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx, d.Dialect().RenumberPlaceholders(`
SELECT `+ssRunCols+` FROM knowledge_source_connector_runs
WHERE connector_id = ?
ORDER BY started_at DESC, id DESC
LIMIT ?`), connectorID, limit)
	if err != nil {
		return nil, entErrToBizErr(err, sourceSyncModule)
	}
	defer rows.Close()
	out := []sourcesync.Run{}
	for rows.Next() {
		run, err := scanRunRow(rows)
		if err != nil {
			return nil, entErrToBizErr(err, sourceSyncModule)
		}
		out = append(out, run)
	}
	return out, rows.Err()
}
