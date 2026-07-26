package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// ErrRootPathRequired 创建 Vault 必须给定本地路径（US-15）。
var ErrRootPathRequired = apierror.BadRequest("KNOWLEDGE", "root_path is required")

// NormalizeRootPath 规范化 vault 根路径（S-1）：
// 绝对路径化 → 解析 symlink → 去尾部斜杠；校验存在、为目录、非系统根。
func NormalizeRootPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", ErrRootPathRequired
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", apierror.BadRequest("KNOWLEDGE", "invalid root_path: %s", p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	abs = filepath.Clean(abs)
	// 系统根禁止挂载（Windows 卷根 C:\ 或 POSIX /）：Dir(root)==root 即为根。
	if filepath.Dir(abs) == abs {
		return "", apierror.BadRequest("KNOWLEDGE", "root_path must not be a filesystem root: %s", abs)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", apierror.BadRequest("KNOWLEDGE", "root_path not found: %s", abs)
	}
	if !info.IsDir() {
		return "", apierror.BadRequest("KNOWLEDGE", "root_path is not a directory: %s", abs)
	}
	return abs, nil
}

// CreateVault 创建 Vault（US-15）：root_path 规范化 + 唯一（DB 约束兜底），
// embedding_model 可选（空 = 无语义层）。
func (u *Usecase) CreateVault(ctx context.Context, in Collection) (Collection, error) {
	if err := u.requireRepo(); err != nil {
		return Collection{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return Collection{}, ErrNameRequired
	}
	root, err := NormalizeRootPath(in.RootPath)
	if err != nil {
		return Collection{}, err
	}
	in.RootPath = root
	in.EmbeddingModel = strings.TrimSpace(in.EmbeddingModel)
	if in.EmbeddingModel != "" && in.Dim <= 0 {
		in.Dim = 1536
	}
	if in.ID == "" {
		in.ID = newKnowledgeID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.SyncState == "" {
		in.SyncState = "active"
	}
	return u.collections.CreateCollection(ctx, in)
}

// ── Vault 同步（P1-3）：SyncEngine/SyncApplier 的 usecase 入口 ────────────────

// GetDocumentByRelPath 按 vault 相对路径寻址文档镜像。
func (u *Usecase) GetDocumentByRelPath(ctx context.Context, collectionID, relPath string) (Document, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, err
	}
	return u.documents.GetDocumentByRelPath(ctx, collectionID, relPath)
}

// UpdateDocumentRelPath 文件移动/重命名时更新镜像路径（保留文档身份与索引）。
func (u *Usecase) UpdateDocumentRelPath(ctx context.Context, id, newRelPath string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.documents.UpdateDocumentRelPath(ctx, id, newRelPath)
}

// UpdateDocumentSyncMeta 文件内容变更时回写同步元数据（hash/摘要卡字段）。
func (u *Usecase) UpdateDocumentSyncMeta(ctx context.Context, id string, meta DocumentSyncMeta) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.documents.UpdateDocumentSyncMeta(ctx, id, meta)
}

// UpdateCollectionSyncState 回写 vault 同步状态与最近一次同步完成时间；零值时间只更新 state。
func (u *Usecase) UpdateCollectionSyncState(ctx context.Context, id, state string, lastSyncAt time.Time) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.collections.UpdateCollectionSyncState(ctx, id, state, lastSyncAt)
}

// DeleteChunksByDocument 清除文档全部 chunks（vault 重建索引前调用）。
func (u *Usecase) DeleteChunksByDocument(ctx context.Context, docID string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.chunks.DeleteChunksByDocument(ctx, docID)
}
