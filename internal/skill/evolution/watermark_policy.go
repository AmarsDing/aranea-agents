package evolution

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcevolution "trpc.group/trpc-go/trpc-agent-go/evolution"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// persistingReviewPolicy wraps the default policy and persists LatestTs when
// the policy declines review (framework also advances watermark on that path).
// Approve/review-success paths rely on watchAndPersist reading SetState.
type persistingReviewPolicy struct {
	inner trpcevolution.ReviewPolicy
	store WatermarkStore
	lg    loggateway.Logger
}

func (p *persistingReviewPolicy) ShouldReview(ctx context.Context, input *trpcevolution.ReviewPolicyInput) (bool, error) {
	if p == nil || p.inner == nil {
		return false, nil
	}
	ok, err := p.inner.ShouldReview(ctx, input)
	if err != nil || ok || input == nil || input.ReviewContext == nil || p.store == nil {
		return ok, err
	}
	ts := input.ReviewContext.LatestTs
	if !ts.IsZero() && input.SessionID != "" {
		p.store.Set(input.SessionID, ts)
		if p.lg != nil {
			p.lg.Debug("evolution watermark persisted (policy decline)",
				loggateway.StepID("skill.evolution_wm_decline"),
				loggateway.SessionID(input.SessionID))
		}
	}
	return ok, err
}

func restoreWatermark(store WatermarkStore, sess *trpcsession.Session) {
	if store == nil || sess == nil || sess.ID == "" {
		return
	}
	ts, ok := store.Get(sess.ID)
	if !ok || ts.IsZero() {
		return
	}
	sess.SetState(sessionStateKeyLastReviewAt, []byte(ts.UTC().Format(time.RFC3339Nano)))
}

func readSessionWatermark(sess *trpcsession.Session) (time.Time, bool) {
	if sess == nil {
		return time.Time{}, false
	}
	raw, ok := sess.GetState(sessionStateKeyLastReviewAt)
	if !ok || len(raw) == 0 {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, string(raw))
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}
