package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/internal/data/ent/skillversion"

	entsql "entgo.io/ent/dialect/sql"
)

type skillRepo struct {
	data *Data
}

// NewSkillRepo implements biz.SkillRepo via Ent (pkg-backend-to-kratos §2.2 — only entClient, no parallel sqlite pool).
func NewSkillRepo(d *Data) biz.SkillRepo {
	return &skillRepo{data: d}
}

func (r *skillRepo) client() *dataent.Client {
	return r.data.entClient
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

func skillListPredicates(q biz.SkillListQuery) []predicate.PlatformSkill {
	ps := []predicate.PlatformSkill{platformskill.DeletedAtEQ("")}
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
		ps = append(ps, platformskill.MetadataJSONContains(`"sync_origin":"`+origin+`"`))
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

func parseTaxonomyPathsFromJSON(blob string) []string {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return nil
	}
	var wrap struct {
		TaxonomyPaths []string `json:"taxonomy_paths"`
	}
	if err := json.Unmarshal([]byte(blob), &wrap); err != nil || len(wrap.TaxonomyPaths) == 0 {
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

func mergeTaxonomyPaths(meta, cfg string) []string {
	a := parseTaxonomyPathsFromJSON(meta)
	b := parseTaxonomyPathsFromJSON(cfg)
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

func (r *skillRepo) enrichSkill(ctx context.Context, e *dataent.PlatformSkill) (biz.Skill, error) {
	c := r.client()
	id := e.ID
	item := biz.Skill{
		ID:                e.ID,
		Slug:              e.SkillKey,
		Name:              e.Name,
		Description:       e.Description,
		Status:            normalizeSkillStatus(e.Status),
		Enabled:           e.Enabled,
		FilesystemMissing: e.FilesystemMissing,
		SyncOrigin:        parseSkillMetadata(e.MetadataJSON).SyncOrigin,
		Visibility:        e.Visibility,
		DefaultConfigJSON: e.FallbackConfigJSON,
		CreatedAt:         e.CreatedAt,
		UpdatedAt:         e.UpdatedAt,
		Permissions:       biz.SkillPermissions{},
	}
	item.Tags = parseSkillTags(e.MetadataJSON)
	if len(item.Tags) == 0 {
		item.Tags = parseSkillTags(e.ConfigJSON)
	}

	totalInv, err := c.SkillInvocation.Query().Where(skillinvocation.SkillIDEQ(id)).Count(ctx)
	if err != nil {
		return biz.Skill{}, err
	}
	success, err := c.SkillInvocation.Query().Where(skillinvocation.SkillIDEQ(id), skillinvocation.StatusEQ("success")).Count(ctx)
	if err != nil {
		return biz.Skill{}, err
	}
	failure, err := c.SkillInvocation.Query().Where(skillinvocation.SkillIDEQ(id), skillinvocation.StatusEQ("failure")).Count(ctx)
	if err != nil {
		return biz.Skill{}, err
	}
	threshold := time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339)
	usage7d, err := c.SkillInvocation.Query().Where(skillinvocation.SkillIDEQ(id), invocationTimeGTE(threshold)).Count(ctx)
	if err != nil {
		return biz.Skill{}, err
	}
	item.InvokeCount = totalInv
	item.SuccessCount = success
	item.FailureCount = failure
	item.UsageCount7d = usage7d

	durs, err := c.SkillInvocation.Query().
		Where(skillinvocation.SkillIDEQ(id), skillinvocation.DurationMsGT(0)).
		Limit(5000).
		All(ctx)
	if err != nil {
		return biz.Skill{}, err
	}
	if len(durs) > 0 {
		var sum int64
		for _, row := range durs {
			sum += int64(row.DurationMs)
		}
		avg := float64(sum) / float64(len(durs))
		item.AvgDurationMS = &avg
	}

	sv, err := c.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(id)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	if err == nil && sv != nil {
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

	last, err := c.SkillInvocation.Query().
		Where(skillinvocation.SkillIDEQ(id)).
		Order(skillinvocation.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	if err == nil && last != nil {
		item.LastAgentID = last.AgentID
		item.LastInvokedAt = coalesceTime(last.StartedAt, last.CreatedAt)
		if last.DurationMs > 0 {
			d := last.DurationMs
			item.LastDurationMS = &d
		}
		if strings.TrimSpace(last.AgentID) != "" {
			a, aerr := c.Agent.Query().Where(agent.IDEQ(last.AgentID)).Only(ctx)
			if aerr == nil && a != nil {
				item.LastAgentDisplayName = a.DisplayName
			}
		}
	}
	return item, nil
}

func coalesceTime(startedAt, createdAt string) string {
	if strings.TrimSpace(startedAt) != "" {
		return startedAt
	}
	return createdAt
}

func (r *skillRepo) SearchSkills(ctx context.Context, q biz.SkillListQuery) (biz.SkillListResult, error) {
	c := r.client()
	preds := skillListPredicates(q)
	total, err := c.PlatformSkill.Query().Where(preds...).Count(ctx)
	if err != nil {
		return biz.SkillListResult{}, err
	}
	rows, err := c.PlatformSkill.Query().
		Where(preds...).
		Order(platformskill.ByUpdatedAt(entsql.OrderDesc()), platformskill.ByCreatedAt(entsql.OrderDesc())).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.SkillListResult{}, err
	}
	items := make([]biz.Skill, 0, len(rows))
	for _, e := range rows {
		sk, err := r.enrichSkill(ctx, e)
		if err != nil {
			return biz.SkillListResult{}, err
		}
		items = append(items, sk)
	}
	return biz.SkillListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

func (r *skillRepo) GetSkillByID(ctx context.Context, id string) (biz.Skill, error) {
	e, err := r.client().PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, sql.ErrNoRows
		}
		return biz.Skill{}, err
	}
	return r.enrichSkill(ctx, e)
}

func (r *skillRepo) UpdateSkillEnabled(ctx context.Context, id string, enabled bool) (biz.Skill, error) {
	if id == "" {
		return biz.Skill{}, errors.New("skill id is required")
	}
	err := r.client().PlatformSkill.UpdateOneID(id).
		SetEnabled(enabled).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, sql.ErrNoRows
		}
		return biz.Skill{}, err
	}
	return r.GetSkillByID(ctx, id)
}

func (r *skillRepo) DuplicateSkill(ctx context.Context, id string) (biz.Skill, error) {
	cur, err := r.client().PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, sql.ErrNoRows
		}
		return biz.Skill{}, err
	}
	latestVer, _ := r.client().SkillVersion.Query().
		Where(skillversion.SkillIDEQ(id)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	newID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	newKey := fmt.Sprintf("%s-copy-%d", cur.SkillKey, time.Now().UTC().Unix())
	if strings.TrimSpace(cur.SkillKey) == "" {
		newKey = newID
	}
	now := nowRFC3339()
	tx, err := r.client().Tx(ctx)
	if err != nil {
		return biz.Skill{}, err
	}
	defer func() { _ = tx.Rollback() }()
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
		Save(ctx); err != nil {
		return biz.Skill{}, err
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
			return biz.Skill{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, err
	}
	return r.GetSkillByID(ctx, newID)
}

func (r *skillRepo) DeleteSkill(ctx context.Context, id string) error {
	now := nowRFC3339()
	return r.client().PlatformSkill.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Exec(ctx)
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
	c := r.client()
	preds := runPredicates(query)
	base := c.SkillInvocation.Query()
	if len(preds) > 0 {
		base = base.Where(preds...)
	}
	total, err := base.Clone().Count(ctx)
	if err != nil {
		return biz.SkillRunResult{}, err
	}
	rows, err := base.Clone().
		Order(skillinvocation.ByCreatedAt(entsql.OrderDesc())).
		Limit(query.Limit).
		Offset(query.Offset).
		All(ctx)
	if err != nil {
		return biz.SkillRunResult{}, err
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
			return biz.SkillRunResult{}, err
		}
		for _, s := range skills {
			skillNames[s.ID] = s.Name
		}
	}
	agentNames := map[string]string{}
	if len(aidList) > 0 {
		agents, err := c.Agent.Query().Where(agent.IDIn(aidList...)).All(ctx)
		if err != nil {
			return biz.SkillRunResult{}, err
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
	e, err := r.client().PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return "", err
	}
	var metadata struct {
		StorageDir string `json:"storage_dir"`
	}
	if err := json.Unmarshal([]byte(e.MetadataJSON), &metadata); err != nil {
		return "", err
	}
	if strings.TrimSpace(metadata.StorageDir) == "" {
		return "", errors.New("skill storage directory is not configured")
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
	c := r.client()
	rows, err := c.PlatformSkill.Query().
		Where(platformskill.DeletedAtEQ("")).
		Order(platformskill.ByUpdatedAt(entsql.OrderDesc()), platformskill.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.SkillSimilaritySource, 0, len(rows))
	for _, s := range rows {
		item := biz.SkillSimilaritySource{
			ID:          s.ID,
			Name:        s.Name,
			Slug:        s.SkillKey,
			Description: s.Description,
		}
		sv, err := c.SkillVersion.Query().
			Where(skillversion.SkillIDEQ(s.ID)).
			Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
			First(ctx)
		if err != nil {
			if dataent.IsNotFound(err) {
				out = append(out, item)
				continue
			}
			return nil, err
		}
		item.Version = sv.Version
		item.Body = sv.ContentMarkdown
		item.BodyPreview = previewSkillBody(item.Body, 240)
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
		return biz.Skill{}, errors.New("skill name, slug and body are required")
	}
	skillID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	versionID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
	metaJSON, err := encodeSkillMetadata(in.Tags, in.StorageDir, in.SyncOrigin)
	if err != nil {
		return biz.Skill{}, err
	}
	now := nowRFC3339()
	tx, err := r.client().Tx(ctx)
	if err != nil {
		return biz.Skill{}, err
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
		Save(ctx); err != nil {
		return biz.Skill{}, err
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
		return biz.Skill{}, err
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, err
	}
	return r.GetSkillByID(ctx, skillID)
}

func (r *skillRepo) GetSkillBySkillKey(ctx context.Context, skillKey string) (biz.Skill, error) {
	skillKey = strings.TrimSpace(skillKey)
	if skillKey == "" {
		return biz.Skill{}, errors.New("skill key is required")
	}
	e, err := r.client().PlatformSkill.Query().
		Where(platformskill.SkillKeyEQ(skillKey), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return biz.Skill{}, err
	}
	return r.enrichSkill(ctx, e)
}

func (r *skillRepo) UpsertSkillFromDisk(ctx context.Context, in biz.SkillDiskSyncInput) (biz.Skill, biz.SkillDiskSyncOutcome, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	in.Description = strings.TrimSpace(in.Description)
	in.Body = strings.TrimSpace(in.Body)
	if in.Name == "" || in.Slug == "" || in.Body == "" {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, errors.New("skill name, slug and body are required")
	}
	skillRow, err := r.client().PlatformSkill.Query().
		Where(platformskill.SkillKeyEQ(in.Slug), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if dataent.IsNotFound(err) {
		sk, createErr := r.CreateSkillWithVersion(ctx, biz.SkillCreateInput{
			Name:        in.Name,
			Slug:        in.Slug,
			Description: in.Description,
			Body:        in.Body,
			Tags:        in.Tags,
			StorageDir:  in.StorageDir,
			SyncOrigin:  biz.SkillSyncOriginFilesystem,
		})
		return sk, biz.SkillDiskSyncOutcome{}, createErr
	}
	if err != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, err
	}
	outcome := biz.SkillDiskSyncOutcome{}
	wasPublished := skillRow.Status == "published" || skillRow.Status == "active"
	now := nowRFC3339()
	metaJSON, err := encodeSkillMetadata(in.Tags, in.StorageDir, biz.SkillSyncOriginFilesystem)
	if err != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, err
	}
	update := r.client().PlatformSkill.UpdateOneID(skillRow.ID).
		SetName(in.Name).
		SetDescription(in.Description).
		SetMetadataJSON(string(metaJSON)).
		SetUpdatedAt(now).
		SetFilesystemMissing(false)
	sv, err := r.client().SkillVersion.Query().
		Where(skillversion.SkillIDEQ(skillRow.ID)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, err
	}
	if strings.TrimSpace(sv.ContentMarkdown) != in.Body {
		outcome.ContentChanged = true
		if _, err := r.client().SkillVersion.UpdateOneID(sv.ID).
			SetContentMarkdown(in.Body).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, err
		}
	}
	if outcome.ContentChanged && wasPublished {
		outcome.RevertedToDraft = true
		update = update.SetStatus("draft").SetEnabled(false)
	}
	if _, err := update.Save(ctx); err != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, err
	}
	sk, err := r.GetSkillByID(ctx, skillRow.ID)
	return sk, outcome, err
}

func (r *skillRepo) ListRegisteredSlugs(ctx context.Context) ([]string, error) {
	rows, err := r.client().PlatformSkill.Query().
		Where(platformskill.DeletedAtEQ("")).
		All(ctx)
	if err != nil {
		return nil, err
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
	rows, err := r.client().PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.EnabledEQ(true),
			platformskill.Or(platformskill.StatusEQ("published"), platformskill.StatusEQ("active")),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.SkillKey)
	}
	return out, nil
}

func (r *skillRepo) ListEnabledPublishedSkillCandidates(ctx context.Context) ([]biz.SkillRuntimeCandidate, error) {
	rows, err := r.client().PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.EnabledEQ(true),
			platformskill.Or(platformskill.StatusEQ("published"), platformskill.StatusEQ("active")),
		).
		All(ctx)
	if err != nil {
		return nil, err
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
			TaxonomyPaths: mergeTaxonomyPaths(row.MetadataJSON, row.ConfigJSON),
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
	_, err := r.client().SkillInvocation.Create().
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
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *skillRepo) GetLatestSkillMarkdown(ctx context.Context, skillID string) (string, error) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "", errors.New("skill id is required")
	}
	sv, err := r.client().SkillVersion.Query().
		Where(skillversion.SkillIDEQ(skillID)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return "", sql.ErrNoRows
		}
		return "", err
	}
	return sv.ContentMarkdown, nil
}

func (r *skillRepo) BatchGetSkillMarkdownBySlugs(ctx context.Context, slugs []string) (map[string]string, error) {
	if len(slugs) == 0 {
		return map[string]string{}, nil
	}
	skills, err := r.client().PlatformSkill.Query().
		Where(platformskill.SkillKeyIn(slugs...)).
		All(ctx)
	if err != nil {
		return nil, err
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
	rows, err := r.client().SkillVersion.Query().
		Where(skillversion.SkillIDIn(skillIDs...)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
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
}

func encodeSkillMetadata(tags []biz.SkillTag, storageDir, syncOrigin string) (string, error) {
	md := skillMetadataEnvelope{
		Tags:       tags,
		StorageDir: strings.TrimSpace(storageDir),
		SyncOrigin: strings.TrimSpace(syncOrigin),
	}
	b, err := json.Marshal(md)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseSkillMetadata(raw string) skillMetadataEnvelope {
	var md skillMetadataEnvelope
	_ = json.Unmarshal([]byte(raw), &md)
	return md
}

func (r *skillRepo) PatchSkill(ctx context.Context, id string, patch biz.SkillUpdateDraft) (biz.Skill, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Skill{}, errors.New("skill id is required")
	}
	e, err := r.client().PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, sql.ErrNoRows
		}
		return biz.Skill{}, err
	}
	now := nowRFC3339()
	upd := r.client().PlatformSkill.UpdateOneID(id).SetUpdatedAt(now)
	if patch.HasName {
		upd.SetName(strings.TrimSpace(patch.Name))
	}
	if patch.HasDescription {
		upd.SetDescription(strings.TrimSpace(patch.Description))
	}
	if patch.HasTags {
		md := parseSkillMetadata(e.MetadataJSON)
		md.Tags = normalizeSkillTags(patch.Tags)
		metaJSON, jerr := json.Marshal(md)
		if jerr != nil {
			return biz.Skill{}, jerr
		}
		upd.SetMetadataJSON(string(metaJSON))
	}
	if err := upd.Exec(ctx); err != nil {
		return biz.Skill{}, err
	}
	if patch.HasBody {
		body := strings.TrimSpace(patch.Body)
		sv, err := r.client().SkillVersion.Query().
			Where(skillversion.SkillIDEQ(id)).
			Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
			First(ctx)
		if err != nil {
			return biz.Skill{}, err
		}
		if _, err := r.client().SkillVersion.UpdateOneID(sv.ID).
			SetContentMarkdown(body).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return biz.Skill{}, err
		}
		fresh, err := r.client().PlatformSkill.Query().
			Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
			Only(ctx)
		if err != nil {
			return biz.Skill{}, err
		}
		md := parseSkillMetadata(fresh.MetadataJSON)
		dir := strings.TrimSpace(md.StorageDir)
		if dir == "" {
			return biz.Skill{}, errors.New("skill storage directory is not configured")
		}
		path := filepath.Join(dir, "SKILL.md")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return biz.Skill{}, err
		}
	}
	return r.GetSkillByID(ctx, id)
}

func (r *skillRepo) PublishSkill(ctx context.Context, id string) (biz.Skill, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Skill{}, errors.New("skill id is required")
	}
	now := nowRFC3339()
	tx, err := r.client().Tx(ctx)
	if err != nil {
		return biz.Skill{}, err
	}
	defer func() { _ = tx.Rollback() }()
	err = tx.PlatformSkill.UpdateOneID(id).
		SetStatus("published").
		SetUpdatedAt(now).
		Exec(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, sql.ErrNoRows
		}
		return biz.Skill{}, err
	}
	sv, err := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(id)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	if err == nil && sv != nil {
		if _, serr := tx.SkillVersion.UpdateOneID(sv.ID).
			SetPublishedAt(now).
			SetValidationStatus("pass").
			SetUpdatedAt(now).
			Save(ctx); serr != nil {
			return biz.Skill{}, serr
		}
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, err
	}
	return r.GetSkillByID(ctx, id)
}

func (r *skillRepo) MarkSkillFilesystemMissing(ctx context.Context, slug string, missing bool) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return errors.New("skill slug is required")
	}
	n, err := r.client().PlatformSkill.Update().
		Where(platformskill.SkillKeyEQ(slug), platformskill.DeletedAtEQ("")).
		SetFilesystemMissing(missing).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *skillRepo) FilesystemHealthStats(ctx context.Context) (biz.SkillFilesystemHealthStats, error) {
	c := r.client()
	missing, err := c.PlatformSkill.Query().
		Where(platformskill.DeletedAtEQ(""), platformskill.FilesystemMissingEQ(true)).
		Count(ctx)
	if err != nil {
		return biz.SkillFilesystemHealthStats{}, err
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
		return biz.SkillFilesystemHealthStats{}, err
	}
	return biz.SkillFilesystemHealthStats{
		MissingCount:           missing,
		PendingFilesystemCount: pending,
	}, nil
}

func (r *skillRepo) ListSkillVersions(ctx context.Context, q biz.SkillVersionListQuery) (biz.SkillVersionListResult, error) {
	c := r.client()
	exists, err := c.PlatformSkill.Query().
		Where(platformskill.IDEQ(q.SkillID), platformskill.DeletedAtEQ("")).
		Exist(ctx)
	if err != nil {
		return biz.SkillVersionListResult{}, err
	}
	if !exists {
		return biz.SkillVersionListResult{}, sql.ErrNoRows
	}
	count, err := c.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(q.SkillID)).
		Count(ctx)
	if err != nil {
		return biz.SkillVersionListResult{}, err
	}
	rows, err := c.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(q.SkillID)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		Offset(q.Offset).
		Limit(q.Limit).
		All(ctx)
	if err != nil {
		return biz.SkillVersionListResult{}, err
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
	_, err := r.client().PlatformSkill.Query().
		Where(platformskill.IDEQ(skillID), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, sql.ErrNoRows
		}
		return biz.Skill{}, err
	}
	targetVer, err := r.client().SkillVersion.Query().
		Where(skillversion.SkillIDEQ(skillID), skillversion.IDEQ(versionID)).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, sql.ErrNoRows
		}
		return biz.Skill{}, err
	}
	now := nowRFC3339()
	newVerID := fmt.Sprintf("sv_%d", time.Now().UTC().UnixNano())
	tx, err := r.client().Tx(ctx)
	if err != nil {
		return biz.Skill{}, err
	}
	defer func() { _ = tx.Rollback() }()
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
		return biz.Skill{}, err
	}
	if _, err := tx.PlatformSkill.UpdateOneID(skillID).
		SetStatus("published").
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return biz.Skill{}, err
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, err
	}
	return r.GetSkillByID(ctx, skillID)
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
