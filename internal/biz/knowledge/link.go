package knowledge

import (
	"context"

	"aranea-agents/pkg/apierror"
)

// 关联来源类型（P2-4 双轨关联；R-3：UI 必须标注来源类型）。
const (
	// LinkTypeExplicit 显式双链（[[...]] 解析），最可靠。
	LinkTypeExplicit = "explicit"
	// LinkTypeEntity LLM 实体共现（经停用词/频次过滤）。
	LinkTypeEntity = "entity"
	// LinkTypeSemantic 向量近邻（P4b 语义层；无语义层降级不建）。
	LinkTypeSemantic = "semantic"
)

// Link 一条文档间关联（派生索引：随文档增删级联，可全量重扫重建）。
type Link struct {
	ID           int64
	CollectionID string
	DocID        string // 出向文档
	TargetDocID  string // 入向文档
	LinkType     string // explicit / entity / semantic
	Context      string // explicit: [[ref]] 原文；entity: 共享实体名（逗号分隔）
}

// DocEntity 一条文档实体提及记录。
type DocEntity struct {
	Name       string
	EntityType string
	Mentions   int
}

// EntityCooccurrence 一对文档的实体共现（entity 轨建链依据）。
type EntityCooccurrence struct {
	DocID          string
	SharedEntities []string
}

// LinkRepo 关联持久化窄接口（P2-4）。Usecase 未接线时关联功能降级关闭。
// Stability:evolving
type LinkRepo interface {
	// ReplaceLinks 事务性替换某文档某类型的全部出链（删旧+插新，空切片=仅清理）。
	ReplaceLinks(ctx context.Context, collectionID, docID, linkType string, links []Link) error
	// ListLinks 列出文档的全部关联（出向+入向）；linkType 空 = 全部类型。
	ListLinks(ctx context.Context, collectionID, docID, linkType string) ([]Link, error)
}

// EntityMergeResult 实体合并重写统计（G5-F B10 返回契约，供 UI 内联反馈）。
type EntityMergeResult struct {
	RewrittenMentions int // 重写提及条数
	RewrittenLinks    int // 重写 entity 轨链接 context 条数
	MergedEntities    int // 实际并入实体数（幂等重跑为 0）
}

// EntityRepo 实体持久化窄接口（P2-4；G5-F B9/B12 归一化治理）。
// Stability:evolving
type EntityRepo interface {
	// ReplaceDocEntities 事务性替换文档实体：归一化 name_norm 查/建字典条目
	// （name 保留首见写法作展示名）→ 别名命中 keeper → 新建；同批归一化撞车
	// mentions 求和；孤儿实体清理。返回解析后的实体 ID（去重、保首现序），
	// 供共现查询按 entity_id 关联。
	ReplaceDocEntities(ctx context.Context, collectionID, docID string, entities []DocEntity) ([]int64, error)
	// FindEntityCooccurrences 按实体 ID 找共享实体的其他文档（excludeDocID 除外）。
	// maxDocFreq 为频次过滤（R-3）：出现在超过 maxDocFreq 个文档的实体视为噪声跳过；<=0 不过滤。
	FindEntityCooccurrences(ctx context.Context, collectionID string, entityIDs []int64, excludeDocID string, maxDocFreq int) ([]EntityCooccurrence, error)
	// MergeEntities 把 mergeeIDs 并入 keeperID（G5-F B10，同事务）：提及重写
	// （(doc_id,entity_id) 冲突 mentions 求和）+ entity 轨链接 context 重写 +
	// mergee name_norm 落 keeper 别名（合并跨同步持久，B12）+ mergee 删除。
	// 幂等：不存在的 mergee 跳过；keeper 不存在返回 NotFound。
	MergeEntities(ctx context.Context, collectionID string, keeperID int64, mergeeIDs []int64) (EntityMergeResult, error)
	// ListEntities 列出库内全部实体字典条目（按 id 有序），供合并建议计算（B11）。
	ListEntities(ctx context.Context, collectionID string) ([]Entity, error)
}

// SetLinkRepos 接线关联/实体持久化（可选能力；未接线时关联方法降级为 no-op）。
// 装配在 service/wire 层（与 NewUsecaseFromRepo 分离，避免破坏既有窄接口构造）。
func (u *Usecase) SetLinkRepos(links LinkRepo, entities EntityRepo) {
	u.links = links
	u.entities = entities
}

// ReplaceExplicitLinks 重建文档 explicit 出链（Vault 同步索引成功后调用）。
// 未接线或 repo 失败返回 error 供调用方降级记录（不回滚索引）。
func (u *Usecase) ReplaceExplicitLinks(ctx context.Context, collectionID, docID string, links []Link) error {
	if u == nil || u.links == nil {
		return nil
	}
	return u.links.ReplaceLinks(ctx, collectionID, docID, LinkTypeExplicit, links)
}

// maxLinkCandidates 单 vault 参与 [[...]] 解析的候选文档上限。
const maxLinkCandidates = 10000

// RebuildExplicitLinks 解析正文 [[...]] 引用并重建该文档的 explicit 出链
// （Vault 同步索引成功后、G3-B4 移动入链修复时调用）。
// 解析候选来自 DB 镜像（最终一致：目标文档尚未同步时引用悬空不建链，
// 待目标索引后、引用方下次变更时自愈）。自链跳过。
// 未接线 LinkRepo 或失败返回 error 供调用方降级记录（不回滚主流程）。
func (u *Usecase) RebuildExplicitLinks(ctx context.Context, collectionID, docID, body string) error {
	if u == nil || u.links == nil {
		return nil
	}
	refs := ParseWikiLinks(body)
	var links []Link
	if len(refs) > 0 {
		candidates, _, err := u.ListDocuments(ctx, collectionID, maxLinkCandidates, 0)
		if err != nil {
			return err
		}
		resolved := ResolveLinkRefs(refs, candidates)
		for _, ref := range refs {
			targetID, ok := resolved[ref]
			if !ok || targetID == docID {
				continue
			}
			links = append(links, Link{
				CollectionID: collectionID,
				DocID:        docID,
				TargetDocID:  targetID,
				LinkType:     LinkTypeExplicit,
				Context:      ref,
			})
		}
	}
	return u.links.ReplaceLinks(ctx, collectionID, docID, LinkTypeExplicit, links)
}

// ReplaceEntityLinks 重建文档 entity 出链（实体共现建链后调用）。
func (u *Usecase) ReplaceEntityLinks(ctx context.Context, collectionID, docID string, links []Link) error {
	if u == nil || u.links == nil {
		return nil
	}
	return u.links.ReplaceLinks(ctx, collectionID, docID, LinkTypeEntity, links)
}

// ListDocumentLinks 列出文档关联（双向；linkType 空 = 全部，供关联区按来源过滤）。
func (u *Usecase) ListDocumentLinks(ctx context.Context, collectionID, docID, linkType string) ([]Link, error) {
	if u == nil || u.links == nil {
		return nil, nil
	}
	return u.links.ListLinks(ctx, collectionID, docID, linkType)
}

// ReplaceDocEntities 重建文档实体记录（实体抽取成功后调用）。
// 返回解析后的实体 ID（去重、保首现序），供共现建链按 entity_id 查询。
func (u *Usecase) ReplaceDocEntities(ctx context.Context, collectionID, docID string, entities []DocEntity) ([]int64, error) {
	if u == nil || u.entities == nil {
		return nil, nil
	}
	return u.entities.ReplaceDocEntities(ctx, collectionID, docID, entities)
}

// FindEntityCooccurrences 委托 EntityRepo 共现查询（按 entity_id 关联，含 R-3 频次过滤）。
func (u *Usecase) FindEntityCooccurrences(ctx context.Context, collectionID string, entityIDs []int64, excludeDocID string, maxDocFreq int) ([]EntityCooccurrence, error) {
	if u == nil || u.entities == nil {
		return nil, nil
	}
	return u.entities.FindEntityCooccurrences(ctx, collectionID, entityIDs, excludeDocID, maxDocFreq)
}

// MergeEntities 手动合并实体（G5-F B10）。未接线 EntityRepo 返回错误（治理不可用）。
func (u *Usecase) MergeEntities(ctx context.Context, collectionID string, keeperID int64, mergeeIDs []int64) (EntityMergeResult, error) {
	if u == nil || u.entities == nil {
		return EntityMergeResult{}, apierror.Internal(apierror.DomainKnowledge, "entity repo not wired")
	}
	return u.entities.MergeEntities(ctx, collectionID, keeperID, mergeeIDs)
}
