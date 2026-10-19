package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
)

// l3FactRepo implements biz L3 interfaces using direct Raw SQL.
type l3FactRepo struct {
	data        *Data
	vectorStore vector.VectorStore
	// bruteForceThreshold overrides biz.DefaultFactBruteForceThreshold when
	// > 0. Tests set it to force the non-brute-force recall paths (recency
	// pool / pgvector+FTS fusion) with small fixtures.
	bruteForceThreshold int
}

// Compile-time interface checks.
var (
	_ biz.L3FactReader               = (*l3FactRepo)(nil)
	_ biz.L3FactWriter               = (*l3FactRepo)(nil)
	_ biz.L3FactReviewStore          = (*l3FactRepo)(nil)
	_ biz.L3ConflictStore            = (*l3FactRepo)(nil)
	_ biz.PIIReviewStore             = (*l3FactRepo)(nil)
	_ biz.DecayScoreWriter           = (*l3FactRepo)(nil)
	_ biz.MemoryPreferenceLister     = (*l3FactRepo)(nil)
	_ biz.FactAccessCounter          = (*l3FactRepo)(nil)
	_ biz.MemoryFactInjectCounter    = (*l3FactRepo)(nil)
	_ biz.MemoryFactCitationRecorder = (*l3FactRepo)(nil)
)

func newL3FactRepo(data *Data, vs vector.VectorStore) *l3FactRepo {
	if data == nil {
		return nil
	}
	return &l3FactRepo{data: data, vectorStore: vs}
}

// NewL3FactWriterAdapter creates a biz.L3FactWriter backed by data.
func NewL3FactWriterAdapter(data *Data, vs vector.VectorStore) biz.L3FactWriter {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, vs)
}

// NewL3FactReaderForUser creates a biz.L3FactReader backed by data.
func NewL3FactReaderForUser(data *Data) biz.L3FactReader {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewL3ConflictStore creates a biz.L3ConflictStore backed by data.
// Used by the chat turn path for conflict governance (supersede/mark).
func NewL3ConflictStore(data *Data) biz.L3ConflictStore {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewL3FactAccessCounter creates a biz.FactAccessCounter backed by data.
// Used by the unified fact write pipeline (P1-3) for dedup-merge access bumps.
func NewL3FactAccessCounter(data *Data) biz.FactAccessCounter {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewL3FactInjectCounter creates a biz.MemoryFactInjectCounter backed by
// data (FR-12.6). Used by the before-model memory inject hook for the
// once-per-turn injected_count bump.
func NewL3FactInjectCounter(data *Data) biz.MemoryFactInjectCounter {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewL3FactCitationRecorder creates a biz.MemoryFactCitationRecorder backed
// by data (FR-12.6). Used by the citation backfill worker.
func NewL3FactCitationRecorder(data *Data) biz.MemoryFactCitationRecorder {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewMemoryPreferenceLister creates a biz.MemoryPreferenceLister backed by
// data. Used by the agent layer for pinned preference injection (FR-M3).
func NewMemoryPreferenceLister(data *Data) biz.MemoryPreferenceLister {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewDecayScoreWriter creates a biz.DecayScoreWriter backed by data.
// Used by the Ebbinghaus decay cron job to persist R_t scores.
func NewDecayScoreWriter(data *Data) biz.DecayScoreWriter {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// factConsistencyAdapter implements internal/memory/trpc.factConsistencyChecker via raw SQL.
// Note: no compile-time var _ check because data cannot import memory/trpc (reverse dependency).
// Wire binding in cmd/admin/wire_memory.go ensures type compatibility at injection time.
type factConsistencyAdapter struct {
	data *Data
}

// NewFactConsistencyAdapter creates a factConsistencyChecker backed by data.
func NewFactConsistencyAdapter(data *Data) *factConsistencyAdapter {
	if data == nil {
		return nil
	}
	return &factConsistencyAdapter{data: data}
}

func (a *factConsistencyAdapter) GetFactConsistencyRow(ctx context.Context, factID string) (status, indexStatus, statement string, err error) {
	err = QueryRowScan(ctx, a.data.RWDB().ReadDB(ctx),
		a.data.Dialect().RenumberPlaceholders(`SELECT status, embedding_status, statement FROM memory_facts WHERE id = ?`),
		[]any{factID}, &status, &indexStatus, &statement)
	return
}

func (a *factConsistencyAdapter) GetFactResyncRow(ctx context.Context, factID string) (agentID, userID, statement string, err error) {
	err = QueryRowScan(ctx, a.data.RWDB().ReadDB(ctx),
		a.data.Dialect().RenumberPlaceholders(`SELECT COALESCE(agent_id, scope_id), user_id, statement FROM memory_facts WHERE id = ?`),
		[]any{factID}, &agentID, &userID, &statement)
	return
}

func (r *l3FactRepo) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword, agentID string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	clauses, args := buildFactFilterClauses(scopeType, scopeID, kind, status, keyword, agentID, true)
	where := " WHERE " + strings.Join(clauses, " AND ")

	countClauses, countArgs := buildFactFilterClauses(scopeType, scopeID, kind, "", keyword, agentID, false)
	countWhere := ""
	if len(countClauses) > 0 {
		countWhere = " WHERE " + strings.Join(countClauses, " AND ")
	}
	var total, active, archived int32
	countRow, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(*) as total, SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) as active, SUM(CASE WHEN status = 'archived' THEN 1 ELSE 0 END) as archived FROM memory_facts`+countWhere),
		countArgs...)
	if err != nil {
		return nil, 0, 0, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	if countRow.Next() {
		var a, ar sql.NullInt32
		if scanErr := countRow.Scan(&total, &a, &ar); scanErr != nil {
			countRow.Close()
			return nil, 0, 0, 0, entErrToBizErr(scanErr, "MEMORY_L3")
		}
		if a.Valid {
			active = a.Int32
		} else {
			active = total
		}
		if ar.Valid {
			archived = ar.Int32
		}
	}
	countRow.Close()

	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, 0, 0, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, 0, 0, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, total, active, archived, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

// CountFactRows returns the count matching the rows-caliber filter (same WHERE
// as ListFactRows items, including the status filter with default 'active').
// ListFactRows' total deliberately ignores status for the stats-row breakdown,
// so the memory center uses this as the server-side pagination total.
func (r *l3FactRepo) CountFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword, agentID string) (int32, error) {
	clauses, args := buildFactFilterClauses(scopeType, scopeID, kind, status, keyword, agentID, true)
	return r.countFacts(ctx, clauses, args...)
}

// AggregateFactOverview returns L3 panorama card stats in one SQL pass
// (active count, created-today count, summed use_count) for an originating agent.
func (r *l3FactRepo) AggregateFactOverview(ctx context.Context, agentID, todayStart string) (biz.FactOverviewAgg, error) {
	var agg biz.FactOverviewAgg
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return agg, nil
	}
	todayStart = strings.TrimSpace(todayStart)
	q := `SELECT
		COALESCE(SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(use_count), 0)
		FROM memory_facts WHERE agent_id = ? AND deleted_at = ''`
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders(q), []any{todayStart, agentID},
		&agg.ActiveCount, &agg.TodayAdded, &agg.RecallHits)
	return agg, entErrToBizErr(err, "MEMORY_L3")
}

// ListRecentFactActivityRows returns the newest facts by created_at for the
// panorama activity feed (fact_extracted).
func (r *l3FactRepo) ListRecentFactActivityRows(ctx context.Context, agentID string, limit int32) ([][]byte, error) {
	return r.listFactOverviewRows(ctx, agentID, "", "created_at DESC", limit)
}

// ListRecentlyInjectedFactRows returns facts recently injected into L0
// (injected_count > 0, last_used_at set) for the panorama activity feed.
func (r *l3FactRepo) ListRecentlyInjectedFactRows(ctx context.Context, agentID string, limit int32) ([][]byte, error) {
	return r.listFactOverviewRows(ctx, agentID, "injected_count > 0 AND last_used_at <> ''", "last_used_at DESC", limit)
}

func (r *l3FactRepo) listFactOverviewRows(ctx context.Context, agentID, extra, orderBy string, limit int32) ([][]byte, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, nil
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	clauses := []string{"agent_id = ?", "deleted_at = ''"}
	args := []any{agentID}
	if extra != "" {
		clauses = append(clauses, extra)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	q := sqlFactSelect + where + " ORDER BY " + orderBy + " LIMIT ?"
	args = append(args, lim)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

// buildFactFilterClauses constructs WHERE clause components for fact queries.
// When withStatusFilter is true, the status parameter is applied; otherwise
// only scope/kind/keyword/deleted_at filters are included (for total counts).
// agentID filters by the originating-agent column (memory_facts.agent_id)
// independently of the scope namespace — see biz.L3FactReader.
func buildFactFilterClauses(scopeType, scopeID, kind, status, keyword, agentID string, withStatusFilter bool) ([]string, []any) {
	clauses := []string{}
	args := []any{}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	if agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if kind != "" {
		clauses = append(clauses, "fact_kind = ?")
		args = append(args, kind)
	}
	if withStatusFilter {
		if status != "" {
			clauses = append(clauses, "status = ?")
			args = append(args, status)
		} else {
			clauses = append(clauses, "status = 'active'")
		}
	}
	if keyword != "" {
		clauses = append(clauses, "(statement_normalized LIKE ? OR id = ?)")
		args = append(args, "%"+strings.ToLower(keyword)+"%", keyword)
	}
	clauses = append(clauses, "deleted_at = ''")
	return clauses, args
}

// countFacts returns the count of facts matching the given filter clauses.
func (r *l3FactRepo) countFacts(ctx context.Context, clauses []string, args ...any) (int32, error) {
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	var count int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders("SELECT COUNT(*) FROM memory_facts"+where), args, &count); err != nil {
		return 0, entErrToBizErr(err, "MEMORY_L3")
	}
	return count, nil
}

func (r *l3FactRepo) ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {

	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''"}
	args := []any{}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	if userID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, userID)
	}
	if keyword != "" {
		clauses = append(clauses, "(statement_normalized LIKE ? OR id = ?)")
		args = append(args, "%"+strings.ToLower(keyword)+"%", keyword)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

// ListFactRowsForUserAll returns facts for a user including invalidated ones
// (valid_until != ”). Used for historical reconstruction queries when
// SearchOptions.IncludeInvalidated is true. Deleted facts are still excluded.
func (r *l3FactRepo) ListFactRowsForUserAll(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	clauses := []string{"status = 'active'", "deleted_at = ''"}
	args := []any{}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	if userID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, userID)
	}
	if keyword != "" {
		clauses = append(clauses, "(statement_normalized LIKE ? OR id = ?)")
		args = append(args, "%"+strings.ToLower(keyword)+"%", keyword)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

func (r *l3FactRepo) GetFactRowsByIDs(ctx context.Context, factIDs []string) ([][]byte, error) {
	if len(factIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(factIDs))
	args := make([]any, len(factIDs))
	for i, id := range factIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := sqlFactSelect + " WHERE id IN (" + strings.Join(placeholders, ",") + ") AND status = 'active' AND deleted_at = '' AND valid_until = ''"
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
}
