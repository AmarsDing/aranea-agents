package sessionmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

type Store struct {
	client *ent.Client
	policy *biz.MemoryPolicyEngine
	lg     loggateway.Logger
}

func NewStore(client *ent.Client, lg loggateway.Logger) *Store {
	if client == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &Store{client: client, lg: lg}
}

// Client returns the shared Ent client (SQLite).
func (st *Store) Client() *ent.Client {
	if st == nil {
		return nil
	}
	return st.client
}

func queryOne(ctx context.Context, client *ent.Client, query string, args []any, dest ...any) error {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return sql.ErrNoRows
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	return rows.Err()
}

const sqlL0Select = `SELECT id, session_id, run_id, turn_id, span_id, agent_id, team_id, provider, model,
 context_window_tokens, budget_tokens, recent_window_turns, recent_window_tokens, summary_token_estimate,
 l1_field_count, l1_token_estimate, l3_chunk_count, l3_token_estimate, l4_path_count, l4_token_estimate,
 prompt_token_estimate, prompt_token_actual, used_ratio, truncate_strategy, truncated_message_count,
 summarized_turn_from, summarized_turn_to, segments_json, warning_codes_json, metadata_json, created_at
 FROM memory_l0_assembly_snapshots`

// ListL0SnapshotRows returns one JSON object per row (**snake_case**), compatible with **memory_decode.pbL0AssemblySnapshot**.
func (st *Store) ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	lim := int(limit)
	if lim <= 0 || lim > 100 {
		lim = 20
	}
	rows, err := st.client.QueryContext(ctx, sqlL0Select+` WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`, sessionID, lim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var (
			id, sessID, runID, turnID, spanID, agentID, teamID, provider, model string
			cwt, bt, rwt, rwtok, ste, l1fc, l1te, l3c, l3te, l4p, l4te          int
			pte, pta                                                            int
			ur                                                                  float64
			ts                                                                  string
			tmc, stf, ste2                                                      int
			segs, warns, meta, cat                                              string
		)
		if err := rows.Scan(
			&id, &sessID, &runID, &turnID, &spanID, &agentID, &teamID, &provider, &model,
			&cwt, &bt, &rwt, &rwtok, &ste, &l1fc, &l1te, &l3c, &l3te, &l4p, &l4te,
			&pte, &pta, &ur, &ts, &tmc, &stf, &ste2,
			&segs, &warns, &meta, &cat,
		); err != nil {
			return nil, err
		}
		m := map[string]any{
			"id": id, "session_id": sessID, "run_id": runID, "turn_id": turnID, "span_id": spanID,
			"agent_id": agentID, "team_id": teamID, "provider": provider, "model": model,
			"context_window_tokens": cwt, "budget_tokens": bt,
			"recent_window_turns": rwt, "recent_window_tokens": rwtok,
			"summary_token_estimate": ste,
			"l1_field_count":         l1fc, "l1_token_estimate": l1te,
			"l3_chunk_count": l3c, "l3_token_estimate": l3te,
			"l4_path_count": l4p, "l4_token_estimate": l4te,
			"prompt_token_estimate": pte, "prompt_token_actual": pta,
			"used_ratio": ur, "truncate_strategy": ts,
			"truncated_message_count": tmc, "summarized_turn_from": stf, "summarized_turn_to": ste2,
			"segments_json": segs, "warning_codes_json": warns, "metadata_json": meta,
			"created_at": cat,
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

const sqlL1Task = `SELECT id, session_id, run_id, team_id, agent_id,
 task_key, task_title, task_goal, status,
 schema_version, budget_tokens, used_tokens,
 parent_task_id, shared_with_json,
 started_at, ended_at, archived_at,
 metadata_json, created_at, updated_at FROM memory_l1_tasks`

// ListL1TaskRows lists tasks for a session (filters mirror legacy **ListL1TasksBySession**).
func (st *Store) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	clauses := []string{"session_id = ?"}
	args := []any{sessionID}
	if agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	} else if strings.TrimSpace(includeEnded) != "true" {
		clauses = append(clauses, "status IN ('active','paused')")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	q := sqlL1Task + where + ` ORDER BY updated_at DESC`
	rows, err := st.client.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var (
			id, sid, rid, tid, aid, tkey, title, goal, st string
			sv, bt, ut                                    int
			parent, shared, meta, sa, ea, aa, ca, ua      string
		)
		if err := rows.Scan(
			&id, &sid, &rid, &tid, &aid, &tkey, &title, &goal, &st,
			&sv, &bt, &ut, &parent, &shared, &sa, &ea, &aa, &meta, &ca, &ua,
		); err != nil {
			return nil, err
		}
		m := map[string]any{
			"id": id, "session_id": sid, "run_id": rid, "team_id": tid, "agent_id": aid,
			"task_key": tkey, "task_title": title, "task_goal": goal, "status": st,
			"schema_version": sv, "budget_tokens": bt, "used_tokens": ut,
			"parent_task_id": parent, "shared_with_json": shared,
			"started_at": sa, "ended_at": ea, "archived_at": aa,
			"metadata_json": meta, "created_at": ca, "updated_at": ua,
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

const sqlL1Field = `SELECT id, task_id, session_id, agent_id,
 field_path, field_kind, visibility, pin_to_prompt, is_required,
 value_text, value_json, value_ref, preview, token_estimate,
 source, source_ref, ttl_seconds, expires_at,
 revision, last_read_at, read_count,
 metadata_json, created_at, updated_at FROM memory_l1_fields`

// ListL1FieldRows lists fields for a task.
func (st *Store) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error) {
	if taskID == "" {
		return nil, errors.New("task id is required")
	}
	q := sqlL1Field + ` WHERE task_id = ?`
	if !includeInternal {
		q += ` AND visibility != 'internal'`
	}
	q += ` ORDER BY field_path ASC`
	rows, err := st.client.QueryContext(ctx, q, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var (
			id, tid, sid, aid, fpath, fkind, vis string
			pin, req                             int
			vt, vj, vr, prev, src, sref, exp     string
			tok, ttl, rev, rc                    int
			lra                                  string
			meta, ca, ua                         string
		)
		if err := rows.Scan(
			&id, &tid, &sid, &aid, &fpath, &fkind, &vis, &pin, &req,
			&vt, &vj, &vr, &prev, &tok, &src, &sref, &ttl, &exp,
			&rev, &lra, &rc, &meta, &ca, &ua,
		); err != nil {
			return nil, err
		}
		m := map[string]any{
			"id": id, "task_id": tid, "session_id": sid, "agent_id": aid,
			"field_path": fpath, "field_kind": fkind, "visibility": vis,
			"pin_to_prompt": pin != 0, "is_required": req != 0,
			"value_text": vt, "value_json": vj, "value_ref": vr, "preview": prev,
			"token_estimate": tok, "source": src, "source_ref": sref,
			"ttl_seconds": ttl, "expires_at": exp,
			"revision": rev, "last_read_at": lra, "read_count": rc,
			"metadata_json": meta, "created_at": ca, "updated_at": ua,
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

const sqlFactSelect = `SELECT id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
 statement, statement_normalized, fingerprint, details_markdown,
 fact_kind, tags_json,
 confidence, importance, use_count, hit_count,
 positive_feedback_count, negative_feedback_count, conflict_count,
 source_kind, source_episode_id, source_session_id, source_message_id, source_external,
 version, status, superseded_by,
 embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
 pii_flag, redacted_statement,
 ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
 metadata_json, created_at, updated_at, archived_at, deleted_at
 FROM memory_facts`

// ListFactRows lists facts with filters compatible with legacy **ListFacts**.
func (st *Store) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	w := []string{"deleted_at = ''"}
	args := []any{}
	if scopeType != "" {
		w = append(w, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		w = append(w, "scope_id = ?")
		args = append(args, scopeID)
	}
	if status != "" {
		w = append(w, "status = ?")
		args = append(args, status)
	}
	if kind != "" {
		w = append(w, "fact_kind = ?")
		args = append(args, kind)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		w = append(w, "(LOWER(statement) LIKE ? OR LOWER(details_markdown) LIKE ?)")
		like := "%" + strings.ToLower(kw) + "%"
		args = append(args, like, like)
	}
	whereSQL := strings.Join(w, " AND ")
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	if lim > 200 {
		lim = 200
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	var total int
	countQ := `SELECT COUNT(1) FROM memory_facts WHERE ` + whereSQL
	if err := queryOne(ctx, st.client, countQ, args, &total); err != nil {
		return nil, 0, int32(lim), int32(off), err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, lim, off)
	q := sqlFactSelect + ` WHERE ` + whereSQL + ` ORDER BY importance DESC, updated_at DESC LIMIT ? OFFSET ?`
	rows, err := st.client.QueryContext(ctx, q, listArgs...)
	if err != nil {
		return nil, int32(total), int32(lim), int32(off), err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, int32(total), int32(lim), int32(off), err
		}
		out = append(out, b)
	}
	return out, int32(total), int32(lim), int32(off), rows.Err()
}

func scanFactRowJSON(rows *sql.Rows) ([]byte, error) {
	var (
		id, stype, sid, wid, uid, tid, aid string
		stmt, snorm, fp, details           string
		fkind, tags                        string
		conf, imp                          float64
		uc, hc, pfc, nfc, cc               int
		srcKind, epID, sessID, msgID, ext  string
		ver                                int
		st, sup                            string
		embSt, embModel                    string
		embDim                             int
		embBlob                            []byte
		embNorm                            float64
		pii                                int
		redacted                           string
		ttlD                               int
		decay                              float64
		nextD, lastU, exp                  string
		meta, ca, ua, arch, del            string
	)
	if err := rows.Scan(
		&id, &stype, &sid, &wid, &uid, &tid, &aid,
		&stmt, &snorm, &fp, &details,
		&fkind, &tags,
		&conf, &imp, &uc, &hc, &pfc, &nfc, &cc,
		&srcKind, &epID, &sessID, &msgID, &ext,
		&ver, &st, &sup,
		&embSt, &embModel, &embDim, &embBlob, &embNorm,
		&pii, &redacted,
		&ttlD, &decay, &nextD, &lastU, &exp,
		&meta, &ca, &ua, &arch, &del,
	); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "scope_type": stype, "scope_id": sid, "workspace_id": wid,
		"user_id": uid, "team_id": tid, "agent_id": aid,
		"statement": stmt, "statement_normalized": snorm, "fingerprint": fp,
		"details_markdown": details, "fact_kind": fkind, "tags_json": tags,
		"confidence": conf, "importance": imp,
		"use_count": uc, "hit_count": hc,
		"positive_feedback_count": pfc, "negative_feedback_count": nfc, "conflict_count": cc,
		"source_kind": srcKind, "source_episode_id": epID,
		"source_session_id": sessID, "source_message_id": msgID, "source_external": ext,
		"version": ver, "status": st, "superseded_by": sup,
		"embedding_status": embSt, "embedding_model": embModel, "embedding_dim": embDim,
		"embedding_norm":     embNorm,
		"pii_flag":           pii != 0,
		"redacted_statement": redacted,
		"ttl_days":           ttlD, "decay_factor": decay,
		"next_decay_at": nextD, "last_used_at": lastU, "expires_at": exp,
		"metadata_json": meta, "created_at": ca, "updated_at": ua,
		"archived_at": arch, "deleted_at": del,
	}
	return json.Marshal(m)
}
