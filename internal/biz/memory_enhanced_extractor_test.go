package biz

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// pathBMockRepo is the mock L4 repo used by PathBExtractor tests. It
// records every UpsertEntity / UpsertRelation call so the test can
// assert identity preservation end-to-end.
type pathBMockRepo struct {
	entities  []L4EntityWrite
	relations []L4RelationWrite

	// resolveFn lets a test inject the read response (snapshot, ok,
	// err) for a given (scope, type, name_normalized) tuple. If nil,
	// the mock behaves as "no existing entity".
	resolveFn func(scopeType, scopeID, entityType, nameNormalized string) (L4EntitySnapshot, bool, error)
}

func (m *pathBMockRepo) GetEntityByScopeKey(_ context.Context, scopeType, scopeID, entityType, nameNormalized string) (L4EntitySnapshot, bool, error) {
	if m.resolveFn == nil {
		return L4EntitySnapshot{}, false, nil
	}
	return m.resolveFn(scopeType, scopeID, entityType, nameNormalized)
}

func (m *pathBMockRepo) UpsertEntity(_ context.Context, params L4EntityWrite) error {
	m.entities = append(m.entities, params)
	return nil
}

func (m *pathBMockRepo) UpsertRelation(_ context.Context, params L4RelationWrite) error {
	m.relations = append(m.relations, params)
	return nil
}

func TestPathBExtractor_ReusesExistingID(t *testing.T) {
	// Pre-condition: an entity for (agent1, "person", "alice") already
	// exists with a known UUID. The extractor MUST reuse that ID
	// instead of minting a new one — otherwise alias rows proliferate
	// and cascade compensation loses its anchor.
	const wantID = "00000000-0000-0000-0000-000000000001"
	repo := &pathBMockRepo{
		resolveFn: func(_, _, entityType, nameNormalized string) (L4EntitySnapshot, bool, error) {
			if entityType == "person" && nameNormalized == "alice" {
				return L4EntitySnapshot{ID: wantID, Name: "Alice", NameNormalized: "alice", Confidence: 0.9}, true, nil
			}
			return L4EntitySnapshot{}, false, nil
		},
	}
	pe := NewPathBExtractor(nil, repo, loggateway.NewNoop())
	pe.WriteEntities(context.Background(), "agent1", "user1", &EnhancedExtractionResult{
		Entities: []ExtractedEntity{
			{Name: "Alice", EntityType: "person", Description: "user mentioned alice", Confidence: 0.9},
		},
	})
	if len(repo.entities) != 1 {
		t.Fatalf("expected 1 entity upsert, got %d", len(repo.entities))
	}
	if got := repo.entities[0].ID; got != wantID {
		t.Fatalf("entity id = %q, want %q (existing ID must be reused)", got, wantID)
	}
}

func TestPathBExtractor_MintsUUIDForNewEntity(t *testing.T) {
	// No pre-existing entity; extractor must generate a fresh UUID.
	repo := &pathBMockRepo{}
	pe := NewPathBExtractor(nil, repo, loggateway.NewNoop())
	pe.WriteEntities(context.Background(), "agent1", "user1", &EnhancedExtractionResult{
		Entities: []ExtractedEntity{
			{Name: "Bob", EntityType: "person", Confidence: 0.7},
		},
	})
	if len(repo.entities) != 1 {
		t.Fatalf("expected 1 entity upsert, got %d", len(repo.entities))
	}
	id := repo.entities[0].ID
	if id == "" {
		t.Fatal("expected non-empty entity id (fresh UUID)")
	}
	// UUID v4 is 36 chars (8-4-4-4-12). Anything else is a regression
	// to the slug-based ID generator.
	if len(id) != 36 || strings.Count(id, "-") != 4 {
		t.Fatalf("entity id %q is not a UUID-shaped value", id)
	}
}

func TestPathBExtractor_ReadFailureDegradesToNewID(t *testing.T) {
	// Read fails (e.g. transient DB outage). The extractor MUST NOT
	// abort the whole WriteEntities call: it logs the error and
	// proceeds with a fresh UUID so consolidation is not lost.
	repo := &pathBMockRepo{
		resolveFn: func(_, _, _, _ string) (L4EntitySnapshot, bool, error) {
			return L4EntitySnapshot{}, false, errors.New("simulated DB read failure")
		},
	}
	pe := NewPathBExtractor(nil, repo, loggateway.NewNoop())
	pe.WriteEntities(context.Background(), "agent1", "user1", &EnhancedExtractionResult{
		Entities: []ExtractedEntity{
			{Name: "Charlie", EntityType: "person", Confidence: 0.6},
		},
	})
	if len(repo.entities) != 1 {
		t.Fatalf("expected 1 entity upsert (best-effort after read failure), got %d", len(repo.entities))
	}
	if id := repo.entities[0].ID; len(id) != 36 {
		t.Fatalf("expected fresh UUID after read failure, got %q", id)
	}
}

func TestPathBExtractor_EmptyNameFallsBackToFreshUUID(t *testing.T) {
	// Empty display name bypasses the resolve step entirely. The
	// resulting entity row uses Name="" but still gets a UUID so the
	// write doesn't violate the PK constraint.
	repo := &pathBMockRepo{
		resolveFn: func(_, _, _, _ string) (L4EntitySnapshot, bool, error) {
			t.Fatal("resolveFn should not be called for empty name")
			return L4EntitySnapshot{}, false, nil
		},
	}
	pe := NewPathBExtractor(nil, repo, loggateway.NewNoop())
	pe.WriteEntities(context.Background(), "agent1", "user1", &EnhancedExtractionResult{
		Entities: []ExtractedEntity{
			{Name: "", EntityType: "person", Confidence: 0.5},
			{Name: "  ", EntityType: "person", Confidence: 0.5},
		},
	})
	if len(repo.entities) != 0 {
		t.Fatalf("expected 0 entity upserts (empty names are skipped), got %d", len(repo.entities))
	}
}

func TestPathBExtractor_AgentIDRequired(t *testing.T) {
	// An empty agent_id is a programmer error in production callers,
	// but WriteEntities must not panic if it happens.
	repo := &pathBMockRepo{}
	pe := NewPathBExtractor(nil, repo, loggateway.NewNoop())
	pe.WriteEntities(context.Background(), "", "user1", &EnhancedExtractionResult{
		Entities: []ExtractedEntity{
			{Name: "Dora", EntityType: "person", Confidence: 0.6},
		},
	})
	if len(repo.entities) != 0 {
		t.Fatalf("expected 0 upserts when agent_id is empty, got %d", len(repo.entities))
	}
}

func TestPathBExtractor_RelationsReferenceCorrectIDs(t *testing.T) {
	// The inter-entity relations must reference the SAME IDs that
	// entity resolution chose (not some re-derived slug). This guards
	// against the BUG-07 regression where relations pointed at stale
	// slug IDs.
	const aliceID = "11111111-1111-1111-1111-111111111111"
	const projectID = "22222222-2222-2222-2222-222222222222"
	repo := &pathBMockRepo{
		resolveFn: func(_, _, entityType, nameNormalized string) (L4EntitySnapshot, bool, error) {
			switch nameNormalized {
			case "alice":
				return L4EntitySnapshot{ID: aliceID, Name: "Alice", NameNormalized: "alice"}, true, nil
			case "project atlas":
				return L4EntitySnapshot{ID: projectID, Name: "Project Atlas", NameNormalized: "project atlas"}, true, nil
			}
			return L4EntitySnapshot{}, false, nil
		},
	}
	pe := NewPathBExtractor(nil, repo, loggateway.NewNoop())
	pe.WriteEntities(context.Background(), "agent1", "user1", &EnhancedExtractionResult{
		Entities: []ExtractedEntity{
			{Name: "Alice", EntityType: "person"},
			{Name: "Project Atlas", EntityType: "project"},
		},
		Relations: []ExtractedRelation{
			{SourceEntity: "Alice", TargetEntity: "Project Atlas", RelationType: "works_on", Confidence: 0.8},
		},
	})
	if got := len(repo.relations); got < 1 {
		t.Fatalf("expected at least 1 inter-entity relation, got %d", got)
	}
	// Find the inter-entity relation and verify it points at the
	// authoritative IDs.
	var found bool
	for _, rel := range repo.relations {
		if rel.RelationType != "works_on" {
			continue
		}
		found = true
		if rel.SourceID != aliceID {
			t.Errorf("inter-entity relation SourceID = %q, want %q", rel.SourceID, aliceID)
		}
		if rel.TargetID != projectID {
			t.Errorf("inter-entity relation TargetID = %q, want %q", rel.TargetID, projectID)
		}
	}
	if !found {
		t.Fatal("inter-entity relation not written")
	}
}

func TestPathBExtractor_PreferenceRelationType(t *testing.T) {
	// "preference" entities must anchor via "prefers" relation, not
	// "knows_as" (the latter would be a semantic regression because
	// preferences don't represent people the agent knows).
	const prefID = "33333333-3333-3333-3333-333333333333"
	repo := &pathBMockRepo{
		resolveFn: func(_, _, entityType, nameNormalized string) (L4EntitySnapshot, bool, error) {
			if entityType == "preference" && nameNormalized == "dark mode" {
				return L4EntitySnapshot{ID: prefID, Name: "dark mode", NameNormalized: "dark mode"}, true, nil
			}
			return L4EntitySnapshot{}, false, nil
		},
	}
	pe := NewPathBExtractor(nil, repo, loggateway.NewNoop())
	pe.WriteEntities(context.Background(), "agent1", "user1", &EnhancedExtractionResult{
		Entities: []ExtractedEntity{
			{Name: "dark mode", EntityType: "preference"},
		},
	})
	var found bool
	for _, rel := range repo.relations {
		if rel.TargetID == prefID && rel.RelationType == "prefers" {
			found = true
		}
	}
	if !found {
		t.Fatal("preference entity should be linked via 'prefers' relation")
	}
}
