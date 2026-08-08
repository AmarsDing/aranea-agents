package data

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"

	"github.com/lib/pq"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// knowledgeBlockRepo SP1-B 块级派生索引物化（Raw SQL，域内一致；见 ddl 20261203）。
// 写模式：整文档删了重插（SiYuan deleteRefsByPathTx 语义），refs 不做 diff——
// 一致性优先，写放大小；级联/dangling 转换由 FK 保证（20261203_knowledge_blocks.sql）。
type knowledgeBlockRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ bizknowledge.BlockIndexRepo = (*knowledgeBlockRepo)(nil)

// NewKnowledgeBlockRepo 构造。data 或 PG 未就绪时返回 nil（与 NewKnowledgeRepo 同款）。
func NewKnowledgeBlockRepo(data *Data, lg loggateway.Logger) bizknowledge.BlockIndexRepo {
	if data == nil || data.Postgres() == nil {
		return nil
	}
	return &knowledgeBlockRepo{data: data, lg: lg}
}

// NewKnowledgeBlockRepoFromData Wire provider（SP1-C）。返回接口同时满足
// bizknowledge.ResolveIndex（同一 struct 实现，装配方经类型断言接线）。
func NewKnowledgeBlockRepoFromData(d *Data) bizknowledge.BlockIndexRepo {
	return NewKnowledgeBlockRepo(d, d.lg)
}

// newBlockID 未锚块 ID：随机 hex（与 knowledge 域 newKnowledgeID 同款风格）。
func newBlockID() string {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "kb-fallback"
	}
	return hex.EncodeToString(buf)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ReplaceDocBlocks 单事务整文档重放：删旧块（FK 级联清出向边、入向边 SET NULL 转
// dangling）→ 插新块 → 按 SrcOrdinal 映射插新边。锚块 ID = ^anchor（设计 S2）。
// 返回本次物化的引用边（SP1-D LinkIndex apply 单元；事务失败时 nil）。
func (r *knowledgeBlockRepo) ReplaceDocBlocks(ctx context.Context, collectionID, docID string, blocks []bizknowledge.KnowledgeBlock, refs []bizknowledge.KnowledgeBlockRefInput) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	var edges []bizknowledge.KnowledgeBlockRefEdge
	err := r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_blocks WHERE doc_id = $1`, docID); err != nil {
			return err
		}
		idByOrdinal := make(map[int]string, len(blocks))
		for _, b := range blocks {
			id := b.ID
			if id == "" {
				id = b.Anchor
			}
			if id == "" {
				id = newBlockID()
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO knowledge_blocks
				 (id, collection_id, doc_id, ordinal, kind, anchor, heading_path, content_hash, text_excerpt, promoted_from, promoted_to)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
				id, collectionID, docID, b.Ordinal, b.Kind, nullIfEmpty(b.Anchor),
				pq.Array(b.HeadingPath), b.ContentHash, b.TextExcerpt,
				nullIfEmpty(b.PromotedFrom), nullIfEmpty(b.PromotedTo),
			); err != nil {
				return err
			}
			idByOrdinal[b.Ordinal] = id
		}
		edges = make([]bizknowledge.KnowledgeBlockRefEdge, 0, len(refs))
		for _, rf := range refs {
			srcID, ok := idByOrdinal[rf.SrcOrdinal]
			if !ok {
				return apierror.BadRequest("knowledge", "block refs: src ordinal %d not in doc %s", rf.SrcOrdinal, docID)
			}
			// 自文档引用：Resolver 只给到目标 ordinal（块未落库无 ID），此处按本次
			// 插入的 ordinal→ID 映射回填；ordinal 不存在属契约违例，拒绝整个事务。
			dstBlockID := rf.DstBlockID
			if dstBlockID == "" && rf.DstSelfOrdinal != nil {
				dstID, ok := idByOrdinal[*rf.DstSelfOrdinal]
				if !ok {
					return apierror.BadRequest("knowledge", "block refs: dst self ordinal %d not in doc %s", *rf.DstSelfOrdinal, docID)
				}
				dstBlockID = dstID
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO knowledge_block_refs
				 (collection_id, src_block_id, dst_doc_id, dst_block_id, raw_target, alias, edge_type, context, ambiguous)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
				collectionID, srcID, nullIfEmpty(rf.DstDocID), nullIfEmpty(dstBlockID),
				rf.RawTarget, rf.Alias, rf.EdgeType, rf.Context, rf.Ambiguous,
			); err != nil {
				return err
			}
			edges = append(edges, bizknowledge.KnowledgeBlockRefEdge{
				CollectionID:    collectionID,
				SrcBlockID:      srcID,
				SrcDocID:        docID,
				DstCollectionID: rf.DstCollectionID,
				DstDocID:        rf.DstDocID,
				DstBlockID:      dstBlockID,
				RawTarget:       rf.RawTarget,
				EdgeType:        rf.EdgeType,
				Context:         rf.Context,
				Ambiguous:       rf.Ambiguous,
			})
		}
		return nil
	})
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	return edges, nil
}

// refEdgeSelect 边查询公共 SELECT+JOIN（SrcDocID 经 src 块推导，DstCollectionID
// 经 dst 文档推导，dangling 为 NULL → 空串）。
const refEdgeSelect = `SELECT r.collection_id, r.src_block_id, sb.doc_id,
        COALESCE(dd.collection_id, ''), COALESCE(r.dst_doc_id, ''), COALESCE(r.dst_block_id, ''),
        r.raw_target, r.edge_type, r.context, r.ambiguous
 FROM knowledge_block_refs r
 JOIN knowledge_blocks sb ON sb.id = r.src_block_id
 LEFT JOIN knowledge_documents dd ON dd.id = r.dst_doc_id`

// ListAllRefEdges 启动全量加载（SP1-D LinkEdgeLoader）：重放 knowledge_block_refs
// 全部边构建内存图。
func (r *knowledgeBlockRepo) ListAllRefEdges(ctx context.Context) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	return r.queryRefEdges(ctx, "")
}

// ── SP1-E：BlockLinkReader 落库兜底（linkIndex 启动窗口未加载时启用） ─────────

var _ bizknowledge.BlockLinkReader = (*knowledgeBlockRepo)(nil)

// ListBacklinksByBlock 块级反链（dst_block_id 精确匹配）。
func (r *knowledgeBlockRepo) ListBacklinksByBlock(ctx context.Context, blockID string) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	return r.queryRefEdges(ctx, ` WHERE r.dst_block_id = $1`, blockID)
}

// ListBacklinksByDoc 文档反链（dst_doc_id 匹配；块级 + 文档级全部入边）。
func (r *knowledgeBlockRepo) ListBacklinksByDoc(ctx context.Context, docID string) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	return r.queryRefEdges(ctx, ` WHERE r.dst_doc_id = $1`, docID)
}

// ListDanglingEdges 集合 dangling 边（dst_doc_id IS NULL，raw_target 保复活线索）。
func (r *knowledgeBlockRepo) ListDanglingEdges(ctx context.Context, collectionID string) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	return r.queryRefEdges(ctx, ` WHERE r.collection_id = $1 AND r.dst_doc_id IS NULL`, collectionID)
}

// GetBlockOwnerDoc 块所属文档 id（SP1-E service 层权限断言前置）。
func (r *knowledgeBlockRepo) GetBlockOwnerDoc(ctx context.Context, blockID string) (string, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT doc_id FROM knowledge_blocks WHERE id = $1`, blockID)
	if err != nil {
		return "", entErrToBizErr(err, "knowledge")
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var docID string
		if err := rows.Scan(&docID); err != nil {
			return "", entErrToBizErr(err, "knowledge")
		}
		return docID, nil
	}
	if err := rows.Err(); err != nil {
		return "", entErrToBizErr(err, "knowledge")
	}
	return "", apierror.NotFound("knowledge", "block not found: %s", blockID)
}

// queryRefEdges 边查询公共执行（SELECT+JOIN + 可选 WHERE 后缀）。
func (r *knowledgeBlockRepo) queryRefEdges(ctx context.Context, where string, args ...any) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, refEdgeSelect+where, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer func() { _ = rows.Close() }()
	var out []bizknowledge.KnowledgeBlockRefEdge
	for rows.Next() {
		var e bizknowledge.KnowledgeBlockRefEdge
		if err := rows.Scan(&e.CollectionID, &e.SrcBlockID, &e.SrcDocID,
			&e.DstCollectionID, &e.DstDocID, &e.DstBlockID,
			&e.RawTarget, &e.EdgeType, &e.Context, &e.Ambiguous); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	return out, nil
}

// ListDocBlocks 按 ordinal 序列出文档全部块。
func (r *knowledgeBlockRepo) ListDocBlocks(ctx context.Context, docID string) ([]bizknowledge.KnowledgeBlock, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, collection_id, doc_id, ordinal, kind, COALESCE(anchor,''), heading_path,
		        content_hash, text_excerpt, COALESCE(promoted_from,''), COALESCE(promoted_to,'')
		 FROM knowledge_blocks WHERE doc_id = $1 ORDER BY ordinal`, docID)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer func() { _ = rows.Close() }()
	var out []bizknowledge.KnowledgeBlock
	for rows.Next() {
		var b bizknowledge.KnowledgeBlock
		var headingPath []string
		if err := rows.Scan(&b.ID, &b.CollectionID, &b.DocID, &b.Ordinal, &b.Kind, &b.Anchor,
			pq.Array(&headingPath), &b.ContentHash, &b.TextExcerpt, &b.PromotedFrom, &b.PromotedTo); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		b.HeadingPath = headingPath
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	return out, nil
}
