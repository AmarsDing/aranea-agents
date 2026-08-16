package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// P1-3：原子写——新建写入、覆盖已存在文件均内容完整，且无 tmp 残留。
func TestAtomicWriteFile_CreateAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "SKILL.md")

	if err := AtomicWriteFile(target, []byte("# v1"), 0o644); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "# v1" {
		t.Fatalf("after create, content = %q", got)
	}

	// 覆盖已存在文件（rename 替换语义，Windows 上 Go 亦支持）。
	if err := AtomicWriteFile(target, []byte("# v2 longer body"), 0o644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "# v2 longer body" {
		t.Fatalf("after overwrite, content = %q (truncated or stale)", got)
	}

	// 无 tmp 残留。
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.Name() != "SKILL.md" {
			t.Fatalf("unexpected residue file: %s", e.Name())
		}
	}
}

// P1-3：目标目录不存在时 CreateTemp 失败，必须返回错误且不产生任何文件。
func TestAtomicWriteFile_MissingDir(t *testing.T) {
	target := filepath.Join(t.TempDir(), "ghost", "SKILL.md")
	if err := AtomicWriteFile(target, []byte("x"), 0o644); err == nil {
		t.Fatal("expected error for missing parent dir")
	}
}
