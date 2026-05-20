package memory

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aranea-agents/internal/data/sessionmemory"
)

var (
	l4NamePattern      = regexp.MustCompile(`(?i)(?:my name is|I(?:'m| am) called)\s+([A-Za-z][A-Za-z0-9 _-]{0,48})`)
	l4PreferencePattern = regexp.MustCompile(`(?i)I\s+(?:prefer|like|love)\s+([^.!?\n]{2,120})`)
)

// L4GraphWriter upserts agent-scoped knowledge-graph entities from extracted user text.
type L4GraphWriter struct {
	store *sessionmemory.Store
}

func NewL4GraphWriter(store *sessionmemory.Store) *L4GraphWriter {
	if store == nil {
		return nil
	}
	return &L4GraphWriter{store: store}
}

func userProfileEntityID(agentID string) string {
	return fmt.Sprintf("l4-user-%s", strings.TrimSpace(agentID))
}

func (w *L4GraphWriter) WriteFromUserText(ctx context.Context, agentID, userID, text string) (int, error) {
	if w == nil || w.store == nil {
		return 0, nil
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return 0, nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	anchorID := userProfileEntityID(agentID)
	written := 0

	_ = w.store.UpsertEventEntity(ctx, sessionmemory.EventEntityParams{
		ID:               anchorID,
		ScopeType:        "agent",
		ScopeID:          agentID,
		UserID:           userID,
		EntityType:       "user_profile",
		Name:             "User profile",
		NameNormalized:   "user profile",
		Description:      "Consolidated user knowledge for this agent",
		Importance:       0.8,
		Confidence:       0.9,
		CreatedAtRFC3339: now,
		UpdatedAtRFC3339: now,
	})

	if m := l4NamePattern.FindStringSubmatch(text); len(m) > 1 {
		name := strings.TrimSpace(m[1])
		nameNorm := strings.ToLower(name)
		existing, _, _ := w.store.GetEntityByScopeKey(ctx, "agent", agentID, "person", nameNorm)
		if existing.ID == "" {
			if prior, ok, _ := w.store.GetFirstEntityByType(ctx, "agent", agentID, "person"); ok {
				existing = prior
			}
		}
		prepared, conflict := preparePersonUpsert(existing, name, text)
		entID := fmt.Sprintf("l4-person-%s-%s", agentID, slugEntityName(name))
		if existing.ID != "" {
			entID = existing.ID
		}
		meta := mergeConflictMetadata(prepared.MetadataJSON, conflict, existing.Name)
		if err := w.store.UpsertEventEntity(ctx, sessionmemory.EventEntityParams{
			ID:               entID,
			ScopeType:        "agent",
			ScopeID:          agentID,
			UserID:           userID,
			EntityType:       "person",
			Name:             prepared.Name,
			NameNormalized:   prepared.NameNormalized,
			Description:      prepared.Description,
			Importance:       0.85,
			Confidence:       prepared.Confidence,
			MetadataJSON:     meta,
			CreatedAtRFC3339: now,
			UpdatedAtRFC3339: now,
		}); err == nil {
			_ = w.store.UpsertRelation(ctx, sessionmemory.RelationParams{
				ScopeType:    "agent",
				ScopeID:      agentID,
				SourceID:     anchorID,
				TargetID:     entID,
				RelationType: "knows_as",
				Weight:       1.0,
				Confidence:   prepared.Confidence,
			})
			cascade := cascadeProfileTouch(anchorID, userID, agentID, name, now)
			_ = w.store.UpsertEventEntity(ctx, cascade)
			written++
		}
	}

	if m := l4PreferencePattern.FindStringSubmatch(text); len(m) > 1 {
		pref := strings.TrimSpace(m[1])
		entID := fmt.Sprintf("l4-pref-%s-%s", agentID, slugEntityName(pref))
		if err := w.store.UpsertEventEntity(ctx, sessionmemory.EventEntityParams{
			ID:               entID,
			ScopeType:        "agent",
			ScopeID:          agentID,
			UserID:           userID,
			EntityType:       "preference",
			Name:             truncate(pref, 80),
			NameNormalized:   strings.ToLower(truncate(pref, 80)),
			Description:      text,
			Importance:       0.7,
			Confidence:       0.7,
			MetadataJSON:     `{"source":"auto_memory"}`,
			CreatedAtRFC3339: now,
			UpdatedAtRFC3339: now,
		}); err == nil {
			_ = w.store.UpsertRelation(ctx, sessionmemory.RelationParams{
				ScopeType:    "agent",
				ScopeID:      agentID,
				SourceID:     anchorID,
				TargetID:     entID,
				RelationType: "prefers",
				Weight:       0.9,
				Confidence:   0.7,
			})
			written++
		}
	}
	w.runDecay(ctx, agentID)
	return written, nil
}

func slugEntityName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
