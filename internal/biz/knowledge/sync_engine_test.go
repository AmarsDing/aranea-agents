package knowledge

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	loggateway "aranea-agents/pkg/loggateway"
)

func newTestScanner() *SyncEngine {
	return NewSyncEngine(loggateway.NewNoop())
}

func writeVaultFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func TestSyncScannerScanFiltersFiles(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "财报/q2.md", "财报内容")
	writeVaultFile(t, root, "notes.md", "笔记")
	writeVaultFile(t, root, ".hidden.md", "隐藏文件")
	writeVaultFile(t, root, ".aranea/vault.json", `{"x":1}`)
	writeVaultFile(t, root, ".aranea/trash/old.md", "回收站内容")
	writeVaultFile(t, root, "image.png", "二进制")
	writeVaultFile(t, root, "doc.txt", "非 md 文本")

	snaps, err := newTestScanner().Scan(root, nil)
	require.NoError(t, err)

	paths := make([]string, 0, len(snaps))
	for _, s := range snaps {
		paths = append(paths, s.RelPath)
	}
	assert.ElementsMatch(t, []string{"财报/q2.md", "notes.md"}, paths,
		"只跟踪 .md；忽略 .aranea/隐藏文件/非 md")
	for _, s := range snaps {
		assert.NotEmpty(t, s.Hash)
		assert.False(t, s.ModTime.IsZero())
		assert.Greater(t, s.Size, int64(0))
	}
}

func TestSyncScannerSkipsOversizeFiles(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "big.md", " oversized ")
	sc := newTestScanner()
	sc.maxBytes = 4 // 测试用小上限
	snaps, err := sc.Scan(root, nil)
	require.NoError(t, err)
	assert.Empty(t, snaps, "超过 maxBytes 的文件不入库")
}

func TestSyncScannerReusesHashWhenMtimeUnchanged(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", "内容A")

	sc := newTestScanner()
	hashCalls := 0
	sc.hashFile = func(path string) (string, error) {
		hashCalls++
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return HashContent(string(data)), nil
	}

	first, err := sc.Scan(root, nil)
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, 1, hashCalls)

	// mtime/size 未变 → 复用 prev hash，不重算
	second, err := sc.Scan(root, first)
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Equal(t, 1, hashCalls, "mtime 未变不应重新 hash")
	assert.Equal(t, first[0].Hash, second[0].Hash)
}

func TestDiffSnapshotsCreatedModifiedDeleted(t *testing.T) {
	prev := []FileSnapshot{
		{RelPath: "keep.md", Hash: "sha1:k", Size: 1, ModTime: time.Now()},
		{RelPath: "mod.md", Hash: "sha1:old", Size: 1, ModTime: time.Now()},
		{RelPath: "del.md", Hash: "sha1:d", Size: 1, ModTime: time.Now()},
	}
	curr := []FileSnapshot{
		{RelPath: "keep.md", Hash: "sha1:k", Size: 1, ModTime: time.Now()},
		{RelPath: "mod.md", Hash: "sha1:new", Size: 2, ModTime: time.Now()},
		{RelPath: "new.md", Hash: "sha1:n", Size: 3, ModTime: time.Now()},
	}

	events := DiffSnapshots(prev, curr)

	byType := map[ChangeType][]ChangeEvent{}
	for _, e := range events {
		byType[e.Type] = append(byType[e.Type], e)
	}
	require.Len(t, byType[ChangeCreated], 1)
	assert.Equal(t, "new.md", byType[ChangeCreated][0].RelPath)
	require.Len(t, byType[ChangeModified], 1)
	assert.Equal(t, "mod.md", byType[ChangeModified][0].RelPath)
	require.Len(t, byType[ChangeDeleted], 1)
	assert.Equal(t, "del.md", byType[ChangeDeleted][0].RelPath)
	assert.Empty(t, byType[ChangeMoved])
}

func TestDiffSnapshotsDetectsMove(t *testing.T) {
	prev := []FileSnapshot{
		{RelPath: "财报/q1.md", Hash: "sha1:same", Size: 10, ModTime: time.Now()},
	}
	curr := []FileSnapshot{
		{RelPath: "archive/2025/q1.md", Hash: "sha1:same", Size: 10, ModTime: time.Now()},
	}

	events := DiffSnapshots(prev, curr)

	require.Len(t, events, 1)
	assert.Equal(t, ChangeMoved, events[0].Type)
	assert.Equal(t, "财报/q1.md", events[0].OldRelPath)
	assert.Equal(t, "archive/2025/q1.md", events[0].RelPath)
}

func TestDiffSnapshotsMovePlusModifyIsDeleteAndCreate(t *testing.T) {
	// 移动且内容也变了 → 不判为移动，按 删除+新增 处理（保守）
	prev := []FileSnapshot{
		{RelPath: "a.md", Hash: "sha1:old", Size: 1, ModTime: time.Now()},
	}
	curr := []FileSnapshot{
		{RelPath: "b.md", Hash: "sha1:new", Size: 2, ModTime: time.Now()},
	}

	events := DiffSnapshots(prev, curr)
	byType := map[ChangeType][]ChangeEvent{}
	for _, e := range events {
		byType[e.Type] = append(byType[e.Type], e)
	}
	assert.Empty(t, byType[ChangeMoved])
	require.Len(t, byType[ChangeDeleted], 1)
	require.Len(t, byType[ChangeCreated], 1)
}

func TestSyncEngineEndToEndScanDiff(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", "内容A")
	writeVaultFile(t, root, "dir/b.md", "内容B")

	sc := newTestScanner()
	prev, err := sc.Scan(root, nil)
	require.NoError(t, err)
	require.Len(t, prev, 2)

	// 模拟外部变更：改 a、删 b、新增 c、把 a 复制到 dir/a-copy.md（不同 hash 不判移动）
	writeVaultFile(t, root, "a.md", "内容A-改")
	writeVaultFile(t, root, "c.md", "内容C")
	require.NoError(t, os.Remove(filepath.Join(root, "dir", "b.md")))

	curr, err := sc.Scan(root, prev)
	require.NoError(t, err)
	events := DiffSnapshots(prev, curr)

	byType := map[ChangeType][]ChangeEvent{}
	for _, e := range events {
		byType[e.Type] = append(byType[e.Type], e)
	}
	require.Len(t, byType[ChangeModified], 1)
	assert.Equal(t, "a.md", byType[ChangeModified][0].RelPath)
	require.Len(t, byType[ChangeDeleted], 1)
	assert.Equal(t, "dir/b.md", byType[ChangeDeleted][0].RelPath)
	require.Len(t, byType[ChangeCreated], 1)
	assert.Equal(t, "c.md", byType[ChangeCreated][0].RelPath)
}
