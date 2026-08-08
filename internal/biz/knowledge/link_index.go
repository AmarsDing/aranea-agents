package knowledge

import (
	"context"
	"strings"
	"sync"
)

// SP1-D 统一链接索引（设计 S5）：进程内内存图，正/反邻接表 + 单调递增版本号。
//
// 写路径：解析事务提交后 ApplyDocDelta 增量（add/remove 边，version+1）；
// 启动时 LoadAll 从 knowledge_block_refs 全量重放构建。
// 读路径：块级/文档级反链、dangling 聚合直读内存图（O(度数)），落库查询兜底（SP1-E）。
//
// 部署约束（N-1）：单进程内存图；多副本化需改事件广播保持副本一致，届时另立 ADR。
// 一致性镜像（FK 语义）：目标文档重建 → 其入向块边转文档级（dst_block SET NULL 镜像）；
// 文档删除 → 出向边清除、入向边转 dangling（保 raw_target）。

// GraphDelta 一次图谱变更的增量负载（WS 事件 knowledge.graph.delta 的载体）。
// Added/Removed 已做集合差收敛（无变化的重建产空 delta）；Version 为变更后版本。
type GraphDelta struct {
	Added   []KnowledgeBlockRefEdge
	Removed []KnowledgeBlockRefEdge
	Version uint64
}

// Empty 报告 delta 是否无实际变更（调用方据此跳过 WS 推送）。
func (d GraphDelta) Empty() bool { return len(d.Added) == 0 && len(d.Removed) == 0 }

// GraphDeltaPublisher 图谱增量事件出口（SP1-D D-2）。service 层装配 event.Bus
// 适配器（SystemNoticeEvent 模式，同 knowledge_ingest）；nil 时仅内存图更新。
// Stability:evolving
type GraphDeltaPublisher interface {
	PublishGraphDelta(ctx context.Context, delta GraphDelta)
}

// LinkIndex 进程内链接内存图。五索引同增同删保一致：
//   - bySrc：正向邻接（块 → 出边）
//   - byDstBlk：块级反向邻接（块 ← 入边）
//   - incoming：文档级反向邻接（文档 ← 全部入边，块级+文档级）
//   - bySrcDoc：文档 → 出边（整文档摘除单元）
//   - danglingByColl：源集合 → dangling 边（「未创建笔记」视图）
//
// 索引存 *KnowledgeBlockRefEdge：单边单分配多索引共享（内存目标 10 万边 <100MB，
// NFR-SP1-4）；转换场景复制后改值，禁止原地改。读侧返回值拷贝切片，无外部写回风险。
type LinkIndex struct {
	mu             sync.RWMutex
	version        uint64
	loaded         bool // LoadAll 完成后置位：SP1-E 读侧据此选内存图/DB 兜底
	bySrc          map[string][]*KnowledgeBlockRefEdge
	byDstBlk       map[string][]*KnowledgeBlockRefEdge
	incoming       map[string][]*KnowledgeBlockRefEdge
	bySrcDoc       map[string][]*KnowledgeBlockRefEdge
	danglingByColl map[string][]*KnowledgeBlockRefEdge
}

// NewLinkIndex 构造空图。
func NewLinkIndex() *LinkIndex {
	return &LinkIndex{
		bySrc:          map[string][]*KnowledgeBlockRefEdge{},
		byDstBlk:       map[string][]*KnowledgeBlockRefEdge{},
		incoming:       map[string][]*KnowledgeBlockRefEdge{},
		bySrcDoc:       map[string][]*KnowledgeBlockRefEdge{},
		danglingByColl: map[string][]*KnowledgeBlockRefEdge{},
	}
}

// Version 当前图谱版本（单调递增；仅实际变更时 +1）。
func (x *LinkIndex) Version() uint64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.version
}

// Loaded 报告全量构建是否完成（SP1-E 读侧路由：未加载 = 启动窗口，查询须落库兜底，
// 避免读空图误判无反链）。
func (x *LinkIndex) Loaded() bool {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.loaded
}

// LoadAll 全量重放构建（启动/索引重建）：重置全部索引，version 归零，置 loaded。
func (x *LinkIndex) LoadAll(edges []KnowledgeBlockRefEdge) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.bySrc = map[string][]*KnowledgeBlockRefEdge{}
	x.byDstBlk = map[string][]*KnowledgeBlockRefEdge{}
	x.incoming = map[string][]*KnowledgeBlockRefEdge{}
	x.bySrcDoc = map[string][]*KnowledgeBlockRefEdge{}
	x.danglingByColl = map[string][]*KnowledgeBlockRefEdge{}
	for _, e := range edges {
		x.addEdge(e)
	}
	x.version = 0
	x.loaded = true
}

// ApplyDocDelta 文档重建后增量 apply：摘除该文档全部旧出边 → 入向块边转文档级
// （镜像 FK SET NULL）→ 加新边。delta 为集合差（内容不变的边不进 delta）；
// 实际有变更时 version+1。
func (x *LinkIndex) ApplyDocDelta(docID string, edges []KnowledgeBlockRefEdge) GraphDelta {
	x.mu.Lock()
	defer x.mu.Unlock()
	var removed, added []KnowledgeBlockRefEdge
	// 1) 摘除旧出边（先于转换：避免转换改变值后摘除失配）。
	// 快照迭代：removeEdge 对同一切片做 in-place 过滤，直接遍历会跳读/复读。
	for _, e := range snapshotEdges(x.bySrcDoc[docID]) {
		x.removeEdge(e)
		removed = append(removed, *e)
	}
	// 2) 入向块边转文档级（目标文档块被删，FK SET NULL 镜像；dst_doc 保留）。
	for _, e := range snapshotEdges(x.incoming[docID]) {
		if e.DstBlockID == "" {
			continue
		}
		x.removeEdge(e)
		removed = append(removed, *e)
		turned := *e
		turned.DstBlockID = ""
		x.addEdge(turned)
		added = append(added, turned)
	}
	// 3) 加新出边。
	for _, e := range edges {
		e.SrcDocID = docID
		x.addEdge(e)
		added = append(added, e)
	}
	return x.commitDelta(removed, added)
}

// RemoveDoc 文档删除（G-3 接线单元）：出向边随块级联清除；入向边转 dangling
// （Dst* 清空、raw_target 保留复活线索）。delta 携带两类变更。
func (x *LinkIndex) RemoveDoc(docID string) GraphDelta {
	x.mu.Lock()
	defer x.mu.Unlock()
	var removed, added []KnowledgeBlockRefEdge
	for _, e := range snapshotEdges(x.bySrcDoc[docID]) {
		x.removeEdge(e)
		removed = append(removed, *e)
	}
	// 出边摘除后 incoming[docID] 只剩它文档来的入边 → 转 dangling。
	for _, e := range snapshotEdges(x.incoming[docID]) {
		x.removeEdge(e)
		removed = append(removed, *e)
		dangling := *e
		dangling.DstCollectionID, dangling.DstDocID, dangling.DstBlockID = "", "", ""
		x.addEdge(dangling)
		added = append(added, dangling)
	}
	return x.commitDelta(removed, added)
}

// snapshotEdges 复制索引切片头（写路径遍历时 removeEdge 会 in-place 过滤同一切片）。
func snapshotEdges(edges []*KnowledgeBlockRefEdge) []*KnowledgeBlockRefEdge {
	return append([]*KnowledgeBlockRefEdge(nil), edges...)
}

// commitDelta 集合差收敛（multiset 差集：removed−added 为真摘除，added−removed
// 为真新增）+ 版本递增（仅实际变更时）。调用方须持写锁。
func (x *LinkIndex) commitDelta(removed, added []KnowledgeBlockRefEdge) GraphDelta {
	realRemoved := edgeDiff(removed, added)
	realAdded := edgeDiff(added, removed)
	if len(realRemoved) == 0 && len(realAdded) == 0 {
		return GraphDelta{Version: x.version}
	}
	x.version++
	return GraphDelta{Added: realAdded, Removed: realRemoved, Version: x.version}
}

// edgeIdentity 边内容身份（不含 BIGSERIAL 行 id——整文档重建后行 id 变，内容身份不变）。
func edgeIdentity(e KnowledgeBlockRefEdge) string {
	var b strings.Builder
	for _, s := range []string{
		e.CollectionID, e.SrcBlockID, e.SrcDocID,
		e.DstCollectionID, e.DstDocID, e.DstBlockID,
		e.RawTarget, e.EdgeType, e.Context,
	} {
		b.WriteString(s)
		b.WriteByte(0)
	}
	if e.Ambiguous {
		b.WriteByte(1)
	}
	return b.String()
}

// edgeDiff multiset 差集：a − b。
func edgeDiff(a, b []KnowledgeBlockRefEdge) []KnowledgeBlockRefEdge {
	if len(a) == 0 {
		return nil
	}
	count := make(map[string]int, len(b))
	for _, e := range b {
		count[edgeIdentity(e)]++
	}
	var out []KnowledgeBlockRefEdge
	for _, e := range a {
		k := edgeIdentity(e)
		if count[k] > 0 {
			count[k]--
			continue
		}
		out = append(out, e)
	}
	return out
}

// addEdge 写全部索引（值拷贝单分配，调用方持写锁）。dangling（DstDocID 空）
// 只进 bySrc/bySrcDoc/danglingByColl。
func (x *LinkIndex) addEdge(e KnowledgeBlockRefEdge) {
	ep := &e
	x.bySrc[e.SrcBlockID] = append(x.bySrc[e.SrcBlockID], ep)
	x.bySrcDoc[e.SrcDocID] = append(x.bySrcDoc[e.SrcDocID], ep)
	if e.DstDocID == "" {
		x.danglingByColl[e.CollectionID] = append(x.danglingByColl[e.CollectionID], ep)
		return
	}
	x.incoming[e.DstDocID] = append(x.incoming[e.DstDocID], ep)
	if e.DstBlockID != "" {
		x.byDstBlk[e.DstBlockID] = append(x.byDstBlk[e.DstBlockID], ep)
	}
}

// removeEdge 从全部索引摘除（按内容身份，等值全摘——同一 doc 版本内
// SrcBlockID 唯一保证不误摘他文档边；调用方持写锁）。
func (x *LinkIndex) removeEdge(e *KnowledgeBlockRefEdge) {
	k := edgeIdentity(*e)
	x.bySrc[e.SrcBlockID] = removeEdgeFrom(x.bySrc[e.SrcBlockID], k)
	x.bySrcDoc[e.SrcDocID] = removeEdgeFrom(x.bySrcDoc[e.SrcDocID], k)
	if e.DstDocID == "" {
		x.danglingByColl[e.CollectionID] = removeEdgeFrom(x.danglingByColl[e.CollectionID], k)
		return
	}
	x.incoming[e.DstDocID] = removeEdgeFrom(x.incoming[e.DstDocID], k)
	if e.DstBlockID != "" {
		x.byDstBlk[e.DstBlockID] = removeEdgeFrom(x.byDstBlk[e.DstBlockID], k)
	}
}

func removeEdgeFrom(slice []*KnowledgeBlockRefEdge, key string) []*KnowledgeBlockRefEdge {
	out := slice[:0]
	for _, e := range slice {
		if edgeIdentity(*e) != key {
			out = append(out, e)
		}
	}
	return out
}

// edgeVisible 可见性过滤（S5 基础规则）：边随源集合可见性；nil = 不过滤（内部/测试）。
func edgeVisible(e KnowledgeBlockRefEdge, visible map[string]bool) bool {
	if visible == nil {
		return true
	}
	return visible[e.CollectionID]
}

func filterVisible(edges []*KnowledgeBlockRefEdge, visible map[string]bool) []KnowledgeBlockRefEdge {
	out := make([]KnowledgeBlockRefEdge, 0, len(edges))
	for _, ep := range edges {
		if edgeVisible(*ep, visible) {
			out = append(out, *ep)
		}
	}
	return out
}

// OutEdges 块的出向边（正向邻接）。
func (x *LinkIndex) OutEdges(blockID string, visible map[string]bool) []KnowledgeBlockRefEdge {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return filterVisible(x.bySrc[blockID], visible)
}

// BacklinksByBlock 块级反链（O(度数)）。
func (x *LinkIndex) BacklinksByBlock(blockID string, visible map[string]bool) []KnowledgeBlockRefEdge {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return filterVisible(x.byDstBlk[blockID], visible)
}

// BacklinksByDoc 文档反链 = 全部块级 + 文档级入边聚合。
func (x *LinkIndex) BacklinksByDoc(docID string, visible map[string]bool) []KnowledgeBlockRefEdge {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return filterVisible(x.incoming[docID], visible)
}

// DanglingByCollection dangling 边（DstDocID 空，raw_target 保复活线索）按源集合聚合。
func (x *LinkIndex) DanglingByCollection(collectionID string, visible map[string]bool) []KnowledgeBlockRefEdge {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return filterVisible(x.danglingByColl[collectionID], visible)
}

// SetLinkIndex 接线统一链接索引与增量事件出口（SP1-D；可选能力）。
// idx 为共享实例（service 层启动时 LoadAll 全量构建）；pub nil 时仅内存图更新。
func (u *Usecase) SetLinkIndex(idx *LinkIndex, pub GraphDeltaPublisher) {
	u.linkIndex = idx
	u.graphPub = pub
}

// applyLinkIndex 解析事务提交后把本次物化边 apply 进内存图并发布 WS 增量
// （设计 S5）。未接线 no-op。
func (u *Usecase) applyLinkIndex(ctx context.Context, docID string, edges []KnowledgeBlockRefEdge) {
	if u == nil || u.linkIndex == nil {
		return
	}
	u.publishGraphDelta(ctx, u.linkIndex.ApplyDocDelta(docID, edges))
}

// removeLinkIndexDoc 文档删除后同步内存图（出边级联清除、入边转 dangling
// 保 raw_target，镜像 DB FK 语义）并发布 WS 增量。未接线 no-op。
func (u *Usecase) removeLinkIndexDoc(ctx context.Context, docID string) {
	if u == nil || u.linkIndex == nil {
		return
	}
	u.publishGraphDelta(ctx, u.linkIndex.RemoveDoc(docID))
}

// publishGraphDelta 发布 WS 增量；空 delta 不推（无变化重建不制造 WS 噪声）。
// 发布失败不回滚内存图（事件可从 DB 全量重放重建，N-2 Informational 分级）。
func (u *Usecase) publishGraphDelta(ctx context.Context, delta GraphDelta) {
	if u.graphPub != nil && !delta.Empty() {
		u.graphPub.PublishGraphDelta(ctx, delta)
	}
}

// LoadLinkIndex 启动全量构建（设计 S5）：经 LinkEdgeLoader 端口从
// knowledge_block_refs 重放全部边，返回加载边数。blockIndex 未实现 loader
// 或未接线时 no-op（降级安全：内存图为派生索引，可随时全量重建）。
func (u *Usecase) LoadLinkIndex(ctx context.Context) (int, error) {
	if u == nil || u.linkIndex == nil || u.blockIndex == nil {
		return 0, nil
	}
	loader, ok := u.blockIndex.(LinkEdgeLoader)
	if !ok {
		return 0, nil
	}
	edges, err := loader.ListAllRefEdges(ctx)
	if err != nil {
		return 0, err
	}
	u.linkIndex.LoadAll(edges)
	return len(edges), nil
}
