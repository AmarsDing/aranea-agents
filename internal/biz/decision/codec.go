package decision

import "encoding/json"

// recordPayload is the JSON wire form stored in decision_record_outbox.payload
// and mirrored into decision_records columns by the data layer. Keep field
// names stable: replay decodes rows written by older binaries.
type recordPayload struct {
	DecisionKey      string         `json:"decision_key"`
	Category         string         `json:"category"`
	Scenario         string         `json:"scenario"`
	Reasoning        string         `json:"reasoning"`
	Outcome          string         `json:"outcome"`
	Confidence       *float64       `json:"confidence,omitempty"`
	ActorType        string         `json:"actor_type"`
	ActorKey         string         `json:"actor_key"`
	ParentDecisionID *int64         `json:"parent_decision_id,omitempty"`
	RelatedEntities  []EntityRef    `json:"related_entities,omitempty"`
	SourceRef        SourceRef      `json:"source_ref,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	WorkspaceID      string         `json:"workspace_id,omitempty"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at"`
}

func encodeRecord(r Record) ([]byte, error) {
	return json.Marshal(recordPayload{
		DecisionKey:      r.DecisionKey,
		Category:         string(r.Category),
		Scenario:         r.Scenario,
		Reasoning:        r.Reasoning,
		Outcome:          r.Outcome,
		Confidence:       r.Confidence,
		ActorType:        string(r.ActorType),
		ActorKey:         r.ActorKey,
		ParentDecisionID: r.ParentDecisionID,
		RelatedEntities:  r.RelatedEntities,
		SourceRef:        r.SourceRef,
		Metadata:         r.Metadata,
		WorkspaceID:      r.WorkspaceID,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	})
}

func decodeRecord(raw []byte) (Record, error) {
	var p recordPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Record{}, err
	}
	return Record{
		DecisionKey:      p.DecisionKey,
		Category:         Category(p.Category),
		Scenario:         p.Scenario,
		Reasoning:        p.Reasoning,
		Outcome:          p.Outcome,
		Confidence:       p.Confidence,
		ActorType:        ActorType(p.ActorType),
		ActorKey:         p.ActorKey,
		ParentDecisionID: p.ParentDecisionID,
		RelatedEntities:  p.RelatedEntities,
		SourceRef:        p.SourceRef,
		Metadata:         p.Metadata,
		WorkspaceID:      p.WorkspaceID,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}, nil
}
