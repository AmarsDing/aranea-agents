package agent

import (
	"context"

	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
)

// SyncPersistedADKSessionToMemory reloads the session snapshot from session.Service after events are committed,
// then ingests assistant/user text events into the active memory backend.
//
// For SQLite-backed runners this updates memory_entities via [SessionSQLiteMemoryService].
// Other implementations (in-memory ADK store) skip work here to avoid duplicate accumulation per turn.
func SyncPersistedADKSessionToMemory(ctx context.Context, sessSvc session.Service, mem memory.Service, appName, userID, sessionID string) error {
	if ctx.Err() != nil || sessSvc == nil || mem == nil {
		return nil
	}
	if _, ok := mem.(*SessionSQLiteMemoryService); !ok {
		return nil
	}
	getResp, err := sessSvc.Get(ctx, &session.GetRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return err
	}
	return mem.AddSessionToMemory(ctx, getResp.Session)
}
