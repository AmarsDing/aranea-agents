package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	biza2a "aranea-agents/internal/biz/a2a"
	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// a2aRepo implements biz.A2ARepo using raw SQL.
type a2aRepo struct {
	data *Data
	lg   loggateway.Logger
}

var (
	_ biza2a.Repo           = (*a2aRepo)(nil)
	_ biza2a.CardRepo       = (*a2aRepo)(nil)
	_ biza2a.InvocationRepo = (*a2aRepo)(nil)
	_ biza2a.AuditRepo      = (*a2aRepo)(nil)
	_ biza2a.RemoteAgentRepo = (*a2aRepo)(nil)
)

func NewA2ARepo(data *Data, lg loggateway.Logger) biz.A2ARepo {
	if data == nil || data.RWDB() == nil {
		return nil
	}
	return &a2aRepo{data: data, lg: lg}
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
			last_health_at   TEXT NOT NULL DEFAULT '',
			last_health_ok   INTEGER NOT NULL DEFAULT 0,
			last_health_error TEXT NOT NULL DEFAULT '',
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return entErrToBizErr(err, "A2A")
		}
	}
	// TECH-DEBT(debt): DEV-10 — A2A schema migration should move to a proper migration framework (e.g. golang-migrate).
	// Current approach: raw ALTER TABLE with duplicate-column error swallowing is fragile and not versioned.
	migrations := []string{
		`ALTER TABLE a2a_remote_agents ADD COLUMN last_health_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE a2a_remote_agents ADD COLUMN last_health_ok INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE a2a_remote_agents ADD COLUMN last_health_error TEXT NOT NULL DEFAULT ''`,
	}
	for _, m := range migrations {
		// SQLite ALTER TABLE ADD COLUMN fails silently if column already exists in some drivers,
		// but returns an error in others. Ignore "duplicate column" errors.
		if _, err := db.ExecContext(ctx, m); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				return entErrToBizErr(err, "A2A")
			}
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
	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT agent_id,display_name,workspace,enabled,capabilities,updated_at FROM a2a_agent_cards WHERE agent_id=?`, agentID)
	if err != nil {
		return biz.A2AAgentCard{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.A2AAgentCard{}, biz.ErrNotFound
	}
	card, err := scanA2ACard(rows, r.lg)
	if err != nil {
		return biz.A2AAgentCard{}, err
	}
	return card, nil
}

func (r *a2aRepo) ListEnabledCards(ctx context.Context, workspace, capability string) ([]biz.A2AAgentCard, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT agent_id,display_name,workspace,enabled,capabilities,updated_at
		 FROM a2a_agent_cards WHERE enabled=1 AND (workspace=? OR ?='')
		 ORDER BY agent_id`, workspace, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.A2AAgentCard
	for rows.Next() {
		card, err := scanA2ACard(rows, r.lg)
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
	if r == nil || r.data == nil || len(agentIDs) == 0 {
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
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
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO a2a_invocations
		 (id,caller_agent_id,callee_agent_id,caller_session_id,capability,payload_json,status,result_json,error_message,duration_ms,timeout_seconds,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		inv.ID, inv.CallerAgentID, inv.CalleeAgentID, inv.CallerSessionID,
		inv.Capability, inv.PayloadJSON, inv.Status, inv.ResultJSON, inv.ErrorMessage,
		inv.DurationMs, inv.TimeoutSeconds, t)
	return inv, err
}

func (r *a2aRepo) UpdateInvocation(ctx context.Context, inv biz.A2AInvocation) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE a2a_invocations SET status=?,result_json=?,error_message=?,duration_ms=? WHERE id=?`,
		inv.Status, inv.ResultJSON, inv.ErrorMessage, inv.DurationMs, inv.ID)
	return err
}

// --- Audit ---

func (r *a2aRepo) InsertAudit(ctx context.Context, entry biz.A2AAuditEntry) error {
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO a2a_audit (id,invoke_id,caller_agent_id,callee_agent_id,capability,status,duration_ms,workspace,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		entry.ID, entry.InvokeID, entry.CallerAgentID, entry.CalleeAgentID,
		entry.Capability, entry.Status, entry.DurationMs, entry.Workspace, entry.CreatedAt)
	return err
}

func (r *a2aRepo) ListAudit(ctx context.Context, callerID, calleeID string, limit, offset int) ([]biz.A2AAuditEntry, int, error) {
	var total int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		`SELECT COUNT(*) FROM a2a_audit WHERE (caller_agent_id=? OR ?='') AND (callee_agent_id=? OR ?='')`,
		[]any{callerID, callerID, calleeID, calleeID}, &total); err != nil {
		return nil, 0, err
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
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
	r.lg.Info("A2A discover remote card", loggateway.StepID("a2a.discover_remote_card"), loggateway.Str("remote_url", in.RemoteURL))
	card, err := a2apkg.FetchRemoteAgentCard(ctx, in.RemoteURL, in.AuthType, in.AuthConfigJSON, r.lg)
	if err != nil {
		r.lg.Error("A2A discover remote card failed", loggateway.StepID("a2a.discover_remote_card"), loggateway.Str("remote_url", in.RemoteURL), loggateway.Err(err))
		return biz.A2AAgentCard{}, err
	}
	return card, nil
}

func (r *a2aRepo) CreateRemoteAgent(ctx context.Context, agent biz.A2ARemoteAgent) (biz.A2ARemoteAgent, error) {
	if r == nil || r.data == nil {
		return biz.A2ARemoteAgent{}, apierror.Internal("A2A", "a2a db nil")
	}
	if agent.ID == "" {
		id, err := biz.NewA2AID()
		if err != nil {
			return biz.A2ARemoteAgent{}, entErrToBizErr(err, "A2A")
		}
		agent.ID = id
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
	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
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
	if r == nil || r.data == nil {
		return nil, nil
	}
	q := `SELECT id,workspace,display_name,remote_url,agent_card_url,auth_type,auth_config_json,enabled,card_json,
	      COALESCE(last_health_at,''),COALESCE(last_health_ok,0),COALESCE(last_health_error,''),created_at,updated_at
	      FROM a2a_remote_agents WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(workspace) != "" {
		q += ` AND workspace=?`
		args = append(args, workspace)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.A2ARemoteAgent
	for rows.Next() {
		item, err := scanRemoteAgent(rows, r.lg)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *a2aRepo) DeleteRemoteAgent(ctx context.Context, id string) error {
	if r == nil || r.data == nil {
		return apierror.Internal("A2A", "a2a db nil")
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `DELETE FROM a2a_remote_agents WHERE id=?`, id)
	return err
}

func (r *a2aRepo) GetRemoteAgent(ctx context.Context, id string) (biz.A2ARemoteAgent, error) {
	if r == nil || r.data == nil {
		return biz.A2ARemoteAgent{}, apierror.Internal("A2A", "a2a db nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.A2ARemoteAgent{}, biz.ErrNotFound
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id,workspace,display_name,remote_url,agent_card_url,auth_type,auth_config_json,enabled,card_json,
		 COALESCE(last_health_at,''),COALESCE(last_health_ok,0),COALESCE(last_health_error,''),created_at,updated_at
		 FROM a2a_remote_agents WHERE id=?`, id)
	if err != nil {
		return biz.A2ARemoteAgent{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.A2ARemoteAgent{}, biz.ErrNotFound
	}
	agent, err := scanRemoteAgent(rows, r.lg)
	if err != nil {
		return biz.A2ARemoteAgent{}, err
	}
	return agent, nil
}

func (r *a2aRepo) UpdateRemoteAgentHealth(ctx context.Context, id string, ok bool, errMsg string) error {
	if r == nil || r.data == nil {
		return apierror.Internal("A2A", "a2a db nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return apierror.BadRequest("A2A", "id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	okInt := 0
	if ok {
		okInt = 1
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE a2a_remote_agents SET last_health_at=?, last_health_ok=?, last_health_error=?, updated_at=? WHERE id=?`,
		now, okInt, strings.TrimSpace(errMsg), now, id)
	return err
}

func scanRemoteAgent(row scannable, lg loggateway.Logger) (biz.A2ARemoteAgent, error) {
	var agent biz.A2ARemoteAgent
	var enabled int
	var healthOK int
	var cardJSON string
	if err := row.Scan(&agent.ID, &agent.Workspace, &agent.DisplayName, &agent.RemoteURL, &agent.AgentCardURL,
		&agent.AuthType, &agent.AuthConfigJSON, &enabled, &cardJSON,
		&agent.LastHealthAt, &healthOK, &agent.LastHealthError, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
		return biz.A2ARemoteAgent{}, err
	}
	agent.Enabled = enabled == 1
	agent.LastHealthOK = healthOK == 1
	if err := json.Unmarshal([]byte(cardJSON), &agent.DiscoveredCard); err != nil {
		lg.Warn("a2a json unmarshal failed", loggateway.StepID("data.a2a"), loggateway.Err(err))
	}
	if agent.DiscoveredCard.AgentID == "" {
		agent.DiscoveredCard.AgentID = agent.ID
	}
	return agent, nil
}

// --- helpers ---

func scanA2ACard(row scannable, lg loggateway.Logger) (biz.A2AAgentCard, error) {
	var card biz.A2AAgentCard
	var capJSON string
	var enabled int
	if err := row.Scan(&card.AgentID, &card.DisplayName, &card.Workspace, &enabled, &capJSON, &card.UpdatedAt); err != nil {
		return biz.A2AAgentCard{}, err
	}
	card.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(capJSON), &card.Capabilities); err != nil {
		lg.Warn("a2a json unmarshal failed", loggateway.StepID("data.a2a"), loggateway.Err(err))
	}
	return card, nil
}
