package data

import (
	"context"
	"database/sql"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
)

// knowledge_chunk_archive.go G-32 T3 归档/版本内核（ADR-KN-20261022-I 决策 3/7/12）：
// 本文件只含新增内部函数与窄读内核，零触碰外源脏面
// （knowledge_chunks.go / knowledge_documents.go / knowledge_citation.go /
// knowledge_search.go / biz/knowledge/knowledge.go 均在他会话脏区）。
//
// T4 接线清单（末段吸收时逐条落位，缺一不可——对抗评审 CON-1 修正后版）：
//  1. CommitIndexedDocument diff-commit 化，十三步同 tx（决策 5 全链）：
//     ① FOR UPDATE 锁行 → ② fast-path（同 body 且 chunk 多重集零变更：零写，
//     仍走 guard 原序）→ ③ nextDocVersionNoTx → ④ loadChunksForPairingTx →
//     ⑤ PairChunksForDiff（配对严格口径见 chunk_pairing.go 头注——kept 行 ID
//     不可变，决策 5⑦「父指针重指」在严格父子约束下恒为空操作，勿当义务
//     实现；服务层提示线的 SELECT 形状与 ORDER BY 必须与 ④ 同形，同输入
//     同输出才有「允许不一致=self-healing」的保界）→ ⑥ mirrorChunkEmbeddingsArchiveTx
//     （先镜档后删孤儿，FK CASCADE 会连侧表向量一并抹掉）→ ⑦ archiveChunksForVersionTx
//     → ⑧ 删孤儿（FK CASCADE 扫侧表）→ ⑨ kept compare-and-skip UPDATE
//     （仅 chunk_index/parent_chunk_id/metadata 实变才写，比对以 ④ 投影的
//     旧行 metadata 为准）→ ⑩ added insertChunksTx——新块 ID 必须代际命名
//     空间 <docID>-v<version_no>-ch-<idx> / -p-<idx>（决策 5⑧，化身视图
//     birth 版本解析唯一锚点）→ ⑪ 缺 dim 补齐条件 upsert（决策 5⑧ 后半段
//     评审 killer）：kept 行带新向量且 chunk_embeddings NOT EXISTS
//     (chunk_id,dim) 时侧表条件落行——G-39 换 dim 主通道同 content 重嵌
//     added=∅，缺此步骤则新向量静默丢弃、pending-reembed 死循环 →
//     ⑫ insertDocumentVersionTx（写入双条件：head 行缺失 OR head.content_hash≠
//     正文 hash OR chunk 多重集变更（added>0∧deleted>0）；head 空串=未知档
//     视为相等）→ ⑬ guard/计数/syncMeta 原序不动 + DiffStats（决策 5⑬）
//     透出供 WS knowledge_chunks_diff 帧（T5）；
//  2. DeleteDocument：SELECT 改 FOR UPDATE 锚锁 + tombstoneDocumentTx
//     （含零账本 mint 护栏 DIS-5；documents 行删除仍是调用方责任——本文件
//     内核不删行，T4 完成接线前不得把 tombstoneDocumentTx 暴露给治理回放
//     等任何其他调用方，否则 current/档案双腿区间重叠双计）；
//  3. knowledge_citation.go loadChunkContents：current 查得的缺口移交
//     queryChunkArchiveLatestContents 合并（DropsMissingChunks 钉同步演化——
//     有档回落补内容、全无档仍弃）；
//  4. 服务层/端口接线：biz/knowledge/chunk_temporal_port.go 声明的窄端口实现到
//     本文件 listChunksAsOfCore / queryChunksAsOfANN / queryChunksAsOfTsvector /
//     queryChunksAsOfTrigram / queryChunksAsOfSubstring / queryChunksAsOfBigramOverlap
//     （SearchQuery.Temporal 复用；词法对型四分支与 SearchChunksBM25 同词素
//     口径、同 RRF 融合形状，决策 12③ sparse 对型的落实位）。

// documentVersionRow document_versions 写入投影（data 内部形参；双条件判定在调用方）。
// CreatedAt 零值 → created_at 落 now()（正常路径：版本时刻=提交时刻）；
// 仅限 tombstone 零账本 mint 护栏显式注入（用 doc 自身的 created_at 构造出生点）。
type documentVersionRow struct {
	DocID       string
	VersionNo   int
	ContentHash string
	AssetHash   string
	ChangeKind  string // seed|create|edit|rename|supersede|tombstone
	ChunkStats  []byte // JSONB；nil/空落 '{}'
	Producer    string
	CreatedAt   time.Time // 零值 = now()
}

// nextDocVersionNoTx 取下一版本号（COALESCE(MAX,0)+1）。并发正确性依赖调用方
// 已持 documents 行 FOR UPDATE（决策 8：doc 行锁是唯一串行点）。
func nextDocVersionNoTx(ctx context.Context, tx *sql.Tx, docID string) (int, error) {
	var v int
	err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version_no), 0) + 1 FROM document_versions WHERE doc_id = $1`, docID).Scan(&v)
	if err != nil {
		return 0, entErrToBizErr(err, "knowledge")
	}
	return v, nil
}

// insertDocumentVersionTx 版本台账行。ON CONFLICT (doc_id,version_no) DO
// NOTHING——治理重放/幂等重试同 (doc,ver) 不产生第二行（决策 9 producer 分级）。
// row.CreatedAt 非零时显式落列（仅限 tombstone 零账本 mint 护栏）；零值由
// COALESCE 落 now()（版本时刻=提交时刻）。区间反序/同 tick 坍缩的视图侧
// 看护在 knowledge_chunk_incarnations（valid_to GREATEST clamp，CON-13）。
func insertDocumentVersionTx(ctx context.Context, tx *sql.Tx, row documentVersionRow) error {
	stats := row.ChunkStats
	if len(stats) == 0 {
		stats = []byte("{}")
	}
	var createdAt any
	if !row.CreatedAt.IsZero() {
		createdAt = row.CreatedAt
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO document_versions (doc_id, version_no, content_hash, asset_hash, change_kind, chunk_stats, producer, created_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, COALESCE($8::timestamptz, now()))
		ON CONFLICT (doc_id, version_no) DO NOTHING`,
		row.DocID, row.VersionNo, row.ContentHash, row.AssetHash, row.ChangeKind, string(stats), row.Producer, createdAt)
	return entErrToBizErr(err, "knowledge")
}

// archiveChunksForVersionTx 删块同事务物化归档（决策 3）：把将死化身的
// 当前行整体快照进 chunk_archive——revision_no=击杀版本号（化身区间闭端
// 由该版本行的 created_at 连接），content/cited_count/metadata 随行快照；
// content_key 如实复制（空串=现算等价口径，配对权威恒在 Go 侧）。ON CONFLICT
// DO NOTHING：同一化身至多归档一次，重放安全。
func archiveChunksForVersionTx(ctx context.Context, tx *sql.Tx, docID string, versionNo int, chunkIDs []string) (int, error) {
	if len(chunkIDs) == 0 {
		return 0, nil
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO chunk_archive
			(doc_id, chunk_id, revision_no, collection_id, content, content_key,
			 chunk_role, parent_chunk_id, chunk_index, cited_count, metadata)
		SELECT doc_id, id, $1, collection_id, content, content_key,
		       chunk_role, parent_chunk_id, chunk_index, cited_count, metadata
		FROM knowledge_chunks
		WHERE doc_id = $2 AND id = ANY($3::text[])
		ON CONFLICT (doc_id, chunk_id, revision_no) DO NOTHING`,
		versionNo, docID, pq.Array(chunkIDs))
	if err != nil {
		return 0, entErrToBizErr(err, "knowledge")
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// mirrorChunkEmbeddingsArchiveTx 向量先镜档（决策 3）：孤儿删除前的同事务
// 前置步骤——FK CASCADE 会连侧表向量一并抹掉，全量时态召回（决策 12）的
// 归档腿依赖此镜像。归档表无 FK，同事务内顺序：先镜档，后归档行，后删孤儿。
// 冲突 = latest-wins（对抗评审 CON-12）：幂等键 (chunk_id,dim) 假设同 id 至多
// 死一次（T4 代际命名保证新化身新 id），但旧二进制回滚窗口允许位置派生 id
// 复活后二次死亡——DO UPDATE 保新化身向量覆盖旧档，杜绝「chunk_archive 新
// 内容 × chunk_embeddings_archive 旧向量」的串化身两条读腿分叉。
func mirrorChunkEmbeddingsArchiveTx(ctx context.Context, tx *sql.Tx, chunkIDs []string) (int, error) {
	if len(chunkIDs) == 0 {
		return 0, nil
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO chunk_embeddings_archive (chunk_id, dim, collection_id, doc_id, model, embedding)
		SELECT chunk_id, dim, collection_id, doc_id, model, embedding
		FROM chunk_embeddings
		WHERE chunk_id = ANY($1::text[])
		ON CONFLICT (chunk_id, dim) DO UPDATE SET
			embedding = EXCLUDED.embedding,
			model = EXCLUDED.model,
			doc_id = EXCLUDED.doc_id,
			collection_id = EXCLUDED.collection_id,
			archived_at = now()`,
		pq.Array(chunkIDs))
	if err != nil {
		return 0, entErrToBizErr(err, "knowledge")
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// archiveWholeDocumentTx 文档级整档（tombstone 前奏）：镜档全部向量 +
// 归档全部 chunk 行（含 parent 语境行），返回归档行数。调用方随后在同
// 事务内写 tombstone 版本行并删除 documents 行（FK CASCADE 清 chunks 与
// chunk_embeddings；chunk_archive / chunk_embeddings_archive /
// document_versions 均无级联 FK——命令12「tombstone 可回放」结构保证）。
func archiveWholeDocumentTx(ctx context.Context, tx *sql.Tx, docID string, versionNo int) (int, error) {
	var ids []string
	if err := func() error {
		rows, err := tx.QueryContext(ctx,
			`SELECT id FROM knowledge_chunks WHERE doc_id = $1 ORDER BY chunk_index, id`, docID)
		if err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return entErrToBizErr(err, "knowledge")
			}
			ids = append(ids, id)
		}
		return entErrToBizErr(rows.Err(), "knowledge")
	}(); err != nil {
		return 0, err
	}
	if _, err := mirrorChunkEmbeddingsArchiveTx(ctx, tx, ids); err != nil {
		return 0, err
	}
	return archiveChunksForVersionTx(ctx, tx, docID, versionNo, ids)
}

// tombstoneDocumentTx 墓碑整档组合（T4 DeleteDocument 接线体；决策10/12）：
// ① nextDocVersionNoTx → ② 零账本护栏（ver==1 先 mint seed——DIS-5）→
// ③ archiveWholeDocumentTx（向量先镜档+全行归档）→ ④ tombstone 版本行
// （change_kind=tombstone，chunk_stats 记 archived/total）。
// 语义钉：tombstone 之后「current 查无，as-of 可回放」——documents 行删除不
// 级联抹账（document_versions 无 FK），归档化身在 [seed_birth, tombstone_t)
// 区间存活可检索。零账本场景（断档写路径挂在 TK-G32-02 的历史形态：有 chunks、
// 零版本行）若直落 tombstone 作首行，归档化身 valid_from=valid_to=tombstone_t
// 区间坍缩 [t,t) 任何时点查无——护栏：先以 doc.created_at 构建 birth 点的
// seed 版本行（change_kind=seed，显式 created_at 注入），tombstone 退至 v2。
// producer 决策9：治理重放 producer='governance'，前台删除走调用方传入值
// （本内核不杜撰）；mint 的 seed 行 producer 固定 g32-tombstone-mint（审计侧
// 可见护发动机）。
func tombstoneDocumentTx(ctx context.Context, tx *sql.Tx, docID, producer string) (int, error) {
	ver, err := nextDocVersionNoTx(ctx, tx, docID)
	if err != nil {
		return 0, err
	}
	if ver == 1 {
		// 调用方已持 documents 行 FOR UPDATE（决策 8 单向锁序），此读稳定。
		// mint 行口径镜像迁移 20261294 种子：content_hash=空串 未知档（首差
		// 才有版本行）、asset_hash=documents.content_hash 列值。
		var docCreatedAt time.Time
		var assetHash string
		if err := tx.QueryRowContext(ctx,
			`SELECT created_at, content_hash FROM knowledge_documents WHERE id = $1`, docID,
		).Scan(&docCreatedAt, &assetHash); err != nil {
			return 0, entErrToBizErr(err, "knowledge")
		}
		if err := insertDocumentVersionTx(ctx, tx, documentVersionRow{
			DocID: docID, VersionNo: 1, AssetHash: assetHash,
			ChangeKind: "seed", Producer: "g32-tombstone-mint", CreatedAt: docCreatedAt,
		}); err != nil {
			return 0, err
		}
		ver = 2
	}
	n, err := archiveWholeDocumentTx(ctx, tx, docID, ver)
	if err != nil {
		return 0, err
	}
	stats := []byte(`{"archived":` + itoa(n) + `}`)
	if err := insertDocumentVersionTx(ctx, tx, documentVersionRow{
		DocID: docID, VersionNo: ver, ChangeKind: "tombstone",
		ChunkStats: stats, Producer: producer,
	}); err != nil {
		return 0, err
	}
	return ver, nil
}

// chunkDiffRow diff-commit tx 现载旧行全列投影（决策 5③ 登记列）：
// id/content/chunk_index/parent_chunk_id/chunk_role 服务配对权威；
// cited_count/metadata 服务 ⑨ kept compare-and-skip（实变判定必须旧行现值，
// 对抗评审 DIS-2——投影缺列会让 T4 回头炸契约）。cited_count 本身不参与
// UPDATE 写回（计数权在 guard/计数原序），但入投影备审计与 DiffStats。
type chunkDiffRow struct {
	ID            string
	Content       string
	ChunkIndex    int
	Role          string
	ParentChunkID string
	CitedCount    int
	Metadata      []byte
}

// pairingOldRows chunkDiffRow → 配对引擎输入投影（列减不减形状——
// SELECT/ORDER BY 不变；DIS-7：服务层提示线必须以同款 SELECT 形状+同 ORDER
// BY 现载，同输入同输出才有「允许不一致=self-healing」保界）。
func pairingOldRows(rows []chunkDiffRow) []bizknowledge.PairingOldRow {
	out := make([]bizknowledge.PairingOldRow, len(rows))
	for i, r := range rows {
		out[i] = bizknowledge.PairingOldRow{
			ID: r.ID, Content: r.Content, ChunkIndex: r.ChunkIndex,
			Role: r.Role, ParentChunkID: r.ParentChunkID,
		}
	}
	return out
}

// loadChunksForPairingTx 配对投影 tx 现载（T2 PairChunksForDiff 的输入供给；
// 服务层预配对 hint 与 tx 权威共用同一投影形状——决策 2：配对权威恒 Go 侧
// 现算，列值 content_key 不参与配对）。决策 5③ 全列一次现载：配对子集经
// pairingOldRows 投影消费，cited_count/metadata 留作 compare-and-skip 与
// 审计侧用（1 次查询喂 3 步，避免同 tx 内二次全表读）。
func loadChunksForPairingTx(ctx context.Context, tx *sql.Tx, docID string) ([]chunkDiffRow, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, content, chunk_index, chunk_role, parent_chunk_id, cited_count, metadata::text
		FROM knowledge_chunks WHERE doc_id = $1
		ORDER BY chunk_index, id`, docID)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	out := []chunkDiffRow{}
	for rows.Next() {
		var r chunkDiffRow
		if err := rows.Scan(&r.ID, &r.Content, &r.ChunkIndex, &r.Role, &r.ParentChunkID, &r.CitedCount, &r.Metadata); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, r)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

// sqlQuerier tx/db 多态查询口（*sql.Tx 与 *sql.DB 均满足）。
type sqlQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// queryChunkArchiveLatestContents 按 chunk_id 精确回落（决策 7：revision 选择
// = 按 chunk_id 精确取最新死亡化身 MAX(revision_no)）。返回 id→content；
// 无档 id 不出现在结果中（调用方维持「无档仍弃」语义）。内容用历史、授权由
// 调用方维持 collection 级当下口径（ADR-KN-20261022-I ChunkArchiveReader 章）。
func queryChunkArchiveLatestContents(ctx context.Context, q sqlQuerier, chunkIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(chunkIDs))
	if len(chunkIDs) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `
		SELECT DISTINCT ON (chunk_id) chunk_id, content
		FROM chunk_archive
		WHERE chunk_id = ANY($1::text[])
		ORDER BY chunk_id, revision_no DESC`, pq.Array(chunkIDs))
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out[id] = content
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

// chunkAsOfItem 时态读投影（决策 12；视图 knowledge_chunk_incarnations 行）。
// 对应 biz/knowledge/chunk_temporal_port.go 的 KnowledgeChunkIncarnation——
// data 层不自引 biz 结构是为端口实现层在 T4 一次映射到位留形。
type chunkAsOfItem struct {
	ChunkID       string
	DocID         string
	CollectionID  string
	Content       string
	ContentKey    string
	ChunkRole     string
	ParentChunkID string
	ChunkIndex    int
	CitedCount    int
	Metadata      []byte
	Archived      bool
	Score         float64 // 仅 ANN 路径有意义
}

// chunkIncarnationSelect 视图投影列（list/ANN/lexical 三路共用，顺序即 Scan 顺序）。
const chunkIncarnationSelect = `i.chunk_id, i.doc_id, i.collection_id, i.content, i.content_key,
  i.chunk_role, i.parent_chunk_id, i.chunk_index, i.cited_count, i.metadata::text, i.archived`

// listChunksAsOfCore 时点截取列表（决策12 T 轴内核）：集合（可收窄单文档）
// 在 t 时点存活的全部化身行，含归档腿；at=now() 时归档腿因 valid_to<infinity
// 天然零泄漏，输出与当前态字节语义一致（T5 e2e 钉⑨之等价口径）。
func listChunksAsOfCore(ctx context.Context, q sqlQuerier, collectionID string, at time.Time, docID string) ([]chunkAsOfItem, error) {
	args := []any{collectionID, at}
	docClause := ""
	if docID != "" {
		docClause = ` AND i.doc_id = $3`
		args = append(args, docID)
	}
	acl, aclArgs := asOfVisibilityClause(ctx, "$1", "i.doc_id", true, len(args)+1)
	args = append(args, aclArgs...)
	rows, err := q.QueryContext(ctx, `
		SELECT `+chunkIncarnationSelect+`, 0.0::float8 AS score
		FROM knowledge_chunk_incarnations i
		WHERE i.collection_id = $1
		  AND i.chunk_role = 'child'
		  AND i.valid_from <= $2 AND $2 < i.valid_to`+docClause+acl+`
		ORDER BY i.doc_id, i.chunk_index, i.chunk_id`, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	return scanChunkAsOfItems(rows)
}

// asOfVisibilityClause 时态读的当下行级可见性谓词（决策12③「授权用当下、内容
// 用历史」的复合落实，T4 接线评审修正——内核初稿无可见性谓词会把当下不可见
// 的 private doc 历史化身借 as-of 外泄）：
//   - 宿主文档当下存活：文档行级可见性照 current 口径（docChunkVisibilityClause
//     同一 visibleDocIDsSubquery 子查询占位）；
//   - 宿主文档已 tombstone（行已删）：无当下行级授权可评，回落 collection 级
//     当下口径（服务端 C-01 集合断言——决策12③ 文本档案保留的可回放语义无
//     行级绑定，死文档历史不借本谓词复活可见性）；
//   - includeArchived=false 的腿（current 腿，并集语义下宿主行必然存活）退化为
//     单纯 IN 子查询（不带 archived 兜支）。
// 参位 argIdx 起排；系统调用方 sub=="" → 无过滤（与 buyer 语义同 current）。
func asOfVisibilityClause(ctx context.Context, collectionPH, docCol string, includeArchived bool, argIdx int) (string, []any) {
	sub, args := visibleDocIDsSubquery(ctx, collectionPH, argIdx)
	if sub == "" {
		return "", nil
	}
	base := docCol + " IN (" + sub + ")"
	if includeArchived {
		base = "(" + base + " OR (i.archived AND NOT EXISTS (SELECT 1 FROM knowledge_documents vd WHERE vd.id = " + docCol + ")))"
	}
	return " AND (" + base + ")", args
}

// queryChunksAsOfANN 全量时态稠密召回（决策12③字面形态，对抗评审 CON-8/14
// 后的重构）：双腿【各自】经化身视图存活谓词裁剪后 per-leg KNN（基表距离
// 排序 + LIMIT——chunk_embeddings / chunk_embeddings_archive 的 per-dim
// partial HNSW 索引因排序键直达基表列而可被规划器选用），再 chunk_id 级
// 去重合并后全局 top-k。dim 以 Go 长度字面量内联进 SQL（与 denseANNSearchSQL
// 同口径：generic plan 可证 partial 谓词、跨 dim 比较结构性不可能）。
// 得分=纯余弦相似度（1-cosine_distance），历史透镜不做现时新鲜度衰减——
// recency/stale 只属 current 快报（决策 12 登记的对称计分）。
// at=now() 时归档腿因 valid_to<infinity 天然零泄漏，输出与 bare current
// 查询等价（T5 e2e 钉⑨口径）。
func queryChunksAsOfANN(ctx context.Context, q sqlQuerier, collectionID string, queryEmbedding []float32, at time.Time, topK int, docID string) ([]chunkAsOfItem, error) {
	if topK <= 0 {
		topK = 5
	}
	dim := len(queryEmbedding)
	if dim == 0 {
		return nil, nil
	}
	vec := pgvector.NewVector(queryEmbedding)
	docClause := ""
	args := []any{vec, collectionID, at, topK}
	if docID != "" {
		docClause = ` AND i.doc_id = $5`
		args = append(args, docID)
	}
	aclIdx := len(args) + 1
	// curr/arch 两腿同 argIdx 调生成器：子查询串与尾随实参组确定性一致（生成器
	// 输出只依赖 ctx+参位），两腿文本各自落位、占位共享同一组实参（同参号复用
	// 有 docBothEndpointsVisibleClause 先例）；系统调用方两腿串皆空、零实参。
	currACL, _ := asOfVisibilityClause(ctx, "$2", "i.doc_id", false, aclIdx)
	archACL, aclArgs := asOfVisibilityClause(ctx, "$2", "i.doc_id", true, aclIdx)
	args = append(args, aclArgs...)
	ds := itoa(dim)
	raw := `
WITH curr AS (
  SELECT ` + chunkIncarnationSelect + `, 1 - (ce.embedding <=> $1::vector) AS score
  FROM knowledge_chunk_incarnations i
  JOIN chunk_embeddings ce ON ce.chunk_id = i.chunk_id AND ce.dim = ` + ds + `
  WHERE i.collection_id = $2
    AND i.chunk_role = 'child'
    AND i.valid_from <= $3 AND $3 < i.valid_to
    AND NOT i.archived
    AND ce.embedding IS NOT NULL` + docClause + currACL + `
  ORDER BY ce.embedding::vector(` + ds + `) <=> $1::vector
  LIMIT $4
),
arch AS (
  SELECT ` + chunkIncarnationSelect + `, 1 - (cea.embedding <=> $1::vector) AS score
  FROM knowledge_chunk_incarnations i
  JOIN chunk_embeddings_archive cea ON cea.chunk_id = i.chunk_id AND cea.dim = ` + ds + `
  WHERE i.collection_id = $2
    AND i.chunk_role = 'child'
    AND i.valid_from <= $3 AND $3 < i.valid_to
    AND i.archived
    AND cea.embedding IS NOT NULL` + docClause + archACL + `
  ORDER BY cea.embedding::vector(` + ds + `) <=> $1::vector
  LIMIT $4
),
merged AS (
  SELECT DISTINCT ON (chunk_id) *
  FROM (SELECT * FROM curr UNION ALL SELECT * FROM arch) u
  ORDER BY chunk_id, score DESC
)
SELECT chunk_id, doc_id, collection_id, content, content_key, chunk_role, parent_chunk_id,
       chunk_index, cited_count, metadata, archived, score
FROM merged
ORDER BY score DESC
LIMIT $4`
	rows, err := q.QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	return scanChunkAsOfItems(rows)
}

// queryChunksAsOfLexical 词法时态分支公共核（决策 12③ sparse 对型，
// 对抗评审 CON-2）：四分支与 SearchChunksBM25 同词素口径（tsvector/trigram/
// substring/CJK bigram），差别仅两处——①载体从 knowledge_chunks current 表
// 换为 knowledge_chunk_incarnations 化身视图并叠加存活谓词（at=now() 时归档
// 腿零泄漏与 current 快报口径一致）；②得分=分支原生分数直出（ts_rank/
// word_similarity/bigram 命中数/1.0），历史透镜不套 recencyScoreSQL 一阶
// 新鲜度衰减（决策 12 对称计分登记；RRF 融合在服务层复用 mergeBM25Rankings
// 同轨——本内核只供单分支排好序的 top-k 列表）。
func queryChunksAsOfLexical(ctx context.Context, q sqlQuerier, branchPredicate, scoreExpr string, collectionID string, queryArg any, at time.Time, topK int, docID string) ([]chunkAsOfItem, error) {
	if topK <= 0 {
		topK = 5
	}
	docClause := ""
	args := []any{queryArg, collectionID, at, topK}
	if docID != "" {
		docClause = ` AND i.doc_id = $5`
		args = append(args, docID)
	}
	acl, aclArgs := asOfVisibilityClause(ctx, "$2", "i.doc_id", true, len(args)+1)
	args = append(args, aclArgs...)
	raw := `
SELECT ` + chunkIncarnationSelect + `, ` + scoreExpr + ` AS score
FROM knowledge_chunk_incarnations i
WHERE i.collection_id = $2
  AND i.chunk_role = 'child'
  AND i.valid_from <= $3 AND $3 < i.valid_to
  AND ` + branchPredicate + docClause + acl + `
ORDER BY score DESC
LIMIT $4`
	rows, err := q.QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	return scanChunkAsOfItems(rows)
}

// queryChunksAsOfTsvector 词法分支 ①：simple 词典 tsquery 精确匹配（口径=
// searchChunksTsvector 去 recency 化）。
func queryChunksAsOfTsvector(ctx context.Context, q sqlQuerier, collectionID, query string, at time.Time, topK int, docID string) ([]chunkAsOfItem, error) {
	return queryChunksAsOfLexical(ctx, q,
		`to_tsvector('simple', i.content) @@ plainto_tsquery('simple', $1)`,
		`ts_rank(to_tsvector('simple', i.content), plainto_tsquery('simple', $1))::float8`,
		collectionID, query, at, topK, docID)
}

// queryChunksAsOfTrigram 词法分支 ②：pg_trgm word_similarity（口径=
// searchChunksTrigram 去 recency 化）。
func queryChunksAsOfTrigram(ctx context.Context, q sqlQuerier, collectionID, query string, at time.Time, topK int, docID string) ([]chunkAsOfItem, error) {
	return queryChunksAsOfLexical(ctx, q,
		`i.content %> $1`,
		`word_similarity($1, i.content)::float8`,
		collectionID, query, at, topK, docID)
}

// queryChunksAsOfSubstring 词法分支 ③：短针大小写不敏感子串（口径=
// searchChunksSubstring 去 recency 化；短针门槛 4/8 runes 判定在服务层
// LexicalSearchQueries 侧，本内核不重复判定）。
func queryChunksAsOfSubstring(ctx context.Context, q sqlQuerier, collectionID, query string, at time.Time, topK int, docID string) ([]chunkAsOfItem, error) {
	return queryChunksAsOfLexical(ctx, q,
		`position(lower($1) in lower(i.content)) > 0`,
		`1.0::float8`,
		collectionID, query, at, topK, docID)
}

// queryChunksAsOfBigramOverlap 词法分支 ④：CJK bigram 命中计数（口径=
// searchChunksBigramOverlap 去 recency 化；bigrams 抽取在服务层 CJKBigrams
// 侧，本内核直接消费 []string）。
func queryChunksAsOfBigramOverlap(ctx context.Context, q sqlQuerier, collectionID string, bigrams []string, at time.Time, topK int, docID string) ([]chunkAsOfItem, error) {
	return queryChunksAsOfLexical(ctx, q,
		`(SELECT COUNT(*) FROM unnest($1::text[]) bg WHERE position(bg in i.content) > 0) > 0`,
		`(SELECT COUNT(*) FROM unnest($1::text[]) bg WHERE position(bg in i.content) > 0)::float8`,
		collectionID, pq.Array(bigrams), at, topK, docID)
}

// itoa 免 fmt 依赖的 int 字面量内联（仅正整数，维度值域安全）。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func scanChunkAsOfItems(rows *sql.Rows) ([]chunkAsOfItem, error) {
	out := []chunkAsOfItem{}
	for rows.Next() {
		var it chunkAsOfItem
		if err := rows.Scan(&it.ChunkID, &it.DocID, &it.CollectionID, &it.Content, &it.ContentKey,
			&it.ChunkRole, &it.ParentChunkID, &it.ChunkIndex, &it.CitedCount, &it.Metadata, &it.Archived, &it.Score); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, it)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}
