package skill

import (
	"context"
	"regexp"
	"strings"

	"aranea-agents/pkg/apierror"
)

// TagInfo 是字典标签 + 实时使用计数（聚合 skill.metadata_json，不落库）。
type TagInfo struct {
	Name      string `json:"name"`
	Dimension string `json:"dimension"` // `:` 前缀维度，无维度为空串
	Source    string `json:"source"`    // system | user
	UsedCount int    `json:"used_count"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// SkillTagReader 标签字典读。
//
// Stability:evolving
type SkillTagReader interface {
	// ListSkillTags 返回字典全量标签，UsedCount 为实时聚合结果。
	ListSkillTags(ctx context.Context) ([]TagInfo, error)
	// ListSkillTagNames 轻量选项源：仅规范标签名（筛选器下拉用）。
	ListSkillTagNames(ctx context.Context) ([]string, error)
}

// SkillTagWriter 标签字典写 + 治理重写。
// Rename/Delete 在同一事务内重写所有引用该标签的 skill metadata_json。
//
// Stability:evolving
type SkillTagWriter interface {
	// CreateSkillTag 预建标签（name 已规范化）。冲突返回 CodeConflict。
	CreateSkillTag(ctx context.Context, name string) (TagInfo, error)
	// RenameSkillTag 字典改名 + 重写所有 skill 引用，返回重写条数。
	RenameSkillTag(ctx context.Context, oldName, newName string) (int, error)
	// DeleteSkillTag 字典删除 + 从所有 skill 引用中移除，返回重写条数。
	DeleteSkillTag(ctx context.Context, name string) (int, error)
}

// TagRepo 标签字典读写端口。独立于 Skill Repo 复合接口（DB-N3）。
type TagRepo interface {
	SkillTagReader
	SkillTagWriter
}

// tagNamePattern 规范标签 token：小写字母数字开头，允许 _ -，可选 dimension: 前缀。
var tagNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(:[a-z0-9][a-z0-9_-]*)?$`)

// normalizeTagName 规范化标签名：trim + 小写 + 格式校验，返回规范名与维度。
func normalizeTagName(raw string) (name string, dimension string, err error) {
	name = strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return "", "", apierror.BadRequest("SKILL", "tag name is empty")
	}
	if len(name) > 128 {
		return "", "", apierror.BadRequest("SKILL", "tag name exceeds 128 chars")
	}
	if !tagNamePattern.MatchString(name) {
		return "", "", apierror.BadRequest("SKILL", "tag name must match [a-z0-9][a-z0-9_-]*(:suffix)?")
	}
	if idx := strings.Index(name, ":"); idx > 0 {
		dimension = name[:idx]
	}
	return name, dimension, nil
}

func (u *Usecase) tagRepoOrErr() (TagRepo, error) {
	if u.tagRepo == nil {
		return nil, apierror.Internal("SKILL", "tag repo not configured")
	}
	return u.tagRepo, nil
}

// ListTags 字典全量 + 实时使用计数，按 dimension + name 排序由 repo 保证。
func (u *Usecase) ListTags(ctx context.Context) ([]TagInfo, error) {
	repo, err := u.tagRepoOrErr()
	if err != nil {
		return nil, err
	}
	return repo.ListSkillTags(ctx)
}

// CreateTag 预建标签（管理员治理或前端选项源兜底）。
func (u *Usecase) CreateTag(ctx context.Context, rawName string) (TagInfo, error) {
	repo, err := u.tagRepoOrErr()
	if err != nil {
		return TagInfo{}, err
	}
	name, _, err := normalizeTagName(rawName)
	if err != nil {
		return TagInfo{}, err
	}
	return repo.CreateSkillTag(ctx, name)
}

// RenameTag 改名并事务重写所有 skill 引用；成功后全量失效路由缓存
// （skillCorpusText 含 tags，向量必须重算）。
func (u *Usecase) RenameTag(ctx context.Context, rawOld, rawNew string) (int, error) {
	repo, err := u.tagRepoOrErr()
	if err != nil {
		return 0, err
	}
	oldName, _, err := normalizeTagName(rawOld)
	if err != nil {
		return 0, err
	}
	newName, _, err := normalizeTagName(rawNew)
	if err != nil {
		return 0, err
	}
	if oldName == newName {
		return 0, apierror.BadRequest("SKILL", "old and new tag names are identical")
	}
	rewritten, err := repo.RenameSkillTag(ctx, oldName, newName)
	if err != nil {
		return 0, err
	}
	u.InvalidateEmbedCache()
	u.invalidateDedupCache()
	return rewritten, nil
}

// DeleteTag 删除标签并从事务内所有 skill 引用中移除。
func (u *Usecase) DeleteTag(ctx context.Context, rawName string) (int, error) {
	repo, err := u.tagRepoOrErr()
	if err != nil {
		return 0, err
	}
	name, _, err := normalizeTagName(rawName)
	if err != nil {
		return 0, err
	}
	rewritten, err := repo.DeleteSkillTag(ctx, name)
	if err != nil {
		return 0, err
	}
	u.InvalidateEmbedCache()
	u.invalidateDedupCache()
	return rewritten, nil
}
