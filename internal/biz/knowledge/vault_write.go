package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// G1-B2：树内新建目录/文档（Vault 写路径）。
// 经 VaultFiler 落盘（sanitize + 原子写）；文档写后立即触发单文档 apply
// （VaultDocApplier 接线时），不等 45s 轮询。共享 VaultFiler 实例与同步链
// 同一，自写标记统一登记，watcher 自动过滤 KB 自身写事件（回环防护）。

// VaultDocApplier 单文档立即应用端口（G1-B2）。
// 生产实现：internal/knowledge.VaultSyncApplier.ApplyOne（索引+关联+摘要 hook 全链路）。
// Stability:evolving
type VaultDocApplier interface {
	ApplyOne(ctx context.Context, vault Collection, relPath string) error
}

// SetVaultApplier 接线立即应用端口（装配在 wire 层）。nil 时 CreateVaultDocument
// 降级跳过立即索引（同步轮询兜底），返回 pending 占位文档。
func (u *Usecase) SetVaultApplier(a VaultDocApplier) {
	u.applier = a
}

// requireVaultCollection 取 collection 并校验其为 vault（root_path 非空）。
func (u *Usecase) requireVaultCollection(ctx context.Context, collectionID string) (Collection, error) {
	if strings.TrimSpace(collectionID) == "" {
		return Collection{}, ErrCollectionIDRequired
	}
	col, err := u.collections.GetCollection(ctx, collectionID)
	if err != nil {
		return Collection{}, err
	}
	if col.RootPath == "" {
		return Collection{}, apierror.BadRequest("knowledge", "collection is not a vault (no root_path): %s", collectionID)
	}
	return col, nil
}

// CreateVaultDir 树内新建目录（幂等；嵌套父级一并创建）。
func (u *Usecase) CreateVaultDir(ctx context.Context, collectionID, dirPath string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if u.filer == nil {
		return apierror.Unavailable("KNOWLEDGE", "vault filer not configured")
	}
	col, err := u.requireVaultCollection(ctx, collectionID)
	if err != nil {
		return err
	}
	return u.filer.Mkdir(col.RootPath, dirPath)
}

// CreateVaultDocument 树内新建文档：模板 md（frontmatter created + 空标题）。
// create 语义：路径已存在 → CodeConflict，原文件保持原样（不覆盖、不备份）。
// 写后立即单文档 apply（applier 接线时）；apply 失败显式返回错误
// （文件已落盘，同步轮询会兜底重试索引）。
func (u *Usecase) CreateVaultDocument(ctx context.Context, collectionID, relPath string) (Document, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, err
	}
	if u.filer == nil {
		return Document{}, apierror.Unavailable("KNOWLEDGE", "vault filer not configured")
	}
	col, err := u.requireVaultCollection(ctx, collectionID)
	if err != nil {
		return Document{}, err
	}
	rel, err := SanitizeRelPath(relPath)
	if err != nil {
		return Document{}, err
	}
	if !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return Document{}, apierror.BadRequest("knowledge", "vault document must be .md: %q", relPath)
	}
	// 存在性预检：WriteDocCAS 冲突仍会写（留双份），不适合 create 语义——
	// 已存在必须保持原文件原样。本地 FS 竞态窗口可忽略（单用户 vault）。
	if _, err := u.filer.SnapshotDoc(col.RootPath, rel); err == nil {
		return Document{}, apierror.Conflict("knowledge", "document already exists: %s", rel)
	} else if !apierror.IsCode(err, apierror.CodeNotFound) {
		return Document{}, err
	}
	if err := u.filer.WriteDoc(col.RootPath, rel, &VaultDoc{
		Frontmatter: DocFrontmatter{Created: time.Now()},
		Body:        "# \n",
	}); err != nil {
		return Document{}, err
	}
	if u.applier == nil {
		return Document{
			CollectionID: col.ID,
			RelPath:      rel,
			Source:       rel,
			MimeType:     "text/markdown",
			Status:       "pending",
		}, nil
	}
	if err := u.applier.ApplyOne(ctx, col, rel); err != nil {
		return Document{}, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainKnowledge)
	}
	return u.documents.GetDocumentByRelPath(ctx, col.ID, rel)
}

// WriteVaultUpload 上传落盘（G1-B3）：把原始字节写入 target_dir/<文件名>，
// 返回 vault 相对路径供调用方落到文档 rel_path。create 语义：同名冲突 →
// CodeConflict（前端弹 覆盖/自动改名/取消）。文件名取 source 末段
// （浏览器/客户端可能给全路径）。索引由调用方走既有 ingest 管线（含提取器链），
// 不经 ApplyOne（同步链仅处理 .md；二进制需 VisionExtractor/PDF 等提取）。
func (u *Usecase) WriteVaultUpload(ctx context.Context, collectionID, targetDir, source string, raw []byte) (string, error) {
	if err := u.requireRepo(); err != nil {
		return "", err
	}
	if u.filer == nil {
		return "", apierror.Unavailable("KNOWLEDGE", "vault filer not configured")
	}
	col, err := u.requireVaultCollection(ctx, collectionID)
	if err != nil {
		return "", err
	}
	// G1 前端约定："/" 表示 vault 根目录（空串 = 历史行为，service 侧不调用）。
	dir := ""
	if strings.TrimSpace(targetDir) != "/" {
		d, err := SanitizeRelPath(targetDir)
		if err != nil {
			return "", err
		}
		dir = d
	}
	name := sourceFileName(source)
	if name == "" {
		return "", apierror.BadRequest("knowledge", "source has no file name: %q", source)
	}
	rel := name
	if dir != "" {
		rel = dir + "/" + name
	}
	if err := u.filer.WriteRaw(col.RootPath, rel, raw); err != nil {
		return "", err
	}
	return rel, nil
}

// RemoveVaultFile 补偿删除（G1-B3：落盘成功但入库失败时回滚 FS）。幂等。
func (u *Usecase) RemoveVaultFile(ctx context.Context, collectionID, relPath string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if u.filer == nil {
		return apierror.Unavailable("KNOWLEDGE", "vault filer not configured")
	}
	col, err := u.requireVaultCollection(ctx, collectionID)
	if err != nil {
		return err
	}
	return u.filer.RemoveDoc(col.RootPath, relPath)
}

// sourceFileName 取 source 的末段文件名（兼容 / 与 \ 分隔；空或目录形式返回空）。
func sourceFileName(source string) string {
	s := strings.TrimSpace(source)
	s = strings.ReplaceAll(s, `\`, `/`)
	if s == "" || strings.HasSuffix(s, "/") {
		return ""
	}
	name := s[strings.LastIndex(s, "/")+1:]
	if name == "." || name == ".." {
		return ""
	}
	return name
}

// ── G2-B5：详情面板编辑保存 ──────────────────────────────────────────────────

// requireVaultDocument 取文档并校验其为 vault 文档（RelPath 非空），返回文档与所属 vault。
func (u *Usecase) requireVaultDocument(ctx context.Context, docID string) (Document, Collection, error) {
	if strings.TrimSpace(docID) == "" {
		return Document{}, Collection{}, apierror.BadRequest("knowledge", "document id is required")
	}
	doc, err := u.documents.GetDocument(ctx, docID)
	if err != nil {
		return Document{}, Collection{}, err
	}
	if doc.RelPath == "" {
		return Document{}, Collection{}, apierror.BadRequest("knowledge", "document is not a vault file (no rel_path): %s", docID)
	}
	col, err := u.requireVaultCollection(ctx, doc.CollectionID)
	if err != nil {
		return Document{}, Collection{}, err
	}
	return doc, col, nil
}

// GetVaultDocumentRaw 读 vault 文档原文 body（编辑器数据源）+ 文件 hash（保存时
// 作 WriteDocCAS 的 expectedHash）。frontmatter 不进编辑器（受管字段 KB 独占，
// 用户 Extra 原样保留在文件里，保存时重新合并）。
func (u *Usecase) GetVaultDocumentRaw(ctx context.Context, docID string) (string, string, error) {
	if err := u.requireRepo(); err != nil {
		return "", "", err
	}
	if u.filer == nil {
		return "", "", apierror.Unavailable("KNOWLEDGE", "vault filer not configured")
	}
	doc, col, err := u.requireVaultDocument(ctx, docID)
	if err != nil {
		return "", "", err
	}
	vd, hash, err := u.filer.ReadDocWithHash(col.RootPath, doc.RelPath)
	if err != nil {
		return "", "", err
	}
	return vd.Body, hash, nil
}

// UpdateVaultDocumentContent 编辑保存（G2-B5）：保留 frontmatter（受管 + 用户 Extra），
// 替换 body，WriteDocCAS 原子写入。baseHash 为前端编辑起点读到的文件 hash；
// 冲突（外部已改/已删/并发创建）时留双份（磁盘版备份进 trash）仍写入并返回
// conflict=true（保守默认不丢数据，前端提示重载）。写后立即重索引（applier 接线时）。
func (u *Usecase) UpdateVaultDocumentContent(ctx context.Context, docID, content, baseHash string) (Document, bool, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, false, err
	}
	if u.filer == nil {
		return Document{}, false, apierror.Unavailable("KNOWLEDGE", "vault filer not configured")
	}
	doc, col, err := u.requireVaultDocument(ctx, docID)
	if err != nil {
		return Document{}, false, err
	}
	// 读出现有 frontmatter（文件可能刚被外部删除——ReadDoc 失败时以空 doc 重建，
	// CAS 会以「expectedHash 非空但文件消失」判冲突，语义一致）。
	vd, _, readErr := u.filer.ReadDocWithHash(col.RootPath, doc.RelPath)
	if readErr != nil {
		vd = &VaultDoc{}
	}
	vd.Body = u.MaybeAutolinkOutgoing(ctx, col.ID, doc.ID, "", content)
	conflict, err := u.filer.WriteDocCAS(col.RootPath, doc.RelPath, vd, baseHash)
	if err != nil {
		return Document{}, false, err
	}
	if u.applier == nil {
		return Document{ID: doc.ID, CollectionID: col.ID, RelPath: doc.RelPath, Status: "pending"}, conflict, nil
	}
	if err := u.applier.ApplyOne(ctx, col, doc.RelPath); err != nil {
		return Document{}, false, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainKnowledge)
	}
	updated, err := u.documents.GetDocumentByRelPath(ctx, col.ID, doc.RelPath)
	if err != nil {
		return Document{}, false, err
	}
	return updated, conflict, nil
}

// ── G3-B4：库内跨目录移动 ───────────────────────────────────────────────────

// MoveVaultDocumentToDir 库内跨目录移动（G3-B4）：VaultFiler 原子 move +
// UpdateDocumentRelPath（保留文档身份/chunks/hash，内容未变无需重索引）+
// 入链修复（引用本文档的其他文档重建 explicit 出链：精确路径引用悬空断链、
// basename 引用移动后仍匹配保留——链接按 docID 关联本不失效，重建是为诚实
// 反映正文 [[ref]] 与目标实际路径的匹配关系）。
// targetDir：""或"/" = vault 根目录。conflictPolicy 见 VaultFiler.Move。
func (u *Usecase) MoveVaultDocumentToDir(ctx context.Context, docID, targetDir, conflictPolicy string) (Document, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, err
	}
	if u.filer == nil {
		return Document{}, apierror.Unavailable("KNOWLEDGE", "vault filer not configured")
	}
	doc, col, err := u.requireVaultDocument(ctx, docID)
	if err != nil {
		return Document{}, err
	}
	// 目标目录归一：""或"/" = 根目录；其余 sanitize。
	dir := ""
	if strings.TrimSpace(targetDir) != "/" && strings.TrimSpace(targetDir) != "" {
		d, err := SanitizeRelPath(targetDir)
		if err != nil {
			return Document{}, err
		}
		dir = d
	}
	name := doc.RelPath[strings.LastIndex(doc.RelPath, "/")+1:]
	newRel := name
	if dir != "" {
		newRel = dir + "/" + name
	}
	// 同目录幂等短路（不动文件系统与 DB）。
	if newRel == doc.RelPath {
		return doc, nil
	}
	finalRel, err := u.filer.Move(col.RootPath, doc.RelPath, newRel, conflictPolicy)
	if err != nil {
		return Document{}, err
	}
	if err := u.documents.UpdateDocumentRelPath(ctx, doc.ID, finalRel); err != nil {
		return Document{}, err
	}
	// 入链修复：重建引用本文档的其他文档的 explicit 出链（失败降级记日志，
	// 不回滚移动——链接最终一致，引用方下次变更时自愈）。
	u.rebuildMovedDocInboundLinks(ctx, col, doc.ID)
	// 返回更新后的镜像（rel_path 已变）。
	doc.RelPath = finalRel
	return doc, nil
}

// rebuildMovedDocInboundLinks 移动后入链修复：对每条指向 movedDocID 的 explicit
// 入链，重建其源文档的出链（源文档正文从 vault 文件系统读取）。LinkRepo 未接线
// 或个别源文档读取失败时降级跳过（不回滚移动主流程）。
func (u *Usecase) rebuildMovedDocInboundLinks(ctx context.Context, col Collection, movedDocID string) {
	if u.links == nil {
		return
	}
	inbound, err := u.links.ListLinks(ctx, col.ID, movedDocID, LinkTypeExplicit)
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, l := range inbound {
		if l.TargetDocID != movedDocID || l.DocID == movedDocID || seen[l.DocID] {
			continue
		}
		seen[l.DocID] = true
		srcDoc, err := u.documents.GetDocument(ctx, l.DocID)
		if err != nil || srcDoc.RelPath == "" {
			continue
		}
		vd, err := u.filer.ReadDoc(col.RootPath, srcDoc.RelPath)
		if err != nil {
			continue
		}
		// 重建失败降级：最终一致语义，引用方下次变更自愈。
		_ = u.RebuildBlockIndex(ctx, col.ID, srcDoc.ID, vd.Body)
	}
}

// ── G2-B6：详情面板多媒体 asset ─────────────────────────────────────────────

// DocumentAssetRef 一个可流式输出的原始文件引用（G2-B6）。
// vault 文档命中 AbsPath（已 resolve 防逃逸）；历史非 vault 文档走 AssetURI
// （由 service 层经 knowledge.AssetStore.Resolve 解析，biz 不依赖生产层存储）。
type DocumentAssetRef struct {
	AbsPath  string
	AssetURI string
	Name     string // 下载文件名
	MimeType string
	ModTime  time.Time
}

// ResolveDocumentAsset 解析文档原始文件引用：vault 文档 → root+rel_path（os.Stat
// 取 ModTime，文件消失返 NotFound）；非 vault → AssetURI（空则 NotFound）。
func (u *Usecase) ResolveDocumentAsset(ctx context.Context, docID string) (DocumentAssetRef, error) {
	if err := u.requireRepo(); err != nil {
		return DocumentAssetRef{}, err
	}
	if strings.TrimSpace(docID) == "" {
		return DocumentAssetRef{}, apierror.BadRequest("knowledge", "document id is required")
	}
	doc, err := u.documents.GetDocument(ctx, docID)
	if err != nil {
		return DocumentAssetRef{}, err
	}
	name := sourceFileName(doc.Source)
	if doc.RelPath != "" {
		col, err := u.requireVaultCollection(ctx, doc.CollectionID)
		if err != nil {
			return DocumentAssetRef{}, err
		}
		rel, err := SanitizeRelPath(doc.RelPath)
		if err != nil {
			return DocumentAssetRef{}, err
		}
		abs, err := resolve(col.RootPath, rel)
		if err != nil {
			return DocumentAssetRef{}, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return DocumentAssetRef{}, apierror.NotFound("knowledge", "asset file missing: %s", rel)
			}
			return DocumentAssetRef{}, apierror.Internal("knowledge", "vault: stat asset %s", rel).WithCause(err)
		}
		if name == "" {
			name = filepath.Base(abs)
		}
		return DocumentAssetRef{AbsPath: abs, Name: name, MimeType: doc.MimeType, ModTime: info.ModTime()}, nil
	}
	if doc.AssetURI == "" {
		return DocumentAssetRef{}, apierror.NotFound("knowledge", "document has no asset: %s", docID)
	}
	if name == "" {
		name = doc.AssetURI
	}
	return DocumentAssetRef{AssetURI: doc.AssetURI, Name: name, MimeType: doc.MimeType}, nil
}
