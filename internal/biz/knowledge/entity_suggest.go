package knowledge

// 实体合并建议（G5-F B11）：归一化冲突组（name 不同 name_norm 相同，迁移期
// 残留——(collection_id,name_norm) 唯一约束下新写入不会再产生）+ 配置 embedding
// 时的实体名高相似对（余弦 ≥0.90 标 auto、0.80–0.90 标 suggest）。实时计算
// 不落队列表（YAGNI）。

import (
	"context"
	"math"
	"sort"
)

// 建议来源/档位常量（proto EntityMergeSuggestion.source/tier 对齐）。
const (
	EntityMergeSourceNorm      = "norm"
	EntityMergeSourceEmbedding = "embedding"
	EntityMergeTierAuto        = "auto"
	EntityMergeTierSuggest     = "suggest"
)

const (
	// embeddingAutoThreshold 余弦 ≥ 此值标 auto（可一键合并的高置信候选）。
	embeddingAutoThreshold = 0.90
	// embeddingSuggestThreshold 余弦 ≥ 此值入建议（0.80–0.90 仅提示，不动数据）。
	embeddingSuggestThreshold = 0.80
	// embeddingSuggestionMaxEntities 参与 embedding 比对的实体上限（按 id 序截取），
	// 约束单次 Embed 批量与 O(N²) 比对成本。
	embeddingSuggestionMaxEntities = 500
)

// Entity 知识实体字典条目（G5-F 治理读取）。
type Entity struct {
	ID         int64
	Name       string // 展示名（首见写法）
	NameNorm   string // 归一化名
	EntityType string
}

// EntityMergeSuggestion 一条合并建议（B11）。
type EntityMergeSuggestion struct {
	KeeperID   int64   // 建议保留实体（id 最小者，与迁移 keeper 规则一致）
	KeeperName string  // 保留实体展示名
	MergeeID   int64   // 建议并入实体
	MergeeName string  // 并入实体展示名
	Source     string  // norm | embedding
	Similarity float64 // embedding 余弦；norm 固定 1.0
	Tier       string  // auto | suggest
}

// EntityEmbedder 实体名 embedding 窄端口（B11；nil = 仅 norm 组，对齐 NFR-15）。
// 生产实现：internal/knowledge.Embedder（方法子集，结构满足）。
type EntityEmbedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// ListEntityMergeSuggestions 实时计算合并建议：norm 冲突组在前，embedding 对
// 按相似度降序追加（同 id 对去重，norm 优先）。embedder 为 nil 或调用失败时
// 仅返回 norm 组（降级不报错，对齐 NFR-15）；未接线 EntityRepo 返回空。
func (u *Usecase) ListEntityMergeSuggestions(ctx context.Context, collectionID string, embedder EntityEmbedder) ([]EntityMergeSuggestion, error) {
	if u == nil || u.entities == nil {
		return nil, nil
	}
	entities, err := u.entities.ListEntities(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	var out []EntityMergeSuggestion
	seen := map[[2]int64]bool{} // id 对去重（小 id 在前）

	// norm 冲突组：keeper = 组内 id 最小者（ListEntities 按 id 有序，组首即 keeper）。
	byNorm := map[string][]Entity{}
	var normOrder []string
	for _, e := range entities {
		if e.NameNorm == "" {
			continue
		}
		if _, ok := byNorm[e.NameNorm]; !ok {
			normOrder = append(normOrder, e.NameNorm)
		}
		byNorm[e.NameNorm] = append(byNorm[e.NameNorm], e)
	}
	for _, norm := range normOrder {
		group := byNorm[norm]
		if len(group) < 2 {
			continue
		}
		keeper := group[0]
		for _, m := range group[1:] {
			seen[[2]int64{keeper.ID, m.ID}] = true
			out = append(out, EntityMergeSuggestion{
				KeeperID: keeper.ID, KeeperName: keeper.Name,
				MergeeID: m.ID, MergeeName: m.Name,
				Source: EntityMergeSourceNorm, Similarity: 1.0, Tier: EntityMergeTierAuto,
			})
		}
	}

	// embedding 高相似对（配置且调用成功时；失败降级 norm-only）。
	if embedder != nil && len(entities) >= 2 {
		capped := entities
		if len(capped) > embeddingSuggestionMaxEntities {
			capped = capped[:embeddingSuggestionMaxEntities]
		}
		names := make([]string, len(capped))
		for i, e := range capped {
			names[i] = e.Name
		}
		if vecs, err := embedder.Embed(ctx, names); err == nil && len(vecs) == len(capped) {
			out = append(out, embeddingPairSuggestions(capped, vecs, seen)...)
		}
	}
	return out, nil
}

// embeddingPairSuggestions O(N²) 余弦比对（N≤500 有界）；keeper = 较小 id；
// 按相似度降序。
func embeddingPairSuggestions(entities []Entity, vecs [][]float32, seen map[[2]int64]bool) []EntityMergeSuggestion {
	var pairs []EntityMergeSuggestion
	for i := 0; i < len(entities); i++ {
		for j := i + 1; j < len(entities); j++ {
			sim := cosineSimilarity(vecs[i], vecs[j])
			if sim < embeddingSuggestThreshold {
				continue
			}
			keeper, mergee := entities[i], entities[j]
			if keeper.ID > mergee.ID {
				keeper, mergee = mergee, keeper
			}
			key := [2]int64{keeper.ID, mergee.ID}
			if seen[key] {
				continue // norm 组已覆盖
			}
			seen[key] = true
			tier := EntityMergeTierSuggest
			if sim >= embeddingAutoThreshold {
				tier = EntityMergeTierAuto
			}
			pairs = append(pairs, EntityMergeSuggestion{
				KeeperID: keeper.ID, KeeperName: keeper.Name,
				MergeeID: mergee.ID, MergeeName: mergee.Name,
				Source: EntityMergeSourceEmbedding, Similarity: sim, Tier: tier,
			})
		}
	}
	sort.Slice(pairs, func(a, b int) bool {
		if pairs[a].Similarity != pairs[b].Similarity {
			return pairs[a].Similarity > pairs[b].Similarity
		}
		return pairs[a].KeeperID < pairs[b].KeeperID
	})
	return pairs
}

// cosineSimilarity float32 向量余弦（长度不等/零向量返回 0）。
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		na += ai * ai
		nb += bi * bi
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
