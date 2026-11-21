package knowledge

// 资料库回收站 / 历史版本（工作台 › 资料，2026-11-20）：本机资料库删掉的文件、被改写前的旧版本，
// 都按原相对路径放在 .aranea/trash 里（同名再进来时文件名插一段纳秒时间戳，见 uniqueTrashPath）。
// 这里只做列出、看内容、放回去、彻底删掉；放回去用普通 rename（不打自写标记），交给目录同步重新入库。

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// VaultTrashItem 回收站里的一个文件。
type VaultTrashItem struct {
	Path       string // trash 内相对路径（正斜杠）
	OrigPath   string // 原来的资料库相对路径
	SizeBytes  int64
	ModifiedAt time.Time
}

// trashStamp uniqueTrashPath 插在扩展名前的纳秒时间戳。
var trashStamp = regexp.MustCompile(`\.\d{16,}$`)

// trashOrigPath 由回收站路径还原原路径：dir/a.1700000000000000000.md → dir/a.md。
func trashOrigPath(p string) string {
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	return trashStamp.ReplaceAllString(stem, "") + ext
}

const vaultTrashMaxContent = 1 << 20

func (u *Usecase) vaultTrashRoot(ctx context.Context, collectionID string) (string, error) {
	col, err := u.requireVaultCollection(ctx, collectionID)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(col.RootPath)
	if err != nil {
		return "", apierror.Internal("knowledge", "vault: resolve root").WithCause(err)
	}
	return filepath.Join(root, ".aranea", "trash"), nil
}

// trashTarget 回收站内相对路径 → 绝对路径（拒绝跳出回收站）。
func trashTarget(trashRoot, p string) (string, error) {
	rel, err := SanitizeRelPath(p)
	if err != nil {
		return "", err
	}
	return filepath.Join(trashRoot, filepath.FromSlash(rel)), nil
}

// ListVaultTrash 列出回收站；relPath 非空时只要这个文件的历史版本。新的在前。
func (u *Usecase) ListVaultTrash(ctx context.Context, collectionID, relPath string) ([]VaultTrashItem, error) {
	trashRoot, err := u.vaultTrashRoot(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	want := ""
	if strings.TrimSpace(relPath) != "" {
		if want, err = SanitizeRelPath(relPath); err != nil {
			return nil, err
		}
	}
	out := make([]VaultTrashItem, 0)
	err = filepath.WalkDir(trashRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(trashRoot, p)
		rel = filepath.ToSlash(rel)
		orig := trashOrigPath(rel)
		if want != "" && orig != want {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, VaultTrashItem{Path: rel, OrigPath: orig, SizeBytes: info.Size(), ModifiedAt: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, apierror.Internal("knowledge", "vault: list trash").WithCause(err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModifiedAt.After(out[j].ModifiedAt) })
	return out, nil
}

// GetVaultTrashContent 读回收站里一个文件的文本（看历史版本用，最多 1MB）。
func (u *Usecase) GetVaultTrashContent(ctx context.Context, collectionID, path string) (string, error) {
	trashRoot, err := u.vaultTrashRoot(ctx, collectionID)
	if err != nil {
		return "", err
	}
	target, err := trashTarget(trashRoot, path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(target)
	if err != nil {
		return "", apierror.NotFound("knowledge", "trash item not found")
	}
	defer f.Close()
	buf := make([]byte, vaultTrashMaxContent)
	n, _ := f.Read(buf)
	return string(buf[:n]), nil
}

// RestoreVaultTrash 放回原位置；原位置已有同名文件时改名成「name (2).ext」。返回放回的相对路径。
func (u *Usecase) RestoreVaultTrash(ctx context.Context, collectionID, path string) (string, error) {
	col, err := u.requireVaultCollection(ctx, collectionID)
	if err != nil {
		return "", err
	}
	trashRoot, err := u.vaultTrashRoot(ctx, collectionID)
	if err != nil {
		return "", err
	}
	src, err := trashTarget(trashRoot, path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(src); err != nil {
		return "", apierror.NotFound("knowledge", "trash item not found")
	}
	rel, err := SanitizeRelPath(trashOrigPath(filepath.ToSlash(path)))
	if err != nil {
		return "", err
	}
	root, _ := filepath.Abs(col.RootPath)
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
		if rel, err = uniqueSiblingRel(col.RootPath, rel); err != nil {
			return "", err
		}
	}
	dst := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", apierror.Internal("knowledge", "vault: mkdir restore dir").WithCause(err)
	}
	if err := os.Rename(src, dst); err != nil {
		return "", apierror.Internal("knowledge", "vault: restore from trash").WithCause(err)
	}
	return rel, nil
}

// PurgeVaultTrash 彻底删除回收站里的文件；paths 空 = 清空。返回删掉的个数。
func (u *Usecase) PurgeVaultTrash(ctx context.Context, collectionID string, paths []string) (int64, error) {
	trashRoot, err := u.vaultTrashRoot(ctx, collectionID)
	if err != nil {
		return 0, err
	}
	if len(paths) == 0 {
		items, err := u.ListVaultTrash(ctx, collectionID, "")
		if err != nil {
			return 0, err
		}
		for _, it := range items {
			paths = append(paths, it.Path)
		}
	}
	var n int64
	for _, p := range paths {
		target, err := trashTarget(trashRoot, p)
		if err != nil {
			return n, err
		}
		if err := os.Remove(target); err == nil {
			n++
		}
	}
	return n, nil
}
