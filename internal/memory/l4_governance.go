package memory

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/data/sessionmemory"
)

const (
	l4DecayAfter     = 30 * 24 * time.Hour
	l4DecayFactor    = 0.92
	l4ConflictMeta   = `{"source":"auto_memory","conflict":true}`
	l4CascadeMeta    = `{"source":"auto_memory","cascade":true}`
)

// preparePersonUpsert applies conflict detection and metadata for person entities.
func preparePersonUpsert(existing sessionmemory.EntitySnapshot, newName, description string) (sessionmemory.EventEntityParams, bool) {
	newName = strings.TrimSpace(newName)
	nameNorm := strings.ToLower(newName)
	conflict := false
	meta := `{"source":"auto_memory"}`
	if existing.ID != "" && !strings.EqualFold(strings.TrimSpace(existing.Name), newName) {
		conflict = true
		meta = l4ConflictMeta
	}
	conf := 0.75
	if existing.ID != "" && !conflict {
		conf = existing.Confidence
		if conf < 0.95 {
			conf += 0.05
		}
	}
	return sessionmemory.EventEntityParams{
		Name:           newName,
		NameNormalized: nameNorm,
		Description:    description,
		Confidence:     conf,
		MetadataJSON:   meta,
	}, conflict
}

// cascadeProfileTouch refreshes anchor profile when a person entity changes.
func cascadeProfileTouch(anchorID, userID, agentID, personName string, now string) sessionmemory.EventEntityParams {
	desc := "Consolidated user knowledge for this agent"
	if strings.TrimSpace(personName) != "" {
		desc = "Profile includes: " + personName
	}
	return sessionmemory.EventEntityParams{
		ID:               anchorID,
		ScopeType:        "agent",
		ScopeID:          agentID,
		UserID:           userID,
		EntityType:       "user_profile",
		Name:             "User profile",
		NameNormalized:   "user profile",
		Description:      desc,
		Importance:       0.8,
		Confidence:       0.9,
		MetadataJSON:     l4CascadeMeta,
		CreatedAtRFC3339: now,
		UpdatedAtRFC3339: now,
	}
}

func (w *L4GraphWriter) runDecay(ctx context.Context, agentID string) {
	if w == nil || w.store == nil {
		return
	}
	cutoff := time.Now().UTC().Add(-l4DecayAfter).Format(time.RFC3339)
	_, _ = w.store.ApplyConfidenceDecay(ctx, "agent", agentID, cutoff, l4DecayFactor)
}

func mergeConflictMetadata(base string, conflict bool, priorName string) string {
	if !conflict {
		return base
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(base), &m); err != nil || m == nil {
		m = map[string]any{}
	}
	m["conflict"] = true
	m["prior_name"] = priorName
	b, _ := json.Marshal(m)
	return string(b)
}
