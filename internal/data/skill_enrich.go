package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/skillversion"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
)

// invStats holds aggregated invocation statistics for a single skill.
type invStats struct {
	Total       int
	Success     int
	Failure     int
	Usage7d     int
	AvgDuration *float64
}

// lastInv holds the last invocation info for a single skill.
type lastInv struct {
	AgentID    string
	InvokedAt  string
	DurationMs *int
}

func (r *skillRepo) batchEnrichSkills(ctx context.Context, rows []*dataent.PlatformSkill) ([]biz.Skill, error) {
	if len(rows) == 0 {
		return []biz.Skill{}, nil
	}

	c := r.data.RW().Read(ctx)
	skillIDs := make([]string, 0, len(rows))
	for _, e := range rows {
		skillIDs = append(skillIDs, e.ID)
	}

	statsMap := r.batchInvocationStats(ctx, c, skillIDs)
	versionMap := r.batchLatestVersions(ctx, c, skillIDs)
	lastInvMap, agentNames := r.batchLastInvocations(ctx, c, skillIDs)

	return r.assembleSkills(rows, statsMap, versionMap, lastInvMap, agentNames), nil
}

// batchInvocationStats queries invocation counts (total, success, failure, 7d, avg_duration)
// grouped by skill_id using a single SQL aggregation.
func (r *skillRepo) batchInvocationStats(ctx context.Context, c *dataent.Client, skillIDs []string) map[string]*invStats {
	statsMap := make(map[string]*invStats, len(skillIDs))
	threshold := time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339)
	placeholders := make([]string, len(skillIDs))
	args := make([]any, 1, 1+len(skillIDs))
	args[0] = threshold
	for i, id := range skillIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	statsSQL := r.data.Dialect().RenumberPlaceholders(fmt.Sprintf(
		`SELECT skill_id,
			COUNT(*) as total,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_count,
			SUM(CASE WHEN status = 'failure' THEN 1 ELSE 0 END) as failure_count,
			SUM(CASE WHEN COALESCE(NULLIF(started_at, ''), created_at) >= ? THEN 1 ELSE 0 END) as usage_7d,
			AVG(CASE WHEN duration_ms > 0 THEN duration_ms ELSE NULL END) as avg_duration
		 FROM skill_invocation
		 WHERE skill_id IN (%s)
		 GROUP BY skill_id`,
		strings.Join(placeholders, ","),
	))
	statsRows, err := c.QueryContext(ctx, statsSQL, args...)
	if err != nil {
		r.data.lg.Warn("batch invocation stats query failed", loggateway.StepID("data.skill"), loggateway.Err(err))
		return statsMap
	}
	defer statsRows.Close()
	for statsRows.Next() {
		var sid string
		var total, success, failure, usage7d int
		var avgDur sql.NullFloat64
		if scanErr := statsRows.Scan(&sid, &total, &success, &failure, &usage7d, &avgDur); scanErr != nil {
			r.data.lg.Warn("batch invocation stats scan failed",
				loggateway.StepID("data.skill"),
				loggateway.Err(scanErr))
			continue
		}
		s := &invStats{Total: total, Success: success, Failure: failure, Usage7d: usage7d}
		if avgDur.Valid {
			v := avgDur.Float64
			s.AvgDuration = &v
		}
		statsMap[sid] = s
	}
	return statsMap
}

// batchLatestVersions queries the latest SkillVersion per skill using Ent with
// desc order, then picks the first per skill_id in Go.
func (r *skillRepo) batchLatestVersions(ctx context.Context, c *dataent.Client, skillIDs []string) map[string]*dataent.SkillVersion {
	versionMap := make(map[string]*dataent.SkillVersion, len(skillIDs))
	versions, vErr := c.SkillVersion.Query().
		Where(skillversion.SkillIDIn(skillIDs...)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if vErr != nil {
		r.data.lg.Warn("batch skill version query failed", loggateway.StepID("data.skill"), loggateway.Err(vErr))
		return versionMap
	}
	for _, v := range versions {
		if _, exists := versionMap[v.SkillID]; !exists {
			versionMap[v.SkillID] = v
		}
	}
	return versionMap
}

// batchLastInvocations queries the last invocation per skill and resolves agent
// display names in two batch queries.
func (r *skillRepo) batchLastInvocations(ctx context.Context, c *dataent.Client, skillIDs []string) (map[string]*lastInv, map[string]string) {
	lastInvMap := make(map[string]*lastInv, len(skillIDs))
	placeholders := make([]string, len(skillIDs))
	args := make([]any, 0, len(skillIDs))
	for i, id := range skillIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	lastSQL := r.data.Dialect().RenumberPlaceholders(fmt.Sprintf(
		`SELECT si.skill_id, si.agent_id,
			COALESCE(NULLIF(si.started_at, ''), si.created_at) as invoked_at,
			si.duration_ms
		 FROM skill_invocation si
		 INNER JOIN (
			SELECT skill_id, MAX(COALESCE(NULLIF(started_at, ''), created_at)) as max_time
			FROM skill_invocation
			WHERE skill_id IN (%s) AND source = '`+biz.SkillInvocationSourceRuntime+`'
			GROUP BY skill_id
		 ) latest ON si.skill_id = latest.skill_id
			AND COALESCE(NULLIF(si.started_at, ''), si.created_at) = latest.max_time
		 WHERE si.source = '`+biz.SkillInvocationSourceRuntime+`'`,
		strings.Join(placeholders, ","),
	))
	lastRows, lErr := c.QueryContext(ctx, lastSQL, args...)
	if lErr != nil {
		r.data.lg.Warn("batch last invocation query failed", loggateway.StepID("data.skill"), loggateway.Err(lErr))
		return lastInvMap, map[string]string{}
	}
	defer lastRows.Close()
	for lastRows.Next() {
		var sid, agentID, invokedAt string
		var durMs sql.NullInt64
		if scanErr := lastRows.Scan(&sid, &agentID, &invokedAt, &durMs); scanErr != nil {
			r.data.lg.Warn("batch last invocation scan failed",
				loggateway.StepID("data.skill"),
				loggateway.Err(scanErr))
			continue
		}
		li := &lastInv{AgentID: agentID, InvokedAt: invokedAt}
		if durMs.Valid {
			d := int(durMs.Int64)
			li.DurationMs = &d
		}
		lastInvMap[sid] = li
	}

	agentIDs := make([]string, 0)
	seen := map[string]bool{}
	for _, li := range lastInvMap {
		if li.AgentID != "" && !seen[li.AgentID] {
			seen[li.AgentID] = true
			agentIDs = append(agentIDs, li.AgentID)
		}
	}
	agentNames := map[string]string{}
	if len(agentIDs) > 0 {
		agents, aErr := c.Agent.Query().Where(agent.IDIn(agentIDs...)).All(ctx)
		if aErr != nil {
			r.data.lg.Warn("batch agent names query failed", loggateway.StepID("data.skill"), loggateway.Err(aErr))
		} else {
			for _, a := range agents {
				agentNames[a.ID] = a.DisplayName
			}
		}
	}
	return lastInvMap, agentNames
}

// assembleSkills maps Ent PlatformSkill rows to biz.Skill with preloaded enrichment data.
func (r *skillRepo) assembleSkills(
	rows []*dataent.PlatformSkill,
	statsMap map[string]*invStats,
	versionMap map[string]*dataent.SkillVersion,
	lastInvMap map[string]*lastInv,
	agentNames map[string]string,
) []biz.Skill {
	items := make([]biz.Skill, 0, len(rows))
	for _, e := range rows {
		md := parseSkillMetadata(r.data.lg, e.MetadataJSON)
		item := biz.Skill{
			ID:                e.ID,
			Slug:              e.SkillKey,
			Name:              e.Name,
			Description:       e.Description,
			Status:            normalizeSkillStatus(e.Status),
			Enabled:           e.Enabled,
			FilesystemMissing: e.FilesystemMissing,
			SyncOrigin:        md.SyncOrigin,
			StorageDir:        md.StorageDir,
			Visibility:        e.Visibility,
			DefaultConfigJSON: e.FallbackConfigJSON,
			ParentVersionID:   e.ParentVersionID,
			EvolutionReason:   e.EvolutionReason,
			LifecycleStatus:   e.LifecycleStatus,
			WorkspaceID:       e.WorkspaceID,
			CreatedAt:         e.CreatedAt,
			UpdatedAt:         e.UpdatedAt,
			Permissions:       biz.SkillPermissions{},
		}
		item.Tags = parseSkillTags(e.MetadataJSON)
		if len(item.Tags) == 0 {
			item.Tags = parseSkillTags(e.ConfigJSON)
		}
		item.Triggers = parseSkillTriggers(e.MetadataJSON)

		if st, ok := statsMap[e.ID]; ok {
			item.InvokeCount = st.Total
			item.SuccessCount = st.Success
			item.FailureCount = st.Failure
			item.UsageCount7d = st.Usage7d
			item.AvgDurationMS = st.AvgDuration
		}

		if sv, ok := versionMap[e.ID]; ok {
			st := sv.Status
			if st == "" || st == "active" {
				st = "pass"
			}
			pubAt := sv.CreatedAt
			if strings.TrimSpace(sv.PublishedAt) != "" {
				pubAt = sv.PublishedAt
			}
			vstat := strings.TrimSpace(sv.ValidationStatus)
			if vstat == "" {
				vstat = st
			}
			item.CurrentVersion = &biz.SkillVersionSummary{
				ID:               sv.ID,
				Version:          sv.Version,
				ValidationStatus: vstat,
				PublishedAt:      pubAt,
			}
		}

		if li, ok := lastInvMap[e.ID]; ok {
			item.LastAgentID = li.AgentID
			item.LastInvokedAt = li.InvokedAt
			item.LastDurationMS = li.DurationMs
			if li.AgentID != "" {
				item.LastAgentDisplayName = agentNames[li.AgentID]
			}
		}

		items = append(items, item)
	}
	return items
}

func (r *skillRepo) enrichSkill(ctx context.Context, e *dataent.PlatformSkill) (biz.Skill, error) {
	results, err := r.batchEnrichSkills(ctx, []*dataent.PlatformSkill{e})
	if err != nil {
		return biz.Skill{}, err
	}
	return results[0], nil
}
