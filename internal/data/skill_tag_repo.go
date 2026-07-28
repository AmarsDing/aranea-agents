package data

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	bizskill "aranea-agents/internal/biz/skill"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/skilltag"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// skillTagRepo implements bizskill.TagRepo：标签字典 CRUD + 治理重写。
// tags 实际存储在 skill.metadata_json 的 tags 数组；skill_tags 表是治理字典。
type skillTagRepo struct {
	data *Data
}

var _ bizskill.TagRepo = (*skillTagRepo)(nil)

// NewSkillTagRepo implements biz.SkillTagRepo via Ent.
func NewSkillTagRepo(d *Data) bizskill.TagRepo {
	return &skillTagRepo{data: d}
}

// skillTagUsage 聚合所有 skill metadata_json 中的标签使用计数（小写 name → count）。
func (r *skillTagRepo) skillTagUsage(ctx context.Context) (map[string]int, error) {
	rows, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.DeletedAtEQ("")).
		Select(platformskill.FieldID, platformskill.FieldMetadataJSON).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	usage := map[string]int{}
	for _, row := range rows {
		for _, t := range parseSkillTags(row.MetadataJSON) {
			usage[strings.ToLower(t.Name)]++
		}
	}
	return usage, nil
}

func entSkillTagToBiz(e *dataent.SkillTag, usage map[string]int) bizskill.TagInfo {
	return bizskill.TagInfo{
		Name:      e.Name,
		Dimension: e.Dimension,
		Source:    e.Source,
		UsedCount: usage[e.Name],
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

func sortTagInfos(items []bizskill.TagInfo) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Dimension != items[j].Dimension {
			return items[i].Dimension < items[j].Dimension
		}
		return items[i].Name < items[j].Name
	})
}

func (r *skillTagRepo) ListSkillTags(ctx context.Context) ([]bizskill.TagInfo, error) {
	usage, err := r.skillTagUsage(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.data.RW().Read(ctx).SkillTag.Query().All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	items := make([]bizskill.TagInfo, 0, len(rows)+len(usage))
	inDict := make(map[string]bool, len(rows))
	for _, e := range rows {
		inDict[e.Name] = true
		items = append(items, entSkillTagToBiz(e, usage))
	}
	// 使用中但未收录进字典的标签：source=orphan，供 UI 提示治理。
	for name, count := range usage {
		if inDict[name] {
			continue
		}
		dim := ""
		if idx := strings.Index(name, ":"); idx > 0 {
			dim = name[:idx]
		}
		items = append(items, bizskill.TagInfo{
			Name:      name,
			Dimension: dim,
			Source:    "orphan",
			UsedCount: count,
		})
	}
	sortTagInfos(items)
	return items, nil
}

func (r *skillTagRepo) ListSkillTagNames(ctx context.Context) ([]string, error) {
	usage, err := r.skillTagUsage(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.data.RW().Read(ctx).SkillTag.Query().
		Select(skilltag.FieldName).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	seen := map[string]bool{}
	names := make([]string, 0, len(rows)+len(usage))
	for _, e := range rows {
		if !seen[e.Name] {
			seen[e.Name] = true
			names = append(names, e.Name)
		}
	}
	for name := range usage {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func (r *skillTagRepo) CreateSkillTag(ctx context.Context, name string) (bizskill.TagInfo, error) {
	dim := ""
	if idx := strings.Index(name, ":"); idx > 0 {
		dim = name[:idx]
	}
	now := nowRFC3339()
	e, err := r.data.RW().Write(ctx).SkillTag.Create().
		SetID(fmt.Sprintf("skilltag_%d", time.Now().UTC().UnixNano())).
		SetName(name).
		SetDimension(dim).
		SetSource("user").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return bizskill.TagInfo{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return entSkillTagToBiz(e, map[string]int{}), nil
}

// rewriteSkillTagReferences scans all skills and applies mutate to each skill's
// tag list. Runs inside txCtx; returns the number of skills rewritten.
func (r *skillTagRepo) rewriteSkillTagReferences(
	txCtx context.Context,
	target string,
	mutate func(tags []bizskill.SkillTag, target string) ([]bizskill.SkillTag, bool),
) (int, error) {
	rows, err := r.data.RW().Read(txCtx).PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.MetadataJSONContainsFold(target),
		).
		Select(platformskill.FieldID, platformskill.FieldMetadataJSON).
		All(txCtx)
	if err != nil {
		return 0, entErrToBizErr(err, apierror.DomainSkill)
	}
	now := nowRFC3339()
	rewritten := 0
	for _, row := range rows {
		tags := parseSkillTags(row.MetadataJSON)
		next, changed := mutate(tags, target)
		if !changed {
			continue
		}
		meta, mErr := rewriteMetadataTags(row.MetadataJSON, next)
		if mErr != nil {
			return rewritten, apierror.Internal(apierror.DomainSkill, "marshal skill metadata failed")
		}
		if uErr := r.data.RW().Write(txCtx).PlatformSkill.UpdateOneID(row.ID).
			SetMetadataJSON(meta).
			SetUpdatedAt(now).
			Exec(txCtx); uErr != nil {
			return rewritten, entErrToBizErr(uErr, apierror.DomainSkill)
		}
		rewritten++
	}
	return rewritten, nil
}

// rewriteMetadataTags replaces only the "tags" key in the metadata JSON,
// preserving all other keys (taxonomy_paths, storage_dir, sync_origin, ...).
func rewriteMetadataTags(raw string, tags []bizskill.SkillTag) (string, error) {
	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil || meta == nil {
		meta = map[string]any{}
	}
	meta["tags"] = tags
	out, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (r *skillTagRepo) RenameSkillTag(ctx context.Context, oldName, newName string) (int, error) {
	rewritten := 0
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		// 字典行：old 必须已收录（未收录返回 NotFound，避免静默吞掉治理操作）。
		dictRow, err := r.data.RW().Read(txCtx).SkillTag.Query().
			Where(skilltag.NameEQ(oldName)).
			Only(txCtx)
		if err != nil {
			if dataent.IsNotFound(err) {
				return apierror.NotFound(apierror.DomainSkill, "tag not found in dictionary")
			}
			return entErrToBizErr(err, apierror.DomainSkill)
		}
		// newName 已收录：删除 old 行（等价合并）；未收录：原地改名。
		_, existErr := r.data.RW().Read(txCtx).SkillTag.Query().
			Where(skilltag.NameEQ(newName)).
			Only(txCtx)
		switch {
		case existErr == nil:
			if err := r.data.RW().Write(txCtx).SkillTag.DeleteOneID(dictRow.ID).Exec(txCtx); err != nil {
				return entErrToBizErr(err, apierror.DomainSkill)
			}
		case dataent.IsNotFound(existErr):
			dim := ""
			if idx := strings.Index(newName, ":"); idx > 0 {
				dim = newName[:idx]
			}
			if err := r.data.RW().Write(txCtx).SkillTag.UpdateOneID(dictRow.ID).
				SetName(newName).
				SetDimension(dim).
				SetUpdatedAt(nowRFC3339()).
				Exec(txCtx); err != nil {
				return entErrToBizErr(err, apierror.DomainSkill)
			}
		default:
			return entErrToBizErr(existErr, apierror.DomainSkill)
		}
		// 重写所有 skill 引用。
		n, err := r.rewriteSkillTagReferences(txCtx, oldName,
			func(tags []bizskill.SkillTag, target string) ([]bizskill.SkillTag, bool) {
				changed := false
				out := make([]bizskill.SkillTag, 0, len(tags))
				for _, t := range tags {
					if strings.EqualFold(t.Name, target) {
						t.Name = newName
						changed = true
					}
					out = append(out, t)
				}
				if !changed {
					return tags, false
				}
				return normalizeSkillTags(out), true
			})
		rewritten = n
		return err
	})
	if err != nil {
		return 0, err
	}
	return rewritten, nil
}

func (r *skillTagRepo) DeleteSkillTag(ctx context.Context, name string) (int, error) {
	rewritten := 0
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		// 字典行删除（未收录不报错：仍需清理 skill 引用中的孤儿标签）。
		if _, err := r.data.RW().Write(txCtx).SkillTag.Delete().
			Where(skilltag.NameEQ(name)).
			Exec(txCtx); err != nil {
			return entErrToBizErr(err, apierror.DomainSkill)
		}
		n, err := r.rewriteSkillTagReferences(txCtx, name,
			func(tags []bizskill.SkillTag, target string) ([]bizskill.SkillTag, bool) {
				out := make([]bizskill.SkillTag, 0, len(tags))
				changed := false
				for _, t := range tags {
					if strings.EqualFold(t.Name, target) {
						changed = true
						continue
					}
					out = append(out, t)
				}
				if !changed {
					return tags, false
				}
				return out, true
			})
		rewritten = n
		return err
	})
	if err != nil {
		return 0, err
	}
	r.data.lg.Info("skill tag deleted", loggateway.StepID("data.skill_tag"),
		loggateway.Str("tag", name), loggateway.Int("rewritten", rewritten))
	return rewritten, nil
}
