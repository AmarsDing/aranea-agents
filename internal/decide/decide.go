// Package decide hosts the JEV (System-One) decision-model integration
// contract: a narrow Decider backend interface, the question/state shapes
// shared with the official training pipeline (row-level JSONL of
// {state, questions, gold}), and the Phase-0 shadow audit recorder that
// double-records incumbent decisions with their full state into the M80
// decision layer (biz/decision, category memory_shadow) for gold harvesting
// and A/B evaluation.
//
// Data flow stays one-way per the M80 iron rule: decide package -> Collector
// -> decision_records audit surface. No module reads shadow records for
// business judgment. The whole chain is gated at the wiring site
// (ARANEA_MEM_SHADOW, default off); a nil Recorder is always safe.
package decide

import "context"

// Question is one systemone-style decision question, mirroring the
// questionsets.json contract (test/jev-plan/p0-finetune/questionsets.json).
// Type is choice | score | noul (laya QTYPES); shadow questions are all
// choice in Phase 0. Criteria maps option key -> neutral-key rubric text
// (red line: noul-style binary choices use neutral keys, never true/false).
type Question struct {
	ID           string            `json:"id"`
	Plane        string            `json:"plane"`
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
	Slots        []string          `json:"slots"`
}

// Answer is one answered question: a probability distribution over option
// keys plus calibration telemetry. Backend identifies the decision model
// (laya-multilingual / jev-style / vir-incumbent); Epoch binds the answer to
// the calibration artifacts of that model generation (P13: rolling back a
// model must roll back its calibration; P20: metrics bucket by
// embedder-epoch x decision-epoch).
type Answer struct {
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
	LatencyMS     int64              `json:"latency_ms,omitempty"`
	Backend       string             `json:"backend"`
	Epoch         string             `json:"epoch,omitempty"`
}

// Decider is the narrow backend contract (systemone request shape): one state
// plus a batch of questions merged into a single forward pass per turn budget
// discipline (maximum_jev_calls). Backends must degrade: unreachable -> caller
// keeps incumbent behavior; low confidence -> conservative branch.
type Decider interface {
	Decide(ctx context.Context, state map[string]any, questions []Question) (map[string]Answer, error)
}

// virShadowQuestions transcribes the three VIR-shadowable question defs from
// questionsets.json 1:1 (transcribed 2026-10-28; re-transcribe if the JSON
// side changes). These are the Phase-0 shadow hookup points: route_mode on
// the read path (applyEvalQueryModes), correction_gate / forget_gate on the
// write path (supersedeContradictions). Gold sources per questionsets:
// fixtures/routing_v1.jsonl + fixtures/write_governance_v1.jsonl (silver);
// shadow harvest promotes to gold.
func virShadowQuestions() map[string]Question {
	return map[string]Question{
		"route_mode": {
			ID:    "route_mode",
			Plane: "read",
			Type:  "choice",
			Instructions: "Which retrieval plan best fits answering `user_message` " +
				"from stored memories about the user?",
			Criteria: map[string]string{
				"direct_lookup": "One specific stored fact such as a date, number, name or preference is enough; no ordering, counting, listing or linking is needed.",
				"chronological": "The answer needs two or more events in time order, or asks about sequence, first/last or before/after.",
				"aggregate":     "The answer needs a count, total, summary or overall pattern across many stored memories.",
				"enumerative":   "The answer needs an exhaustive list of distinct remembered items such as names, places or possessions.",
				"relational":    "The answer needs relationships between people, or evidence and chains of events connecting memories.",
				"trend":         "The answer needs a pattern over repeated numeric readings, or an extrapolation or forecast.",
				"no_recall":     "Chit-chat or general world knowledge; no stored memory is needed.",
			},
			Slots: []string{"user_message"},
		},
		"correction_gate": {
			ID:    "correction_gate",
			Plane: "write",
			Type:  "choice",
			Instructions: "Does `user_message` retract or change a fact or preference " +
				"the user stated before, so the stored value should be updated?",
			Criteria: map[string]string{
				"update_fact": "The user says an earlier value is wrong, replaced or no longer true, and supplies the new value.",
				"keep_as_is":  "The user reaffirms the existing value or adds no retraction; nothing previously stated is withdrawn.",
			},
			Slots: []string{"user_message"},
		},
		"forget_gate": {
			ID:    "forget_gate",
			Plane: "write",
			Type:  "choice",
			Instructions: "Does `user_message` ask the assistant to erase or stop " +
				"retaining a stored memory?",
			Criteria: map[string]string{
				"erase":  "An explicit request to forget, delete or stop keeping specific past information.",
				"retain": "No deletion request; mentions of remembering, unforgettable events or requests to keep a memory must not be erased.",
			},
			Slots: []string{"user_message"},
		},
	}
}

// QuestionDef returns the shadow question definition for one question id,
// or false when the id is not a registered shadow question.
func QuestionDef(id string) (Question, bool) {
	q, ok := virShadowQuestions()[id]
	return q, ok
}
