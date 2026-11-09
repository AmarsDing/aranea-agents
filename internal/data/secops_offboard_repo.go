package data

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// ============================================================================
// secops_offboard_requests 台账（E 阶段 W2a D3 离职处置五步，DDL 见迁移
// 20261325）。TEXT 时间戳列（RFC3339）与 approval_risk_matrix 同例；
// approved_at/note 内嵌 steps_json（DDL 无独立列，迁移已定稿）。
// 查询一律走 RWDB().ReadDB/WriteDB 返回的 execer（事务感知，与
// approval_risk_matrix_repo 同例）；execer 无 QueryRowContext，单行读
// 用 QueryContext + LIMIT 1 取首行。
// ============================================================================

type secopsOffboardRepo struct {
	data *Data
}

var _ biz.SecopsOffboardRequestRepo = (*secopsOffboardRepo)(nil)

// 离职导出/删除对称的事实存取口由 l3FactRepo 承载（ListFactRows 解密导出、
// ClearUserDomainFacts scope 级软删）；窄口构造与 NewL3FactWriterAdapter 同例。
var (
	_ biz.SecopsOffboardFactLister  = (*l3FactRepo)(nil)
	_ biz.SecopsOffboardFactClearer = (*l3FactRepo)(nil)
)

// NewSecopsOffboardFactLister 创建离职导出的事实列读口（解密在 data 层完成，
// 明文只到 usecase/RPC 响应为止，不进日志/审计）。
func NewSecopsOffboardFactLister(d *Data) biz.SecopsOffboardFactLister {
	if d == nil {
		return nil
	}
	return newL3FactRepo(d, nil)
}

// NewSecopsOffboardFactClearer 创建离职删除的事实清理口（vectorStore 供
// 向量级联软删；与导出同一 l3FactRepo 实现，保证导出/删除口径对称）。
func NewSecopsOffboardFactClearer(d *Data) biz.SecopsOffboardFactClearer {
	if d == nil {
		return nil
	}
	return newL3FactRepo(d, d.VectorStore())
}

// NewSecopsOffboardRepo 返回离职申请单台账仓储。
func NewSecopsOffboardRepo(d *Data) biz.SecopsOffboardRequestRepo {
	return &secopsOffboardRepo{data: d}
}

// offboardSteps 是 steps_json 的承载结构（ApprovedAt/Note 内嵌其中）。
type offboardSteps struct {
	Note       string `json:"note,omitempty"`
	ApprovedAt string `json:"approved_at,omitempty"`
}

func (r *secopsOffboardRepo) Insert(ctx context.Context, req *biz.SecopsOffboardRequest) error {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return apierror.Internal(apierror.DomainAdmin, "repository unavailable")
	}
	if req == nil {
		return apierror.BadRequest(apierror.DomainAdmin, "offboard request is nil")
	}
	id := req.ID
	if id == "" {
		id = newOffboardID()
	}
	now := nowRFC3339()
	if _, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO secops_offboard_requests
			(id, target_user_id, requester_id, approver_id, status, export_checksum, exported_at, steps_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, '', '', ?, ?, ?)`),
		id, req.TargetUserID, req.RequesterID, req.ApproverID, req.Status,
		offboardStepsJSON("", ""), now, now); err != nil {
		return entErrToBizErr(err, "SECOPS")
	}
	req.ID = id
	req.CreatedAt = parseRFC3339Time(now)
	req.UpdatedAt = req.CreatedAt
	return nil
}

func (r *secopsOffboardRepo) GetLatestByTargetUser(ctx context.Context, userID int64) (*biz.SecopsOffboardRequest, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT id, target_user_id, requester_id, approver_id, status, export_checksum, exported_at, steps_json, created_at, updated_at
			FROM secops_offboard_requests WHERE target_user_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`),
		userID)
	if err != nil {
		return nil, entErrToBizErr(err, "SECOPS")
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, entErrToBizErr(err, "SECOPS")
		}
		return nil, nil
	}
	return scanOffboardRow(rows)
}

func (r *secopsOffboardRepo) ListRecent(ctx context.Context, limit int) ([]*biz.SecopsOffboardRequest, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	return r.scanList(ctx, `SELECT id, target_user_id, requester_id, approver_id, status, export_checksum, exported_at, steps_json, created_at, updated_at
		FROM secops_offboard_requests ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
}

func (r *secopsOffboardRepo) ListPending(ctx context.Context, limit int) ([]*biz.SecopsOffboardRequest, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	return r.scanList(ctx, `SELECT id, target_user_id, requester_id, approver_id, status, export_checksum, exported_at, steps_json, created_at, updated_at
		FROM secops_offboard_requests WHERE status = 'requested' ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
}

// MarkDecision 记审批：approver/status 上列，note/approved_at 内嵌 steps_json
//（approved 才记 approved_at；rejected 记 note 供重发参考）。read-modify-write
// 走 ExecInTx，读段经 ReadDB(txCtx) 参与同一事务。
func (r *secopsOffboardRepo) MarkDecision(ctx context.Context, id string, approverID int64, status, note string, at time.Time) error {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return apierror.Internal(apierror.DomainAdmin, "repository unavailable")
	}
	atStr := at.UTC().Format(time.RFC3339)
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		var stepsRaw string
		rdr, err := r.data.RWDB().ReadDB(txCtx).QueryContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`SELECT steps_json FROM secops_offboard_requests WHERE id = ?`), id)
		if err != nil {
			return entErrToBizErr(err, "SECOPS")
		}
		if rdr.Next() {
			if err := rdr.Scan(&stepsRaw); err != nil {
				rdr.Close()
				return entErrToBizErr(err, "SECOPS")
			}
		}
		rdr.Close()
		if err := rdr.Err(); err != nil {
			return entErrToBizErr(err, "SECOPS")
		}
		var steps offboardSteps
		_ = json.Unmarshal([]byte(stepsRaw), &steps)
		steps.Note = note
		if status == biz.OffboardStatusApproved {
			steps.ApprovedAt = atStr
		}
		_, err = r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE secops_offboard_requests
				SET approver_id = ?, status = ?, steps_json = ?, updated_at = ? WHERE id = ?`),
			approverID, status, offboardStepsMustJSON(steps), atStr, id)
		return entErrToBizErr(err, "SECOPS")
	})
}

func (r *secopsOffboardRepo) MarkExported(ctx context.Context, id, checksum string, at time.Time) error {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return apierror.Internal(apierror.DomainAdmin, "repository unavailable")
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE secops_offboard_requests
			SET export_checksum = ?, exported_at = ?, updated_at = ? WHERE id = ?`),
		checksum, at.UTC().Format(time.RFC3339), nowRFC3339(), id)
	return entErrToBizErr(err, "SECOPS")
}

func (r *secopsOffboardRepo) MarkCompleted(ctx context.Context, id string, at time.Time) error {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return apierror.Internal(apierror.DomainAdmin, "repository unavailable")
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE secops_offboard_requests
			SET status = 'completed', updated_at = ? WHERE id = ?`),
		at.UTC().Format(time.RFC3339), id)
	return entErrToBizErr(err, "SECOPS")
}

// ── scan helpers ─────────────────────────────────────────────────────

func (r *secopsOffboardRepo) scanList(ctx context.Context, query string, limit int) ([]*biz.SecopsOffboardRequest, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(query), limit)
	if err != nil {
		return nil, entErrToBizErr(err, "SECOPS")
	}
	defer rows.Close()
	out := make([]*biz.SecopsOffboardRequest, 0)
	for rows.Next() {
		req, err := scanOffboardRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, entErrToBizErr(rows.Err(), "SECOPS")
}

// scanOffboardRow 扫描当前行 → 结构体（TEXT 时间解析 + steps_json 提取
// approved_at/note）。
func scanOffboardRow(rows *sql.Rows) (*biz.SecopsOffboardRequest, error) {
	var req biz.SecopsOffboardRequest
	var exportedAt, stepsRaw, createdAt, updatedAt string
	if err := rows.Scan(&req.ID, &req.TargetUserID, &req.RequesterID, &req.ApproverID, &req.Status,
		&req.ExportChecksum, &exportedAt, &stepsRaw, &createdAt, &updatedAt); err != nil {
		return nil, entErrToBizErr(err, "SECOPS")
	}
	req.ExportedAt = parseRFC3339Time(exportedAt)
	req.CreatedAt = parseRFC3339Time(createdAt)
	req.UpdatedAt = parseRFC3339Time(updatedAt)
	req.StepsJSON = stepsRaw
	var steps offboardSteps
	if json.Unmarshal([]byte(stepsRaw), &steps) == nil {
		req.Note = steps.Note
		req.ApprovedAt = parseRFC3339Time(steps.ApprovedAt)
	}
	return &req, nil
}

func offboardStepsJSON(note, approvedAt string) string {
	return offboardStepsMustJSON(offboardSteps{Note: note, ApprovedAt: approvedAt})
}

// ── 时间有序 id ──────────────────────────────────────────────────────
//
// created_at 为秒级 TEXT（与迁移 20261325 同粒度），同秒插入两单时
// ORDER BY created_at DESC 并列，须靠 id 决胜——纯随机 uuid 会使同秒
// 第二单约 50% 概率排不到首位（同链路复跑现形）。newOffboardID 生成
// obr- + 26 位 Crockford base32，载荷 = ms(48) ‖ 同毫秒序号(16) ‖
// rand(64)：字典序恒等于生成序，同毫秒由序号段严格递增。

const offboardIDCrockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var (
	offboardIDMu   sync.Mutex
	offboardIDLast int64 // 最近分配的毫秒
	offboardIDSeq  uint32
)

func newOffboardID() string {
	offboardIDMu.Lock()
	ms := time.Now().UnixMilli() & 0xFFFFFFFFFFFF
	if ms == offboardIDLast {
		offboardIDSeq++
	} else {
		offboardIDLast = ms
		offboardIDSeq = 0
	}
	seq := offboardIDSeq
	offboardIDMu.Unlock()

	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		// crypto/rand 失败不可恢复性近似为零；退化用时间纳秒低 64 位。
		binary.BigEndian.PutUint64(rnd[:], uint64(time.Now().UnixNano()))
	}
	hi := (ms << 16) | int64(seq&0xFFFF)
	lo := binary.BigEndian.Uint64(rnd[:])

	var out [26]byte
	for i := 0; i < 26; i++ {
		var v uint32
		base := 128 - 5*(i+1) // 位窗起点（<0 的位视为 0，顶字符只含 3 有效位）
		for j := 0; j < 5; j++ {
			v <<= 1
			k := base + j
			switch {
			case k < 0:
			case k < 64:
				v |= uint32((lo >> k) & 1)
			default:
				v |= uint32((hi >> (k - 64)) & 1)
			}
		}
		out[i] = offboardIDCrockford[v]
	}
	return "obr-" + string(out[:])
}

func offboardStepsMustJSON(steps offboardSteps) string {
	raw, err := json.Marshal(steps)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
