package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// VaultSyncApplier 把 SyncEngine 产出的文件变更事件应用到派生索引（P1-3）：
// 文档镜像（documents 表）+ chunks（可选 embedding，无语义层写 NULL）。
//
// 可靠性契约：content_hash 只在 chunks 索引成功后落库——索引失败时下轮扫描
// 仍判为变更（DB hash 落后），自动重试自愈。
type VaultSyncApplier struct {
	uc           *bizknowledge.Usecase
	filer        *bizknowledge.VaultFiler
	embedder     Embedder // nil = 无语义层（R-4）
	lg           loggateway.Logger
	summaryHook  func(root, relPath string)       // nil = 不触发摘要生成
	entityHook   func(collectionID, docID string) // nil = 不触发实体抽取
	relationHook func(collectionID, docID string) // nil = 不触发 typed 关系抽取
	compiler     BodyCompiler                     // nil = 二进制文件降级 error（M0）
}

// BodyCompiler 把需抽取的文件（office/图片）原始字节编译为 Markdown 正文（M0 摄取编译）。
// 由 ExtractorRegistry 适配实现；文本直读文件不经过本端口（保留 frontmatter 解析）。
// 返回：编译后 Markdown 正文 + 规范 MIME（统一 text/markdown）。
type BodyCompiler interface {
	Compile(ctx context.Context, relPath string, raw []byte) (body string, mimeType string, err error)
}

// NewVaultSyncApplier 构造。lg 为 nil 时使用 Noop。
func NewVaultSyncApplier(uc *bizknowledge.Usecase, filer *bizknowledge.VaultFiler, embedder Embedder, lg loggateway.Logger) *VaultSyncApplier {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &VaultSyncApplier{
		uc:       uc,
		filer:    filer,
		embedder: embedder,
		lg:       lg.With(loggateway.Domain("knowledge")),
	}
}

// SetSummaryHook 注入摘要卡 stale 触发器（P2-2）：索引成功且摘要过期时调用。
// 同步/异步由装配方决定（生产：safego.Go 异步包装；测试：同步闭包）。
func (a *VaultSyncApplier) SetSummaryHook(hook func(root, relPath string)) {
	a.summaryHook = hook
}

// SetEntityHook 注入实体共现抽取触发器（P2-4 entity 轨）：索引成功后调用。
// 同步/异步约定同 SetSummaryHook；extractor 内部按 docID+contentHash 幂等。
func (a *VaultSyncApplier) SetEntityHook(hook func(collectionID, docID string)) {
	a.entityHook = hook
}

// SetRelationHook 注入 typed 关系抽取触发器：索引成功后调用（与实体钩子同生命周期）。
// 冷文档不再等热度工人；抽取器内部 content_hash 幂等。
func (a *VaultSyncApplier) SetRelationHook(hook func(collectionID, docID string)) {
	a.relationHook = hook
}

// SetCompiler 注入二进制编译端口（M0 摄取编译）：office/图片经抽取器编译为 Markdown。
// nil（默认）时二进制文件降级 status=error，不 panic；文本直读文件不经过本端口。
func (a *VaultSyncApplier) SetCompiler(c BodyCompiler) {
	a.compiler = c
}

// ApplyEvents 顺序应用一批变更事件。单事件失败不阻塞后续事件（记录日志），
// 返回首个错误供调用方标记 sync_state。
func (a *VaultSyncApplier) ApplyEvents(ctx context.Context, vault bizknowledge.Collection, events []bizknowledge.ChangeEvent) error {
	return a.applyEvents(ctx, vault, events, false)
}

// ApplyEventsForced 同 ApplyEvents，但绕过幂等短路（hash 一致也强制重建 chunks）。
// 用于 ReindexVault（P1-4）：索引损坏/embedding 模型升级/分块策略变更后的全量修复。
func (a *VaultSyncApplier) ApplyEventsForced(ctx context.Context, vault bizknowledge.Collection, events []bizknowledge.ChangeEvent) error {
	return a.applyEvents(ctx, vault, events, true)
}

// ApplyOne 立即应用单个文件变更（G1-B2：树内新建文档不等同步轮询）。
// 实现 bizknowledge.VaultDocApplier 端口。幂等：文件 hash 未变时 upsertDoc 短路。
func (a *VaultSyncApplier) ApplyOne(ctx context.Context, vault bizknowledge.Collection, relPath string) error {
	snap, err := a.filer.SnapshotDoc(vault.RootPath, relPath)
	if err != nil {
		return err
	}
	return a.applyOne(ctx, vault, bizknowledge.ChangeEvent{
		Type:     bizknowledge.ChangeCreated,
		RelPath:  snap.RelPath,
		Snapshot: snap,
	}, false)
}

func (a *VaultSyncApplier) applyEvents(ctx context.Context, vault bizknowledge.Collection, events []bizknowledge.ChangeEvent, force bool) error {
	var firstErr error
	for _, ev := range events {
		if err := a.applyOne(ctx, vault, ev, force); err != nil {
			a.lg.Error("vault sync apply failed",
				loggateway.Str("vault_id", vault.ID),
				loggateway.Str("rel_path", ev.RelPath),
				loggateway.Err(err),
			)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (a *VaultSyncApplier) applyOne(ctx context.Context, vault bizknowledge.Collection, ev bizknowledge.ChangeEvent, force bool) error {
	switch ev.Type {
	case bizknowledge.ChangeCreated, bizknowledge.ChangeModified:
		return a.upsertDoc(ctx, vault, ev, force)
	case bizknowledge.ChangeDeleted:
		return a.deleteDoc(ctx, vault, ev.RelPath)
	case bizknowledge.ChangeMoved:
		return a.moveDoc(ctx, vault, ev)
	default:
		return nil
	}
}

// upsertDoc 创建/重建文档镜像与 chunks。force=true 时跳过幂等短路（reindex 场景）。
// 顺序：建/更文档行 → 删旧 chunks → 分块（+可选 embed）→ 插入 → 回写 hash → 状态/计数。
func (a *VaultSyncApplier) upsertDoc(ctx context.Context, vault bizknowledge.Collection, ev bizknowledge.ChangeEvent, force bool) error {
	existing, getErr := a.uc.GetDocumentByRelPath(ctx, vault.ID, ev.RelPath)
	notFound := apierror.IsCode(getErr, apierror.CodeNotFound)
	if getErr != nil && !notFound {
		return getErr
	}
	// 幂等短路：DB hash 已与文件一致且已索引（prev 快照落后场景，如上轮部分失败重试）。
	// force（reindex）时绕过——必须重建 chunks。
	if !force && !notFound && existing.Status == "indexed" && existing.ContentHash == ev.Snapshot.Hash {
		return nil
	}

	// M0 摄取编译：需抽取文件（office/图片）经编译端口得 Markdown；文本直读走 ReadDoc（保留 frontmatter）。
	var body, mimeType string
	var fm bizknowledge.DocFrontmatter
	if bizknowledge.NeedsExtraction(ev.RelPath) {
		raw, rerr := a.readRawBytes(vault.RootPath, ev.RelPath)
		if rerr != nil {
			return rerr
		}
		if a.compiler == nil {
			// 无编译端口：建文档行标 error 降级（下轮重试），不 panic。
			doc, cerr := a.upsertCompiledDoc(ctx, vault, ev, "", mimeTypeFor(ev.RelPath), notFound, existing)
			if cerr != nil {
				return cerr
			}
			err := apierror.Internal("knowledge", "no body compiler configured for %q", ev.RelPath)
			a.markError(ctx, doc.ID, err)
			return err
		}
		md, mt, cerr := a.compiler.Compile(ctx, ev.RelPath, raw)
		if cerr != nil {
			doc, derr := a.upsertCompiledDoc(ctx, vault, ev, "", mimeTypeFor(ev.RelPath), notFound, existing)
			if derr != nil {
				return derr
			}
			a.markError(ctx, doc.ID, cerr)
			return cerr
		}
		body, mimeType = md, mt
	} else {
		vdoc, err := a.filer.ReadDoc(vault.RootPath, ev.RelPath)
		if err != nil {
			return err
		}
		body, mimeType = vdoc.Body, "text/markdown"
		fm = vdoc.Frontmatter
	}

	var doc bizknowledge.Document
	var err error
	if notFound {
		// ContentHash 留空：索引成功后才经 UpdateDocumentSyncMeta 落库（失败可重试）。
		doc, err = a.uc.CreateDocument(ctx, bizknowledge.Document{
			CollectionID: vault.ID,
			Source:       ev.RelPath,
			MimeType:     mimeType,
			SizeBytes:    ev.Snapshot.Size,
			RelPath:      ev.RelPath,
			ContentText:  body,
			Organized:    true,
			Summary:      fm.Summary,
			SummaryHash:  fm.SummaryHash,
			Tags:         fm.Tags,
			DocType:      fm.Type,
		})
		if err != nil {
			return err
		}
	} else {
		doc = existing
		if err := a.uc.UpdateDocumentContent(ctx, doc.ID, body, true); err != nil {
			return err
		}
	}
	if err := a.uc.UpdateDocumentStatus(ctx, doc.ID, "indexing", "", doc.ChunkCount); err != nil {
		return err
	}

	// SP2 #9：熔断窗口内跳过 embed 尝试（故障期不打 API，词法索引照常）。
	allowEmbed := bizknowledge.EmbedCircuitAllow(existing.EmbedFailCount, existing.EmbedLastTried, time.Now())
	chunks, embedOutcome, err := a.buildChunks(ctx, vault, doc.ID, body, allowEmbed)
	if err != nil {
		a.markError(ctx, doc.ID, err)
		return err
	}

	// 计数：仅「未计入 → indexed」的文档 +1（error/pending 重试不重复计数）。
	docDelta := 0
	if notFound || existing.Status != "indexed" {
		docDelta = 1
	}
	// 索引成功：chunks 替换 + hash/摘要卡 + indexed + 集合计数同一提交。
	if err := a.uc.CommitIndexedDocumentMeta(ctx, vault.ID, doc.ID, chunks, docDelta, bizknowledge.DocumentSyncMeta{
		ContentHash: ev.Snapshot.Hash,
		Summary:     fm.Summary,
		SummaryHash: fm.SummaryHash,
		Tags:        fm.Tags,
		DocType:     fm.Type,
	}); err != nil {
		a.markError(ctx, doc.ID, err)
		return err
	}

	// SP2 #9 熔断状态维护：embed 失败递增计数（退避加深，K3 降级）；embed 成功且
	// 曾有失败 → 复位。熔断窗口内未尝试（skipped）时计数保持不变。
	switch embedOutcome {
	case embedFailed:
		if err := a.uc.UpdateDocumentEmbedCircuit(ctx, doc.ID, existing.EmbedFailCount+1, time.Now()); err != nil {
			a.lg.Warn("vault sync: record embed circuit failure failed",
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(err),
			)
		}
		a.lg.Warn("vault embed failed; degraded to lexical-only indexing",
			loggateway.Str("vault_id", vault.ID),
			loggateway.Str("rel_path", ev.RelPath),
			loggateway.Int("fail_count", existing.EmbedFailCount+1),
		)
	case embedOK:
		if existing.EmbedFailCount > 0 {
			if err := a.uc.UpdateDocumentEmbedCircuit(ctx, doc.ID, 0, time.Time{}); err != nil {
				a.lg.Warn("vault sync: reset embed circuit failed",
					loggateway.Str("doc_id", doc.ID),
					loggateway.Err(err),
				)
			}
		}
	}

	a.lg.Debug("vault doc indexed",
		loggateway.Str("vault_id", vault.ID),
		loggateway.Str("rel_path", ev.RelPath),
		loggateway.Int("chunks", len(chunks)),
	)
	// SP1-C 块级双链：解析 [[...]] 重建块/refs 索引并投影 explicit 文档轨
	// （失败仅降级记日志，不回滚索引；未接线块端口时 no-op）。
	if err := a.rebuildBlockIndex(ctx, vault, doc.ID, body); err != nil {
		a.lg.Warn("vault block index rebuild failed",
			loggateway.Str("vault_id", vault.ID),
			loggateway.Str("rel_path", ev.RelPath),
			loggateway.Err(err),
		)
	}
	// P2-4 entity 轨：触发实体共现抽取（hook 为 nil 时跳过，实体为可选增强）。
	if a.entityHook != nil {
		a.entityHook(vault.ID, doc.ID)
	}
	// typed 关系：与上传/写回图谱钩子同口径，不要求文档先成为热文档。
	if a.relationHook != nil {
		a.relationHook(vault.ID, doc.ID)
	}
	// P2-2：摘要卡过期 → 触发异步重生成（hook 为 nil 时跳过，摘要为可选增强）。
	if a.summaryHook != nil && bizknowledge.SummaryStale(body, fm.SummaryHash) {
		a.summaryHook(vault.RootPath, ev.RelPath)
	}
	return nil
}

// rebuildBlockIndex 重建该文档的块级派生索引（块/refs 物化 + explicit 轨投影）。
// 委托 biz 公共实现（SP1-C；语义：候选来自 DB 镜像最终一致，悬空引用转 dangling）。
func (a *VaultSyncApplier) rebuildBlockIndex(ctx context.Context, vault bizknowledge.Collection, docID, body string) error {
	return a.uc.RebuildBlockIndex(ctx, vault.ID, docID, body)
}

// embedOutcome 表示本轮 embed 尝试结果（SP2 #9）。
type embedOutcome int

const (
	embedSkipped embedOutcome = iota // 无语义层或熔断窗口内：未尝试
	embedOK                          // 尝试成功
	embedFailed                      // 尝试失败：降级写 NULL 向量
)

// buildChunks 分块 + 可选 embed。无语义层（embedding_model 空或 embedder nil）或
// 熔断窗口内（allowEmbed=false）时 Embedding 留空（data 层写 NULL，R-4 降级）。
// SP2 #9：embed 失败不再返回 error——词法索引不被语义故障绑架，降级 NULL 向量继续，
// 熔断计数由调用方维护。
func (a *VaultSyncApplier) buildChunks(ctx context.Context, vault bizknowledge.Collection, docID, body string, allowEmbed bool) ([]bizknowledge.Chunk, embedOutcome, error) {
	splits, err := SplitWithStrategy(ChunkByMarkdown, body, 0, 0)
	if err != nil {
		return nil, embedSkipped, err
	}
	if len(splits) == 0 {
		return nil, embedSkipped, nil
	}
	var vecs [][]float32
	outcome := embedSkipped
	if strings.TrimSpace(vault.EmbeddingModel) != "" && a.embedder != nil && allowEmbed {
		texts := make([]string, len(splits))
		for i, sp := range splits {
			texts[i] = sp.Content
		}
		vecs, err = a.embedder.Embed(ctx, texts)
		if err != nil {
			// 降级：写 NULL 向量，词法索引照常；熔断计数由 upsertDoc 维护。
			vecs = nil
			outcome = embedFailed
		} else if len(vecs) != len(splits) {
			return nil, embedSkipped, apierror.Internal(apierror.DomainKnowledge,
				"vault sync: embedding count mismatch: expected %d, got %d", len(splits), len(vecs))
		} else {
			outcome = embedOK
		}
	}
	out := make([]bizknowledge.Chunk, 0, len(splits))
	for i, sp := range splits {
		ch := bizknowledge.Chunk{
			ID:           fmt.Sprintf("%s-ch-%d", docID, sp.ChunkIndex),
			DocID:        docID,
			CollectionID: vault.ID,
			Content:      sp.Content,
			ChunkIndex:   sp.ChunkIndex,
			MetadataJSON: "{}",
		}
		if vecs != nil {
			ch.Embedding = vecs[i]
		}
		out = append(out, ch)
	}
	return out, outcome, nil
}

// deleteDoc 外部删除 → 镜像内容抢救进 trash 后删文档镜像（R-2：不丢用户数据）。
// chunks 与计数由 repo 级联校正。幂等（无镜像直接返回）。
// 救援失败返回错误——下轮扫描重试，避免镜像删除后内容永久丢失。
func (a *VaultSyncApplier) deleteDoc(ctx context.Context, vault bizknowledge.Collection, relPath string) error {
	doc, err := a.uc.GetDocumentByRelPath(ctx, vault.ID, relPath)
	if apierror.IsCode(err, apierror.CodeNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// 抢救：文件已被外部删除，从镜像重建内容写入 trash（不写回原路径，尊重删除意图）。
	if doc.ContentText != "" {
		if _, err := a.filer.WriteTrashFromMirror(vault.RootPath, relPath, &bizknowledge.VaultDoc{
			Frontmatter: bizknowledge.DocFrontmatter{
				Summary:     doc.Summary,
				SummaryHash: doc.SummaryHash,
				Tags:        doc.Tags,
				Type:        doc.DocType,
			},
			Body: doc.ContentText,
		}); err != nil {
			return err
		}
	}
	return a.uc.DeleteDocument(ctx, doc.ID)
}

// moveDoc 移动/重命名：保留文档身份与索引，仅更新镜像路径。
// 原路径无镜像时兜底按 Created 处理（索引自愈）。
func (a *VaultSyncApplier) moveDoc(ctx context.Context, vault bizknowledge.Collection, ev bizknowledge.ChangeEvent) error {
	doc, err := a.uc.GetDocumentByRelPath(ctx, vault.ID, ev.OldRelPath)
	if apierror.IsCode(err, apierror.CodeNotFound) {
		return a.upsertDoc(ctx, vault, bizknowledge.ChangeEvent{
			Type:     bizknowledge.ChangeCreated,
			RelPath:  ev.RelPath,
			Snapshot: ev.Snapshot,
		}, false)
	}
	if err != nil {
		return err
	}
	return a.uc.UpdateDocumentRelPath(ctx, doc.ID, ev.RelPath)
}

// markError best-effort 回写 error 状态（hash 未落库，下轮自动重试）。
func (a *VaultSyncApplier) markError(ctx context.Context, docID string, cause error) {
	if err := a.uc.UpdateDocumentStatus(ctx, docID, "error", cause.Error(), 0); err != nil {
		a.lg.Warn("vault sync: mark error status failed",
			loggateway.Str("doc_id", docID),
			loggateway.Err(err),
		)
	}
}

// readRawBytes 读 vault 内文件的原始字节（M0：供二进制编译，绕过 parseVaultDoc）。
func (a *VaultSyncApplier) readRawBytes(root, relPath string) ([]byte, error) {
	rel, err := bizknowledge.SanitizeRelPath(relPath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
}

// upsertCompiledDoc 编译失败路径：确保文档行存在（标 error 用），不建 chunks。
// 幂等：已存在则复用；新建时 ContentText 为空、状态由调用方 markError 置 error。
func (a *VaultSyncApplier) upsertCompiledDoc(ctx context.Context, vault bizknowledge.Collection, ev bizknowledge.ChangeEvent, body, mime string, notFound bool, existing bizknowledge.Document) (bizknowledge.Document, error) {
	if !notFound {
		return existing, nil
	}
	return a.uc.CreateDocument(ctx, bizknowledge.Document{
		CollectionID: vault.ID,
		Source:       ev.RelPath,
		MimeType:     mime,
		SizeBytes:    ev.Snapshot.Size,
		RelPath:      ev.RelPath,
		ContentText:  body,
		Organized:    false,
	})
}

// mimeTypeFor 按扩展名给出源文件 MIME（编译失败留痕用；成功时以编译器返回为准）。
func mimeTypeFor(relPath string) string {
	switch strings.ToLower(filepath.Ext(relPath)) {
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// convergeMissingBatchSize 是单轮块索引漂移收敛的文档上限（SP2 #4）。
const convergeMissingBatchSize = 50

// ConvergeMissingBlockIndex 检出「已索引但块索引缺失」的漂移文档并自动重建
// （SP2 #4 下游收敛：rebuildBlockIndex 失败仅 Warn 降级而 content_hash 已落库，
// 下轮不再重试——不收敛则块级双链长期静默滞后）。单文档失败不阻塞后续文档。
// 未接线块端口/无漂移时廉价 no-op；由 Runner 低频调用（convergeInterval 门控）。
func (a *VaultSyncApplier) ConvergeMissingBlockIndex(ctx context.Context, vault bizknowledge.Collection) error {
	docIDs, err := a.uc.ListDocsMissingBlockIndex(ctx, vault.ID, convergeMissingBatchSize)
	if err != nil {
		return err
	}
	if len(docIDs) == 0 {
		return nil
	}
	recovered := 0
	for _, docID := range docIDs {
		doc, err := a.uc.GetDocument(ctx, docID)
		if err != nil {
			a.lg.Warn("vault block index converge: fetch doc failed",
				loggateway.Str("vault_id", vault.ID),
				loggateway.Str("doc_id", docID),
				loggateway.Err(err),
			)
			continue
		}
		if err := a.rebuildBlockIndex(ctx, vault, doc.ID, doc.ContentText); err != nil {
			a.lg.Warn("vault block index converge: rebuild failed",
				loggateway.Str("vault_id", vault.ID),
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(err),
			)
			continue
		}
		recovered++
	}
	// K3 降级恢复：漂移检出即收敛动作发生，一条 Info 汇总（低频调用，非噪声）。
	a.lg.Info("vault block index converge sweep done",
		loggateway.Str("vault_id", vault.ID),
		loggateway.Int("drifted", len(docIDs)),
		loggateway.Int("recovered", recovered),
	)
	return nil
}

// embedRetryBatchSize 是单轮退避重试的文档上限（SP2 #9）。
const embedRetryBatchSize = 50

// RetryDegradedEmbeddings 按指数退避重试熔断中文档的向量补齐（SP2 #9）。
// 熔断窗口内的文档跳过（不打 API）；成功复位熔断并回填向量，失败递增计数加深退避。
// 单文档失败不阻塞后续文档。无语义层/无 embedder 时 no-op。
func (a *VaultSyncApplier) RetryDegradedEmbeddings(ctx context.Context, vault bizknowledge.Collection) error {
	if a.embedder == nil || strings.TrimSpace(vault.EmbeddingModel) == "" {
		return nil
	}
	docs, err := a.uc.ListEmbedDegradedDocuments(ctx, vault.ID, embedRetryBatchSize)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, d := range docs {
		if !bizknowledge.EmbedCircuitAllow(d.EmbedFailCount, d.EmbedLastTried, now) {
			continue
		}
		if err := a.retryDocEmbeddings(ctx, d); err != nil {
			// K4 重试失败（每文档一条，批量上限 50 天然限流）。
			a.lg.Warn("vault embed retry failed; backoff deepened",
				loggateway.Str("vault_id", vault.ID),
				loggateway.Str("doc_id", d.ID),
				loggateway.Int("fail_count", d.EmbedFailCount+1),
				loggateway.Err(err),
			)
		}
	}
	return nil
}

// retryDocEmbeddings 重嵌单文档：从镜像正文重分块 → embed → 回填向量 → 复位熔断；
// 失败递增计数并刷新退避起点。
func (a *VaultSyncApplier) retryDocEmbeddings(ctx context.Context, doc bizknowledge.Document) error {
	splits, err := SplitWithStrategy(ChunkByMarkdown, doc.ContentText, 0, 0)
	if err != nil || len(splits) == 0 {
		return err
	}
	texts := make([]string, len(splits))
	for i, sp := range splits {
		texts[i] = sp.Content
	}
	vecs, err := a.embedder.Embed(ctx, texts)
	if err == nil && len(vecs) != len(splits) {
		err = apierror.Internal(apierror.DomainKnowledge,
			"vault embed retry: embedding count mismatch: expected %d, got %d", len(splits), len(vecs))
	}
	if err != nil {
		if uerr := a.uc.UpdateDocumentEmbedCircuit(ctx, doc.ID, doc.EmbedFailCount+1, time.Now()); uerr != nil {
			a.lg.Warn("vault sync: deepen embed circuit failed",
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(uerr),
			)
		}
		return err
	}
	// 按 chunk ID 精确寻址回填（与 buildChunks 的确定性 ID 构造一致）。
	updates := make([]bizknowledge.ChunkEmbedding, len(splits))
	for i, sp := range splits {
		updates[i] = bizknowledge.ChunkEmbedding{
			ChunkID:   fmt.Sprintf("%s-ch-%d", doc.ID, sp.ChunkIndex),
			Embedding: vecs[i],
		}
	}
	if err := a.uc.UpdateChunkEmbeddings(ctx, doc.ID, updates); err != nil {
		return err
	}
	return a.uc.UpdateDocumentEmbedCircuit(ctx, doc.ID, 0, time.Time{})
}
