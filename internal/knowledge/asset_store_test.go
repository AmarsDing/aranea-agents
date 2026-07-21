package knowledge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssetStoreSaveAndResolve(t *testing.T) {
	root := t.TempDir()
	s := NewAssetStore(root)
	raw := []byte{0x89, 0x50, 0x4E, 0x47}

	uri, err := s.Save("doc-123", "photo.PNG", raw)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if uri == "" {
		t.Fatal("Save returned empty uri")
	}
	// URI 可解析回文件内容（血缘可查）。
	got, err := os.ReadFile(s.Resolve(uri))
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", s.Resolve(uri), err)
	}
	if string(got) != string(raw) {
		t.Errorf("asset content mismatch: got %d bytes", len(got))
	}
	// 扩展名小写保留，便于后续按类型服务。
	if filepath.Ext(uri) != ".png" {
		t.Errorf("uri ext = %q, want .png", filepath.Ext(uri))
	}
}

func TestAssetStoreNilSafe(t *testing.T) {
	var s *AssetStore
	uri, err := s.Save("doc-1", "a.png", []byte{1})
	if err != nil || uri != "" {
		t.Errorf("nil store Save = (%q, %v), want (\"\", nil)", uri, err)
	}
	if got := s.Resolve("anything"); got != "" {
		t.Errorf("nil store Resolve = %q, want \"\"", got)
	}
}

func TestAssetStoreDocIDSanitized(t *testing.T) {
	root := t.TempDir()
	s := NewAssetStore(root)
	// docID 不应允许路径穿越。
	uri, err := s.Save("../evil", "a.png", []byte{1})
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	resolved := s.Resolve(uri)
	if rel, _ := filepath.Rel(root, resolved); rel == ".." || filepath.IsAbs(rel) && len(rel) > 0 && rel[0] == '.' {
		t.Errorf("asset escaped root: %q", resolved)
	}
	if _, err := os.Stat(resolved); err != nil {
		t.Errorf("asset not written under root: %v", err)
	}
}
