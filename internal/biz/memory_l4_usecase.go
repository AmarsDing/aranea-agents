package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"
)

var (
	l4NamePattern             = regexp.MustCompile(`(?i)(?:my name is|I(?:'m| am) called)\s+([A-Za-z][A-Za-z0-9 _-]{0,48})`)
	l4PreferencePattern       = regexp.MustCompile(`(?i)I\s+(?:prefer|like|love)\s+([^.!?\n]{2,120})`)
	l4ChineseNamePattern      = regexp.MustCompile(`(?:我叫|我的名字是|我是)\s*([^\s,.，。!！?？]{1,20})`)
	l4ChinesePreferencePattern = regexp.MustCompile(`(?:我喜欢|我偏好|我偏爱|我爱吃|我爱喝|我爱看|我爱听)\s*([^.!?\n，。！？]{2,80})`)
)

const (
	l4DecayAfter  = 30 * 24 * time.Hour
	l4DecayFactor = 0.92
	l4ConflictMeta = `{"source":"auto_memory","conflict":true}`
	l4CascadeMeta  = `{"source":"auto_memory","cascade":true}`

	l4AnchorImportance    = 0.8
	l4AnchorConfidence    = 0.9
	l4PersonImportance    = 0.85
	l4PrefImportance      = 0.7
	l4PrefConfidence      = 0.7
	l4PrefRelWeight       = 0.9
	l4PrefRelConfidence   = 0.7
	l4ArchiveThreshold    = 0.1
	l4ConflictBaseConf    = 0.75
	l4ConflictConfStep    = 0.05
	l4ConflictConfCap     = 0.95
	l4CascadeEntImportance = 0.85
	l4CascadeEntConfidence = 0.8
	l4CascadeTouchImportance = 0.5
	l4CascadeTouchConfidence = 0.7
)

type L4GraphUsecase struct {
	repo    L4GraphRepo
	cascade *L4CascadeUsecase
	lg      loggateway.Logger
}

func NewL4GraphUsecase(repo L4GraphRepo, lg loggateway.Logger) *L4GraphUsecase {
	if repo == nil {
		return nil
	}
	return &L4GraphUsecase{repo: repo, lg: lg}
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

	if err := uc.repo.UpsertEntity(ctx, L4EntityWrite{
		ID:             anchorID,
		ScopeType:      "agent",
		ScopeID:        agentID,
		UserID:         userID,
		EntityType:     "user_profile",
		Name:           "用户画像",
		NameNormalized: "user profile",
		Description:    "本 Agent 的用户知识汇总",
		Importance:     l4AnchorImportance,
		Confidence:     l4AnchorConfidence,
	}); err != nil {
		uc.lg.Warn("L4Graph: failed to upsert anchor entity", loggateway.StepID("memory.l4_fail"), loggateway.Str("anchor_id", anchorID), loggateway.Err(err))
	}

	// Extract name from English patterns first, then Chinese fallback.
	name := ""
	if m := l4NamePattern.FindStringSubmatch(text); len(m) > 1 {
		name = strings.TrimSpace(m[1])
	}
	if name == "" {
		if m := l4ChineseNamePattern.FindStringSubmatch(text); len(m) > 1 {
			name = strings.TrimSpace(m[1])
		}
	}
	if name != "" {
		nameNorm := strings.ToLower(name)
		existing, _, err := uc.repo.GetEntityByScopeKey(ctx, "agent", agentID, "person", nameNorm)
		if err != nil {
			uc.lg.Warn("L4Graph: failed to get entity by scope key", loggateway.StepID("memory.l4_fail"), loggateway.Str("agent_id", agentID), loggateway.Str("name", name), loggateway.Err(err))
		}
		if existing.ID == "" {
			prior, ok, err := uc.repo.GetFirstEntityByType(ctx, "agent", agentID, "person")
			if err != nil {
				uc.lg.Warn("L4Graph: failed to get first entity by type", loggateway.StepID("memory.l4_fail"), loggateway.Str("agent_id", agentID), loggateway.Err(err))
			}
			if ok && prior.ID != "" {
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
				if err := uc.cascade.ProposeNameConflict(ctx, agentID, entID, existing.Name, name); err != nil {
					uc.lg.Warn("L4Graph: failed to propose name conflict", loggateway.StepID("memory.l4_fail"), loggateway.Str("entity_id", entID), loggateway.Err(err))
				}
			}
			// Gate: keep authoritative name until cascade proposal is approved.
			prepared.Name = existing.Name
			prepared.NameNormalized = strings.ToLower(strings.TrimSpace(existing.Name))
			profileName = existing.Name
		}
		meta := mergeConflictMetadata(prepared.MetadataJSON, conflict, existing.Name, name, uc.lg)
		if err := uc.repo.UpsertEntity(ctx, L4EntityWrite{
			ID:             entID,
			ScopeType:      "agent",
			ScopeID:        agentID,
			UserID:         userID,
			EntityType:     "person",
			Name:           prepared.Name,
			NameNormalized: prepared.NameNormalized,
			Description:    prepared.Description,
			Importance:     l4PersonImportance,
			Confidence:     prepared.Confidence,
			MetadataJSON:   meta,
		}); err == nil {
			if err := uc.repo.UpsertRelation(ctx, L4RelationWrite{
				ScopeType:    "agent",
				ScopeID:      agentID,
				SourceID:     anchorID,
				TargetID:     entID,
				RelationType: "knows_as",
				Weight:       1.0,
				Confidence:   prepared.Confidence,
			}); err != nil {
				uc.lg.Warn("L4Graph: failed to upsert knows_as relation", loggateway.StepID("memory.l4_fail"), loggateway.Str("entity_id", entID), loggateway.Err(err))
			}
			cascade := uc.cascadeProfileTouch(anchorID, userID, agentID, profileName, name, conflict, now)
			if err := uc.repo.UpsertEntity(ctx, cascade); err != nil {
				uc.lg.Warn("L4Graph: failed to upsert cascade profile", loggateway.StepID("memory.l4_fail"), loggateway.Str("anchor_id", anchorID), loggateway.Err(err))
			}
			written++
		}
	}

	// Extract preference from English patterns first, then Chinese fallback.
	pref := ""
	if m := l4PreferencePattern.FindStringSubmatch(text); len(m) > 1 {
		pref = strings.TrimSpace(m[1])
	}
	if pref == "" {
		if m := l4ChinesePreferencePattern.FindStringSubmatch(text); len(m) > 1 {
			pref = strings.TrimSpace(m[1])
		}
	}
	if pref != "" {
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
			Importance:     l4PrefImportance,
			Confidence:     l4PrefConfidence,
			MetadataJSON:   `{"source":"auto_memory"}`,
		}); err == nil {
			if err := uc.repo.UpsertRelation(ctx, L4RelationWrite{
				ScopeType:    "agent",
				ScopeID:      agentID,
				SourceID:     anchorID,
				TargetID:     entID,
				RelationType: "prefers",
				Weight:       l4PrefRelWeight,
				Confidence:   l4PrefRelConfidence,
			}); err != nil {
				uc.lg.Warn("L4Graph: failed to upsert prefers relation", loggateway.StepID("memory.l4_fail"), loggateway.Str("entity_id", entID), loggateway.Err(err))
			}
			written++
		}
	}

	return written, nil
}

func (uc *L4GraphUsecase) RunDecay(ctx context.Context, agentID string) {
	uc.runDecay(ctx, agentID)
}

func (uc *L4GraphUsecase) RunDecayWithConfig(ctx context.Context, agentID string, cfg L4DecayConfig) L4DecayResult {
	if uc == nil || uc.repo == nil {
		return L4DecayResult{}
	}
	nowMs := time.Now().UTC().UnixMilli()
	decayed, err := uc.repo.ApplyBusinessConfidenceDecay(ctx, "agent", agentID, cfg, nowMs)
	if err != nil {
		return L4DecayResult{}
	}
	archived, err := uc.repo.ArchiveLowConfidenceEntities(ctx, "agent", agentID, l4ArchiveThreshold)
	if err != nil {
		uc.lg.Warn("L4Graph: failed to archive low confidence entities", loggateway.StepID("memory.l4_fail"), loggateway.Str("agent_id", agentID), loggateway.Err(err))
	}
	return L4DecayResult{
		Decayed:  int(decayed),
		Archived: int(archived),
	}
}

func (uc *L4GraphUsecase) RecordEntityReinforcement(ctx context.Context, entityID string, signal ReinforcementSignal, source string) error {
	if uc == nil || uc.repo == nil {
		return nil
	}
	return uc.repo.RecordEntityReinforcement(ctx, entityID, signal, source)
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
	conf := l4ConflictBaseConf
	if existing.ID != "" && !conflict {
		conf = existing.Confidence
		if conf < l4ConflictConfCap {
			conf += l4ConflictConfStep
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
		Importance:     l4AnchorImportance,
		Confidence:     l4AnchorConfidence,
		MetadataJSON:   meta,
	}
}

func (uc *L4GraphUsecase) runDecay(ctx context.Context, agentID string) {
	if uc == nil || uc.repo == nil {
		return
	}
	cutoff := time.Now().UTC().Add(-l4DecayAfter).Format(time.RFC3339)
	if _, err := uc.repo.ApplyConfidenceDecay(ctx, "agent", agentID, cutoff, l4DecayFactor); err != nil {
		uc.lg.Warn("L4Graph: failed to apply confidence decay", loggateway.StepID("memory.l4_fail"), loggateway.Str("agent_id", agentID), loggateway.Err(err))
	}
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

func mergeConflictMetadata(base string, conflict bool, priorName, pendingName string, lg loggateway.Logger) string {
	if !conflict {
		return base
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(base), &m); err != nil || m == nil {
		if err != nil {
			lg.Warn("解析 conflict metadata 失败", loggateway.StepID("memory.l4.merge_conflict"), loggateway.Err(err))
		}
		m = map[string]any{}
	}
	m["conflict"] = true
	m["prior_name"] = priorName
	m["pending_name"] = pendingName
	m["gate"] = "cascade_proposal"
	b, _ := json.Marshal(m)
	return string(b)
}
