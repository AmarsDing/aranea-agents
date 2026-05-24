package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aranea-agents/pkg/strutil"
)

var (
	l4NamePattern      = regexp.MustCompile(`(?i)(?:my name is|I(?:'m| am) called)\s+([A-Za-z][A-Za-z0-9 _-]{0,48})`)
	l4PreferencePattern = regexp.MustCompile(`(?i)I\s+(?:prefer|like|love)\s+([^.!?\n]{2,120})`)
)

const (
	l4DecayAfter  = 30 * 24 * time.Hour
	l4DecayFactor = 0.92
	l4ConflictMeta = `{"source":"auto_memory","conflict":true}`
	l4CascadeMeta  = `{"source":"auto_memory","cascade":true}`
)

type L4GraphUsecase struct {
	repo    L4GraphRepo
	cascade *L4CascadeUsecase
}

func NewL4GraphUsecase(repo L4GraphRepo) *L4GraphUsecase {
	if repo == nil {
		return nil
	}
	return &L4GraphUsecase{repo: repo}
}

func (uc *L4GraphUsecase) SetCascade(c *L4CascadeUsecase) {
	if uc != nil {
		uc.cascade = c
	}
}

func (uc *L4GraphUsecase) WriteFromUserText(ctx context.Context, agentID, userID, text string) (int, error) {
	if uc == nil || uc.repo == nil {
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

	_ = uc.repo.UpsertEntity(ctx, L4EntityWrite{
		ID:             anchorID,
		ScopeType:      "agent",
		ScopeID:        agentID,
		UserID:         userID,
		EntityType:     "user_profile",
		Name:           "User profile",
		NameNormalized: "user profile",
		Description:    "Consolidated user knowledge for this agent",
		Importance:     0.8,
		Confidence:     0.9,
	})

	if m := l4NamePattern.FindStringSubmatch(text); len(m) > 1 {
		name := strings.TrimSpace(m[1])
		nameNorm := strings.ToLower(name)
		existing, _, _ := uc.repo.GetEntityByScopeKey(ctx, "agent", agentID, "person", nameNorm)
		if existing.ID == "" {
			if prior, ok, _ := uc.repo.GetFirstEntityByType(ctx, "agent", agentID, "person"); ok {
				existing = prior
			}
		}
		prepared, conflict := uc.preparePersonUpsert(existing, name, text)
		entID := fmt.Sprintf("l4-person-%s-%s", agentID, slugEntityName(name))
		if existing.ID != "" {
			entID = existing.ID
		}
		profileName := name
		if conflict {
			if uc.cascade != nil {
				_ = uc.cascade.ProposeNameConflict(ctx, agentID, entID, existing.Name, name)
			}
			// Gate: keep authoritative name until cascade proposal is approved.
			prepared.Name = existing.Name
			prepared.NameNormalized = strings.ToLower(strings.TrimSpace(existing.Name))
			profileName = existing.Name
		}
		meta := mergeConflictMetadata(prepared.MetadataJSON, conflict, existing.Name, name)
		if err := uc.repo.UpsertEntity(ctx, L4EntityWrite{
			ID:             entID,
			ScopeType:      "agent",
			ScopeID:        agentID,
			UserID:         userID,
			EntityType:     "person",
			Name:           prepared.Name,
			NameNormalized: prepared.NameNormalized,
			Description:    prepared.Description,
			Importance:     0.85,
			Confidence:     prepared.Confidence,
			MetadataJSON:   meta,
		}); err == nil {
			_ = uc.repo.UpsertRelation(ctx, L4RelationWrite{
				ScopeType:    "agent",
				ScopeID:      agentID,
				SourceID:     anchorID,
				TargetID:     entID,
				RelationType: "knows_as",
				Weight:       1.0,
				Confidence:   prepared.Confidence,
			})
			cascade := uc.cascadeProfileTouch(anchorID, userID, agentID, profileName, name, conflict, now)
			_ = uc.repo.UpsertEntity(ctx, cascade)
			written++
		}
	}

	if m := l4PreferencePattern.FindStringSubmatch(text); len(m) > 1 {
		pref := strings.TrimSpace(m[1])
		entID := fmt.Sprintf("l4-pref-%s-%s", agentID, slugEntityName(pref))
		if err := uc.repo.UpsertEntity(ctx, L4EntityWrite{
			ID:             entID,
			ScopeType:      "agent",
			ScopeID:        agentID,
			UserID:         userID,
			EntityType:     "preference",
			Name:           strutil.TruncateBytes(pref, 80),
		NameNormalized: strings.ToLower(strutil.TruncateBytes(pref, 80)),
			Description:    text,
			Importance:     0.7,
			Confidence:     0.7,
			MetadataJSON:   `{"source":"auto_memory"}`,
		}); err == nil {
			_ = uc.repo.UpsertRelation(ctx, L4RelationWrite{
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

	uc.runDecay(ctx, agentID)
	return written, nil
}

func (uc *L4GraphUsecase) preparePersonUpsert(existing L4EntitySnapshot, newName, description string) (L4EntityWrite, bool) {
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
	return L4EntityWrite{
		Name:           newName,
		NameNormalized: nameNorm,
		Description:    description,
		Confidence:     conf,
		MetadataJSON:   meta,
	}, conflict
}

func (uc *L4GraphUsecase) cascadeProfileTouch(anchorID, userID, agentID, personName, pendingName string, conflict bool, now string) L4EntityWrite {
	desc := "Consolidated user knowledge for this agent"
	if strings.TrimSpace(personName) != "" {
		desc = "Profile includes: " + personName
	}
	meta := l4CascadeMeta
	if conflict && strings.TrimSpace(pendingName) != "" {
		desc = fmt.Sprintf("Profile includes: %s (pending name change to %s)", personName, pendingName)
		meta = `{"source":"auto_memory","cascade":true,"pending_name_review":true}`
	}
	_ = now
	return L4EntityWrite{
		ID:             anchorID,
		ScopeType:      "agent",
		ScopeID:        agentID,
		UserID:         userID,
		EntityType:     "user_profile",
		Name:           "User profile",
		NameNormalized: "user profile",
		Description:    desc,
		Importance:     0.8,
		Confidence:     0.9,
		MetadataJSON:   meta,
	}
}

func (uc *L4GraphUsecase) runDecay(ctx context.Context, agentID string) {
	if uc == nil || uc.repo == nil {
		return
	}
	cutoff := time.Now().UTC().Add(-l4DecayAfter).Format(time.RFC3339)
	_, _ = uc.repo.ApplyConfidenceDecay(ctx, "agent", agentID, cutoff, l4DecayFactor)
}

func userProfileEntityID(agentID string) string {
	return fmt.Sprintf("l4-user-%s", strings.TrimSpace(agentID))
}

func slugEntityName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

func mergeConflictMetadata(base string, conflict bool, priorName, pendingName string) string {
	if !conflict {
		return base
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(base), &m); err != nil || m == nil {
		m = map[string]any{}
	}
	m["conflict"] = true
	m["prior_name"] = priorName
	m["pending_name"] = pendingName
	m["gate"] = "cascade_proposal"
	b, _ := json.Marshal(m)
	return string(b)
}
