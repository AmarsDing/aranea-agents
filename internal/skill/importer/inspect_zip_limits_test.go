package importer

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// R3: zip-bomb 防护 — 文件数与总解压字节上限（单文件 2MB 限制的补充）。
func TestInspectSkillZip_TooManyFiles(t *testing.T) {
	files := map[string]string{
		"SKILL.md": makeSkillMD("Root Skill", "root layout"),
	}
	for i := 0; i < maxImportFiles; i++ {
		files[fmt.Sprintf("f%03d.txt", i)] = "x"
	}
	eng := &Engine{repo: &stubSkillRepo{}}
	err := eng.inspectSkillZip(context.Background(), buildZipBytes(t, files), newInspectJob())
	if !errors.Is(err, ErrTooManyFiles) {
		t.Fatalf("expected ErrTooManyFiles, got %v", err)
	}
}

func TestInspectSkillZip_TotalSizeExceeded(t *testing.T) {
	old := maxImportTotalBytes
	maxImportTotalBytes = 64
	defer func() { maxImportTotalBytes = old }()

	files := map[string]string{
		"SKILL.md": makeSkillMD("Root Skill", "root layout"),
		// 两个文件各自远低于单文件上限，但合计超过总量上限。
		"a.txt": string(make([]byte, 64)),
		"b.txt": string(make([]byte, 64)),
	}
	eng := &Engine{repo: &stubSkillRepo{}}
	err := eng.inspectSkillZip(context.Background(), buildZipBytes(t, files), newInspectJob())
	if !errors.Is(err, ErrTotalSizeExceeded) {
		t.Fatalf("expected ErrTotalSizeExceeded, got %v", err)
	}
}
