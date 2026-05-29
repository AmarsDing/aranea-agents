package data

import (
	"context"
	"encoding/json"
	"strings"

	entsession "aranea-agents/internal/data/ent/session"
	"aranea-agents/internal/event"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (r *sessionRepo) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	row, err := r.data.entClient.Session.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	state := map[string]string{}
	if row.StateJSON != "" {
		if err := json.Unmarshal([]byte(row.StateJSON), &state); err != nil {
		event.SysLogWarn("session.state", "state json unmarshal failed", event.P("error", err.Error()))
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
	_, err = r.data.entClient.Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetStateJSON(string(raw)).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	return err
}
