package data

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	entsession "aranea-agents/internal/data/ent/session"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (r *sessionRepo) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	row, err := r.txClient(ctx).Session.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	state := map[string]string{}
	if row.StateJSON != "" {
		if err := json.Unmarshal([]byte(row.StateJSON), &state); err != nil {
			r.data.lg.Warn("session state json unmarshal failed", loggateway.StepID("data.session_state"), loggateway.Err(err))
		}
	}
	return state, nil
}

func (r *sessionRepo) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = r.txClient(ctx).Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetStateJSON(string(raw)).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	return err
}

func (r *sessionRepo) PatchSessionState(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	if len(sets) == 0 && len(deletes) == 0 {
		return nil
	}

	expr := "state_json"
	var args []any

	orderedKeys := sortedKeys(sets)
	for _, k := range orderedKeys {
		expr = fmt.Sprintf("json_set(%s, ?, ?)", expr)
		args = append(args, "$."+k, sets[k])
	}
	for _, k := range deletes {
		expr = fmt.Sprintf("json_remove(%s, ?)", expr)
		args = append(args, "$."+k)
	}

	args = append(args, nowRFC3339(), sessionID)
	query := fmt.Sprintf(
		"UPDATE sessions SET state_json = %s, updated_at = ? WHERE id = ? AND deleted_at = ''",
		expr,
	)
	_, err := r.txClient(ctx).ExecContext(ctx, query, args...)
	return err
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
