package knowledge

import (
	"context"
	"sort"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// Vault 资源管理器（P3）：文件夹树懒加载 + 关联区已解析关联。
// G1-B1：目录节点 = FS 扫描 ∪ 文档 rel_path 聚合（Vault 模式空目录可见、带 mtime；
// 历史 Collection 纯索引聚合），文件节点始终镜像已索引文档（携带摘要卡字段），
// 供三栏 UI 中栏列表与 hover 卡一级密度直接渲染。

// DocumentPath 轻量文档路径行（树构建用，不含正文/向量）。
type DocumentPath struct {
	ID        string
	RelPath   string
	Source    string
	Summary   string
	Tags      []string
	DocType   string
	Status    string
	SizeBytes int64
	UpdatedAt string
	// ErrorMessage 解析失败原因（status=error 时非空）。
	ErrorMessage string
}

// VaultTreeNode 一层目录列表节点：dir 为聚合节点，file 镜像一篇已索引文档。
type VaultTreeNode struct {
	Name      string   // 末段名称
	Path      string   // vault 相对路径（dir 带尾斜杠）
	Kind      string   // dir | file
	DocID     string   // file 专属
	Summary   string   // file 专属
	Tags      []string // file 专属
	DocType   string   // file 专属
	Status    string   // file 专属
	SizeBytes int64    // file 专属
	UpdatedAt string   // file 专属
	// ErrorMessage 解析失败原因（status=error 时非空；dir 恒空）。
	ErrorMessage string
}

// ResolvedLink 关联区展示用已解析关联（R-3：UI 必须标注来源类型）。
type ResolvedLink struct {
	TargetDocID   string
	TargetSource  string
	TargetRelPath string
	LinkType      string // explicit | entity | semantic
	Context       string // explicit: [[ref]] 原文；entity: 共享实体名
	Direction     string // out（本文引用目标）| in（目标引用本文）
}

// DocumentPathReader 树构建窄接口（P3-1）。
// Stability:evolving
type DocumentPathReader interface {
	// ListDocumentPaths 返回 vault 全部文档的轻量路径行（聚合在内存完成）。
	ListDocumentPaths(ctx context.Context, collectionID string) ([]DocumentPath, error)
}

// ResolvedLinkReader 关联解析窄接口（P3-3；data 层 JOIN 一次取回，禁 N+1）。
// Stability:evolving
type ResolvedLinkReader interface {
	ListResolvedLinks(ctx context.Context, collectionID, docID, linkType string) ([]ResolvedLink, error)
}

// SetExplorerRepos 接线资源管理器能力（装配在 service/wire 层）。
// paths 未接线时 ListVaultTree 显式报错（不可静默返回空树）；
// links 未接线时 ListDocumentResolvedLinks 降级返回空（关联为可选增强，与 P2-4 一致）。
func (u *Usecase) SetExplorerRepos(paths DocumentPathReader, links ResolvedLinkReader) {
	u.paths = paths
	u.resolvedLinks = links
}

// SetVaultFiler 接线 vault 文件系统边界（G1-B1：树目录 FS 扫描）。
// 未接线时 ListVaultTree 目录退化为纯索引聚合（历史行为，兼容非 vault 场景）。
func (u *Usecase) SetVaultFiler(f *VaultFiler) {
	u.filer = f
}

// ListVaultTree 返回 prefix 直接子节点（懒加载）。
// Vault 模式（collection 有 root_path 且 filer 已接线）：目录 = FS 扫描 ∪ 索引聚合——
// FS 目录带 mtime（空目录亦可见）；索引独有目录为外部删除待同步的兜底（无 mtime）。
// 历史 Collection（无 root_path）退化为纯索引聚合。文件节点始终来自索引
// （summary/tags/status 等摘要卡字段不变）。
// prefix 归一为「无首斜杠、目录带尾斜杠」；非 vault 文档（rel_path 空）归入根层。
func (u *Usecase) ListVaultTree(ctx context.Context, collectionID, prefix string) ([]VaultTreeNode, error) {
	if err := u.requireRepo(); err != nil {
		return nil, err
	}
	if u.paths == nil {
		return nil, apierror.Unavailable("KNOWLEDGE", "vault explorer not configured")
	}
	if strings.TrimSpace(collectionID) == "" {
		return nil, ErrCollectionIDRequired
	}
	prefix = normalizeTreePrefix(prefix)

	dirs := map[string]VaultTreeNode{}
	if u.filer != nil {
		col, err := u.collections.GetCollection(ctx, collectionID)
		if err != nil {
			return nil, err
		}
		if col.RootPath != "" {
			infos, err := u.filer.ListSubdirs(col.RootPath, strings.TrimSuffix(prefix, "/"))
			if err != nil {
				return nil, err
			}
			for _, d := range infos {
				dirs[d.Name] = VaultTreeNode{
					Name:      d.Name,
					Path:      prefix + d.Name + "/",
					Kind:      "dir",
					UpdatedAt: d.ModTime.UTC().Format(time.RFC3339),
				}
			}
		}
	}

	paths, err := u.paths.ListDocumentPaths(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	var files []VaultTreeNode
	for _, p := range paths {
		rel := p.RelPath
		if rel == "" {
			// 旧库文档无 rel_path：仅在根层以 source 为名呈现为文件。
			if prefix == "" {
				files = append(files, fileTreeNode(p, p.Source, ""))
			}
			continue
		}
		if !strings.HasPrefix(rel, prefix) {
			continue
		}
		rest := rel[len(prefix):]
		if i := strings.Index(rest, "/"); i >= 0 {
			name := rest[:i]
			if _, ok := dirs[name]; !ok {
				dirs[name] = VaultTreeNode{Name: name, Path: prefix + name + "/", Kind: "dir"}
			}
			continue
		}
		files = append(files, fileTreeNode(p, rest, prefix))
	}
	out := make([]VaultTreeNode, 0, len(dirs)+len(files))
	for _, n := range dirs {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })
	return append(out, files...), nil
}

// ListDocumentResolvedLinks 列出文档已解析关联（双向；linkType 空 = 全部三类）。
func (u *Usecase) ListDocumentResolvedLinks(ctx context.Context, collectionID, docID, linkType string) ([]ResolvedLink, error) {
	if u == nil || u.resolvedLinks == nil {
		return nil, nil
	}
	return u.resolvedLinks.ListResolvedLinks(ctx, collectionID, docID, linkType)
}

// normalizeTreePrefix 归一树前缀：去首尾空白与首斜杠；非空时保证尾斜杠
// （"notes" → "notes/"，防止误配 "notes2/"）。
func normalizeTreePrefix(prefix string) string {
	p := strings.Trim(strings.TrimSpace(prefix), "/")
	if p == "" {
		return ""
	}
	return p + "/"
}

func fileTreeNode(p DocumentPath, name, prefix string) VaultTreeNode {
	return VaultTreeNode{
		Name:         name,
		Path:         prefix + name,
		Kind:         "file",
		DocID:        p.ID,
		Summary:      p.Summary,
		Tags:         p.Tags,
		DocType:      p.DocType,
		Status:       p.Status,
		SizeBytes:    p.SizeBytes,
		UpdatedAt:    p.UpdatedAt,
		ErrorMessage: p.ErrorMessage,
	}
}
