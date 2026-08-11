package knowledge

import (
	"context"
	"strings"
	"time"
)

// ── B4 #8：wikilink 落链 recency（[[ 补全最近引用排序，SiYuan refUsed 语义） ──
//
// 用户在 [[ 补全中选中目标文档时写路径 upsert (collection, doc) 的 last_used_at；
// 空关键词补全按最近引用优先排序候选（截断 32，对齐 SiYuan 截断语义）。
// recency 是体验增强而非正确性依赖：端口未接线时读写均降级 no-op。

const (
	// linkUseDefaultLimit 缺省返回条数（SiYuan refUsed 截断 32）。
	linkUseDefaultLimit = 32
	// linkUseMaxLimit 单次查询上限。
	linkUseMaxLimit = 128
)

// LinkUse 一条最近引用记录（按 last_used_at 降序消费）。
type LinkUse struct {
	DocID      string
	LastUsedAt time.Time
}

// LinkUsageRepo wikilink 落链 recency 持久化端口（B4 #8）。
// Stability:evolving
type LinkUsageRepo interface {
	// TouchLinkUse upsert (collectionID, docID) 的 last_used_at = at。
	TouchLinkUse(ctx context.Context, collectionID, docID string, at time.Time) error
	// ListRecentLinkUses 按 last_used_at 降序返回 collection 内最近引用，至多 limit 条。
	ListRecentLinkUses(ctx context.Context, collectionID string, limit int) ([]LinkUse, error)
}

// SetLinkUsageRepo 接线落链 recency 端口（B4 #8；可选能力，未接线降级 no-op）。
func (u *Usecase) SetLinkUsageRepo(repo LinkUsageRepo) {
	u.linkUsage = repo
}

// RecordLinkUse 记录一次 wikilink 落链（补全选中目标文档）。best-effort：
// 端口未接线时 no-op 成功；collection/doc 空白返回 ErrIDRequired。
// doc 存在性不强制校验（删除竞态产生的孤儿行会被后续引用挤出截断窗口，
// 且前端按本地文档表映射 doc_id→候选，孤儿行天然不命中）。
func (u *Usecase) RecordLinkUse(ctx context.Context, collectionID, docID string) error {
	if u == nil || u.linkUsage == nil {
		return nil
	}
	collectionID = strings.TrimSpace(collectionID)
	docID = strings.TrimSpace(docID)
	if collectionID == "" || docID == "" {
		return ErrIDRequired
	}
	return u.linkUsage.TouchLinkUse(ctx, collectionID, docID, time.Now())
}

// ListRecentLinkUses 返回 collection 的最近引用目标（last_used_at 降序）。
// limit ≤0 取缺省 32，>128 截断 128；端口未接线返回空。
func (u *Usecase) ListRecentLinkUses(ctx context.Context, collectionID string, limit int) ([]LinkUse, error) {
	if u == nil || u.linkUsage == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = linkUseDefaultLimit
	}
	if limit > linkUseMaxLimit {
		limit = linkUseMaxLimit
	}
	return u.linkUsage.ListRecentLinkUses(ctx, strings.TrimSpace(collectionID), limit)
}
