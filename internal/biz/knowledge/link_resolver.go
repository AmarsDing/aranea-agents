package knowledge

import (
	"context"
	"sort"
	"strings"
	"time"
)

// ResolveDocCandidate 跨库解析候选文档（统一索引视图）。
// CollectionCreatedAt 参与 B-1 ③ 多义确定性排序（collection 创建序）。
type ResolveDocCandidate struct {
	DocID               string
	CollectionID        string
	RelPath             string // vault 相对路径（team 库文档可为空，仅靠 title/alias 匹配）
	Title               string
	Aliases             []string
	CollectionCreatedAt time.Time
}

// ResolveIndex 解析端口（data 层实现）。可见性过滤由实现方 SQL 保证：
// 不可见 collection 的候选不得返回（B-1：防文档名泄漏）。
// Stability:evolving
type ResolveIndex interface {
	// ListResolveCandidates 列出可见集合内的候选文档。
	ListResolveCandidates(ctx context.Context, collectionIDs []string) ([]ResolveDocCandidate, error)
	// FindBlockByAnchor 按显式锚点定位块（库内唯一由部分唯一索引保证）。
	FindBlockByAnchor(ctx context.Context, docID, anchor string) (blockID string, ok bool, err error)
	// FindBlockByHeadingPath 按标题路径定位 heading 块。
	FindBlockByHeadingPath(ctx context.Context, docID string, path []string) (blockID string, ok bool, err error)
}

// LinkResolver 两阶段解析的第二阶段（设计 S3，SP1-ADR-2）：
// 把 blockparse 产出的 raw_target 解析为 dst_doc_id / dst_block_id。
// 查不到 → dst 留空转 dangling（raw_target 保留复活线索），永不出错；
// 仅数据端口故障上抛 error（调用方降级，不回滚主流程）。
type LinkResolver struct {
	idx ResolveIndex
}

// NewLinkResolver 构造 Resolver。idx 为 nil 时全部引用落 dangling（降级安全）。
func NewLinkResolver(idx ResolveIndex) *LinkResolver { return &LinkResolver{idx: idx} }

// ResolveRefs 批量解析整文档引用。selfBlocks 为本次解析出的源文档块（内存态），
// 用于自文档引用（[[#^a]]/[[#H]]）与「按名引用回自身」的块级定位——
// 重建期间新块尚未提交，远端端口查不到，必须走内存。
// 返回与 refs 等长的新切片（Dst* / Ambiguous 已填充），输入不被修改。
func (r *LinkResolver) ResolveRefs(ctx context.Context, srcCollectionID, srcDocID string, visibleCollectionIDs []string, refs []KnowledgeBlockRefInput, selfBlocks []KnowledgeBlock) ([]KnowledgeBlockRefInput, error) {
	if len(refs) == 0 {
		return refs, nil
	}
	// 仅当存在跨文档引用时才查候选端口（纯自文档引用零 IO）。
	needCandidates := false
	for _, rf := range refs {
		if docPart, _ := splitRefTarget(rf.RawTarget); docPart != "" {
			needCandidates = true
			break
		}
	}
	var candidates []ResolveDocCandidate
	if needCandidates && r.idx != nil {
		var err error
		candidates, err = r.idx.ListResolveCandidates(ctx, visibleCollectionIDs)
		if err != nil {
			return nil, err
		}
	}
	out := make([]KnowledgeBlockRefInput, len(refs))
	for i, rf := range refs {
		got, err := r.resolveOne(ctx, srcCollectionID, srcDocID, candidates, rf, selfBlocks)
		if err != nil {
			return nil, err
		}
		out[i] = got
	}
	return out, nil
}

func (r *LinkResolver) resolveOne(ctx context.Context, srcColl, srcDoc string, candidates []ResolveDocCandidate, rf KnowledgeBlockRefInput, self []KnowledgeBlock) (KnowledgeBlockRefInput, error) {
	docPart, blockPart := splitRefTarget(rf.RawTarget)
	if docPart == "" {
		// 自文档引用：[[#^anchor]] / [[#H1#H2]]。doc 即源文档，块走内存。
		rf.DstDocID = srcDoc
		rf.DstCollectionID = srcColl
		if blockPart != "" {
			if ord, _, ok := resolveSelfBlock(blockPart, self); ok {
				rf.DstSelfOrdinal = &ord // 块 ID 由存储层按 ordinal 映射（解析期无 ID）
			}
		}
		return rf, nil
	}
	dstDocID, dstCollID, ambiguous := resolveDoc(docPart, srcColl, candidates)
	rf.DstDocID = dstDocID
	rf.DstCollectionID = dstCollID
	rf.Ambiguous = ambiguous
	if dstDocID == "" || blockPart == "" {
		return rf, nil // 文档级 dangling，或纯文档引用（无块部分）
	}
	// 按名引用回自身：新块未提交，走内存；否则查远端端口。
	if dstDocID == srcDoc {
		if ord, _, ok := resolveSelfBlock(blockPart, self); ok {
			rf.DstSelfOrdinal = &ord
		}
		return rf, nil
	}
	blockID, ok, err := r.resolveRemoteBlock(ctx, dstDocID, blockPart)
	if err != nil {
		return rf, err
	}
	if ok {
		rf.DstBlockID = blockID
	}
	// 块未命中：doc 级已解析，块级悬空（dst_block 空，复活靠重建时重跑 Resolver）。
	return rf, nil
}

func (r *LinkResolver) resolveRemoteBlock(ctx context.Context, docID, blockPart string) (string, bool, error) {
	if r.idx == nil {
		return "", false, nil
	}
	if anchor, ok := strings.CutPrefix(blockPart, "^"); ok {
		return r.idx.FindBlockByAnchor(ctx, docID, anchor)
	}
	return r.idx.FindBlockByHeadingPath(ctx, docID, splitHeadingPath(blockPart))
}

// splitRefTarget 以首个 # 切分文档键与块部分：
// "Note#H1#H2" → ("Note", "H1#H2")；"#^a1" → ("", "^a1")；"Note" → ("Note", "")。
func splitRefTarget(raw string) (docPart, blockPart string) {
	doc, block, found := strings.Cut(raw, "#")
	if !found {
		return strings.TrimSpace(doc), ""
	}
	return strings.TrimSpace(doc), strings.TrimSpace(block)
}

// splitHeadingPath 块部分按 # 切段为标题路径（"H1#H2" → ["H1","H2"]）。
func splitHeadingPath(blockPart string) []string {
	segs := strings.Split(blockPart, "#")
	for i := range segs {
		segs[i] = strings.TrimSpace(segs[i])
	}
	return segs
}

// resolveSelfBlock 在内存块中定位自文档引用目标。
// ^anchor 匹配 Anchor；其余按标题路径匹配（仅 heading 块——段落同路径不命中）。
// 返回首个命中的 ordinal；multi 标记多义（重复锚/重复标题路径，取首确定性）。
func resolveSelfBlock(blockPart string, self []KnowledgeBlock) (ord int, multi bool, ok bool) {
	if anchor, isAnchor := strings.CutPrefix(blockPart, "^"); isAnchor {
		for _, b := range self {
			if b.Anchor != "" && b.Anchor == anchor {
				if ok {
					return ord, true, true
				}
				ord, ok = b.Ordinal, true
			}
		}
		return ord, false, ok
	}
	path := splitHeadingPath(blockPart)
	for _, b := range self {
		if b.Kind != "heading" || !headingPathEqual(b.HeadingPath, path) {
			continue
		}
		if ok {
			return ord, true, true
		}
		ord, ok = b.Ordinal, true
	}
	return ord, false, ok
}

func headingPathEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// 匹配质量分级（小=强）：文件名（含带路径引用的完整路径）> 标题/别名。
const (
	rankName = iota
	rankTitleAlias
)

// resolveDoc 文档键解析（设计 B-1 确定性规则）：
//  1. 同 collection 命中优先（压过匹配质量与他库更短路径）；
//  2. 组内按（匹配质量 → 路径长度 → collection 创建序 → 路径字典序）排序取首；
//  3. 最佳质量级多候选时 ambiguous=true（确定性取首仍返回）。
//
// 返回命中文档的 docID 与其 collectionID（SP1-D 边 scope 装配）；未命中返回空串。
func resolveDoc(docPart, srcColl string, candidates []ResolveDocCandidate) (dstDocID, dstCollectionID string, ambiguous bool) {
	norm := normalizeLinkPath(docPart)
	if norm == "" {
		return "", "", false
	}
	type scored struct {
		c    ResolveDocCandidate
		rank int
	}
	var same, other []scored
	for _, c := range candidates {
		rank := docMatchRank(norm, docPart, c)
		if rank < 0 {
			continue
		}
		if c.CollectionID == srcColl {
			same = append(same, scored{c, rank})
		} else {
			other = append(other, scored{c, rank})
		}
	}
	group := same
	if len(group) == 0 {
		group = other
	}
	if len(group) == 0 {
		return "", "", false
	}
	sort.SliceStable(group, func(i, j int) bool {
		a, b := group[i], group[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if len(a.c.RelPath) != len(b.c.RelPath) {
			return len(a.c.RelPath) < len(b.c.RelPath)
		}
		if !a.c.CollectionCreatedAt.Equal(b.c.CollectionCreatedAt) {
			return a.c.CollectionCreatedAt.Before(b.c.CollectionCreatedAt)
		}
		return a.c.RelPath < b.c.RelPath
	})
	best := 0
	for _, g := range group[1:] {
		if g.rank == group[0].rank {
			best++
		}
	}
	return group[0].c.DocID, group[0].c.CollectionID, best > 0
}

// normalizeLinkPath 归一化链接路径：正斜杠、去 .md 后缀、小写、去首尾空白。
// 仅去 .md——其他扩展名（.png 等嵌入资源）保留，basename 带扩展名命中。
func normalizeLinkPath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, `/`))
	p = strings.TrimPrefix(p, "/")
	if strings.HasSuffix(strings.ToLower(p), ".md") {
		p = p[:len(p)-len(".md")]
	}
	return strings.ToLower(p)
}

// docMatchRank 计算候选与文档键的匹配质量；不匹配返回 -1。
// Obsidian 语义分两种模式：
//   - 引用含路径分隔（"notes/idea"）：仅完整路径匹配（大小写不敏感、去 .md）；
//   - 裸名引用（"note"）：文件名（basename）同级匹配——根级 "note.md" 与
//     "a/b/note.md" 对 [[note]] 同属文件名命中，多候选即 ambiguous；
//     标题/别名（EqualFold）次于文件名。
func docMatchRank(norm, raw string, c ResolveDocCandidate) int {
	candPath := normalizeLinkPath(c.RelPath)
	if strings.Contains(norm, "/") {
		if candPath != "" && norm == candPath {
			return rankName
		}
		return -1
	}
	if candPath != "" {
		base := candPath
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		if norm == base {
			return rankName
		}
	}
	raw = strings.TrimSpace(raw)
	if c.Title != "" && strings.EqualFold(raw, c.Title) {
		return rankTitleAlias
	}
	for _, a := range c.Aliases {
		if strings.EqualFold(raw, strings.TrimSpace(a)) {
			return rankTitleAlias
		}
	}
	return -1
}
