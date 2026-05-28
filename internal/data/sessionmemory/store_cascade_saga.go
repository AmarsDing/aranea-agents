package sessionmemory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type CascadeSagaStep struct {
	ID          int64  `json:"id"`
	ProposalID  string `json:"proposal_id"`
	StepIndex   int    `json:"step_index"`
	StepName    string `json:"step_name"`
	State       string `json:"state"`
	IsCritical  bool   `json:"is_critical"`
	Attempts    int    `json:"attempts"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	PayloadJSON string `json:"payload_json,omitempty"`
	ResultJSON  string `json:"result_json,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (st *Store) InitCascadeSagaSteps(ctx context.Context, proposalID string, steps []CascadeSagaStep) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	proposalID = strings.TrimSpace(proposalID)
	if proposalID == "" {
		return errors.New("proposal_id is required")
	}
	for i, s := range steps {
		isCritical := 0
		if s.IsCritical {
			isCritical = 1
		}
		payload := strings.TrimSpace(s.PayloadJSON)
		if payload == "" {
			payload = "{}"
		}
		_, err := st.client.ExecContext(ctx, `
INSERT INTO cascade_saga_steps (proposal_id, step_index, step_name, state, is_critical, attempts, payload_json)
VALUES (?, ?, ?, 'pending', ?, 0, ?)`,
			proposalID, i, strings.TrimSpace(s.StepName), isCritical, payload)
		if err != nil {
			return fmt.Errorf("init saga step %d: %w", i, err)
		}
	}
	return nil
}

func (st *Store) GetCascadeSagaSteps(ctx context.Context, proposalID string) ([]CascadeSagaStep, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	proposalID = strings.TrimSpace(proposalID)
	if proposalID == "" {
		return nil, errors.New("proposal_id is required")
	}
	rows, err := st.client.QueryContext(ctx, `
SELECT id, proposal_id, step_index, step_name, state, is_critical, attempts,
       COALESCE(started_at,''), COALESCE(finished_at,''),
       COALESCE(payload_json,''), COALESCE(result_json,''), COALESCE(error,'')
FROM cascade_saga_steps
WHERE proposal_id = ?
ORDER BY step_index`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CascadeSagaStep
	for rows.Next() {
		var s CascadeSagaStep
		var isCrit int
		if err := rows.Scan(&s.ID, &s.ProposalID, &s.StepIndex, &s.StepName, &s.State,
			&isCrit, &s.Attempts, &s.StartedAt, &s.FinishedAt,
			&s.PayloadJSON, &s.ResultJSON, &s.Error); err != nil {
			return nil, err
		}
		s.IsCritical = isCrit == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

func (st *Store) UpdateSagaStepState(ctx context.Context, stepID int64, state, errMsg string) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch state {
	case "running":
		_, err := st.client.ExecContext(ctx, `
UPDATE cascade_saga_steps SET state = 'running', attempts = attempts + 1, started_at = ?
WHERE id = ?`, now, stepID)
		return err
	case "succeeded":
		_, err := st.client.ExecContext(ctx, `
UPDATE cascade_saga_steps SET state = 'succeeded', finished_at = ?
WHERE id = ?`, now, stepID)
		return err
	case "failed":
		_, err := st.client.ExecContext(ctx, `
UPDATE cascade_saga_steps SET state = 'failed', finished_at = ?, error = ?
WHERE id = ?`, now, strings.TrimSpace(errMsg), stepID)
		return err
	case "compensated":
		_, err := st.client.ExecContext(ctx, `
UPDATE cascade_saga_steps SET state = 'compensated', finished_at = ?
WHERE id = ?`, now, stepID)
		return err
	case "skipped":
		_, err := st.client.ExecContext(ctx, `
UPDATE cascade_saga_steps SET state = 'skipped', finished_at = ?
WHERE id = ?`, now, stepID)
		return err
	default:
		return fmt.Errorf("unsupported saga step state: %s", state)
	}
}

func (st *Store) UpdateSagaStepResult(ctx context.Context, stepID int64, resultJSON string) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	_, err := st.client.ExecContext(ctx, `
UPDATE cascade_saga_steps SET result_json = ? WHERE id = ?`,
		strings.TrimSpace(resultJSON), stepID)
	return err
}

func (st *Store) HasCascadeSaga(ctx context.Context, proposalID string) (bool, error) {
	if st == nil || st.client == nil {
		return false, errors.New("session memory store not wired")
	}
	proposalID = strings.TrimSpace(proposalID)
	if proposalID == "" {
		return false, nil
	}
	rows, err := st.client.QueryContext(ctx,
		`SELECT COUNT(*) FROM cascade_saga_steps WHERE proposal_id = ?`, proposalID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, nil
	}
	var cnt int
	if err := rows.Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func (st *Store) SaveCascadeOriginalStatements(ctx context.Context, agentID, oldName string, factIDs []string) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	if len(factIDs) == 0 {
		return nil
	}
	for _, fid := range factIDs {
		rows, err := st.client.QueryContext(ctx,
			`SELECT statement FROM memory_facts WHERE id = ? AND deleted_at = ''`, fid)
		if err != nil {
			continue
		}
		var stmt string
		if rows.Next() {
			if err := rows.Scan(&stmt); err != nil {
				rows.Close()
				continue
			}
		}
		rows.Close()
		if _, err := st.client.ExecContext(ctx, `
UPDATE memory_facts SET last_cascade_original_statement = ?
WHERE id = ? AND deleted_at = '' AND last_cascade_original_statement = ''`, stmt, fid); err != nil {
			return err
		}
	}
	return nil
}

func (st *Store) RevertCascadeFactStatements(ctx context.Context, agentID string) (int, error) {
	if st == nil || st.client == nil {
		return 0, errors.New("session memory store not wired")
	}
	rows, err := st.client.QueryContext(ctx, `
SELECT id, last_cascade_original_statement FROM memory_facts
WHERE scope_type = 'agent' AND scope_id = ? AND deleted_at = ''
AND last_cascade_original_statement != ''`, agentID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type revertItem struct {
		id       string
		original string
	}
	var items []revertItem
	for rows.Next() {
		var it revertItem
		if err := rows.Scan(&it.id, &it.original); err != nil {
			continue
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	reverted := 0
	for _, it := range items {
		if _, err := st.client.ExecContext(ctx, `
UPDATE memory_facts SET statement = ?, statement_normalized = ?, last_cascade_original_statement = '',
version = version + 1, updated_at = ?
WHERE id = ? AND deleted_at = ''`,
			it.original, strings.ToLower(strings.TrimSpace(it.original)), now, it.id); err != nil {
			continue
		}
		reverted++
	}
	return reverted, nil
}

func (st *Store) ListCascadeFactDiffs(ctx context.Context, agentID, oldName, newName string, limit int) ([]map[string]any, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	like := "%" + strings.ToLower(oldName) + "%"
	rows, err := st.client.QueryContext(ctx, `
SELECT id, statement, scope, scope_type, scope_id
FROM memory_facts
WHERE scope_type = 'agent' AND scope_id = ? AND deleted_at = '' AND status = 'active'
AND LOWER(statement) LIKE ?
LIMIT ?`, agentID, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, stmt, scope, scopeType, scopeID string
		if err := rows.Scan(&id, &stmt, &scope, &scopeType, &scopeID); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"fact_id":           id,
			"before_statement":  stmt,
			"after_statement":   stmt,
			"scope":             scope,
			"scope_type":        scopeType,
			"scope_id":          scopeID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	re, err := nameReplacePattern(oldName)
	if err != nil {
		return nil, err
	}
	for i, d := range out {
		before, _ := d["before_statement"].(string)
		if re.MatchString(before) {
			out[i]["after_statement"] = replaceNameWordBoundary(before, re, newName)
		}
	}
	return out, nil
}

func (st *Store) MarkFactsIndexStaleByAgent(ctx context.Context, agentID string) (int64, error) {
	if st == nil || st.client == nil {
		return 0, errors.New("session memory store not wired")
	}
	res, err := st.client.ExecContext(ctx, `
UPDATE memory_facts SET index_status = 'stale'
WHERE scope_type = 'agent' AND scope_id = ? AND deleted_at = '' AND index_status = 'fresh'`, agentID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanCascadeSagaStepJSON(rows interface{ Scan(...any) error }) ([]byte, error) {
	var s CascadeSagaStep
	var isCrit int
	if err := rows.Scan(&s.ID, &s.ProposalID, &s.StepIndex, &s.StepName, &s.State,
		&isCrit, &s.Attempts, &s.StartedAt, &s.FinishedAt,
		&s.PayloadJSON, &s.ResultJSON, &s.Error); err != nil {
		return nil, err
	}
	s.IsCritical = isCrit == 1
	return json.Marshal(s)
}
