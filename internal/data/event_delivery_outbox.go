package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

const (
	defaultEventOutboxListLimit    = 100
	defaultEventOutboxPendingLimit = 200
	// maxOutboxSeqAllocAttempts bounds the allocator's conflict-repair-retry
	// loop (D7-1): a (session_id, seq) conflict inside the allocating tx can
	// only come from a legacy Go-counter replica in the mixed rolling-version
	// window; each attempt repair-bumps the counter past the durable
	// watermark, so convergence takes one retry in practice.
	maxOutboxSeqAllocAttempts = 3
	// defaultEventOutboxClaimLimit clamps ClaimPendingOutboxDeliveries
	// (channel_delivery_claim.go 先例：<=0 或超上限回落默认值).
	defaultEventOutboxClaimLimit = 200
	maxEventOutboxClaimLimit     = 500
)

type eventDeliveryOutboxRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.EventDeliveryOutboxRepo = (*eventDeliveryOutboxRepo)(nil)
var _ biz.EventOutboxSeqAllocatingInserter = (*eventDeliveryOutboxRepo)(nil)
var _ biz.EventOutboxClaimer = (*eventDeliveryOutboxRepo)(nil)

// NewEventDeliveryOutboxRepo constructs the raw-SQL outbox repo (B-06).
func NewEventDeliveryOutboxRepo(data *Data, lg loggateway.Logger) biz.EventDeliveryOutboxRepo {
	if data == nil || data.RWDB() == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &eventDeliveryOutboxRepo{data: data, lg: lg.With(loggateway.Domain("event_outbox"))}
}

// NewEventDeliveryOutboxRepoFromData is the Wire-friendly constructor.
func NewEventDeliveryOutboxRepoFromData(d *Data) biz.EventDeliveryOutboxRepo {
	if d == nil {
		return nil
	}
	return NewEventDeliveryOutboxRepo(d, d.lg)
}

func (r *eventDeliveryOutboxRepo) Insert(ctx context.Context, row biz.EventDeliveryOutboxRow) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil
	}
	sessionID := strings.TrimSpace(row.SessionID)
	eventID := strings.TrimSpace(row.EventID)
	if sessionID == "" || eventID == "" || row.Seq <= 0 {
		return nil
	}
	id := strings.TrimSpace(row.ID)
	if id == "" {
		id = uuid.NewString()
	}
	createdAt := row.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	var published any
	if row.PublishedAt != nil && !row.PublishedAt.IsZero() {
		published = row.PublishedAt.UTC().Format(time.RFC3339Nano)
	}
	d := r.data.Dialect()
	cols := "id, session_id, seq, event_id, kind, entity_id, payload, published_at, created_at"
	placeholders := d.Placeholders(9)
	q := d.BuildInsertOrIgnore(
		"event_delivery_outbox",
		cols,
		placeholders,
		"session_id, seq",
	)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q,
		id, sessionID, row.Seq, eventID, row.Kind, row.EntityID, row.Payload, published,
		createdAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return entErrToBizErr(err, "EVENT_OUTBOX")
	}
	// FR-9：ON CONFLICT DO NOTHING 命中 0 行即 (session_id, seq) 已被占用——
	// 同 event_id 视为幂等重放成功（崩溃-重试窗口）；否则返回哨兵错误，供
	// insertCriticalOutbox 重取 seq 重试。无水位洞见会让重启后的 counter
	// 静默撞上既有 outbox 行并整批丢帧（INSERT OR IGNORE 静默吞错）。
	if res != nil {
		if n, rerr := res.RowsAffected(); rerr == nil && n == 0 {
			existingEventID := r.lookupEventIDAtSeq(ctx, sessionID, row.Seq)
			if existingEventID != "" && existingEventID == eventID {
				return nil
			}
			return fmt.Errorf("%w: session=%s seq=%d", biz.ErrEventOutboxSeqConflict, sessionID, row.Seq)
		}
	}
	return nil
}

// lookupEventIDAtSeq reads the occupying row's event_id ("" when absent) so
// the conflict path can distinguish same-event idempotent retries from true
// watermark collisions.
func (r *eventDeliveryOutboxRepo) lookupEventIDAtSeq(ctx context.Context, sessionID string, seq int64) string {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT event_id FROM event_delivery_outbox WHERE session_id = ? AND seq = ?`),
		sessionID, seq,
	)
	if err != nil {
		return ""
	}
	defer rows.Close()
	if rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			return id
		}
	}
	return ""
}

// MaxSeqBySession implements biz.EventOutboxWatermarkReader (FR-9): the
// per-session persisted delivery-seq high-watermark used to re-base the shared
// SeqAssigner after restart.
func (r *eventDeliveryOutboxRepo) MaxSeqBySession(ctx context.Context, sessionID string) (int64, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return 0, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, nil
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT COALESCE(MAX(seq), 0) FROM event_delivery_outbox WHERE session_id = ?`),
		sessionID,
	)
	if err != nil {
		return 0, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	defer rows.Close()
	if rows.Next() {
		var maxSeq int64
		if err := rows.Scan(&maxSeq); err != nil {
			return 0, entErrToBizErr(err, "EVENT_OUTBOX")
		}
		return maxSeq, nil
	}
	return 0, nil
}

// InsertAllocatingSeq implements biz.EventOutboxSeqAllocatingInserter (D7-1):
// the seq allocation (session_seq_counters UPSERT ... RETURNING) and the
// outbox INSERT run in ONE transaction. Concurrent replicas serialize on the
// counter row lock, so allocation order == commit order == visibility order —
// a last_event_id strict-cursor replay never sees seq N+1 without N.
//
// A (session_id, seq) INSERT conflict here can only come from a legacy
// Go-counter replica in the mixed rolling-version window (steady-state pg
// peers all allocate through the counter row): repair-bump the counter past
// the durable watermark and retry, bounded by maxOutboxSeqAllocAttempts.
func (r *eventDeliveryOutboxRepo) InsertAllocatingSeq(ctx context.Context, sessionID string, build func(seq int64) (biz.EventDeliveryOutboxRow, error)) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || build == nil {
		return nil
	}
	var err error
	for attempt := 0; attempt < maxOutboxSeqAllocAttempts; attempt++ {
		var conflict bool
		conflict, err = r.insertAllocatingSeqOnce(ctx, sessionID, build)
		if err == nil {
			return nil
		}
		if !conflict {
			return err
		}
		// 冲突=混版本窗口 Go 计数器副本占了该 seq：把计数器顶到持久水位
		// 之上再重试（幂等语句，并发安全——GREATEST 单调不回退）。
		if rerr := r.bumpSessionSeqCounterPastWatermark(ctx, sessionID); rerr != nil {
			return rerr
		}
	}
	return err
}

// insertAllocatingSeqOnce runs one allocate+build+insert transaction.
// conflict=true marks a true watermark collision (different event_id holds
// the allocated seq); the caller repair-bumps and retries.
func (r *eventDeliveryOutboxRepo) insertAllocatingSeqOnce(ctx context.Context, sessionID string, build func(seq int64) (biz.EventDeliveryOutboxRow, error)) (conflict bool, err error) {
	db := r.data.RWDB().WriteHandle()
	if db == nil {
		return false, apierror.Internal("EVENT_OUTBOX", "repository unavailable")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	defer func() { _ = tx.Rollback() }()

	// 首分配种子 MAX(seq)+2（off→pg 翻档时存量 Go 计数器行不被撞号）；
	// 并发首分配在计数器行锁上串行——阻塞方醒后走 DO UPDATE 分支取
	// 已提交值 +1，种子子查询只在行缺席时求值一次。
	allocQ := r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO session_seq_counters (session_id, next_seq)
		 VALUES (?, (SELECT COALESCE(MAX(seq), 0) + 2 FROM event_delivery_outbox WHERE session_id = ?))
		 ON CONFLICT (session_id) DO UPDATE
		   SET next_seq = session_seq_counters.next_seq + 1
		 RETURNING next_seq - 1`,
	)
	rows, qerr := tx.QueryContext(ctx, allocQ, sessionID, sessionID)
	if qerr != nil {
		return false, entErrToBizErr(qerr, "EVENT_OUTBOX")
	}
	var seq int64
	if rows.Next() {
		if scanErr := rows.Scan(&seq); scanErr != nil {
			rows.Close()
			return false, entErrToBizErr(scanErr, "EVENT_OUTBOX")
		}
	}
	if closeErr := rows.Close(); closeErr != nil {
		return false, entErrToBizErr(closeErr, "EVENT_OUTBOX")
	}
	if seq <= 0 {
		return false, apierror.Internal("EVENT_OUTBOX", "seq allocator returned no row")
	}

	// build 在事务内执行：payload 内嵌 event_id 内嵌 seq，拿到权威 seq
	// 才能 marshal；build 失败整事务回滚（计数器不消耗、行不落）。
	row, berr := build(seq)
	if berr != nil {
		return false, berr
	}
	// 分配器拥有 seq/session 寻址——build 产出行不得偏离（防调用方
	// 把行建到别的寻址空间造成计数器与行分叉）。
	if strings.TrimSpace(row.SessionID) != sessionID || row.Seq != seq || strings.TrimSpace(row.EventID) == "" {
		return false, apierror.Internal("EVENT_OUTBOX", "allocator build returned row off the allocated address")
	}
	id := strings.TrimSpace(row.ID)
	if id == "" {
		id = uuid.NewString()
	}
	createdAt := row.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	var published any
	if row.PublishedAt != nil && !row.PublishedAt.IsZero() {
		published = row.PublishedAt.UTC().Format(time.RFC3339Nano)
	}
	d := r.data.Dialect()
	insertQ := d.BuildInsertOrIgnore(
		"event_delivery_outbox",
		"id, session_id, seq, event_id, kind, entity_id, payload, published_at, created_at",
		d.Placeholders(9),
		"session_id, seq",
	)
	res, ierr := tx.ExecContext(ctx, insertQ,
		id, sessionID, seq, strings.TrimSpace(row.EventID), row.Kind, row.EntityID, row.Payload, published,
		createdAt.UTC().Format(time.RFC3339Nano),
	)
	if ierr != nil {
		return false, entErrToBizErr(ierr, "EVENT_OUTBOX")
	}
	if res != nil {
		if n, rerr := res.RowsAffected(); rerr == nil && n == 0 {
			// 占用者同 event_id = 幂等重放成功（Insert 同律）；否则真水位
			// 冲突——回滚释放计数器增量，交外层 repair-bump 重试。
			var existing string
			erows, lerr := tx.QueryContext(ctx,
				d.RenumberPlaceholders(`SELECT event_id FROM event_delivery_outbox WHERE session_id = ? AND seq = ?`),
				sessionID, seq)
			if lerr == nil {
				if erows.Next() {
					_ = erows.Scan(&existing)
				}
				_ = erows.Close()
			}
			if existing != "" && existing == strings.TrimSpace(row.EventID) {
				if cerr := tx.Commit(); cerr != nil {
					return false, entErrToBizErr(cerr, "EVENT_OUTBOX")
				}
				return false, nil
			}
			return true, fmt.Errorf("%w: session=%s seq=%d", biz.ErrEventOutboxSeqConflict, sessionID, seq)
		}
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if cerr := tx.Commit(); cerr != nil {
		return false, entErrToBizErr(cerr, "EVENT_OUTBOX")
	}
	return false, nil
}

// bumpSessionSeqCounterPastWatermark repair-bumps the counter row to at least
// MAX(seq)+2 (GREATEST keeps it monotone — a concurrent allocator's higher
// value is never regressed). Idempotent; runs outside the allocating tx so
// the repair survives that tx's rollback.
func (r *eventDeliveryOutboxRepo) bumpSessionSeqCounterPastWatermark(ctx context.Context, sessionID string) error {
	q := r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO session_seq_counters (session_id, next_seq)
		 VALUES (?, (SELECT COALESCE(MAX(seq), 0) + 2 FROM event_delivery_outbox WHERE session_id = ?))
		 ON CONFLICT (session_id) DO UPDATE
		   SET next_seq = GREATEST(session_seq_counters.next_seq,
		         (SELECT COALESCE(MAX(seq), 0) + 2 FROM event_delivery_outbox WHERE session_id = ?))`,
	)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, sessionID, sessionID, sessionID)
	if err != nil {
		return entErrToBizErr(err, "EVENT_OUTBOX")
	}
	return nil
}

func (r *eventDeliveryOutboxRepo) MarkPublished(ctx context.Context, id string, publishedAt time.Time) error {
	return r.MarkPublishedBatch(ctx, []string{id}, publishedAt)
}

func (r *eventDeliveryOutboxRepo) MarkPublishedBatch(ctx context.Context, ids []string, publishedAt time.Time) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil
	}
	dedup := make(map[string]struct{}, len(ids))
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := dedup[id]; ok {
			continue
		}
		dedup[id] = struct{}{}
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return nil
	}
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}
	// published_at is timestamptz on Postgres (raw-DDL column) — comparing it
	// to '' raises 22007 (invalid input syntax), so only NULL is tested.
	unpublishedCond := "published_at IS NULL"
	placeholders := make([]string, len(clean))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`UPDATE event_delivery_outbox SET published_at=? WHERE id IN (` + strings.Join(placeholders, ",") + `) AND ` + unpublishedCond,
	)
	args := make([]any, 0, len(clean)+1)
	args = append(args, publishedAt.UTC().Format(time.RFC3339Nano))
	for _, id := range clean {
		args = append(args, id)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, args...)
	if err != nil {
		return entErrToBizErr(err, "EVENT_OUTBOX")
	}
	return nil
}

func (r *eventDeliveryOutboxRepo) ListAfter(ctx context.Context, sessionID, afterEventID string, afterSeq int64, limit int) ([]biz.EventDeliveryOutboxRow, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultEventOutboxListLimit
	}
	d := r.data.Dialect()
	cursorSeq := afterSeq
	afterEventID = strings.TrimSpace(afterEventID)
	if afterEventID != "" {
		var seq int64
		q := d.RenumberPlaceholders(
			`SELECT seq FROM event_delivery_outbox WHERE session_id=? AND event_id=? LIMIT 1`,
		)
		err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), q, []any{sessionID, afterEventID}, &seq)
		if err != nil {
			if apierror.IsCode(err, apierror.CodeNotFound) {
				return nil, nil
			}
			return nil, entErrToBizErr(err, "EVENT_OUTBOX")
		}
		cursorSeq = seq
	}
	if cursorSeq < 0 {
		cursorSeq = 0
	}
	// E2E-C2: a stale reconnect cursor (client transport re-uses the
	// establishment-time last_event_id) can sit far behind the current tail.
	// Plain `ORDER BY seq ASC LIMIT ?` would then replay the OLDEST 100 rows
	// after the cursor — ancient frames the client no longer cares about —
	// while the truly-missed LATEST critical frames (e.g. team_summary, which
	// REST never carries) fall outside the window and are lost permanently.
	// Fetch the NEWEST `limit` rows after the cursor (inner DESC), then
	// re-order ascending (outer) so on-the-wire replay order stays causal.
	q := d.RenumberPlaceholders(
		`SELECT id, session_id, seq, event_id, kind, entity_id, payload, published_at, created_at
		 FROM (
			SELECT id, session_id, seq, event_id, kind, entity_id, payload, published_at, created_at
			FROM event_delivery_outbox
			WHERE session_id=? AND seq>?
			ORDER BY seq DESC
			LIMIT ?
		 ) AS tail
		 ORDER BY seq ASC`,
	)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, sessionID, cursorSeq, limit)
	if err != nil {
		return nil, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	defer rows.Close()
	out, err := scanEventOutboxRows(rows, limit)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *eventDeliveryOutboxRepo) ListPending(ctx context.Context, cutoff time.Time, limit int) ([]biz.EventDeliveryOutboxRow, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultEventOutboxPendingLimit
	}
	d := r.data.Dialect()
	// See MarkPublished: published_at is timestamptz on Postgres, so only
	// NULL is tested for "unpublished".
	unpublishedCond := "published_at IS NULL"
	args := make([]any, 0, 3)
	createdCond := ""
	if !cutoff.IsZero() {
		// created_at is written as UTC RFC3339Nano TEXT; a same-format string
		// compare coerces cleanly to timestamptz on PG.
		createdCond = " AND created_at < ?"
		args = append(args, cutoff.UTC().Format(time.RFC3339Nano))
	}
	args = append(args, limit)
	q := d.RenumberPlaceholders(
		`SELECT id, session_id, seq, event_id, kind, entity_id, payload, published_at, created_at
		 FROM event_delivery_outbox
		 WHERE ` + unpublishedCond + createdCond + `
		 ORDER BY created_at ASC
		 LIMIT ?`,
	)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	defer rows.Close()
	out, err := scanEventOutboxRows(rows, limit)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ClaimPendingOutboxDeliveries implements biz.EventOutboxClaimer (D7-2): one
// CTE statement picks claimable rows FOR UPDATE SKIP LOCKED and stamps this
// worker's claim in the same UPDATE (channel_delivery_claim.go 先例).
// Multi-replica sweepers partition the backlog instead of double-broadcasting
// it; a crashed worker's claims become reclaimable once claimed_at falls
// behind claimLeaseDeadline (sweeper passes now-lease, 缺省 60s).
//
// picked 行已被 FOR UPDATE 锁定至提交，UPDATE 不再重校验认领谓词
// （channel claim 的重校验是其 status CAS 语义遗留，此处不可达）。
// claimed_at 用定宽布局（字典序==时序，board_run_leases 同律）；created_at
// 截止比较沿用 ListPending 的 RFC3339Nano 口径（既有表行为不变）。
func (r *eventDeliveryOutboxRepo) ClaimPendingOutboxDeliveries(ctx context.Context, workerID string, cutoff, claimLeaseDeadline time.Time, limit int) ([]biz.EventDeliveryOutboxRow, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > maxEventOutboxClaimLimit {
		limit = defaultEventOutboxClaimLimit
	}
	db := r.data.RWDB().WriteHandle()
	if db == nil {
		return nil, apierror.Internal("EVENT_OUTBOX", "repository unavailable")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	defer func() { _ = tx.Rollback() }()

	cutoffStr := cutoff.UTC().Format(time.RFC3339Nano)
	leaseStr := formatBoardRunLeaseTime(claimLeaseDeadline.UTC())
	claimedAt := formatBoardRunLeaseTime(time.Now().UTC())
	// published_at 是 timestamptz（raw-DDL 列）——只测 NULL（MarkPublished 同律）。
	q := r.data.Dialect().RenumberPlaceholders(
		`WITH picked AS (
		   SELECT id FROM event_delivery_outbox
		   WHERE published_at IS NULL
		     AND created_at < ?
		     AND (claimed_by = '' OR claimed_at < ?)
		   ORDER BY created_at ASC
		   LIMIT ?
		   FOR UPDATE SKIP LOCKED
		 )
		 UPDATE event_delivery_outbox AS o
		 SET claimed_by = ?, claimed_at = ?
		 FROM picked
		 WHERE o.id = picked.id
		 RETURNING o.id, o.session_id, o.seq, o.event_id, o.kind, o.entity_id, o.payload, o.published_at, o.created_at`,
	)
	rows, err := tx.QueryContext(ctx, q, cutoffStr, leaseStr, limit, workerID, claimedAt)
	if err != nil {
		return nil, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	out, err := scanEventOutboxRows(rows, limit)
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := tx.Commit(); err != nil {
		return nil, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	return out, nil
}

// scanEventOutboxRows decodes rows produced by the shared outbox SELECT
// column list. Timestamps are stored as RFC3339(Nano) strings (timestamptz on
// Postgres returns the same layout), parsed leniently — an unparseable value
// leaves the field zero/nil rather than failing the whole batch.
func scanEventOutboxRows(rows *sql.Rows, capHint int) ([]biz.EventDeliveryOutboxRow, error) {
	out := make([]biz.EventDeliveryOutboxRow, 0, capHint)
	for rows.Next() {
		var row biz.EventDeliveryOutboxRow
		var published, created sql.NullString
		if err := rows.Scan(
			&row.ID, &row.SessionID, &row.Seq, &row.EventID, &row.Kind, &row.EntityID,
			&row.Payload, &published, &created,
		); err != nil {
			return nil, entErrToBizErr(err, "EVENT_OUTBOX")
		}
		if published.Valid && published.String != "" {
			if t, err := time.Parse(time.RFC3339Nano, published.String); err == nil {
				row.PublishedAt = &t
			} else if t, err := time.Parse(time.RFC3339, published.String); err == nil {
				row.PublishedAt = &t
			}
		}
		if created.Valid && created.String != "" {
			if t, err := time.Parse(time.RFC3339Nano, created.String); err == nil {
				row.CreatedAt = t
			} else if t, err := time.Parse(time.RFC3339, created.String); err == nil {
				row.CreatedAt = t
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	return out, nil
}
