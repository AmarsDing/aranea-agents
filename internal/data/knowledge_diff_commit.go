package data

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
)

// knowledge_diff_commit.go — G-32 T4 diff-commit tx 十三步接线（ADR-KN-20261022-I 决策 5
// 全链；清单唯一权威：knowledge_chunk_archive.go:19-51，本文件为执行体，步骤编号与清单
// 逐条对齐）。
//
// 版本行 producer 值空间扩展登记（决策 9；新增值须同步三件套决策 9 行）：
//   ingest              摄取 diff 管道（本文件 ⑫）
//   user-delete         前台删除墓碑（knowledge_documents.go DeleteDocument）
//   g32-tombstone-mint  零账本 seed 护栏（内核 tombstoneDocumentTx 固定值）
//   governance          治理回放（治理线既有登记）
//   connector_source_*  保留给 G-38 连接器（37-knowledge.design.md §G-38⑧）
const knowledgeDiffProducerIngest = "ingest"

// commitIndexedDocumentTx 十三步同 tx（维度批次断言由包外入口完成）：
//  ① FOR UPDATE 锁行（content_text/collection_id/content_hash 同读；决策 8 doc 行锁
//    唯一串行点；content_hash 供 ②⑫ 复用免于二次查询）→
//  ② fast-path（同 body 且 chunk 多重集恒等：chunks/侧表/档案/版本行全零写，仅 ⑬
//    guard/计数/syncMeta 原序执行——判定实现依赖 ④⑤，故代码序按数据依赖排）→
//  ③ nextDocVersionNoTx → ④ loadChunksForPairingTx → ⑤ PairChunksForDiff →
//  ⑥ mirrorChunkEmbeddingsArchiveTx(deleted) → ⑦ archiveChunksForVersionTx →
//  ⑧ 删孤儿（FK CASCADE 扫 chunk_embeddings 侧表，⑥ 已镜档故归档腿无损）→
//  ⑨ kept compare-and-skip（chunk_index/parent_chunk_id/metadata 实变才 UPDATE；
//    IS DISTINCT FROM 谓词内嵌——jsonb 结构等价由 PG 判，杜绝 jsonb::text 空白伪漂移）→
//  ⑩ added 代际命名 insertChunksTx（<docID>-v<ver>-ch-<idx> / -p-<idx>，决策 5⑧
//    化身出生版本解析唯一锚点）→
//  ⑪ 缺 dim 条件补齐（kept 行带新向量且 (chunk_id,dim) NOT EXISTS 时侧表落行——
//    G-39 换 dim 同 content 重嵌 added=∅ 场景下新向量的唯一落盘通道；缺此步则
//    新向量静默丢弃、pending-reembed 死循环）→
//  ⑫ 版本行双条件：head 行缺失 OR head.content_hash≠正文 hash OR added>0∧deleted>0；
//    head.content_hash 空串=未知档视为相等（CON-19：同 body 重嵌防伪版本——此时
//    added=∅∧deleted=∅ 天然不触发本步，head hash 亦相等）→
//  ⑬ guard CAS/计数/syncMeta 原序不动 + ChunkDiffStats（决策 5⑬ 透出，T5 WS
//    knowledge_chunks_diff 帧消费）。
func (r *knowledgeRepo) commitIndexedDocumentTx(ctx context.Context, tx *sql.Tx, collectionID, docID string, chunks []biz.KnowledgeChunk, docDelta int, syncMeta *biz.KnowledgeDocumentSyncMeta) (bizknowledge.ChunkDiffStats, error) {
	stats := bizknowledge.ChunkDiffStats{}

	// ① FOR UPDATE 锁行（content_hash 同读：② fast-path 判定 + ⑫ 档锚 AssetHash）。
	var currentBody, currentCollection, docContentHash string
	if err := tx.QueryRowContext(ctx,
		`SELECT content_text, collection_id, content_hash FROM knowledge_documents WHERE id = $1 FOR UPDATE`, docID,
	).Scan(&currentBody, &currentCollection, &docContentHash); err != nil {
		return stats, entErrToBizErr(err, "knowledge")
	}
	if currentCollection != collectionID {
		return stats, apierror.Conflict(apierror.DomainKnowledge, "document moved while indexing")
	}
	// 与 T4 前实现逐字节同文的防串扰守卫（b8 吸收期新增，非本批语义变更）。
	if expected, ok := bizknowledge.DocumentContentVersionFromContext(ctx, docID); ok && bizknowledge.HashContent(currentBody) != expected {
		return stats, apierror.Conflict(apierror.DomainKnowledge, "document content changed while indexing")
	}
	bodyHash := bizknowledge.HashContent(currentBody)

	// ⑬ 前置读：旧 child 计数（FOR UPDATE 串行点后稳定；计数口径=子块，G-06）。
	var oldCount int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_chunks WHERE doc_id = $1 AND chunk_role = 'child'`, docID).Scan(&oldCount); err != nil {
		return stats, entErrToBizErr(err, "knowledge")
	}

	// ④⑤ 现载+配对（配对权威恒 Go 侧现算，决策 5⑤）。
	oldRows, err := loadChunksForPairingTx(ctx, tx, docID)
	if err != nil {
		return stats, err
	}
	plan := bizknowledge.PairChunksForDiff(pairingOldRows(oldRows), diffNewChunkProjections(chunks))
	stats.Added = len(plan.Added)
	stats.Deleted = len(plan.Deleted)
	stats.Kept = len(plan.Kept)

	// ② fast-path：同 body 且多重集恒等 → 零写，仅 ⑬ 原序。VersionNo 透出当前 head
	// （无账本行的历史形态=0）。
	// CON-19 修正前哨：同 body 换 dim 重嵌也是 added=∅∧deleted=∅（多重集恒等），
	// 若在此零写会让 ⑪（新向量唯一落盘通道）旁路、pending-reembed 死循环——
	// fast-path 判定必须用 ⑨⑪ 共用的 kept 映射现检缺 dim，缺则降级走全链
	// （⑥⑦⑧ 自然空转，⑫ 双条件天然抑制版本行）。
	keptOldByNewIdx := make(map[int]chunkDiffRow, len(plan.Kept))
	for _, kp := range plan.Kept {
		for i := range oldRows {
			if oldRows[i].ID == kp.OldID {
				keptOldByNewIdx[kp.NewIdx] = oldRows[i]
				break
			}
		}
	}
	if stats.Added == 0 && stats.Deleted == 0 && bodyHash == docContentHash {
		missing, err := r.pendingDimFillTx(ctx, tx, plan.Kept, chunks, keptOldByNewIdx)
		if err != nil {
			return stats, err
		}
		if !missing {
			stats.FastPath = true
			if stats.VersionNo, err = headDocVersionNoTx(ctx, tx, docID); err != nil {
				return stats, err
			}
			return stats, r.commitGuardCountsSyncMetaTx(ctx, tx, collectionID, docID, oldCount, docDelta, newChildCount(chunks), syncMeta)
		}
	}

	// ③ 版本号（非 fast-path 才取——fast-path 零写不留新 head）。
	ver, err := nextDocVersionNoTx(ctx, tx, docID)
	if err != nil {
		return stats, err
	}
	stats.VersionNo = ver

	// ⑥⑦⑧ 孤儿链：先镜档（向量）→ 后归档（行）→ 后删（FK CASCADE 扫侧表）。
	if stats.Deleted > 0 {
		if _, err := mirrorChunkEmbeddingsArchiveTx(ctx, tx, plan.Deleted); err != nil {
			return stats, err
		}
		if _, err := archiveChunksForVersionTx(ctx, tx, docID, ver, plan.Deleted); err != nil {
			return stats, err
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM knowledge_chunks WHERE doc_id = $1 AND id = ANY($2::text[])`,
			docID, pq.Array(plan.Deleted)); err != nil {
			return stats, entErrToBizErr(err, "knowledge")
		}
	}

	// ⑨⑩⑪ 共享两份映射：kept 对按新块索引→旧行（已在 ② 前哨现载）；父序号→
	// 终态 ID（kept 父=旧 ID，added 父=代际新 ID）。父序号=chunker ParentIndex
	// =父块自身 ChunkIndex（G-06 契约：child.ParentChunkID 的 -p- 尾码即父块
	// ChunkIndex）。
	parentFinalID := map[int]string{}
	for i, ch := range chunks {
		if !ch.IsParent {
			continue
		}
		if kp, ok := keptOldByNewIdx[i]; ok {
			parentFinalID[ch.ChunkIndex] = kp.ID
			continue
		}
		parentFinalID[ch.ChunkIndex] = fmt.Sprintf("%s-v%d-p-%d", docID, ver, ch.ChunkIndex)
	}

	// ⑨ kept compare-and-skip（实变判定整式下放 SQL；audit 投影见 keptOldByNewIdx）。
	for _, kp := range plan.Kept {
		ch := chunks[kp.NewIdx]
		newParent := ""
		if !ch.IsParent {
			newParent = r.resolveParentFinalID(ch, parentFinalID)
		}
		newMeta := ch.MetadataJSON
		if newMeta == "" {
			newMeta = "{}"
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE knowledge_chunks
			SET chunk_index = $2, parent_chunk_id = $3, metadata = $4::jsonb
			WHERE doc_id = $1 AND id = $5
			  AND (chunk_index, parent_chunk_id, metadata) IS DISTINCT FROM ($2, $3, $4::jsonb)`,
			docID, ch.ChunkIndex, newParent, newMeta, kp.OldID); err != nil {
			return stats, entErrToBizErr(err, "knowledge")
		}
	}

	// ⑩ added 代际命名插入（父块先定 ID 后子块解父；父死子随已是 plan.Added 语义
	// 的一部分，见 chunk_pairing.go 头注）。
	addedRows := make([]biz.KnowledgeChunk, 0, stats.Added)
	for _, idx := range plan.Added {
		ch := chunks[idx]
		if ch.IsParent {
			ch.ID = fmt.Sprintf("%s-v%d-p-%d", docID, ver, ch.ChunkIndex)
			ch.ParentChunkID = ""
		} else {
			ch.ID = fmt.Sprintf("%s-v%d-ch-%d", docID, ver, ch.ChunkIndex)
			ch.ParentChunkID = r.resolveParentFinalID(ch, parentFinalID)
		}
		addedRows = append(addedRows, ch)
	}
	if err := r.insertChunksTx(ctx, tx, addedRows); err != nil {
		return stats, err
	}

	// ⑪ 缺 dim 条件补齐：kept 行带新向量而侧表 (chunk_id,dim) 缺席时落行。同 dim
	// 已存在 → DO NOTHING 保留现值（kept 语义=补缺口不覆盖；同 dim 重嵌由 ②
	// fast-path 拦截正文不变场景，正文已变场景走 deleted→added 新 ID 自然换向量）。
	sideModel, sideModelReady := "", false
	for _, kp := range plan.Kept {
		ch := chunks[kp.NewIdx]
		if ch.IsParent || len(ch.Embedding) == 0 {
			continue
		}
		if !sideModelReady {
			sideModel = r.chunkSideModelTx(ctx, tx, collectionID)
			sideModelReady = true
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chunk_embeddings (chunk_id, dim, collection_id, doc_id, model, embedding)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (chunk_id, dim) DO NOTHING`,
			keptOldByNewIdx[kp.NewIdx].ID, len(ch.Embedding), collectionID, docID, sideModel, pgvector.NewVector(ch.Embedding)); err != nil {
			return stats, entErrToBizErr(err, "knowledge")
		}
	}

	// ⑫ 版本行双条件（CON-19 双条件补遗：防同 body 重嵌伪版本）。
	headHash, headOK, err := headDocVersionHashTx(ctx, tx, docID)
	if err != nil {
		return stats, err
	}
	writeVersion := !headOK ||
		(headHash != "" && headHash != bodyHash) ||
		(stats.Added > 0 && stats.Deleted > 0)
	if writeVersion {
		chunkStats := fmt.Sprintf(`{"added":%d,"deleted":%d,"kept":%d}`, stats.Added, stats.Deleted, stats.Kept)
		if err := insertDocumentVersionTx(ctx, tx, documentVersionRow{
			DocID: docID, VersionNo: ver,
			ContentHash: bodyHash,
			// AssetHash 镜像迁移 20261294 seed 定义：documents.content_hash 列值
			// （本 tx ① 现值，syncMeta 更新尚未发生=上代提交定格）。
			AssetHash:  docContentHash,
			ChangeKind: "edit",
			ChunkStats: []byte(chunkStats),
			Producer:   knowledgeDiffProducerIngest,
		}); err != nil {
			return stats, err
		}
	}

	// ⑬ guard/计数/syncMeta 原序不动。
	return stats, r.commitGuardCountsSyncMetaTx(ctx, tx, collectionID, docID, oldCount, docDelta, newChildCount(chunks), syncMeta)
}

// commitGuardCountsSyncMetaTx ⑬ 共享尾部：syncMeta → documents CAS（indexing→indexed）
// → collections 计数。两个路径（fast-path 零写 / diff-commit）调用点唯一化。
func (r *knowledgeRepo) commitGuardCountsSyncMetaTx(ctx context.Context, tx *sql.Tx, collectionID, docID string, oldCount, docDelta, newCount int, syncMeta *biz.KnowledgeDocumentSyncMeta) error {
	if syncMeta != nil {
		if err := r.updateDocumentSyncMetaTx(ctx, tx, docID, *syncMeta); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET status = 'indexed', error_message = '', chunk_count = $2, updated_at = NOW()
		 WHERE id = $1 AND status = 'indexing'`, docID, newCount)
	if err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return bizknowledge.ErrDocumentStatusConflict
	}
	chunkDelta := newCount - oldCount
	if _, err := tx.ExecContext(ctx,
		`UPDATE knowledge_collections
		 SET document_count = document_count + $2,
		     chunk_count    = chunk_count    + $3,
		     updated_at     = NOW()
		 WHERE id = $1`, collectionID, docDelta, chunkDelta); err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	return nil
}

// newChildCount 子块计数（G-06：父块语境行不进 chunk_count，与旧实现同口径）。
func newChildCount(chunks []biz.KnowledgeChunk) int {
	n := 0
	for _, ch := range chunks {
		if !ch.IsParent {
			n++
		}
	}
	return n
}

// headDocVersionNoTx 当前 head 版本号（无账本行=0）。fast-path 透出用。
func headDocVersionNoTx(ctx context.Context, tx *sql.Tx, docID string) (int, error) {
	var v int
	err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version_no), 0) FROM document_versions WHERE doc_id = $1`, docID).Scan(&v)
	if err != nil {
		return 0, entErrToBizErr(err, "knowledge")
	}
	return v, nil
}

// headDocVersionHashTx 当前 head 的 content_hash。第二返回值=false 表示无账本行。
func headDocVersionHashTx(ctx context.Context, tx *sql.Tx, docID string) (string, bool, error) {
	var h string
	err := tx.QueryRowContext(ctx,
		`SELECT content_hash FROM document_versions WHERE doc_id = $1 ORDER BY version_no DESC LIMIT 1`, docID).Scan(&h)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, entErrToBizErr(err, "knowledge")
	}
	return h, true, nil
}

// diffNewChunkProjections 摄取块 → 配对引擎输入投影。ParentIdx=chunker
// ParentIndex（子块 ParentChunkID 的 -p- 尾码），空值/解析失败=-1（无父声明）。
func diffNewChunkProjections(chunks []biz.KnowledgeChunk) []bizknowledge.PairingNewChunk {
	out := make([]bizknowledge.PairingNewChunk, len(chunks))
	for i, ch := range chunks {
		nc := bizknowledge.PairingNewChunk{Content: ch.Content, Role: "child", ParentIdx: -1}
		if ch.IsParent {
			nc.Role = "parent"
		} else {
			nc.ParentIdx = parseParentOrdinal(ch.ParentChunkID)
		}
		out[i] = nc
	}
	return out
}

// resolveParentFinalID 子块父指针重指：incoming ParentChunkID（旧代际命名）→
// 本代终态 ID（kept 父=旧行 ID；added 父=代际新 ID）。解析失败保留原值并告警
// （父悬空属 chunker 契约违例，宁可落原值可观测，不静默清零）。
func (r *knowledgeRepo) resolveParentFinalID(ch biz.KnowledgeChunk, parentFinalID map[int]string) string {
	pid := strings.TrimSpace(ch.ParentChunkID)
	if pid == "" {
		return ""
	}
	if fid, ok := parentFinalID[parseParentOrdinal(pid)]; ok {
		return fid
	}
	r.lg.Warn("child chunk parent ordinal unresolved, keeping incoming pointer",
		loggateway.StepID("knowledge.diff_commit.parent_orphan"),
		loggateway.Str("doc_id", ch.DocID),
		loggateway.Str("parent_chunk_id", pid))
	return pid
}

// parseParentOrdinal 解析 <docID>[-vN]-p-<n> 尾码。非 -p- 尾或非整数=-1。
func parseParentOrdinal(id string) int {
	i := strings.LastIndex(id, "-p-")
	if i < 0 {
		return -1
	}
	n, err := strconv.Atoi(id[i+3:])
	if err != nil {
		return -1
	}
	return n
}

// pendingDimFillTx ② fast-path 前哨（CON-19 修正）：kept 行携新向量而侧表
// (chunk_id,dim) 缺席时返回 true——fast-path 必须降级放行 ⑪，否则同 body 换 dim
// 重嵌的新向量静默丢失（pending-reembed 死循环）。任一探测报错上抛中止（宁
// 不提交，不拿半知情判定零写）。
func (r *knowledgeRepo) pendingDimFillTx(ctx context.Context, tx *sql.Tx, kept []bizknowledge.ChunkPair, chunks []biz.KnowledgeChunk, keptOldByNewIdx map[int]chunkDiffRow) (bool, error) {
	for _, kp := range kept {
		ch := chunks[kp.NewIdx]
		if ch.IsParent || len(ch.Embedding) == 0 {
			continue
		}
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM chunk_embeddings WHERE chunk_id = $1 AND dim = $2)`,
			keptOldByNewIdx[kp.NewIdx].ID, len(ch.Embedding)).Scan(&exists); err != nil {
			return false, entErrToBizErr(err, "knowledge")
		}
		if !exists {
			return true, nil
		}
	}
	return false, nil
}

// chunkSideModelTx 侧表 model 列溯源（集合台账 embedding_model；无语义层集合
// 无台账语义 → 空串）。insertChunksTx 批内缓存闭包同源——本方法为单点口径。
func (r *knowledgeRepo) chunkSideModelTx(ctx context.Context, tx *sql.Tx, collectionID string) string {
	var m string
	if err := tx.QueryRowContext(ctx,
		`SELECT embedding_model FROM knowledge_collections WHERE id = $1`, collectionID).Scan(&m); err != nil {
		r.lg.Warn("chunk side-embedding model lookup fell back to empty",
			loggateway.StepID("knowledge.chunk_side_model_skip"),
			loggateway.Str("collection_id", collectionID),
			loggateway.Err(err))
		return ""
	}
	return m
}
