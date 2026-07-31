package knowledge

import "context"

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

// EntityRepo 实体持久化窄接口（P2-4）。
// Stability:evolving
type EntityRepo interface {
	// ReplaceDocEntities 事务性替换文档实体（实体表 upsert + 提及表重建 + 孤儿实体清理）。
	ReplaceDocEntities(ctx context.Context, collectionID, docID string, entities []DocEntity) error
	// FindEntityCooccurrences 找共享实体的其他文档（excludeDocID 除外）。
	// maxDocFreq 为频次过滤（R-3）：出现在超过 maxDocFreq 个文档的实体视为噪声跳过；<=0 不过滤。
	FindEntityCooccurrences(ctx context.Context, collectionID string, entityNames []string, excludeDocID string, maxDocFreq int) ([]EntityCooccurrence, error)
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
func (u *Usecase) ReplaceDocEntities(ctx context.Context, collectionID, docID string, entities []DocEntity) error {
	if u == nil || u.entities == nil {
		return nil
	}
	return u.entities.ReplaceDocEntities(ctx, collectionID, docID, entities)
}

// FindEntityCooccurrences 委托 EntityRepo 共现查询（含 R-3 频次过滤）。
func (u *Usecase) FindEntityCooccurrences(ctx context.Context, collectionID string, entityNames []string, excludeDocID string, maxDocFreq int) ([]EntityCooccurrence, error) {
	if u == nil || u.entities == nil {
		return nil, nil
	}
	return u.entities.FindEntityCooccurrences(ctx, collectionID, entityNames, excludeDocID, maxDocFreq)
}
