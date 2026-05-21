package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	a2apkg "aranea-agents/internal/a2a"
)

// a2aRepo implements biz.A2ARepo using raw SQL.
type a2aRepo struct {
	db *sql.DB
}

// NewA2ARepo returns a biz.A2ARepo backed by the provided *sql.DB.
func NewA2ARepo(db *sql.DB) biz.A2ARepo {
	return &a2aRepo{db: db}
}

// EnsureA2ASchema creates the A2A tables when they do not exist.
func EnsureA2ASchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS a2a_agent_cards (
			agent_id     TEXT PRIMARY KEY,
			display_name TEXT NOT NULL DEFAULT '',
			workspace    TEXT NOT NULL DEFAULT '',
			enabled      INTEGER NOT NULL DEFAULT 0,
			capabilities TEXT NOT NULL DEFAULT '[]',
			updated_at   TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS a2a_invocations (
			id               TEXT PRIMARY KEY,
			caller_agent_id  TEXT NOT NULL DEFAULT '',
			callee_agent_id  TEXT NOT NULL,
			caller_session_id TEXT NOT NULL DEFAULT '',
			capability       TEXT NOT NULL,
			payload_json     TEXT NOT NULL DEFAULT '{}',
			status           TEXT NOT NULL DEFAULT 'pending',
			result_json      TEXT NOT NULL DEFAULT '',
			error_message    TEXT NOT NULL DEFAULT '',
			duration_ms      INTEGER NOT NULL DEFAULT 0,
			timeout_seconds  INTEGER NOT NULL DEFAULT 30,
			created_at       TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS a2a_audit (
			id              TEXT PRIMARY KEY,
			invoke_id       TEXT NOT NULL,
			caller_agent_id TEXT NOT NULL DEFAULT '',
			callee_agent_id TEXT NOT NULL,
			capability      TEXT NOT NULL,
			status          TEXT NOT NULL,
			duration_ms     INTEGER NOT NULL DEFAULT 0,
			workspace       TEXT NOT NULL DEFAULT '',
			created_at      TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS a2a_remote_agents (
			id               TEXT PRIMARY KEY,
			workspace        TEXT NOT NULL DEFAULT '',
			display_name     TEXT NOT NULL DEFAULT '',
			remote_url       TEXT NOT NULL,
			agent_card_url   TEXT NOT NULL DEFAULT '',
			auth_type        TEXT NOT NULL DEFAULT '',
			auth_config_json TEXT NOT NULL DEFAULT '',
			enabled          INTEGER NOT NULL DEFAULT 1,
			card_json        TEXT NOT NULL DEFAULT '{}',
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("a2a schema: %w", err)
		}
	}
	return nil
}

// --- Agent Card ---

func (r *a2aRepo) UpsertAgentCard(ctx context.Context, card biz.A2AAgentCard) (biz.A2AAgentCard, error) {
	caps, err := json.Marshal(card.Capabilities)
	if err != nil {
		return biz.A2AAgentCard{}, err
	}
	enabled := 0
	if card.Enabled {
		enabled = 1
	}
	t := time.Now().UTC().Format(time.RFC3339)
	card.UpdatedAt = t
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO a2a_agent_cards (agent_id,display_name,workspace,enabled,capabilities,updated_at)
		 VALUES (?,?,?,?,?,?)
		 ON CONFLICT(agent_id) DO UPDATE SET
		   display_name=excluded.display_name,
		   workspace=excluded.workspace,
		   enabled=excluded.enabled,
		   capabilities=excluded.capabilities,
		   updated_at=excluded.updated_at`,
		card.AgentID, card.DisplayName, card.Workspace, enabled, string(caps), t)
	return card, err
}

func (r *a2aRepo) GetAgentCard(ctx context.Context, agentID string) (biz.A2AAgentCard, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT agent_id,display_name,workspace,enabled,capabilities,updated_at FROM a2a_agent_cards WHERE agent_id=?`, agentID)
	card, err := scanA2ACard(row)
	if err == sql.ErrNoRows {
		return biz.A2AAgentCard{}, biz.ErrNotFound
	}
	return card, err
}

func (r *a2aRepo) ListEnabledCards(ctx context.Context, workspace, capability string) ([]biz.A2AAgentCard, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT agent_id,display_name,workspace,enabled,capabilities,updated_at
		 FROM a2a_agent_cards WHERE enabled=1 AND (workspace=? OR ?='')
		 ORDER BY agent_id`, workspace, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.A2AAgentCard
	for rows.Next() {
		card, err := scanA2ACard(rows)
		if err != nil {
			return nil, err
		}
		// filter by capability if requested
		if capability != "" {
			found := false
			for _, c := range card.Capabilities {
				if c.Name == capability {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		out = append(out, card)
	}
	return out, rows.Err()
}

func (r *a2aRepo) MapEndpointEnabled(ctx context.Context, agentIDs []string) (map[string]bool, error) {
	out := make(map[string]bool)
	if r == nil || r.db == nil || len(agentIDs) == 0 {
		return out, nil
	}
	seen := make(map[string]struct{}, len(agentIDs))
	var ids []string
	for _, id := range agentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	q := fmt.Sprintf(`SELECT agent_id FROM a2a_agent_cards WHERE enabled=1 AND agent_id IN (%s)`, placeholders)
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// --- Invocations ---

func (r *a2aRepo) CreateInvocation(ctx context.Context, inv biz.A2AInvocation) (biz.A2AInvocation, error) {
	t := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO a2a_invocations
		 (id,caller_agent_id,callee_agent_id,caller_session_id,capability,payload_json,status,result_json,error_message,duration_ms,timeout_seconds,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		inv.ID, inv.CallerAgentID, inv.CalleeAgentID, inv.CallerSessionID,
		inv.Capability, inv.PayloadJSON, inv.Status, inv.ResultJSON, inv.ErrorMessage,
		inv.DurationMs, inv.TimeoutSeconds, t)
	return inv, err
}

func (r *a2aRepo) UpdateInvocation(ctx context.Context, inv biz.A2AInvocation) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE a2a_invocations SET status=?,result_json=?,error_message=?,duration_ms=? WHERE id=?`,
		inv.Status, inv.ResultJSON, inv.ErrorMessage, inv.DurationMs, inv.ID)
	return err
}

// --- Audit ---

func (r *a2aRepo) InsertAudit(ctx context.Context, entry biz.A2AAuditEntry) error {
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO a2a_audit (id,invoke_id,caller_agent_id,callee_agent_id,capability,status,duration_ms,workspace,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		entry.ID, entry.InvokeID, entry.CallerAgentID, entry.CalleeAgentID,
		entry.Capability, entry.Status, entry.DurationMs, entry.Workspace, entry.CreatedAt)
	return err
}

func (r *a2aRepo) ListAudit(ctx context.Context, callerID, calleeID string, limit, offset int) ([]biz.A2AAuditEntry, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM a2a_audit WHERE (caller_agent_id=? OR ?='') AND (callee_agent_id=? OR ?='')`,
		callerID, callerID, calleeID, calleeID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,invoke_id,caller_agent_id,callee_agent_id,capability,status,duration_ms,workspace,created_at
		 FROM a2a_audit WHERE (caller_agent_id=? OR ?='') AND (callee_agent_id=? OR ?='')
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		callerID, callerID, calleeID, calleeID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []biz.A2AAuditEntry
	for rows.Next() {
		var e biz.A2AAuditEntry
		if err := rows.Scan(&e.ID, &e.InvokeID, &e.CallerAgentID, &e.CalleeAgentID,
			&e.Capability, &e.Status, &e.DurationMs, &e.Workspace, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (r *a2aRepo) DiscoverRemoteCard(ctx context.Context, in biz.RemoteCardDiscoverInput) (biz.A2AAgentCard, error) {
	return a2apkg.FetchRemoteAgentCard(ctx, in.RemoteURL, in.AuthType, in.AuthConfigJSON)
}

func (r *a2aRepo) CreateRemoteAgent(ctx context.Context, agent biz.A2ARemoteAgent) (biz.A2ARemoteAgent, error) {
	if r == nil || r.db == nil {
		return biz.A2ARemoteAgent{}, fmt.Errorf("a2a db nil")
	}
	if agent.ID == "" {
		agent.ID = fmt.Sprintf("remote-%d", time.Now().UnixNano())
	}
	cardJSON, err := json.Marshal(agent.DiscoveredCard)
	if err != nil {
		return biz.A2ARemoteAgent{}, err
	}
	enabled := 0
	if agent.Enabled {
		enabled = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO a2a_remote_agents
		 (id,workspace,display_name,remote_url,agent_card_url,auth_type,auth_config_json,enabled,card_json,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		agent.ID, agent.Workspace, agent.DisplayName, agent.RemoteURL, agent.AgentCardURL,
		agent.AuthType, agent.AuthConfigJSON, enabled, string(cardJSON), now, now)
	if err != nil {
		return biz.A2ARemoteAgent{}, err
	}
	agent.CreatedAt = now
	agent.UpdatedAt = now
	return agent, nil
}

func (r *a2aRepo) ListRemoteAgents(ctx context.Context, workspace string) ([]biz.A2ARemoteAgent, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	q := `SELECT id,workspace,display_name,remote_url,agent_card_url,auth_type,auth_config_json,enabled,card_json,created_at,updated_at
	      FROM a2a_remote_agents WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(workspace) != "" {
		q += ` AND workspace=?`
		args = append(args, workspace)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.A2ARemoteAgent
	for rows.Next() {
		item, err := scanRemoteAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *a2aRepo) DeleteRemoteAgent(ctx context.Context, id string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("a2a db nil")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM a2a_remote_agents WHERE id=?`, id)
	return err
}

func (r *a2aRepo) GetRemoteAgent(ctx context.Context, id string) (biz.A2ARemoteAgent, error) {
	if r == nil || r.db == nil {
		return biz.A2ARemoteAgent{}, fmt.Errorf("a2a db nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.A2ARemoteAgent{}, biz.ErrNotFound
	}
	row := r.db.QueryRowContext(ctx,
		`SELECT id,workspace,display_name,remote_url,agent_card_url,auth_type,auth_config_json,enabled,card_json,created_at,updated_at
		 FROM a2a_remote_agents WHERE id=?`, id)
	agent, err := scanRemoteAgent(row)
	if err == sql.ErrNoRows {
		return biz.A2ARemoteAgent{}, biz.ErrNotFound
	}
	return agent, err
}

func scanRemoteAgent(row scannable) (biz.A2ARemoteAgent, error) {
	var agent biz.A2ARemoteAgent
	var enabled int
	var cardJSON string
	if err := row.Scan(&agent.ID, &agent.Workspace, &agent.DisplayName, &agent.RemoteURL, &agent.AgentCardURL,
		&agent.AuthType, &agent.AuthConfigJSON, &enabled, &cardJSON, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
		return biz.A2ARemoteAgent{}, err
	}
	agent.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(cardJSON), &agent.DiscoveredCard)
	if agent.DiscoveredCard.AgentID == "" {
		agent.DiscoveredCard.AgentID = agent.ID
	}
	return agent, nil
}

// --- helpers ---

func scanA2ACard(row scannable) (biz.A2AAgentCard, error) {
	var card biz.A2AAgentCard
	var capJSON string
	var enabled int
	if err := row.Scan(&card.AgentID, &card.DisplayName, &card.Workspace, &enabled, &capJSON, &card.UpdatedAt); err != nil {
		return biz.A2AAgentCard{}, err
	}
	card.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(capJSON), &card.Capabilities)
	return card, nil
}
