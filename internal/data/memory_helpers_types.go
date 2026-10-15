package data

// memoryFactUpsert captures inputs for upsertFactRow.
type memoryFactUpsert struct {
	ID                    string
	ScopeType             string
	ScopeID               string
	WorkspaceID           string
	UserID                string
	TeamID                string
	AgentID               string
	Statement             string
	Fingerprint           string
	DetailsMarkdown       string
	FactKind              string
	TagsJSON              string
	Confidence            float64
	Importance            float64
	UseCount              int32
	HitCount              int32
	PositiveFeedbackCount int32
	NegativeFeedbackCount int32
	ConflictCount         int32
	SourceKind            string
	SourceEpisodeID       string
	SourceSessionID       string
	SourceMessageID       string
	SourceExternal        string
	Version               int32
	Status                string
	PIIFlag               bool
	RedactedStatement     string
	PIIPolicy             string
	PIITypes              []string
	QualityScore          float64
	MetadataJSON          string
	CreatedAt             string
	UpdatedAt             string
}

// episodeInsert captures inputs for insertEpisodeRow.
type episodeInsert struct {
	ID                  string
	SessionID           string
	AgentID             string
	UserID              string
	Title               string
	OutcomeSummary      string
	Importance          float64
	MessageCount        int
	ConsolidatedL3      int
	ConsolidationStatus string
	MetadataJSON        string
	SourceSessionID     string
	L1TaskID            string
	L1SnapshotJSON      string
	KeyDecisionsJSON    string
	KeyArtifactsJSON    string
}

// cascadeProposalInsert captures one L4 cascade review row.
type cascadeProposalInsert struct {
	AgentID           string
	WorkspaceID       string
	TriggerEntityID   string
	TriggerEntityName string
	TriggerAttribute  string
	OldValue          string
	NewValue          string
	AffectedJSON      string
	RiskLevel         string
	Rationale         string
	MetadataJSON      string
	ExpiresAt         string
}

// cascadeSagaStep is one step in a cascade saga.
type cascadeSagaStep struct {
	ID          int64  `json:"id"`
	ProposalID  string `json:"proposal_id"`
	StepIndex   int    `json:"step_index"`
	StepName    string `json:"step_name"`
	State       string `json:"state"`
	IsCritical  bool   `json:"is_critical"`
	Attempts    int    `json:"attempts"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	PayloadJSON string `json:"payload_json,omitempty"`
	ResultJSON  string `json:"result_json,omitempty"`
	Error       string `json:"error,omitempty"`
}

// evolutionEventInsert is service-layer input for insertEvolutionEventRow.
type evolutionEventInsert struct {
	AgentID       string
	WorkspaceID   string
	EventKind     string
	Kind          string
	TargetField   string
	Reason        string
	TriggerKind   string
	TriggerSource string
	MetadataJSON  string
}

// eventEntityParams is an L4 entity upsert payload.
type eventEntityParams struct {
	ID               string
	ScopeType        string
	ScopeID          string
	WorkspaceID      string
	UserID           string
	EntityType       string
	Name             string
	NameNormalized   string
	Description      string
	Importance       float64
	Confidence       float64
	UseCount         int
	MetadataJSON     string
	CreatedAtRFC3339 string
	UpdatedAtRFC3339 string
}

// relationParams is an L4 graph edge upsert payload.
type relationParams struct {
	ID               string
	ScopeType        string
	ScopeID          string
	WorkspaceID      string
	SourceID         string
	TargetID         string
	RelationType     string
	Weight           float64
	Confidence       float64
	MetadataJSON     string
	ValidFromRFC3339 string
	ValidToRFC3339   string
	CreatedAtRFC3339 string
	UpdatedAtRFC3339 string
}

// memoryActionLogInsert records a policy-level memory mutation (Ledger audit trail).
type memoryActionLogInsert struct {
	Action         string
	TargetKind     string
	TargetID       string
	Reason         string
	PolicyVersion  string
	TurnID         string
	SourceEventIDs []string
	MetadataJSON   string
}

// entitySnapshot is a minimal row for L4 governance decisions.
type entitySnapshot struct {
	ID             string
	Name           string
	NameNormalized string
	Confidence     float64
	MetadataJSON   string
	UpdatedAt      string
}

// episodeEmbedCandidate is one episode row pending vector embedding.
type episodeEmbedCandidate struct {
	ID      string
	AgentID string
	Title   string
	Summary string
}

// consolidateWriteResult holds JSON rows written atomically by upsertFactsAndEpisodeBatch.
type consolidateWriteResult struct {
	FactRows     [][]byte
	EpisodeRow   []byte
	FactsWritten int
}
