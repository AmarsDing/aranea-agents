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

type L4EntitySnapshot struct {
	ID             string
	Name           string
	NameNormalized string
	Confidence     float64
	MetadataJSON   string
}

type L4GraphWriter interface {
	WriteFromUserText(ctx context.Context, agentID, userID, text string) (int, error)
	RunDecay(ctx context.Context, agentID string)
}

type L4GraphRepo interface {
	UpsertEntity(ctx context.Context, params L4EntityWrite) error
	UpsertRelation(ctx context.Context, params L4RelationWrite) error
	GetEntityByScopeKey(ctx context.Context, scopeType, scopeID, entityType, nameNormalized string) (L4EntitySnapshot, bool, error)
	GetFirstEntityByType(ctx context.Context, scopeType, scopeID, entityType string) (L4EntitySnapshot, bool, error)
	ApplyConfidenceDecay(ctx context.Context, scopeType, scopeID, olderThanRFC3339 string, factor float64) (int64, error)
}
