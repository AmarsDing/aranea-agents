package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizskill "aranea-agents/internal/biz/skill"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/internal/data/ent/skillversion"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
)

type skillRepo struct {
	data *Data
}

var _ bizskill.Repo = (*skillRepo)(nil)

// NewSkillRepo implements biz.SkillRepo via Ent (pkg-backend-to-kratos §2.2 — only entClient, no parallel sqlite pool).
func NewSkillRepo(d *Data) biz.SkillRepo {
	return &skillRepo{data: d}
}

func invocationTimeGTE(threshold string) predicate.SkillInvocation {
	return predicate.SkillInvocation(func(s *entsql.Selector) {
		s.Where(entsql.ExprP(
			"COALESCE(NULLIF("+s.C(skillinvocation.FieldStartedAt)+", ''), "+s.C(skillinvocation.FieldCreatedAt)+") >= ?",
			threshold,
		))
	})
}

func invocationTimeRange(from, to string) []predicate.SkillInvocation {
	out := []predicate.SkillInvocation{}
	if strings.TrimSpace(from) != "" {
		out = append(out, predicate.SkillInvocation(func(s *entsql.Selector) {
			s.Where(entsql.ExprP(
				"COALESCE(NULLIF("+s.C(skillinvocation.FieldStartedAt)+", ''), "+s.C(skillinvocation.FieldCreatedAt)+") >= ?",
				from,
			))
		}))
	}
	if strings.TrimSpace(to) != "" {
		out = append(out, predicate.SkillInvocation(func(s *entsql.Selector) {
			s.Where(entsql.ExprP(
				"COALESCE(NULLIF("+s.C(skillinvocation.FieldStartedAt)+", ''), "+s.C(skillinvocation.FieldCreatedAt)+") <= ?",
				to,
			))
		}))
	}
	return out
}

func escapeJSONStringValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func skillListPredicates(q biz.SkillListQuery) []predicate.PlatformSkill {
	ps := []predicate.PlatformSkill{platformskill.DeletedAtEQ("")}
	// P2-B: workspace visibility filter.
	// empty WorkspaceID = system caller (see all); non-empty = tenant caller (shared + own).
	if ws := strings.TrimSpace(q.WorkspaceID); ws != "" {
		ps = append(ps, platformskill.Or(
			platformskill.WorkspaceIDEQ(""),
			platformskill.WorkspaceIDEQ(ws),
		))
	}
	if s := strings.TrimSpace(q.Search); s != "" {
		ps = append(ps, platformskill.Or(
			platformskill.SkillKeyContainsFold(s),
			platformskill.NameContainsFold(s),
			platformskill.DescriptionContainsFold(s),
		))
	}
	if q.Tags != "" {
		for _, tag := range strings.Split(q.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			ps = append(ps, platformskill.MetadataJSONContainsFold(tag))
		}
	}
	if q.Enabled == "true" {
		ps = append(ps, platformskill.EnabledEQ(true))
	}
	if q.Enabled == "false" {
		ps = append(ps, platformskill.EnabledEQ(false))
	}
	if q.Status != "" {
		if q.Status == "published" {
			ps = append(ps, platformskill.Or(platformskill.StatusEQ("published"), platformskill.StatusEQ("active")))
		} else {
			ps = append(ps, platformskill.StatusEQ(q.Status))
		}
	}
	if q.FilesystemMissing == "true" {
		ps = append(ps, platformskill.FilesystemMissingEQ(true))
	}
	if q.FilesystemMissing == "false" {
		ps = append(ps, platformskill.FilesystemMissingEQ(false))
	}
	if origin := strings.TrimSpace(q.SyncOrigin); origin != "" {
		ps = append(ps, platformskill.MetadataJSONContains(`"sync_origin":"`+escapeJSONStringValue(origin)+`"`))
	}
	return ps
}

func normalizeSkillStatus(status string) string {
	switch status {
	case "", "active", "published":
		return "published"
	case "inactive":
		return "archived"
	default:
		return status
	}
}

func parseSkillTags(raw string) []biz.SkillTag {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []biz.SkillTag{}
	}
	var envelope struct {
		Tags []biz.SkillTag `json:"tags"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil && len(envelope.Tags) > 0 {
		return normalizeSkillTags(envelope.Tags)
	}
	var tags []biz.SkillTag
	if err := json.Unmarshal([]byte(raw), &tags); err == nil {
		return normalizeSkillTags(tags)
	}
	return []biz.SkillTag{}
}

func normalizeSkillTags(tags []biz.SkillTag) []biz.SkillTag {
	result := []biz.SkillTag{}
	seen := map[string]bool{}
	for _, tag := range tags {
		tag.Name = strings.TrimSpace(tag.Name)
		if tag.Name == "" || seen[strings.ToLower(tag.Name)] {
			continue
		}
		if tag.Source == "" {
			tag.Source = "user"
		}
		seen[strings.ToLower(tag.Name)] = true
		result = append(result, tag)
	}
	return result
}

func parseTaxonomyPathsFromJSON(lg loggateway.Logger, blob string) []string {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return nil
	}
	var wrap struct {
		TaxonomyPaths []string `json:"taxonomy_paths"`
	}
	if err := json.Unmarshal([]byte(blob), &wrap); err != nil {
		lg.Warn("unmarshal taxonomy_paths failed", loggateway.StepID("data.skill"), loggateway.Err(err))
		return nil
	}
	if len(wrap.TaxonomyPaths) == 0 {
		return nil
	}
	out := make([]string, 0, len(wrap.TaxonomyPaths))
	seen := map[string]bool{}
	for _, p := range wrap.TaxonomyPaths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func mergeTaxonomyPaths(lg loggateway.Logger, meta, cfg string) []string {
	a := parseTaxonomyPathsFromJSON(lg, meta)
	b := parseTaxonomyPathsFromJSON(lg, cfg)
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, p := range append(append([]string{}, a...), b...) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

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

	// Batch Agent display names.
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
			WorkspaceID:       e.WorkspaceID, // P2-B: tenant isolation
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

func coalesceTime(startedAt, createdAt string) string {
	if strings.TrimSpace(startedAt) != "" {
		return startedAt
	}
	return createdAt
}

// skillListOrder 构造列表排序。生产唯一驱动为 Postgres（NewData 硬编码），
// tag 排序依赖 jsonb 提取 metadata_json 中首个标签名（envelope 与裸数组两种历史格式）。
func skillListOrder(q biz.SkillListQuery) []platformskill.OrderOption {
	dir := "ASC"
	if strings.EqualFold(strings.TrimSpace(q.SortOrder), "desc") {
		dir = "DESC"
	}
	switch strings.TrimSpace(q.SortBy) {
	case "name":
		return []platformskill.OrderOption{
			func(s *entsql.Selector) {
				s.OrderBy("LOWER(" + s.C(platformskill.FieldName) + ") " + dir)
				s.OrderBy(entsql.Asc(s.C(platformskill.FieldID)))
			},
		}
	case "tag":
		return []platformskill.OrderOption{
			func(s *entsql.Selector) {
				firstTag := "COALESCE(" +
					s.C(platformskill.FieldMetadataJSON) + "::jsonb -> 'tags' -> 0 ->> 'name', " +
					s.C(platformskill.FieldMetadataJSON) + "::jsonb -> 0 ->> 'name')"
				s.OrderBy("LOWER(" + firstTag + ") " + dir + " NULLS LAST")
				s.OrderBy("LOWER(" + s.C(platformskill.FieldName) + ") " + dir)
				s.OrderBy(entsql.Asc(s.C(platformskill.FieldID)))
			},
		}
	default:
		return []platformskill.OrderOption{
			platformskill.ByUpdatedAt(entsql.OrderDesc()),
			platformskill.ByCreatedAt(entsql.OrderDesc()),
		}
	}
}

func (r *skillRepo) SearchSkills(ctx context.Context, q biz.SkillListQuery) (biz.SkillListResult, error) {
	c := r.data.RW().Read(ctx)
	preds := skillListPredicates(q)
	total, err := c.PlatformSkill.Query().Where(preds...).Count(ctx)
	if err != nil {
		return biz.SkillListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	rows, err := c.PlatformSkill.Query().
		Where(preds...).
		Order(skillListOrder(q)...).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.SkillListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	items, err := r.batchEnrichSkills(ctx, rows)
	if err != nil {
		return biz.SkillListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return biz.SkillListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

func (r *skillRepo) GetSkillByID(ctx context.Context, id string) (biz.Skill, error) {
	preds := []predicate.PlatformSkill{
		platformskill.IDEQ(id),
		platformskill.DeletedAtEQ(""),
	}
	if ids := workspaceSharedOrOwnIDs(ctx); ids != nil {
		preds = append(preds, platformskill.WorkspaceIDIn(ids...))
	}
	e, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(preds...).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.enrichSkill(ctx, e)
}

func (r *skillRepo) UpdateSkillEnabled(ctx context.Context, id string, enabled bool) (biz.Skill, error) {
	if id == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill id is required")
	}
	if enabled {
		existing, err := r.data.RW().Read(ctx).PlatformSkill.Query().
			Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
			Only(ctx)
		if err != nil {
			if dataent.IsNotFound(err) {
				return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
			}
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}
		status := strings.TrimSpace(strings.ToLower(existing.Status))
		if status != "published" && status != "active" {
			return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "only published skills can be enabled")
		}
	}
	err := r.data.RW().Write(ctx).PlatformSkill.UpdateOneID(id).
		SetEnabled(enabled).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, id)
}

func (r *skillRepo) DuplicateSkill(ctx context.Context, id string) (biz.Skill, error) {
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	cur, err := tx.PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	latestVer, verErr := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(id)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	if verErr != nil && !dataent.IsNotFound(verErr) {
		return biz.Skill{}, entErrToBizErr(verErr, apierror.DomainSkill)
	}
	newID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	newKey := fmt.Sprintf("%s-copy-%d", cur.SkillKey, time.Now().UTC().Unix())
	if strings.TrimSpace(cur.SkillKey) == "" {
		newKey = newID
	}
	now := nowRFC3339()
	if _, err = tx.PlatformSkill.Create().
		SetID(newID).
		SetSkillKey(newKey).
		SetName(cur.Name + " Copy").
		SetDescription(cur.Description).
		SetStatus("draft").
		SetEnabled(false).
		SetSortOrder(0).
		SetConfigJSON(cur.ConfigJSON).
		SetMetadataJSON(cur.MetadataJSON).
		SetVisibility(cur.Visibility).
		SetFallbackConfigJSON(cur.FallbackConfigJSON).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetDeletedAt("").
		SetWorkspaceID(cur.WorkspaceID). // P2-B: inherit workspace from source
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if latestVer != nil {
		verID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
		if _, err = tx.SkillVersion.Create().
			SetID(verID).
			SetSkillID(newID).
			SetVersion("1.0.0").
			SetStatus("pass").
			SetContentMarkdown(latestVer.ContentMarkdown).
			SetMetadataJSON(latestVer.MetadataJSON).
			SetManifestJSON(latestVer.ManifestJSON).
			SetFileManifestJSON(latestVer.FileManifestJSON).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, newID)
}

func (r *skillRepo) DeleteSkill(ctx context.Context, id string) error {
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	// B1 fix: physical delete. Soft-delete tombstones blocked same-slug
	// re-creation because skill_key has a full-table unique index (no status
	// filter). Delete versions first to maintain referential integrity; any
	// failure rolls back the whole tx so skill + versions stay atomic.
	if _, vErr := tx.SkillVersion.Delete().
		Where(skillversion.SkillIDEQ(id)).
		Exec(ctx); vErr != nil {
		return entErrToBizErr(vErr, apierror.DomainSkill)
	}

	n, err := tx.PlatformSkill.Delete().
		Where(platformskill.IDEQ(id)).
		Exec(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	if n == 0 {
		return apierror.NotFound(apierror.DomainSkill, "skill not found")
	}

	return entErrToBizErr(tx.Commit(), apierror.DomainSkill)
}

func runPredicates(q biz.SkillRunQuery) []predicate.SkillInvocation {
	ps := []predicate.SkillInvocation{}
	if q.SkillID != "" {
		ps = append(ps, skillinvocation.SkillIDEQ(q.SkillID))
	}
	if q.AgentID != "" {
		ps = append(ps, skillinvocation.AgentIDEQ(q.AgentID))
	}
	if q.SessionID != "" {
		ps = append(ps, skillinvocation.SessionIDEQ(q.SessionID))
	}
	if q.Status != "" {
		ps = append(ps, skillinvocation.StatusEQ(q.Status))
	}
	ps = append(ps, invocationTimeRange(q.From, q.To)...)
	return ps
}

func (r *skillRepo) SearchSkillInvocations(ctx context.Context, query biz.SkillRunQuery) (biz.SkillRunResult, error) {
	c := r.data.RW().Read(ctx)
	preds := runPredicates(query)
	base := c.SkillInvocation.Query()
	if len(preds) > 0 {
		base = base.Where(preds...)
	}
	total, err := base.Clone().Count(ctx)
	if err != nil {
		return biz.SkillRunResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	rows, err := base.Clone().
		Order(skillinvocation.ByCreatedAt(entsql.OrderDesc())).
		Limit(query.Limit).
		Offset(query.Offset).
		All(ctx)
	if err != nil {
		return biz.SkillRunResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}

	skillIDs := map[string]struct{}{}
	agentIDs := map[string]struct{}{}
	for _, row := range rows {
		if row.SkillID != "" {
			skillIDs[row.SkillID] = struct{}{}
		}
		if row.AgentID != "" {
			agentIDs[row.AgentID] = struct{}{}
		}
	}
	sidList := make([]string, 0, len(skillIDs))
	for id := range skillIDs {
		sidList = append(sidList, id)
	}
	aidList := make([]string, 0, len(agentIDs))
	for id := range agentIDs {
		aidList = append(aidList, id)
	}
	skillNames := map[string]string{}
	if len(sidList) > 0 {
		skills, err := c.PlatformSkill.Query().Where(platformskill.IDIn(sidList...)).All(ctx)
		if err != nil {
			return biz.SkillRunResult{}, entErrToBizErr(err, apierror.DomainSkill)
		}
		for _, s := range skills {
			skillNames[s.ID] = s.Name
		}
	}
	agentNames := map[string]string{}
	if len(aidList) > 0 {
		agents, err := c.Agent.Query().Where(agent.IDIn(aidList...)).All(ctx)
		if err != nil {
			return biz.SkillRunResult{}, entErrToBizErr(err, apierror.DomainSkill)
		}
		for _, a := range agents {
			agentNames[a.ID] = a.DisplayName
		}
	}

	items := make([]biz.SkillInvocation, 0, len(rows))
	for _, row := range rows {
		items = append(items, biz.SkillInvocation{
			ID:               row.ID,
			SkillID:          row.SkillID,
			SkillName:        skillNames[row.SkillID],
			SkillVersion:     row.SkillVersion,
			AgentID:          row.AgentID,
			AgentDisplayName: agentNames[row.AgentID],
			UserID:           row.UserID,
			SessionID:        row.SessionID,
			Status:           row.Status,
			DurationMS:       row.DurationMs,
			StartedAt:        coalesceTime(row.StartedAt, row.CreatedAt),
			EndedAt:          row.EndedAt,
			InputPreview:     row.InputPreview,
			InputHash:        row.InputHash,
			OutputPreview:    row.OutputPreview,
			ErrorCode:        row.ErrorCode,
			ErrorMessage:     row.ErrorMessage,
			Source:           row.Source,
			ActivationID:     row.ActivationID,
			MessageID:        row.MessageID,
			Permissions:      biz.SkillInvocationPermissions{},
		})
	}
	return biz.SkillRunResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (r *skillRepo) GetSkillStorageDir(ctx context.Context, id string) (string, error) {
	e, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return "", entErrToBizErr(err, apierror.DomainSkill)
	}
	var metadata struct {
		StorageDir string `json:"storage_dir"`
	}
	if err := json.Unmarshal([]byte(e.MetadataJSON), &metadata); err != nil {
		r.data.lg.Warn("unmarshal skill metadata failed", loggateway.StepID("data.skill"), loggateway.Err(err))
		return "", err
	}
	if strings.TrimSpace(metadata.StorageDir) == "" {
		return "", apierror.Internal(apierror.DomainSkill, "skill storage directory is not configured")
	}
	return metadata.StorageDir, nil
}

func previewSkillBody(body string, limit int) string {
	body = strings.TrimSpace(body)
	if limit <= 0 {
		return body
	}
	runes := []rune(body)
	if len(runes) <= limit {
		return body
	}
	return string(runes[:limit]) + "..."
}

func (r *skillRepo) ListSkillSimilaritySources(ctx context.Context) ([]biz.SkillSimilaritySource, error) {
	c := r.data.RW().Read(ctx)
	rows, err := c.PlatformSkill.Query().
		Where(platformskill.DeletedAtEQ("")).
		Order(platformskill.ByUpdatedAt(entsql.OrderDesc()), platformskill.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}

	// Batch-fetch latest SkillVersion per skill to avoid N+1.
	skillIDs := make([]string, 0, len(rows))
	for _, s := range rows {
		skillIDs = append(skillIDs, s.ID)
	}
	versionMap := make(map[string]*dataent.SkillVersion, len(skillIDs))
	if len(skillIDs) > 0 {
		versions, vErr := c.SkillVersion.Query().
			Where(skillversion.SkillIDIn(skillIDs...)).
			Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
			All(ctx)
		if vErr != nil {
			return nil, entErrToBizErr(vErr, apierror.DomainSkill)
		}
		for _, v := range versions {
			if _, exists := versionMap[v.SkillID]; !exists {
				versionMap[v.SkillID] = v
			}
		}
	}

	out := make([]biz.SkillSimilaritySource, 0, len(rows))
	for _, s := range rows {
		item := biz.SkillSimilaritySource{
			ID:          s.ID,
			Name:        s.Name,
			Slug:        s.SkillKey,
			Description: s.Description,
		}
		if sv, ok := versionMap[s.ID]; ok {
			item.Version = sv.Version
			item.Body = sv.ContentMarkdown
			item.BodyPreview = previewSkillBody(item.Body, 240)
		}
		out = append(out, item)
	}
	return out, nil
}

func (r *skillRepo) CreateSkillWithVersion(ctx context.Context, in biz.SkillCreateInput) (biz.Skill, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	in.Description = strings.TrimSpace(in.Description)
	in.Body = strings.TrimSpace(in.Body)
	if in.Name == "" || in.Slug == "" || in.Body == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill name, slug and body are required")
	}
	skillID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	versionID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
	metaJSON, err := encodeSkillMetadata(in.Tags, in.StorageDir, in.SyncOrigin, in.Triggers)
	if err != nil {
		return biz.Skill{}, err
	}
	now := nowRFC3339()
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.PlatformSkill.Create().
		SetID(skillID).
		SetSkillKey(in.Slug).
		SetName(in.Name).
		SetDescription(in.Description).
		SetStatus("draft").
		SetEnabled(false).
		SetSortOrder(0).
		SetConfigJSON("{}").
		SetMetadataJSON(string(metaJSON)).
		SetVisibility(in.Visibility).
		SetFallbackConfigJSON(in.DefaultConfigJSON).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetDeletedAt("").
		SetWorkspaceID(in.WorkspaceID). // P2-B: tenant isolation
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if _, err = tx.SkillVersion.Create().
		SetID(versionID).
		SetSkillID(skillID).
		SetVersion("1.0.0").
		SetStatus("pass").
		SetContentMarkdown(in.Body).
		SetMetadataJSON(string(metaJSON)).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, skillID)
}

func (r *skillRepo) GetSkillBySkillKey(ctx context.Context, skillKey string) (biz.Skill, error) {
	skillKey = strings.TrimSpace(skillKey)
	if skillKey == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill key is required")
	}
	preds := []predicate.PlatformSkill{
		platformskill.SkillKeyEQ(skillKey),
		platformskill.DeletedAtEQ(""),
	}
	if ids := workspaceSharedOrOwnIDs(ctx); ids != nil {
		preds = append(preds, platformskill.WorkspaceIDIn(ids...))
	}
	e, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(preds...).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.enrichSkill(ctx, e)
}

func (r *skillRepo) UpsertSkillFromDisk(ctx context.Context, in biz.SkillDiskSyncInput) (biz.Skill, biz.SkillDiskSyncOutcome, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	in.Description = strings.TrimSpace(in.Description)
	in.Body = strings.TrimSpace(in.Body)
	if in.Name == "" || in.Slug == "" || in.Body == "" {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, apierror.BadRequest(apierror.DomainSkill, "skill name, slug and body are required")
	}

	tx, txErr := r.data.RW().Write(ctx).Tx(ctx)
	if txErr != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(txErr, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	skillRow, err := tx.PlatformSkill.Query().
		Where(platformskill.SkillKeyEQ(in.Slug), platformskill.DeletedAtEQ("")).
		Only(ctx)

	if dataent.IsNotFound(err) {
		skillID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
		versionID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
		metaJSON, encErr := encodeSkillMetadata(in.Tags, in.StorageDir, biz.SkillSyncOriginFilesystem, in.Triggers)
		if encErr != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, encErr
		}
		now := nowRFC3339()
		if _, createErr := tx.PlatformSkill.Create().
			SetID(skillID).
			SetSkillKey(in.Slug).
			SetName(in.Name).
			SetDescription(in.Description).
			SetStatus("draft").
			SetEnabled(false).
			SetSortOrder(0).
			SetConfigJSON("{}").
			SetMetadataJSON(string(metaJSON)).
			SetVisibility("").
			SetCreatedAt(now).
			SetUpdatedAt(now).
			SetDeletedAt("").
			Save(ctx); createErr != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(createErr, apierror.DomainSkill)
		}
		if _, createErr := tx.SkillVersion.Create().
			SetID(versionID).
			SetSkillID(skillID).
			SetVersion("1.0.0").
			SetStatus("pass").
			SetContentMarkdown(in.Body).
			SetMetadataJSON(string(metaJSON)).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx); createErr != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(createErr, apierror.DomainSkill)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(commitErr, apierror.DomainSkill)
		}
		sk, getErr := r.GetSkillByID(ctx, skillID)
		return sk, biz.SkillDiskSyncOutcome{}, getErr
	}
	if err != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(err, apierror.DomainSkill)
	}

	outcome := biz.SkillDiskSyncOutcome{}
	wasPublished := skillRow.Status == "published" || skillRow.Status == "active"
	now := nowRFC3339()
	metaJSON, encErr := encodeSkillMetadata(in.Tags, in.StorageDir, biz.SkillSyncOriginFilesystem, in.Triggers)
	if encErr != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, encErr
	}
	// P-r4-1：watcher 重建 metadata 时必须保留 merge 血缘（derived_from），
	// 否则每次 5 分钟扫描都会抹除 merge_group_with_ai 记录的溯源信息。
	if derived := parseSkillMetadata(r.data.lg, skillRow.MetadataJSON).DerivedFrom; len(derived) > 0 {
		md := parseSkillMetadata(r.data.lg, metaJSON)
		md.DerivedFrom = derived
		if merged, mErr := json.Marshal(md); mErr == nil {
			metaJSON = string(merged)
		}
	}
	update := tx.PlatformSkill.UpdateOneID(skillRow.ID).
		SetName(in.Name).
		SetDescription(in.Description).
		SetMetadataJSON(string(metaJSON)).
		SetUpdatedAt(now).
		SetFilesystemMissing(false)
	// 缓存键契约（internal/agent/version_hash.go）：updated_at 是
	// SkillVersionHash 的内容版本标记，仅在内容/元数据/可用性实际变化时
	// 才允许 bump。reconcile 每 5 分钟全量无变化扫描不得触碰，否则
	// SkillVersionHash 漂移导致全量 agent 构建缓存周期性失效。
	rowChanged := skillRow.Name != in.Name ||
		skillRow.Description != in.Description ||
		skillRow.MetadataJSON != string(metaJSON) ||
		skillRow.FilesystemMissing

	sv, svErr := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(skillRow.ID)).
		// 同 CreateSkillVersion：秒级 created_at 并列时按 ID 递减兜底，确保读到真正最新版本。
		Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
		First(ctx)
	if svErr != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(svErr, apierror.DomainSkill)
	}
	// refreshDiskBody 非空表示命中 F3 进化版保护：提交后以 DB 内容刷新磁盘。
	var refreshDiskBody string
	if strings.TrimSpace(sv.ContentMarkdown) != in.Body {
		if strings.TrimSpace(sv.EvolutionReason) != "" {
			// F3 (P-evo-1)：最新版本是进化成果——磁盘内容陈旧，不得反向覆盖
			// （不落新版本、不回退 draft），提交后以 DB 为准刷新磁盘。
			refreshDiskBody = sv.ContentMarkdown
		} else {
			outcome.ContentChanged = true
			// Create a new version to preserve immutability of existing versions
			newVerID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
			newVerMetaJSON := string(metaJSON)
			if _, createErr := tx.SkillVersion.Create().
				SetID(newVerID).
				SetSkillID(skillRow.ID).
				SetVersion(incrementVersion(sv.Version)).
				SetStatus("pass").
				SetContentMarkdown(in.Body).
				SetMetadataJSON(newVerMetaJSON).
				SetManifestJSON(sv.ManifestJSON).
				SetFileManifestJSON(sv.FileManifestJSON).
				SetCreatedAt(now).
				SetUpdatedAt(now).
				Save(ctx); createErr != nil {
				return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(createErr, apierror.DomainSkill)
			}
		}
	}
	if outcome.ContentChanged && wasPublished {
		outcome.RevertedToDraft = true
		update = update.SetStatus("draft").SetEnabled(false)
	}
	if rowChanged || outcome.ContentChanged {
		if _, updateErr := update.Save(ctx); updateErr != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(updateErr, apierror.DomainSkill)
		}
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(commitErr, apierror.DomainSkill)
	}
	if refreshDiskBody != "" {
		r.data.lg.Warn("skill disk sync: stale disk content blocked by evolution version, refreshing disk from DB",
			loggateway.StepID("data.skill"),
			loggateway.Str("skill_id", skillRow.ID),
			loggateway.Str("version_id", sv.ID),
			loggateway.Str("evolution_reason", sv.EvolutionReason))
		writeSkillBodyToDisk(r.data.lg, in.StorageDir, refreshDiskBody)
	}
	sk, getErr := r.GetSkillByID(ctx, skillRow.ID)
	return sk, outcome, getErr
}

func (r *skillRepo) ListRegisteredSlugs(ctx context.Context) ([]string, error) {
	rows, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.DeletedAtEQ("")).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if slug := strings.TrimSpace(row.SkillKey); slug != "" {
			out = append(out, slug)
		}
	}
	return out, nil
}

func (r *skillRepo) ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error) {
	rows, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.EnabledEQ(true),
			platformskill.Or(platformskill.StatusEQ("published"), platformskill.StatusEQ("active")),
		).
		// Deterministic order: results feed the skill overview injected into
		// the system prompt; byte-stable ordering is required for prompt-cache
		// hits across turns.
		Order(platformskill.BySkillKey(entsql.OrderAsc())).
		Select(platformskill.FieldSkillKey).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.SkillKey)
	}
	return out, nil
}

// ListEnabledPublishedSkillRefs returns slug + updated_at for every enabled
// published skill. updated_at is bumped by every data-layer skill mutation,
// so consumers can build a content-based cache-key hash that changes exactly
// when skill content changes. Only two columns are selected: this runs on the
// per-request agent-build hot path.
func (r *skillRepo) ListEnabledPublishedSkillRefs(ctx context.Context) ([]biz.SkillEnabledRef, error) {
	rows, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.EnabledEQ(true),
			platformskill.Or(platformskill.StatusEQ("published"), platformskill.StatusEQ("active")),
		).
		Order(platformskill.BySkillKey(entsql.OrderAsc())).
		Select(platformskill.FieldSkillKey, platformskill.FieldUpdatedAt).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	out := make([]biz.SkillEnabledRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, biz.SkillEnabledRef{Slug: row.SkillKey, UpdatedAt: row.UpdatedAt})
	}
	return out, nil
}

func (r *skillRepo) ListEnabledPublishedSkillCandidates(ctx context.Context) ([]biz.SkillRuntimeCandidate, error) {
	rows, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.EnabledEQ(true),
			platformskill.Or(platformskill.StatusEQ("published"), platformskill.StatusEQ("active")),
		).
		// Deterministic order: candidates feed routing and the prompt skill
		// overview; byte-stable ordering is required for prompt-cache hits.
		Order(platformskill.BySkillKey(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	out := make([]biz.SkillRuntimeCandidate, 0, len(rows))
	for _, row := range rows {
		tags := parseSkillTags(row.MetadataJSON)
		if len(tags) == 0 {
			tags = parseSkillTags(row.ConfigJSON)
		}
		out = append(out, biz.SkillRuntimeCandidate{
			Slug:          row.SkillKey,
			Name:          row.Name,
			Description:   row.Description,
			Tags:          tags,
			TaxonomyPaths: mergeTaxonomyPaths(r.data.lg, row.MetadataJSON, row.ConfigJSON),
			Triggers:      parseSkillTriggers(row.MetadataJSON),
		})
	}
	return out, nil
}

func (r *skillRepo) RecordSkillInvocation(ctx context.Context, in biz.SkillInvocationWrite) error {
	id := fmt.Sprintf("skillinv_%d", time.Now().UTC().UnixNano())
	now := nowRFC3339()
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "runtime"
	}
	started := strings.TrimSpace(in.StartedAt)
	if started == "" {
		started = now
	}
	ended := strings.TrimSpace(in.EndedAt)
	if ended == "" {
		ended = now
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "success"
	}
	builder := r.data.RW().Write(ctx).SkillInvocation.Create().
		SetID(id).
		SetSkillID(strings.TrimSpace(in.SkillID)).
		SetAgentID(strings.TrimSpace(in.AgentID)).
		SetStatus(status).
		SetInputJSON("{}").
		SetOutputJSON("{}").
		SetSkillVersion(strings.TrimSpace(in.SkillVersion)).
		SetUserID(strings.TrimSpace(in.UserID)).
		SetSessionID(strings.TrimSpace(in.SessionID)).
		SetDurationMs(in.DurationMS).
		SetStartedAt(started).
		SetEndedAt(ended).
		SetInputPreview(strings.TrimSpace(in.InputPreview)).
		SetInputHash(strings.TrimSpace(in.InputHash)).
		SetOutputPreview(strings.TrimSpace(in.OutputPreview)).
		SetErrorCode(strings.TrimSpace(in.ErrorCode)).
		SetErrorMessage(strings.TrimSpace(in.ErrorMessage)).
		SetSource(source).
		SetActivationID(strings.TrimSpace(in.ActivationID)).
		SetMessageID(strings.TrimSpace(in.MessageID)).
		SetCreatedAt(now).
		SetUpdatedAt(now)
	if in.SelectionReason != nil {
		builder = builder.SetSelectionReason(in.SelectionReason)
	}
	if strings.TrimSpace(in.Outcome) != "" {
		builder = builder.SetOutcome(strings.TrimSpace(in.Outcome))
	}
	if in.TokenUsage != nil {
		builder = builder.SetTokenUsage(in.TokenUsage)
	}
	if len(in.RoutedSlugs) > 0 {
		builder = builder.SetRoutedSlugs(in.RoutedSlugs)
	}
	if strings.TrimSpace(in.LoadedSlug) != "" {
		builder = builder.SetLoadedSlug(strings.TrimSpace(in.LoadedSlug))
	}
	_, err := builder.Save(ctx)
	return entErrToBizErr(err, apierror.DomainSkill)
}

func (r *skillRepo) GetLatestSkillMarkdown(ctx context.Context, skillID string) (string, error) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "", apierror.BadRequest(apierror.DomainSkill, "skill id is required")
	}
	sv, err := r.data.RW().Read(ctx).SkillVersion.Query().
		Where(skillversion.SkillIDEQ(skillID)).
		// created_at 为秒级 TEXT，同秒并列时用 sv_<unixnano> ID 递减兜底（新生成 ID 更大）。
		Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return "", apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return "", entErrToBizErr(err, apierror.DomainSkill)
	}
	return sv.ContentMarkdown, nil
}

func (r *skillRepo) BatchGetSkillMarkdownBySlugs(ctx context.Context, slugs []string) (map[string]string, error) {
	if len(slugs) == 0 {
		return map[string]string{}, nil
	}
	skills, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.SkillKeyIn(slugs...), platformskill.DeletedAtEQ("")).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	skillIDs := make([]string, 0, len(skills))
	keyToID := make(map[string]string, len(skills))
	for _, s := range skills {
		skillIDs = append(skillIDs, s.ID)
		keyToID[s.SkillKey] = s.ID
	}
	if len(skillIDs) == 0 {
		return map[string]string{}, nil
	}
	rows, err := r.data.RW().Read(ctx).SkillVersion.Query().
		Where(skillversion.SkillIDIn(skillIDs...)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	idToMarkdown := make(map[string]string, len(skillIDs))
	seen := make(map[string]bool, len(skillIDs))
	for _, sv := range rows {
		if seen[sv.SkillID] {
			continue
		}
		seen[sv.SkillID] = true
		idToMarkdown[sv.SkillID] = sv.ContentMarkdown
	}
	out := make(map[string]string, len(slugs))
	for key, id := range keyToID {
		if md, ok := idToMarkdown[id]; ok {
			out[key] = md
		}
	}
	return out, nil
}

type skillMetadataEnvelope struct {
	Tags       []biz.SkillTag `json:"tags"`
	StorageDir string         `json:"storage_dir"`
	SyncOrigin string         `json:"sync_origin"`
	// Triggers 确定性触发词（P1-3），来自 SKILL.md frontmatter。
	Triggers []string `json:"triggers,omitempty"`
	// DerivedFrom records merge provenance (source skill IDs) for skills
	// created by merge_group_with_ai with retire_sources=true.
	DerivedFrom []string `json:"derived_from,omitempty"`
}

func encodeSkillMetadata(tags []biz.SkillTag, storageDir, syncOrigin string, triggers []string) (string, error) {
	md := skillMetadataEnvelope{
		Tags:       tags,
		StorageDir: strings.TrimSpace(storageDir),
		SyncOrigin: strings.TrimSpace(syncOrigin),
		Triggers:   normalizeSkillTriggers(triggers),
	}
	b, err := json.Marshal(md)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// parseSkillTriggers 从 metadata envelope 提取确定性触发词（P1-3）。
// 旧格式（无 triggers 字段）返回空切片。
func parseSkillTriggers(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var envelope struct {
		Triggers []string `json:"triggers"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil
	}
	return normalizeSkillTriggers(envelope.Triggers)
}

// normalizeSkillTriggers 去空白、去空串、大小写不敏感去重。
func normalizeSkillTriggers(triggers []string) []string {
	if len(triggers) == 0 {
		return nil
	}
	out := make([]string, 0, len(triggers))
	seen := map[string]bool{}
	for _, t := range triggers {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}

func parseSkillMetadata(lg loggateway.Logger, raw string) skillMetadataEnvelope {
	var md skillMetadataEnvelope
	if err := json.Unmarshal([]byte(raw), &md); err != nil {
		lg.Warn("unmarshal skill metadata envelope failed", loggateway.StepID("data.skill"), loggateway.Err(err))
	}
	return md
}

func (r *skillRepo) PatchSkill(ctx context.Context, id string, patch biz.SkillUpdateDraft) (biz.Skill, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill id is required")
	}
	e, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	now := nowRFC3339()

	if patch.HasBody {
		body := strings.TrimSpace(patch.Body)

		// Resolve storage directory first (before transaction)
		md := parseSkillMetadata(r.data.lg, e.MetadataJSON)
		storageDir := strings.TrimSpace(md.StorageDir)
		if storageDir == "" {
			return biz.Skill{}, apierror.Internal(apierror.DomainSkill, "skill storage directory is not configured")
		}
		// Validate path is within expected root
		skillPath := filepath.Join(storageDir, "SKILL.md")
		if !isPathWithinRoot(storageDir, skillPath) {
			return biz.Skill{}, apierror.Internal(apierror.DomainSkill, "skill file path escapes storage directory")
		}

		// B-08 fix: commit DB transaction FIRST, then write filesystem.
		// Previous order (filesystem → DB) left filesystem modified when DB
		// transaction failed, causing divergence between SKILL.md and the
		// SkillVersion.content_markdown row. DB is the source of truth; if
		// the filesystem write fails after commit we log and continue — the
		// agent runtime can be re-synced from DB.
		tx, txErr := r.data.RW().Write(ctx).Tx(ctx)
		if txErr != nil {
			return biz.Skill{}, entErrToBizErr(txErr, apierror.DomainSkill)
		}
		defer func() { _ = tx.Rollback() }()

		upd := tx.PlatformSkill.UpdateOneID(id).SetUpdatedAt(now)
		if patch.HasName {
			upd.SetName(strings.TrimSpace(patch.Name))
		}
		if patch.HasDescription {
			upd.SetDescription(strings.TrimSpace(patch.Description))
		}
		if patch.HasTags || patch.HasTriggers {
			md := parseSkillMetadata(r.data.lg, e.MetadataJSON)
			if patch.HasTags {
				md.Tags = normalizeSkillTags(patch.Tags)
			}
			if patch.HasTriggers {
				md.Triggers = normalizeSkillTriggers(patch.Triggers)
			}
			metaJSON, jerr := json.Marshal(md)
			if jerr != nil {
				return biz.Skill{}, jerr
			}
			upd.SetMetadataJSON(string(metaJSON))
		}
		if err := upd.Exec(ctx); err != nil {
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}

		sv, err := tx.SkillVersion.Query().
			Where(skillversion.SkillIDEQ(id)).
			Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
			First(ctx)
		if err != nil {
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}
		if _, err := tx.SkillVersion.UpdateOneID(sv.ID).
			SetContentMarkdown(body).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}
		if err = tx.Commit(); err != nil {
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}

		// Filesystem write AFTER successful DB commit. Failure here is
		// non-fatal: DB is the source of truth; filesystem is a cache the
		// agent runtime reads. Log for operator follow-up.
		if writeErr := os.WriteFile(skillPath, []byte(body), 0o644); writeErr != nil {
			r.data.lg.Warn("PatchSkill: filesystem write failed after DB commit",
				loggateway.StepID("data.skill"),
				loggateway.Str("skill_id", id),
				loggateway.Str("path", skillPath),
				loggateway.Err(writeErr))
		}
	} else {
		// B-07 fix: wrap PlatformSkill metadata update and SkillVersion tag
		// sync in a single transaction. Previously these were two independent
		// writes — if the second failed (silently), tags diverged between
		// PlatformSkill.metadata_json and SkillVersion.metadata_json.
		txErr := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
			client := r.data.RW().Write(txCtx)
			upd := client.PlatformSkill.UpdateOneID(id).SetUpdatedAt(now)
			if patch.HasName {
				upd.SetName(strings.TrimSpace(patch.Name))
			}
			if patch.HasDescription {
				upd.SetDescription(strings.TrimSpace(patch.Description))
			}
			if patch.HasTags || patch.HasTriggers {
				md := parseSkillMetadata(r.data.lg, e.MetadataJSON)
				if patch.HasTags {
					md.Tags = normalizeSkillTags(patch.Tags)
				}
				if patch.HasTriggers {
					md.Triggers = normalizeSkillTriggers(patch.Triggers)
				}
				metaJSON, jerr := json.Marshal(md)
				if jerr != nil {
					return jerr
				}
				upd.SetMetadataJSON(string(metaJSON))
			}
			if err := upd.Exec(txCtx); err != nil {
				return entErrToBizErr(err, apierror.DomainSkill)
			}

			// Sync tags to latest SkillVersion metadata within the same tx.
			if patch.HasTags {
				sv, svErr := client.SkillVersion.Query().
					Where(skillversion.SkillIDEQ(id)).
					Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
					First(txCtx)
				if svErr != nil {
					if dataent.IsNotFound(svErr) {
						// No version exists yet — nothing to sync. Not fatal.
						return nil
					}
					return entErrToBizErr(svErr, apierror.DomainSkill)
				}
				svMd := parseSkillMetadata(r.data.lg, sv.MetadataJSON)
				svMd.Tags = normalizeSkillTags(patch.Tags)
				svMetaJSON, jerr := json.Marshal(svMd)
				if jerr != nil {
					return jerr
				}
				if err := client.SkillVersion.UpdateOneID(sv.ID).
					SetMetadataJSON(string(svMetaJSON)).
					SetUpdatedAt(now).
					Exec(txCtx); err != nil {
					return entErrToBizErr(err, apierror.DomainSkill)
				}
			}
			return nil
		})
		if txErr != nil {
			return biz.Skill{}, txErr
		}
	}
	return r.GetSkillByID(ctx, id)
}

func (r *skillRepo) PublishSkill(ctx context.Context, id string, validationStatus string) (biz.Skill, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill id is required")
	}
	validationStatus = strings.TrimSpace(strings.ToLower(validationStatus))
	switch validationStatus {
	case "pass", "warn":
	default:
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "invalid validation_status for publish")
	}

	// Validate current skill state before publishing
	existing, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if existing.Status != "draft" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "only draft skills can be published")
	}

	now := nowRFC3339()
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()
	err = tx.PlatformSkill.UpdateOneID(id).
		SetStatus("published").
		SetUpdatedAt(now).
		Exec(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	sv, err := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(id)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "SKILL.md body is required")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if sv == nil || strings.TrimSpace(sv.ContentMarkdown) == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "SKILL.md body is required")
	}
	if _, serr := tx.SkillVersion.UpdateOneID(sv.ID).
		SetPublishedAt(now).
		SetValidationStatus(validationStatus).
		SetUpdatedAt(now).
		Save(ctx); serr != nil {
		return biz.Skill{}, entErrToBizErr(serr, apierror.DomainSkill)
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, id)
}

func (r *skillRepo) MarkSkillFilesystemMissing(ctx context.Context, slug string, missing bool) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return apierror.BadRequest(apierror.DomainSkill, "skill slug is required")
	}
	// 缓存键契约（同 UpsertSkillFromDisk）：仅在 missing 标志真正翻转时写库
	// 并 bump updated_at；reconcile 对每个存活 skill 都会调 missing=false，
	// 无条件写库会导致 SkillVersionHash 周期性漂移、agent 构建缓存失效。
	n, err := r.data.RW().Write(ctx).PlatformSkill.Update().
		Where(
			platformskill.SkillKeyEQ(slug),
			platformskill.DeletedAtEQ(""),
			platformskill.FilesystemMissingEQ(!missing),
		).
		SetFilesystemMissing(missing).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	if n == 0 {
		// 区分"已是目标状态"（no-op 成功）与"slug 不存在"（NotFound）。
		exist, qErr := r.data.RW().Read(ctx).PlatformSkill.Query().
			Where(platformskill.SkillKeyEQ(slug), platformskill.DeletedAtEQ("")).
			Exist(ctx)
		if qErr != nil {
			return entErrToBizErr(qErr, apierror.DomainSkill)
		}
		if !exist {
			return apierror.NotFound(apierror.DomainSkill, "not found")
		}
	}
	return nil
}

func (r *skillRepo) FilesystemHealthStats(ctx context.Context) (biz.SkillFilesystemHealthStats, error) {
	c := r.data.RW().Read(ctx)
	missing, err := c.PlatformSkill.Query().
		Where(platformskill.DeletedAtEQ(""), platformskill.FilesystemMissingEQ(true)).
		Count(ctx)
	if err != nil {
		return biz.SkillFilesystemHealthStats{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	pending, err := c.PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.StatusEQ("draft"),
			platformskill.FilesystemMissingEQ(false),
			platformskill.MetadataJSONContains(`"sync_origin":"filesystem"`),
		).
		Count(ctx)
	if err != nil {
		return biz.SkillFilesystemHealthStats{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return biz.SkillFilesystemHealthStats{
		MissingCount:           missing,
		PendingFilesystemCount: pending,
	}, nil
}

func (r *skillRepo) ListSkillVersions(ctx context.Context, q biz.SkillVersionListQuery) (biz.SkillVersionListResult, error) {
	c := r.data.RW().Read(ctx)
	exists, err := c.PlatformSkill.Query().
		Where(platformskill.IDEQ(q.SkillID), platformskill.DeletedAtEQ("")).
		Exist(ctx)
	if err != nil {
		return biz.SkillVersionListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if !exists {
		return biz.SkillVersionListResult{}, apierror.NotFound(apierror.DomainSkill, "not found")
	}
	count, err := c.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(q.SkillID)).
		Count(ctx)
	if err != nil {
		return biz.SkillVersionListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	rows, err := c.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(q.SkillID)).
		// 秒级 created_at 并列时按 ID 递减兜底，确保 Limit(1) 锚定真正最新版本。
		Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
		Offset(q.Offset).
		Limit(q.Limit).
		All(ctx)
	if err != nil {
		return biz.SkillVersionListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	items := make([]biz.SkillVersionDetail, 0, len(rows))
	for _, row := range rows {
		items = append(items, entSkillVersionToBiz(row))
	}
	return biz.SkillVersionListResult{
		Items:  items,
		Total:  count,
		Limit:  q.Limit,
		Offset: q.Offset,
	}, nil
}

func (r *skillRepo) RollbackSkillVersion(ctx context.Context, skillID string, versionID string) (biz.Skill, error) {
	now := nowRFC3339()
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	skillRow, err := tx.PlatformSkill.Query().
		Where(platformskill.IDEQ(skillID), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}

	// Validate skill state: only published skills can be rolled back
	if skillRow.Status != "published" && skillRow.Status != "active" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "only published skills can be rolled back")
	}

	targetVer, err := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(skillID), skillversion.IDEQ(versionID)).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	newVerID := fmt.Sprintf("sv_%d", time.Now().UTC().UnixNano())
	if _, err := tx.SkillVersion.Create().
		SetID(newVerID).
		SetSkillID(skillID).
		SetVersion(incrementVersion(targetVer.Version)).
		SetStatus("pass").
		SetContentMarkdown(targetVer.ContentMarkdown).
		SetMetadataJSON(targetVer.MetadataJSON).
		SetManifestJSON(targetVer.ManifestJSON).
		SetFileManifestJSON(targetVer.FileManifestJSON).
		SetValidationStatus("pass").
		SetPublishedAt(now).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if _, err := tx.PlatformSkill.UpdateOneID(skillID).
		SetStatus("published").
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, skillID)
}

// CreateSkillVersion appends an evolved version to an existing skill.
// "Current" resolves by created_at DESC (see GetLatestSkillMarkdown), so
// inserting the row activates it — no platform_skill pointer to switch.
// Version metadata/manifests are inherited from the latest existing version;
// the parent anchor is explicit or falls back to that same latest version.
func (r *skillRepo) CreateSkillVersion(ctx context.Context, in biz.SkillCreateVersionInput) (biz.SkillVersionDetail, error) {
	in.SkillID = strings.TrimSpace(in.SkillID)
	in.Body = strings.TrimSpace(in.Body)
	if in.SkillID == "" || in.Body == "" {
		return biz.SkillVersionDetail{}, apierror.BadRequest(apierror.DomainSkill, "skill id and body are required")
	}
	now := nowRFC3339()
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.SkillVersionDetail{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	latest, err := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(in.SkillID)).
		// 同 GetLatestSkillMarkdown：秒级 created_at 并列时按 ID 递减兜底。
		Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.SkillVersionDetail{}, apierror.NotFound(apierror.DomainSkill, "no existing version for skill: %s", in.SkillID)
		}
		return biz.SkillVersionDetail{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	parentID := strings.TrimSpace(in.ParentVersionID)
	if parentID == "" {
		parentID = latest.ID
	}
	newVerID := fmt.Sprintf("sv_%d", time.Now().UTC().UnixNano())
	row, err := tx.SkillVersion.Create().
		SetID(newVerID).
		SetSkillID(in.SkillID).
		SetVersion(incrementVersion(latest.Version)).
		SetStatus("pass").
		SetContentMarkdown(in.Body).
		SetMetadataJSON(latest.MetadataJSON).
		SetManifestJSON(latest.ManifestJSON).
		SetFileManifestJSON(latest.FileManifestJSON).
		SetParentVersionID(parentID).
		SetEvolutionReason(in.EvolutionReason).
		SetValidationStatus("pass").
		SetPublishedAt(now).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.SkillVersionDetail{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if err = tx.Commit(); err != nil {
		return biz.SkillVersionDetail{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	// F3 (P-evo-1)：版本创建成功后同步落盘，保持 DB 与磁盘两个真相源一致，
	// 防止 filesystem watcher 以陈旧磁盘内容回滚进化成果。
	r.syncSkillBodyToDisk(ctx, in.SkillID, in.Body)
	return entSkillVersionToBiz(row), nil
}

// syncSkillBodyToDisk best-effort 将 body 写入 skill 磁盘目录的 SKILL.md。
// 失败只记日志不返回错误——UpsertSkillFromDisk 的进化版保护会在下次扫描时
// 以 DB 为准重新收敛磁盘。无磁盘载体（storage_dir 未配置）的 skill 直接跳过。
func (r *skillRepo) syncSkillBodyToDisk(ctx context.Context, skillID, body string) {
	dir, err := r.GetSkillStorageDir(ctx, skillID)
	if err != nil {
		return
	}
	writeSkillBodyToDisk(r.data.lg, dir, body)
}

func writeSkillBodyToDisk(lg loggateway.Logger, dir, body string) {
	if strings.TrimSpace(dir) == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		lg.Warn("skill disk sync: mkdir failed", loggateway.StepID("data.skill"), loggateway.Str("dir", dir), loggateway.Err(err))
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		lg.Warn("skill disk sync: write SKILL.md failed", loggateway.StepID("data.skill"), loggateway.Str("dir", dir), loggateway.Err(err))
	}
}

func entSkillVersionToBiz(row *dataent.SkillVersion) biz.SkillVersionDetail {
	return biz.SkillVersionDetail{
		ID:               row.ID,
		SkillID:          row.SkillID,
		Version:          row.Version,
		Status:           row.Status,
		ContentMarkdown:  row.ContentMarkdown,
		ValidationStatus: row.ValidationStatus,
		PublishedAt:      row.PublishedAt,
		CreatedAt:        row.CreatedAt,
		FileManifestJSON: row.FileManifestJSON,
	}
}

func incrementVersion(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return v + "+rollback"
	}
	patch := 0
	if n, err := fmt.Sscanf(parts[2], "%d", &patch); err == nil && n == 1 {
		return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
	}
	return v + "+rollback"
}

func isPathWithinRoot(root, path string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) || absPath == absRoot
}
