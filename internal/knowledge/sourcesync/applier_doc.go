// applier_doc.go —— G-38 W3 文档 Applier：connector 提交唯一正道
// （设计 ②「Upsert/Delete 薄封装 vault_sync 提交序列：幂等短路 → CreateDocument
// （adopt 判定在前）→ indexing CAS → BuildIndexedChunks → CommitIndexedDocumentMeta
// → 图谱四 hook」；更新唯走 CommitIndexedDocumentMeta、删除唯走 Usecase.DeleteDocument）。
//
// W3 as-built 登记（design 条文以条目注册）：
//   - 身份寻址＝items 表唯一真相（①）：文档 rel_path 落空——免疫 vault diff 与
//     扩展名族 switch；Source=DisplayPath 剥末尾类扩展名片段（title 语义）。
//   - content_hash＝Applier 以 payload 字节重算 HashContent（不信传输指纹——
//     shared_dir walker 指纹同空间；adopt 命中判定与 UI 上传共用同一 hash 空间，
//     红线 3「同库同 hash 不重复建档」由 GetDocumentByContentHash 命中保）。
//   - adopt 语义（①）：命中即登记（own_lifecycle=false）不改写既有文档、
//     不接管生命周期、不 tombstone——adopt 档内容冻结不随源更新为已知限制
//     （治理面提示挂 kickoff 挂账条目）；源内容漂移后 adopt 按新 hash 再判，
//     未命中即转 own 自建（同库不同 hash 不算撞红线）。
//   - 幂等短路：documents.content_hash 已一致 ∧ status=indexed → 零写返回。
//   - 计数信号（评审修 #D1）：docDelta 唯看 content_hash 空白度——hash 只随
//     成功提交写入，非空（含 error 残态重试）⇒已计数补 0，空⇒首次提交计 1。
//     vault status 口径在「indexed→error(re-ingest 败)→重试」序列双倍计数；
//     connector 侧先行修正，vault 漂移挂账 kickoff/W9 收口待办。
//   - indexing CAS 保计数（评审修 #D2）：UpdateDocumentStatus 传 prevChunkCount
//     （PG 无条件写 chunk_count 列，传 0 清零旧账→窗内删除按 0 减账永久漂移）。
//   - hook 护栏（评审修 #D3）：图谱四 hook 全经 safeInvoke——panic recover 记
//     Warn 不外逸（图谱增强不劫持收轮/审计行/租约归还）。
//   - 孤儿补偿（评审修 #W3-F1）：createDoc 建档后 indexCommit 失败即
//     best-effort DeleteDocument 回滚——无 hash 残档 adopt 不可见、
//     W7 反扫无锚，重试每轮重复建档必泄；回滚失败仅告警登记。
//   - 悬挂绑定线路（评审 W2 修 #5 派生条款）：绑定 docID 的 GetDocument
//     not-found（带外删档）→ 落到 adopt/create 重新建档，不整轮 abort；
//     旧 item 行的绑定由 W7 反扫归口清理。
//   - 图谱四 hook 全部函数注入（entity/relation/blockIndex/diffStats）：
//     未注入即跳（R-4 可选增强纪律）；hook 内部幂等由实现侧承担。
//     block/reindex 失败仅降级 Warn 不回滚主提交（vault rebuildBlockIndex 同口径）。
//   - MIME 直读口径：text/*|json|xml 直读为正文（connector 无 vault frontmatter
//     语义）；其余 MIME 走 Extractor 归一化，未注入 Extractor 即报错进
//     fetch_failed 桶（幂等重试靠下轮 needsWork：ContentHash 未回填）。
//   - embed 熔断不背远端锅（②）：本层不落 EmbedCircuitAllow 计数——BuildIndexedChunks
//     错误的重试节奏由 interval≥900s 天然限流承载；doc 停 pending/error 态
//     下轮自然重跑（submit 成功才回填 content_hash）。
package sourcesync

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// maxSourcePayloadBytes 单件抓取上限（镜像 SyncEngine 32MB 白名单口径，
// 防御二闸——budget 长闸在 runner，单件体 sizing 在此兜底）。
const maxSourcePayloadBytes = 32 << 20

// BodyExtractor 需抽取正文归一化端口（②：Payload 显式 MIME 交 ExtractorRegistry
// 归一化；签名同 knowledge.ExtractorRegistry.Extract——装配层直接透传）。
// Stability:evolving
type BodyExtractor interface {
	Extract(ctx context.Context, raw []byte, source, mimeType string) (string, error)
}

// DocApplierDeps 文档 Applier 依赖面（全部构造注入；图谱四 hook 可后注）。
type DocApplierDeps struct {
	Log       loggateway.Logger     // nil→NewNoop；构造注入铁律
	UC        *bizknowledge.Usecase // 必：提交序列唯一管道
	Items     bizsync.ItemRepo      // 必：items 表唯一寻址/管辖权边界
	Embedder  knowledge.Embedder    // nil→词法降级（R-4；语义层可选）
	Extractor BodyExtractor         // nil→需抽取 MIME 降级 error
}

// DocApplier connector→文档的提交唯一正道（无内部状态；并发安全）。
type DocApplier struct {
	d  DocApplierDeps
	lg loggateway.Logger

	entityHook   func(collectionID, docID string)
	relationHook func(collectionID, docID string)
	blockHook    func(ctx context.Context, collectionID, docID, body string) error
	diffHook     func(collectionID, docID string, stats bizknowledge.ChunkDiffStats)
}

// NewDocApplier 构造（UC/Items 为 nil 即早错——静默 nil 会变成提交路径 panic）。
func NewDocApplier(d DocApplierDeps) (*DocApplier, error) {
	lg := d.Log
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if d.UC == nil || d.Items == nil {
		return nil, fmt.Errorf("sourcesync: doc applier requires UC and Items")
	}
	return &DocApplier{d: d, lg: lg.With(loggateway.Domain("knowledge/sourcesync"))}, nil
}

// SetEntityHook / SetRelationHook / SetBlockHook / SetDiffHook 图谱四 hook
// 后注（vault_sync 同款 setter 纪律；装配层按需接，未接即跳）。
func (a *DocApplier) SetEntityHook(h func(collectionID, docID string)) { a.entityHook = h }
func (a *DocApplier) SetRelationHook(h func(collectionID, docID string)) {
	a.relationHook = h
}
func (a *DocApplier) SetBlockHook(h func(ctx context.Context, collectionID, docID, body string) error) {
	a.blockHook = h
}
func (a *DocApplier) SetDiffHook(h func(collectionID, docID string, stats bizknowledge.ChunkDiffStats)) {
	a.diffHook = h
}

// Upsert 幂等短路 → adopt 判定 → 建/更 → indexing CAS → chunks+meta 同提交 → 图谱 hook。
// Applier 持有 payload.Body 关闭所有权（types.go ⑦ 契约）。
func (a *DocApplier) Upsert(ctx context.Context, conn bizsync.Connector, snap Snapshot, payload *Payload) (ApplyOutcome, error) {
	raw, mime, err := readAndClassify(ctx, payload)
	if err != nil {
		return ApplyOutcome{}, err
	}
	body, docMime, err := a.compileBody(ctx, snap, mime, raw)
	if err != nil {
		return ApplyOutcome{}, err
	}
	docHash := bizknowledge.HashContent(string(raw)) // 不信指纹传输值，字节自证

	it, found, err := a.d.Items.GetItem(ctx, conn.ID, snap.ExternalID)
	if err != nil {
		return ApplyOutcome{}, err
	}
	if found && it.DocumentID != "" && it.OwnLifecycle {
		doc, gerr := a.d.UC.GetDocument(ctx, it.DocumentID)
		switch {
		case gerr == nil:
			return a.updateDoc(ctx, conn, doc, it.OwnLifecycle, body, docMime, docHash)
		case apierror.IsCode(gerr, apierror.CodeNotFound):
			// 悬挂绑定：带外删档 → 落 adopt/create（W7 反扫归口 item 清理）
			a.lg.Warn("sourcesync dangling item binding, falling back to adopt/create",
				loggateway.Str("external_id", snap.ExternalID),
				loggateway.Str("document_id", it.DocumentID))
		default:
			return ApplyOutcome{}, gerr
		}
	}

	// adopt 判定在前（红线 3：同库同 hash 不重复建档）
	if hit, herr := a.d.UC.GetDocumentByContentHash(ctx, conn.CollectionID, docHash); herr == nil && hit.ID != "" {
		return ApplyOutcome{DocumentID: hit.ID, OwnLifecycle: false, ContentHash: docHash}, nil
	} else if herr != nil && !apierror.IsCode(herr, apierror.CodeNotFound) {
		return ApplyOutcome{}, herr
	}
	return a.createDoc(ctx, conn, snap, body, docMime, docHash, int64(len(raw)))
}

// updateDoc 既有绑定更新序列（幂等短路 → 正文 → indexing CAS → 提交 → hook）。
func (a *DocApplier) updateDoc(ctx context.Context, conn bizsync.Connector, doc bizknowledge.Document, own bool, body, docMime, docHash string) (ApplyOutcome, error) {
	if doc.Status == "indexed" && doc.ContentHash == docHash {
		return ApplyOutcome{DocumentID: doc.ID, OwnLifecycle: own, ContentHash: docHash}, nil // 幂等短路
	}
	if err := a.d.UC.UpdateDocumentContent(ctx, doc.ID, body, true); err != nil {
		return ApplyOutcome{}, err
	}
	docDelta := 0
	if doc.ContentHash == "" {
		docDelta = 1 // 唯一计数信号（评审修 #D1，超 vault status 口径）：content_hash
		// 只随成功提交写入——非空⇒此前已计数（含 error 残态重试，彼时 status 信号
		// 会双倍计数）；空⇒首次成功提交=计 1。
	}
	if err := a.indexCommit(ctx, conn, doc.ID, body, docHash, docDelta, doc.ChunkCount); err != nil {
		return ApplyOutcome{}, err
	}
	return ApplyOutcome{DocumentID: doc.ID, OwnLifecycle: own, ContentHash: docHash}, nil
}

// createDoc 新建序列（pending → indexing → 提交 → hook）。
func (a *DocApplier) createDoc(ctx context.Context, conn bizsync.Connector, snap Snapshot, body, docMime, docHash string, size int64) (ApplyOutcome, error) {
	doc, err := a.d.UC.CreateDocument(ctx, bizknowledge.Document{
		CollectionID: conn.CollectionID,
		Source:       displayTitle(snap),
		MimeType:     docMime,
		SizeBytes:    size,
		RelPath:      "", // 免疫 vault diff 与扩展名族 switch（① 身份寻址条）
		ContentText:  body,
		Organized:    true,
	})
	if err != nil {
		return ApplyOutcome{}, err
	}
	if err := a.indexCommit(ctx, conn, doc.ID, body, docHash, 1, 0); err != nil {
		// 孤儿补偿（评审修 #D4/W3-F1）：建档后提交失败——无 hash 的 error 残档对
		// adopt 不可见、W7 反扫无锚，重试每轮重复建档必泄。best-effort 回滚；
		// 失败仅告警（error 残档不进文档计数，W7 tombstone 保洁面可见）。
		if derr := a.d.UC.DeleteDocument(ctx, doc.ID); derr != nil {
			a.lg.Warn("sourcesync create rollback failed; error-status orphan may remain",
				loggateway.Str("doc_id", doc.ID), loggateway.Err(derr))
		}
		return ApplyOutcome{}, err
	}
	return ApplyOutcome{DocumentID: doc.ID, OwnLifecycle: true, ContentHash: docHash}, nil
}

// indexCommit indexing CAS → BuildIndexedChunks → CommitIndexedDocumentMeta → 图谱四 hook。
// 失败路径 best-effort 标 error 态（hash 未回填，下轮 needsWork 自然重试）。
// prevChunkCount 镜像 vault 口径（评审修 #D2）：PG UpdateDocumentStatus 无条件
// 写 chunk_count——CAS 传 0 会在再摄取窗清零旧计数，窗内删除按 0 减账永久漂移；
// 新建传 0（本身无账）。
func (a *DocApplier) indexCommit(ctx context.Context, conn bizsync.Connector, docID, body, docHash string, docDelta, prevChunkCount int) error {
	lg := a.lg.With(loggateway.Str("doc_id", docID), loggateway.Str("connector_id", conn.ID))
	if err := a.d.UC.UpdateDocumentStatus(ctx, docID, "indexing", "", prevChunkCount); err != nil {
		return err
	}
	chunks, err := knowledge.BuildIndexedChunks(ctx, a.d.Embedder, knowledge.IngestParams{
		DocID: docID, CollectionID: conn.CollectionID, Text: body, MetadataJSON: "{}",
	}, nil)
	if err != nil {
		a.markError(ctx, docID, prevChunkCount, err)
		return err
	}
	stats, err := a.d.UC.CommitIndexedDocumentMeta(ctx, conn.CollectionID, docID, chunks, docDelta,
		bizknowledge.DocumentSyncMeta{ContentHash: docHash})
	if err != nil {
		a.markError(ctx, docID, prevChunkCount, err)
		return err
	}
	// 图谱四 hook（未注入即跳；hook 失败仅降级——主提交已成立不回滚；
	// 评审修 #D3：safeInvoke 拦 panic——hook 炸点不外逸越过 runner.finalize，
	// 收轮/审计行/租约归还优先于图谱增强）。
	if a.diffHook != nil {
		a.safeInvoke("diff", docID, func() { a.diffHook(conn.CollectionID, docID, stats) })
	}
	if a.blockHook != nil {
		a.safeInvoke("block", docID, func() {
			if berr := a.blockHook(ctx, conn.CollectionID, docID, body); berr != nil {
				lg.Warn("sourcesync block index rebuild failed", loggateway.Err(berr))
			}
		})
	}
	if a.entityHook != nil {
		a.safeInvoke("entity", docID, func() { a.entityHook(conn.CollectionID, docID) })
	}
	if a.relationHook != nil {
		a.safeInvoke("relation", docID, func() { a.relationHook(conn.CollectionID, docID) })
	}
	return nil
}

// safeInvoke hook panic 护栏：recover 记 Warn 不外逸（图谱增强挂账不重解锁轮）。
func (a *DocApplier) safeInvoke(what, docID string, fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			a.lg.Warn("sourcesync graph hook panic contained",
				loggateway.Str("hook", what), loggateway.Str("doc_id", docID),
				loggateway.Str("panic", fmt.Sprint(rec)))
		}
	}()
	fn()
}

// Delete ⑥-7 唯一正道：own_lifecycle=true ∧ 有绑定才动文档级删；
// not-found 幂等吞（带外删档与墓碑回收本就让目标缺席）。
func (a *DocApplier) Delete(ctx context.Context, conn bizsync.Connector, item bizsync.Item) error {
	if !item.OwnLifecycle || item.DocumentID == "" {
		return nil
	}
	err := a.d.UC.DeleteDocument(ctx, item.DocumentID)
	if err != nil && !apierror.IsCode(err, apierror.CodeNotFound) {
		return err
	}
	return nil
}

// markError best-effort 回写 error 态（#D2 姊妹闸：传 prevChunkCount 保旧账——
// PG 无条件写 chunk_count 列，error 写 0 即清零窗内删除漂移；不回滚、不伪造）。
func (a *DocApplier) markError(ctx context.Context, docID string, prevChunkCount int, cause error) {
	if err := a.d.UC.UpdateDocumentStatus(ctx, docID, "error", cause.Error(), prevChunkCount); err != nil {
		a.lg.Warn("sourcesync mark error status failed",
			loggateway.Str("doc_id", docID), loggateway.Err(err))
	}
}

// readAndClassify 抓取体读取（32MB 二闸）+ MIME 归类。
func readAndClassify(ctx context.Context, payload *Payload) (raw []byte, mime string, err error) {
	if payload == nil || payload.Body == nil {
		return nil, "", fmt.Errorf("sourcesync: nil payload body")
	}
	defer func() { _ = payload.Body.Close() }() // Applier owns Close（⑦ 契约）
	if payload.SizeBytes > maxSourcePayloadBytes {
		return nil, payload.MIME, fmt.Errorf("sourcesync: payload oversize %d > %d", payload.SizeBytes, maxSourcePayloadBytes)
	}
	if ctx.Err() != nil {
		return nil, payload.MIME, ctx.Err()
	}
	raw, err = io.ReadAll(io.LimitReader(payload.Body, maxSourcePayloadBytes+1))
	if err != nil {
		return nil, payload.MIME, fmt.Errorf("sourcesync: read payload: %w", err)
	}
	if int64(len(raw)) > maxSourcePayloadBytes {
		return nil, payload.MIME, fmt.Errorf("sourcesync: payload oversize %d > %d", len(raw), maxSourcePayloadBytes)
	}
	return raw, payload.MIME, nil
}

// compileBody MIME 归一化：文本直读；其余走 Extractor（未配置即硬错——
// fetch_failed 桶记账，下轮 needsWork 自然重试，不静默降级为乱码正文）。
func (a *DocApplier) compileBody(ctx context.Context, snap Snapshot, mime string, raw []byte) (body, docMime string, err error) {
	if isTextLikeMIME(mime) {
		return string(raw), "text/markdown", nil
	}
	if a.d.Extractor == nil {
		return "", mime, fmt.Errorf("sourcesync: no extractor configured for %q (%s)", snap.ExternalID, mime)
	}
	source := snap.DisplayPath
	if source == "" {
		source = snap.ExternalID
	}
	body, err = a.d.Extractor.Extract(ctx, raw, source, mime)
	if err != nil {
		return "", mime, err
	}
	if strings.TrimSpace(body) == "" {
		return "", mime, fmt.Errorf("sourcesync: extractor empty body for %q", snap.ExternalID)
	}
	return body, mime, nil
}

// isTextLikeMIME 文本直读家族（镜像 biz isTextMime 口径：text/*|json|xml|空）。
func isTextLikeMIME(mime string) bool {
	m := strings.ToLower(strings.TrimSpace(mime))
	return m == "" || strings.HasPrefix(m, "text/") || m == "application/json" || m == "application/xml"
}

// displayTitle title 落 source 前剥末尾类扩展名片段（①：title 语义，
// rel_path 免疫替代物）。目录部保留（a/b.md → a/b——层级即命名空间）。
func displayTitle(snap Snapshot) string {
	dp := strings.TrimSpace(snap.DisplayPath)
	if dp == "" {
		dp = snap.ExternalID
	}
	if ext := filepath.Ext(dp); ext != "" {
		dp = strings.TrimSuffix(dp, ext)
	}
	if dp == "" {
		return snap.ExternalID
	}
	return dp
}
