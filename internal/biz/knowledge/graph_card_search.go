package knowledge

import "context"

// G-33 读侧窄端口（ADR-KN-20261020-H/I）：卡片表自身无 visibility 列，所有
// 读路径一律经宿主文档投影下推 G-08 中央谓词 docVisibilityPredicate——
// 三个窄接口的 data 实现都必须把该谓词编进 WHERE/EXISTS，禁止绕行。

const (
	// GraphCardSearchKindLimit 每 kind 的 ANN/词法命中上限（锚数沿用 graphrag
	// dual.go ≤8 先例）；配合宿主上限，每查询新增候选硬上界 8×3+8×2=40。
	GraphCardSearchKindLimit = 8
	// GraphCardEntityHostLimit entity 卡出题宿主文档上限。
	GraphCardEntityHostLimit = 3
	// GraphCardEdgeHostLimit edge 卡出题宿主文档上限（doc_id 与 target_doc_id
	// 双端各一条同一谓词的可见性 EXISTS——沿用 links 双端点纪律）。
	GraphCardEdgeHostLimit = 2
)

// GraphCardHit 卡片命中（ANN 相似度或词法命中分）。
type GraphCardHit struct {
	Kind  string // GraphCardKindEntity / GraphCardKindEdge
	RefID int64  // entity=kg_entities.id / edge=knowledge_links.id
	Text  string // ADR-G 卡片文（候选 content 逐字继承）
	Score float32
}

// GraphCardVectorSearcher 卡片 ANN（knowledge_graph_embeddings，kind 过滤）：
// WHERE 含宿主投影 EXISTS——entity 卡至少一个宿主文档可见、edge 卡双端各至少
// 一条可见（数据层三窄接口其一）。
type GraphCardVectorSearcher interface {
	SearchGraphCards(ctx context.Context, collectionID, kind string, vec []float32, limit int) ([]GraphCardHit, error)
}

// GraphCardLexicalSearcher 词法降级（嵌入向量缺失时）：实体卡 name_norm 精确/
// 前缀 ∪ alias_norm 精确命中——直接读 knowledge_entities/aliases（词法库不物化
// 卡片行，见 projection 工人集合闸），同一宿主投影可见性谓词；edge 腿此模式
// 静默关闭（边陈述无结构词法匹配是噪声源）。
type GraphCardLexicalSearcher interface {
	SearchEntityCardsLexical(ctx context.Context, collectionID, query string, limit int) ([]GraphCardHit, error)
}

// GraphCardHostDocLister 卡 → 宿主文档投影：entity=提及该实体的可见文档
// （mentions 降序限 limit）；edge=双端可见文档。返回真实 doc_id——合成候选
// ID（g33e/<ref>/<doc>）与下游继承（父回填/base-boost/access_log/Hebbian/
// GraphExpander 种子）全靠它。
type GraphCardHostDocLister interface {
	ListGraphCardHostDocs(ctx context.Context, collectionID, kind string, refID int64, limit int) ([]string, error)
}
