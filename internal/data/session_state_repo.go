package data

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	entsession "aranea-agents/internal/data/ent/session"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func (r *sessionRepo) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, apierror.BadRequest("SESSION", "session id is required")
	}
	row, err := r.data.RW().Read(ctx).Session.Get(ctx, sessionID)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION_STATE")
	}
	out := map[string]string{}
	if row.StateJSON != "" {
		raw := map[string]any{}
		if err := json.Unmarshal([]byte(row.StateJSON), &raw); err != nil {
			r.data.lg.Warn("session state json unmarshal failed", loggateway.StepID("data.session_state"), loggateway.Err(err))
		} else {
			for k, v := range raw {
				if s, ok := v.(string); ok {
					out[k] = s
				}
			}
		}
	}
	return out, nil
}

func (r *sessionRepo) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return apierror.BadRequest("SESSION", "session id is required")
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = r.data.RW().Write(ctx).Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetStateJSON(string(raw)).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	return entErrToBizErr(err, "SESSION_STATE")
}

func (r *sessionRepo) PatchSessionState(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return apierror.BadRequest("SESSION", "session id is required")
	}
	if len(sets) == 0 && len(deletes) == 0 {
		return nil
	}

	d := r.data.Dialect()
	// Normalize the TEXT column into a json-typed base expression once;
	// JSONSet/JSONRemove then chain on it (Postgres requires jsonb input).
	expr := d.JSONBBase("state_json")
	var args []any

	orderedKeys := sortedKeys(sets)
	for _, k := range orderedKeys {
		// SQLite: json_set(base, '$.key', ?)
		// Postgres: jsonb_set(base, '{key}', to_jsonb(?::text))
		// d.JSONSet embeds the key directly into the SQL, so only the value
		// is passed as a placeholder arg. The ::text cast resolves Postgres
		// polymorphic-type inference for to_jsonb(anyelement).
		expr = d.JSONSet(expr, k, "?")
		args = append(args, sets[k])
	}
	for _, k := range deletes {
		sqlExpr, arg := d.JSONRemove(expr, k)
		expr = sqlExpr
		args = append(args, arg)
	}

	args = append(args, nowRFC3339(), sessionID)
	query := fmt.Sprintf(
		"UPDATE sessions SET state_json = %s, updated_at = ? WHERE id = ? AND deleted_at = ''",
		expr,
	)
	query = d.RenumberPlaceholders(query)
	_, err := r.data.RW().Write(ctx).ExecContext(ctx, query, args...)
	return entErrToBizErr(err, "SESSION_STATE")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
