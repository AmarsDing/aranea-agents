// Package knowledge — Phase 9 多模态血缘：AssetStore 将原始文件留存到本地目录，
// Document.asset_uri 记录相对路径，供预览回链与追溯。
package knowledge

import (
	"os"
	"path/filepath"
	"strings"
)

// AssetStore 是原始文件的本地留存存储。nil 接收方安全（跳过留存）。
type AssetStore struct {
	root string
}

// NewAssetStore 构造留存存储；root 为空时退化为 nil 行为（不留存）。
func NewAssetStore(root string) *AssetStore {
	return &AssetStore{root: strings.TrimSpace(root)}
}

// Save 将原始字节写入 <root>/<docID><ext>（扩展名小写），返回相对 URI。
// docID 经净化（仅保留字母数字、-、_），杜绝路径穿越。
func (s *AssetStore) Save(docID, source string, raw []byte) (string, error) {
	if s == nil || s.root == "" {
		return "", nil
	}
	name := sanitizeAssetName(docID) + strings.ToLower(filepath.Ext(source))
	if err := os.MkdirAll(s.root, 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(s.root, name), raw, 0o600); err != nil {
		return "", err
	}
	return name, nil
}

// Resolve 将 Save 返回的 URI 解析为磁盘绝对路径；非法/越界 URI 返回空串。
func (s *AssetStore) Resolve(uri string) string {
	if s == nil || s.root == "" {
		return ""
	}
	name := filepath.Base(strings.TrimSpace(uri))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return ""
	}
	return filepath.Join(s.root, name)
}

// sanitizeAssetName 仅保留字母数字、-、_，其余替换为 _。
func sanitizeAssetName(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "asset"
	}
	return b.String()
}
