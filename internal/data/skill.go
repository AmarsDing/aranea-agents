package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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

func (r *skillRepo) enrichSkill(ctx context.Context, e *dataent.PlatformSkill) (biz.Skill, error) {
	c := r.client()
	id := e.ID
	item := biz.Skill{
		ID:          e.ID,
		Slug:        e.SkillKey,
		Name:        e.Name,
		Description: e.Description,
		Status:      normalizeSkillStatus(e.Status),
		Enabled:     e.Enabled,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
		Permissions: biz.SkillPermissions{
			CanEdit: true, CanDelete: true, CanToggleEnabled: true, CanDuplicate: true,
		},
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
		item.CurrentVersion = &biz.SkillVersionSummary{
			ID:               sv.ID,
			Version:          sv.Version,
			ValidationStatus: st,
			PublishedAt:      sv.CreatedAt,
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
	newID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	newKey := fmt.Sprintf("%s-copy-%d", cur.SkillKey, time.Now().UTC().Unix())
	if strings.TrimSpace(cur.SkillKey) == "" {
		newKey = newID
	}
	now := nowRFC3339()
	_, err = r.client().PlatformSkill.Create().
		SetID(newID).
		SetSkillKey(newKey).
		SetName(cur.Name + " Copy").
		SetDescription(cur.Description).
		SetStatus("draft").
		SetEnabled(false).
		SetSortOrder(0).
		SetConfigJSON(cur.ConfigJSON).
		SetMetadataJSON(cur.MetadataJSON).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetDeletedAt("").
		Save(ctx)
	if err != nil {
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
			Permissions:      biz.SkillInvocationPermissions{CanViewDetail: true},
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
	metadata := struct {
		Tags       []biz.SkillTag `json:"tags"`
		StorageDir string         `json:"storage_dir"`
	}{Tags: in.Tags, StorageDir: in.StorageDir}
	metaJSON, err := json.Marshal(metadata)
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
