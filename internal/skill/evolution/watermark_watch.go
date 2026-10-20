package evolution

import (
	"time"

	"aranea-agents/pkg/loggateway"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// watchAndPersist polls the in-memory session for evolution:last_review_at
// updates written by the async framework worker, then mirrors them into the
// durable WatermarkStore. Bounded wait — if the worker never writes (no
// delta / review error), we exit without advancing the store.
func (s *Service) watchAndPersist(sess *trpcsession.Session, sessionID string) {
	if s == nil || s.store == nil || sess == nil || sessionID == "" {
		return
	}
	deadline := time.Now().Add(2 * time.Minute)
	var last string
	if raw, ok := sess.GetState(sessionStateKeyLastReviewAt); ok {
		last = string(raw)
	}
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		raw, ok := sess.GetState(sessionStateKeyLastReviewAt)
		if !ok || len(raw) == 0 {
			continue
		}
		cur := string(raw)
		if cur == last {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, cur)
		if err != nil {
			continue
		}
		s.store.Set(sessionID, ts)
		s.cfg.Lg.Debug("evolution watermark persisted (worker write)",
			loggateway.StepID("skill.evolution_wm_watch"),
			loggateway.SessionID(sessionID))
		return
	}
}
