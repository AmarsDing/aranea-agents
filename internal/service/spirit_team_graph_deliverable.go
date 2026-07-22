package service

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"
	araneasession "aranea-agents/internal/session"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// graphDeliverableReader adapts the trpc session service to
// biz.SpiritGraphDeliverableReader for the B.10.15.4 Graph StateFields
// bridge. Team runs persist graph state under AppName = anchor (manager)
// agent ID — not the default app scope — so the session key is constructed
// explicitly from the coordinates the biz layer resolved.
type graphDeliverableReader struct {
	rt *araneasession.Runtime
}

// NewGraphDeliverableReader builds the adapter. A nil runtime yields a reader
// that always reports the state as absent, so the bridge degrades to the
// reply-extraction path (v1-only deployments).
func NewGraphDeliverableReader(rt *araneasession.Runtime) biz.SpiritGraphDeliverableReader {
	return graphDeliverableReader{rt: rt}
}

func (g graphDeliverableReader) ReadGraphDeliverable(ctx context.Context, appName, userID, sessionID string) (map[string]any, error) {
	if g.rt == nil || g.rt.Service() == nil {
		return nil, nil
	}
	key := trpcsession.Key{AppName: appName, UserID: userID, SessionID: sessionID}
	if err := key.CheckSessionKey(); err != nil {
		return nil, err
	}
	sess, err := g.rt.Service().GetSession(ctx, key)
	if err != nil || sess == nil {
		return nil, err
	}
	raw, ok := sess.State[biz.DeliverableStateKey]
	if !ok || len(raw) == 0 {
		return nil, nil
	}
	var deliverable map[string]any
	if err := json.Unmarshal(raw, &deliverable); err != nil {
		return nil, err
	}
	return deliverable, nil
}
