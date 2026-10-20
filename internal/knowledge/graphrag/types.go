// Package graphrag 双层图谱检索与社区检测纯库（U4-1/U4-2 底座）。
//
// 定位：在既有 Lazy GraphRAG（internal/knowledge/graph_expander.go：1 跳邻域 /
// M1-4 受限扩散激活叠加召回）之上，提供可独立测试的双层检索核心——
//   - 低层：query → 实体锚点 → 候选文档 → chunk 排序；
//   - 扩展：低层命中文档 1 跳邻域扩展候选再排序，RRF 贡献加权 0.7；
//   - 高层：候选图（vault 双链文档级投影）上跑确定性标签传播社区检测，
//     给融合命中数最高的社区产出主题 Trace（摘要可选、不进排序）。
//
// 纯库零接入：本包不 import 任何 internal 包，全部类型自带；与生产数据源的
// 接线由 internal/knowledge 装配层完成。
//
// 数据形态对齐（字段映射建议）：
//   - LinkEdge ← bizknowledge.Link（knowledge_links 表）：SrcDocID=DocID、
//     DstDocID=TargetDocID、Type=LinkType（explicit/entity/semantic）、
//     Weight=float64(Weight)（explicit 轨 = 块级 refs 投影边数，N-3 聚合权重）。
//     块级双链由 blockparse.Parse 产出（refs 边 edge_type=ref/嵌入），
//     文档级投影（并双向化读取）后即本包的候选图输入。
//   - EntityRef ← bizknowledge.Entity（实体字典，G5-F 治理）：ID=strconv(Entity.ID)、
//     Name=Name（展示名，首见写法）；Weight=实体-查询相关度（生产可用归一化命
//     中或 embedding 余弦）。EntityDocLinker ← knowledge_doc_entities（提及表）。
//   - ChunkHit ← bizknowledge.Chunk（knowledge_chunks 表）：ChunkID=ID、
//     DocID=DocID、Score=float64(Score)。ChunkRanker 生产实现建议复用
//     hybrid_retriever 的候选排序输出。
//   - Summarizer ← LLM 摘要口（U4-2 接线；nil = 跳过高层摘要）。
//
// 社区检测在候选图的无向化投影上运行：同一无序文档对的多条边权重累加
// （Weight<=0 按 1 计），自环忽略；初始每点一社区（社区号 = 文档 ID 字典序位
// 次），同步标签传播，平票取最小社区号——同一输入必得同一输出。
package graphrag

// ChunkHit 一次 chunk 级命中（低层排序 / 融合输出）。
type ChunkHit struct {
	ChunkID string  // knowledge_chunks.id
	DocID   string  // knowledge_chunks.doc_id
	Score   float64 // Ranker 输出分；融合输出为 RRF 融合分
}

// EntityRef 实体检索命中（低层查询→实体锚点）。
type EntityRef struct {
	ID     string  // 实体字典 id 的字符串形式
	Name   string  // 实体展示名（首见写法）
	Weight float64 // 实体-查询相关度（解释/排序辅助）
}

// LinkEdge 文档间关联边（vault 双链图的文档级投影）。
type LinkEdge struct {
	SrcDocID string  // 出向文档（knowledge_links.doc_id）
	DstDocID string  // 入向文档（knowledge_links.target_doc_id）
	Type     string  // explicit / entity / semantic
	Weight   float64 // 聚合权重（<=0 时社区检测按 1 计）
}
