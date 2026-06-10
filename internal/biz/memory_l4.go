package biz

import "context"

type L4EntityWrite struct {
	ID             string
	ScopeType      string
	ScopeID        string
	UserID         string
	EntityType     string
	Name           string
	NameNormalized string
	Description    string
	Importance     float64
	Confidence     float64
	MetadataJSON   string
}

type L4RelationWrite struct {
	ScopeType    string
	ScopeID      string
	SourceID     string
	TargetID     string
	RelationType string
	Weight       float64
	Confidence   float64
}

// L4Entity represents a full entity read from the knowledge graph.
type L4Entity struct {
	ID             string
	ScopeType      string
	ScopeID        string
	UserID         string
	EntityType     string
	Name           string
	NameNormalized string
	Description    string
	Importance     float64
	Confidence     float64
	MetadataJSON   string
}

// L4Relation represents a relation read from the knowledge graph.
type L4Relation struct {
	ID           string
	ScopeType    string
	ScopeID      string
	SourceID     string
	TargetID     string
	RelationType string
	Weight       float64
	Confidence   float64
	MetadataJSON string
}

type L4EntitySnapshot struct {
	ID             string
	Name           string
	NameNormalized string
	Confidence     float64
	MetadataJSON   string
}

type ReinforcementSignal string

const (
	ReinforcementHit       ReinforcementSignal = "hit"
	ReinforcementConfirmed ReinforcementSignal = "confirmed"
	ReinforcementRefuted   ReinforcementSignal = "refuted"
	ReinforcementEdited    ReinforcementSignal = "edited"
)

type L4DecayConfig struct {
	HalfLifeDays map[string]float64
	Alpha        float64
}

func DefaultL4DecayConfig() L4DecayConfig {
	return L4DecayConfig{
		HalfLifeDays: map[string]float64{
			"person_core":  180,
			"person":       60,
			"place":        365,
			"preference":   90,
			"event":        14,
			"concept":      270,
			"user_profile": 365,
		},
		Alpha: 0.15,
	}
}

func (c L4DecayConfig) HalfLifeForEntityType(entityType string, isCore bool) float64 {
	if isCore {
		if hl, ok := c.HalfLifeDays["person_core"]; ok {
			return hl
		}
	}
	if hl, ok := c.HalfLifeDays[entityType]; ok {
		return hl
	}
	return 60
}

type L4DecayResult struct {
	Decayed  int
	Archived int
}

type L4GraphWriter interface {
	WriteFromUserText(ctx context.Context, agentID, userID, text string) (int, error)
	RunDecay(ctx context.Context, agentID string)
	RunDecayWithConfig(ctx context.Context, agentID string, cfg L4DecayConfig) L4DecayResult
	RecordEntityReinforcement(ctx context.Context, entityID string, signal ReinforcementSignal, source string) error
}

type L4EntityReader interface {
	GetEntityByScopeKey(ctx context.Context, scopeType, scopeID, entityType, nameNormalized string) (L4EntitySnapshot, bool, error)
	GetFirstEntityByType(ctx context.Context, scopeType, scopeID, entityType string) (L4EntitySnapshot, bool, error)
	GetEntityRelations(ctx context.Context, entityID string) ([]L4Relation, error)
	GetEntitiesByType(ctx context.Context, scope, entityType string) ([]L4Entity, error)
	SearchEntitiesByName(ctx context.Context, scope, nameQuery string, limit int) ([]L4Entity, error)
}

type L4EntityWriter interface {
	UpsertEntity(ctx context.Context, params L4EntityWrite) error
	UpsertRelation(ctx context.Context, params L4RelationWrite) error
}

type L4DecayWriter interface {
	ApplyConfidenceDecay(ctx context.Context, scopeType, scopeID, olderThanRFC3339 string, factor float64) (int64, error)
	ApplyBusinessConfidenceDecay(ctx context.Context, scopeType, scopeID string, cfg L4DecayConfig, nowUnixMs int64) (int64, error)
	ArchiveLowConfidenceEntities(ctx context.Context, scopeType, scopeID string, threshold float64) (int64, error)
	RecordEntityReinforcement(ctx context.Context, entityID string, signal ReinforcementSignal, source string) error
	GetRecentReinforcementCounts(ctx context.Context, scopeType, scopeID string, windowDays int) (map[string]int, error)
}

type L4GraphRepo interface {
	L4EntityReader
	L4EntityWriter
	L4DecayWriter
}
