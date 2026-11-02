package decide

import (
	"context"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// ShadowEvent is one shadow-recordable decision occurrence. The recorder
// persists it as a decision.Record (category memory_shadow) whose metadata
// carries the full payload; Scenario/Outcome stay short for the audit index.
// Incumbent is always present (the live mechanism being shadowed); JEV is nil
// until a Decider backend is wired (post T6), at which point both answers
// share CorrelationID for harvest-time pairing.
type ShadowEvent struct {
	// CorrelationID pairs the incumbent and JEV sides of one decision
	// occurrence. Callers derive it deterministically from the decision
	// inputs (e.g. question + message id) so retries do not fork pairs.
	CorrelationID string
	QuestionID    string
	// State is the T2-contract slot map; absent slots are omitted, never
	// fabricated. Keys are slot names (user_message, locale, ...).
	State map[string]any
	// Incumbent is the live mechanism answer. Key is the winning option
	// (e.g. direct_lookup); Scores holds every evaluated option margin so
	// harvest can rebuild soft distributions; Method names the mechanism
	// (vir | lexical | ...).
	Incumbent Incumbent
	// JEV is the decision-model answer when a backend is wired; nil in
	// Phase 0 (incumbent-only collection).
	JEV *Answer
	// SessionID / UserID / WorkspaceID attribute the occurrence for
	// harvest filtering; all optional.
	SessionID   string
	UserID      string
	WorkspaceID string
}

// Incumbent is the live mechanism side of a shadow pair.
type Incumbent struct {
	Key       string             `json:"key"`
	Scores    map[string]float64 `json:"scores,omitempty"`
	Threshold float64            `json:"threshold,omitempty"`
	Method    string             `json:"method"`
	// Matched reports the mechanism boolean verdict (e.g. Match result for
	// one gate); route-style questions derive the verdict from Key instead.
	Matched bool `json:"matched"`
}

// Recorder emits shadow records into the M80 decision layer. A nil Recorder
// (or nil Collector inside) makes every emit a no-op, so hot paths can hold
// one unconditionally. Construction is the wiring-site kill switch
// (ARANEA_MEM_SHADOW / ARANEA_JEV_SHADOW, default off).
//
// When ARANEA_JEV_URL is set, NewRecorder auto-wires an HTTP Decider so Emit
// fills ShadowEvent.JEV before persistence (incumbent+JEV double-record).
// Decider errors leave jev=pending and never fail the turn.
type Recorder struct {
	c       decision.Collector
	lg      loggateway.Logger
	decider Decider
}

// ShadowRecorderWired is the optional wiring contract a store implements to
// accept a shadow recorder post-construction. Defined on the consumer side so
// wiring sites (cmd/memoryeval) do not import the store's package for a type
// assertion; a store not implementing it simply stays unshadowed.
type ShadowRecorderWired interface {
	SetShadowRecorder(*Recorder)
}

// NewRecorder builds a Recorder; c may be nil (fully disabled, zero-cost).
// Auto-wires DeciderFromEnv when ARANEA_JEV_URL is present.
func NewRecorder(c decision.Collector, lg loggateway.Logger) *Recorder {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	lg = lg.With(loggateway.Domain("mem_shadow"))
	return &Recorder{c: c, lg: lg, decider: DeciderFromEnv(lg)}
}

// WithDecider replaces the auto-wired Decider (tests / explicit injection).
func (r *Recorder) WithDecider(d Decider) *Recorder {
	if r != nil {
		r.decider = d
	}
	return r
}

// Enabled reports whether emits actually reach the decision layer.
func (r *Recorder) Enabled() bool { return r != nil && r.c != nil }

const jevFillTimeout = 5 * time.Second

// Emit records one shadow event. It never fails the turn: validation errors
// or a disabled recorder log and drop, mirroring decision.EmitGate discipline.
// When a Decider is wired and JEV is unset, Emit asks it once under a short
// timeout; failure keeps jev=pending.
func (r *Recorder) Emit(ctx context.Context, ev ShadowEvent) {
	if !r.Enabled() {
		return
	}
	if ev.JEV == nil && r.decider != nil {
		if ans := r.fillJEV(ctx, ev); ans != nil {
			ev.JEV = ans
		}
	}
	rec, err := r.build(ev)
	if err != nil {
		r.lg.Warn("shadow record dropped", loggateway.Err(err))
		return
	}
	r.c.Emit(ctx, rec)
}

// fillJEV asks the Decider for one question; returns nil on any failure.
func (r *Recorder) fillJEV(ctx context.Context, ev ShadowEvent) *Answer {
	qdef, ok := QuestionDef(ev.QuestionID)
	if !ok {
		return nil
	}
	q := qdef
	if len(q.Criteria) == 0 {
		q.Criteria = criteriaFromState(q, ev.State)
	}
	if len(q.Criteria) == 0 {
		return nil
	}
	dctx, cancel := context.WithTimeout(ctx, jevFillTimeout)
	defer cancel()
	answers, err := r.decider.Decide(dctx, ev.State, []Question{q})
	if err != nil {
		r.lg.Warn("jev fill skipped", loggateway.Err(err), loggateway.StepID("jev.shadow_fill"))
		return nil
	}
	if a, ok := answers[q.ID]; ok {
		cp := a
		return &cp
	}
	return nil
}

// build maps one event to a decision.Record. The shadow payload lives in
// metadata.shadow (JSON, uncapped); Scenario and Outcome are index-friendly
// short forms. Outcome format q=<question>;k=<key> stays within the 64-char
// cap for every registered question.
func (r *Recorder) build(ev ShadowEvent) (decision.Record, error) {
	qid := strings.TrimSpace(ev.QuestionID)
	if qid == "" {
		return decision.Record{}, errString("decide: question_id is required")
	}
	qdef, ok := QuestionDef(qid)
	if !ok {
		return decision.Record{}, errString("decide: unknown shadow question " + qid)
	}
	corr := strings.TrimSpace(ev.CorrelationID)
	if corr == "" {
		return decision.Record{}, errString("decide: correlation_id is required")
	}
	key := strings.TrimSpace(ev.Incumbent.Key)
	if key == "" {
		return decision.Record{}, errString("decide: incumbent key is required")
	}

	shadow := map[string]any{
		"correlation_id": corr,
		"plane":          qdef.Plane,
		"question":       qid,
		"question_def":   qdef,
		"state":          ev.State,
		"incumbent":      ev.Incumbent,
	}
	if ev.JEV != nil {
		shadow["jev"] = *ev.JEV
	}
	reasoning := "incumbent=" + ev.Incumbent.Method
	if ev.Incumbent.Threshold > 0 {
		reasoning += " threshold=" + trimFloat(ev.Incumbent.Threshold)
	}
	if ev.JEV == nil {
		reasoning += "; jev=pending"
	} else {
		reasoning += "; jev=" + ev.JEV.Backend
	}

	conf := 0.0
	if m, ok := ev.Incumbent.Scores[key]; ok {
		conf = m
	}
	cat, actor := shadowCategoryActor(qdef.Plane)
	return decision.Record{
		DecisionKey: uuid.NewString(),
		Category:    cat,
		Scenario:    "jev shadow " + qdef.Plane + "/" + qid + ": " + scenarioHint(ev.State),
		Reasoning:   reasoning,
		Outcome:     "q=" + qid + ";k=" + key,
		Confidence:  &conf,
		ActorType:   decision.ActorSystem,
		ActorKey:    actor,
		SourceRef: decision.SourceRef{
			SessionID: strings.TrimSpace(ev.SessionID),
		},
		Metadata:    map[string]any{"shadow": shadow},
		WorkspaceID: strings.TrimSpace(ev.WorkspaceID),
	}, nil
}

// shadowCategoryActor maps plane → M80 category + actor_key.
// Memory planes keep CategoryMemoryShadow (Phase 0 harvest compatibility);
// tool/skill/mcp land in CategoryJEVShadow (Phase 1).
func shadowCategoryActor(plane string) (decision.Category, string) {
	switch strings.TrimSpace(plane) {
	case "tool", "skill", "mcp":
		return decision.CategoryJEVShadow, "system:jev_shadow"
	default:
		return decision.CategoryMemoryShadow, "system:mem_shadow"
	}
}

// scenarioHint renders one short state hint for the audit index. It prefers
// the first registered slot present in state; empty state yields a bare tag.
func scenarioHint(state map[string]any) string {
	const maxHint = 120
	for _, slot := range []string{"user_message", "query", "statement", "task"} {
		if v, ok := state[slot]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				rs := []rune(strings.TrimSpace(s))
				if len(rs) > maxHint {
					rs = rs[:maxHint]
				}
				return string(rs)
			}
		}
	}
	return "(no slot text)"
}

func trimFloat(f float64) string {
	// Margins live in [0,1]; 4 decimals exceed the routers float32
	// precision by orders of magnitude.
	s := strconv.FormatFloat(f, 'f', 4, 64)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}

type errString string

func (e errString) Error() string { return string(e) }
