package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aranea-agents/pkg/apierror"
	loggateway "aranea-agents/pkg/loggateway"
)

func newTestFiler() *VaultFiler {
	return NewVaultFiler(loggateway.NewNoop())
}

func TestSanitizeRelPath(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "普通相对路径", in: "财报/2026Q2.md", want: "财报/2026Q2.md"},
		{name: "反斜杠归一", in: `财报\2026\q2.md`, want: "财报/2026/q2.md"},
		{name: "前导点斜杠", in: "./x.md", want: "x.md"},
		{name: "空路径", in: "", wantErr: true},
		{name: "空白路径", in: "   ", wantErr: true},
		{name: "父目录穿越", in: "../x.md", wantErr: true},
		{name: "中间父目录穿越", in: "a/../../b.md", wantErr: true},
		{name: "绝对路径", in: "/etc/passwd", wantErr: true},
		{name: "Windows 盘符绝对路径", in: `C:\Windows\system32\x.md`, wantErr: true},
		{name: "Windows 盘符正斜杠", in: "C:/Windows/x.md", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SanitizeRelPath(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestVaultFilerWriteReadRoundTrip(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	created := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	doc := &VaultDoc{
		Frontmatter: DocFrontmatter{
			ID:          "doc_01J8TEST",
			Title:       "2026 Q2 财报分析",
			Tags:        []string{"财报", "季度"},
			Type:        "report",
			Summary:     "本文分析 2026 Q2 营收结构。",
			SummaryHash: "sha1:ab12cd",
			Source:      "财报Q2.pdf",
			Created:     created,
		},
		Extra: map[string]any{
			"rating":  5,
			"authors": []any{"张三", "李四"},
		},
		Body: "# 2026 Q2 财报分析\n\n正文…… 相关：[[预算制度]]\n",
	}

	require.NoError(t, f.WriteDoc(root, "财报/2026Q2.md", doc))

	got, err := f.ReadDoc(root, "财报/2026Q2.md")
	require.NoError(t, err)

	assert.Equal(t, doc.Frontmatter.ID, got.Frontmatter.ID)
	assert.Equal(t, doc.Frontmatter.Title, got.Frontmatter.Title)
	assert.Equal(t, doc.Frontmatter.Tags, got.Frontmatter.Tags)
	assert.Equal(t, doc.Frontmatter.Type, got.Frontmatter.Type)
	assert.Equal(t, doc.Frontmatter.Summary, got.Frontmatter.Summary)
	assert.Equal(t, doc.Frontmatter.SummaryHash, got.Frontmatter.SummaryHash)
	assert.Equal(t, doc.Frontmatter.Source, got.Frontmatter.Source)
	assert.True(t, created.Equal(got.Frontmatter.Created), "created 应保留")
	// R-1：用户自定义字段原样保留
	assert.Equal(t, 5, got.Extra["rating"])
	assert.Equal(t, []any{"张三", "李四"}, got.Extra["authors"])
	// 正文原样保留
	assert.Equal(t, doc.Body, got.Body)
}

func TestVaultFilerUserFieldCannotOverrideManaged(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	doc := &VaultDoc{
		Frontmatter: DocFrontmatter{ID: "doc_managed", Summary: "系统摘要"},
		Extra:       map[string]any{"summary": "用户试图覆盖", "custom": "保留"},
		Body:        "正文",
	}
	require.NoError(t, f.WriteDoc(root, "a.md", doc))

	got, err := f.ReadDoc(root, "a.md")
	require.NoError(t, err)
	// R-1：受管字段以 KB 值为准，用户同名字段不覆盖
	assert.Equal(t, "系统摘要", got.Frontmatter.Summary)
	assert.Equal(t, "保留", got.Extra["custom"])
}

func TestVaultFilerWriteCreatesDirsAndIsAtomic(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	doc := &VaultDoc{Frontmatter: DocFrontmatter{ID: "doc_x"}, Body: "内容"}
	require.NoError(t, f.WriteDoc(root, "deep/nested/dir/x.md", doc))

	// 原子写入：目录中不残留临时文件
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() {
			assert.False(t, strings.HasPrefix(d.Name(), ".aranea-tmp-"), "不应残留临时文件: %s", d.Name())
		}
		return nil
	})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(root, "deep", "nested", "dir", "x.md"))
	require.NoError(t, err)
}

func TestVaultFilerOverwriteBacksUpToTrash(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, f.WriteDoc(root, "a.md", &VaultDoc{Body: "旧版本内容"}))
	require.NoError(t, f.WriteDoc(root, "a.md", &VaultDoc{Body: "新版本内容"}))

	// R-1/R-6：覆盖前旧版本应备份到 .aranea/trash
	trashDir := filepath.Join(root, ".aranea", "trash")
	entries, err := os.ReadDir(trashDir)
	require.NoError(t, err, "覆盖后应存在 trash 目录")
	require.Len(t, entries, 1)

	backup, err := os.ReadFile(filepath.Join(trashDir, entries[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(backup), "旧版本内容")

	// 当前文件是新版本
	cur, err := os.ReadFile(filepath.Join(root, "a.md"))
	require.NoError(t, err)
	assert.Contains(t, string(cur), "新版本内容")
}

func TestVaultFilerReadPlainMarkdownWithoutFrontmatter(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	raw := "# 没有 frontmatter\n\n用户手写的普通 markdown。\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "plain.md"), []byte(raw), 0o644))

	got, err := f.ReadDoc(root, "plain.md")
	require.NoError(t, err)
	assert.Equal(t, raw, got.Body)
	assert.Empty(t, got.Frontmatter.ID)
	assert.Empty(t, got.Extra)
}

func TestVaultFilerMoveToTrash(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, f.WriteDoc(root, "sub/del.md", &VaultDoc{Body: "待删除"}))

	trashPath, err := f.MoveToTrash(root, "sub/del.md")
	require.NoError(t, err)

	// 原文件不存在
	_, err = os.Stat(filepath.Join(root, "sub", "del.md"))
	assert.True(t, os.IsNotExist(err))
	// trash 中存在且内容一致
	data, err := os.ReadFile(trashPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "待删除")

	// 同名冲突去重：再次移入同名文件应产生不同 trash 路径
	require.NoError(t, f.WriteDoc(root, "sub/del.md", &VaultDoc{Body: "第二轮"}))
	trashPath2, err := f.MoveToTrash(root, "sub/del.md")
	require.NoError(t, err)
	assert.NotEqual(t, trashPath, trashPath2)
}

func TestVaultFilerRejectsTraversalOnAllOps(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.Error(t, f.WriteDoc(root, "../escape.md", &VaultDoc{Body: "x"}))
	_, err := f.ReadDoc(root, "../../etc/passwd")
	require.Error(t, err)
	_, err = f.MoveToTrash(root, "../outside.md")
	require.Error(t, err)
}

func TestVaultFilerSelfWriteMarking(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	// WriteDoc 打标：内容 hash + 非删除标记
	require.NoError(t, f.WriteDoc(root, "a.md", &VaultDoc{Body: "v1"}))
	hash, deleted, ok := f.ConsumeSelfWrite("a.md")
	assert.True(t, ok, "WriteDoc 后应有自写标记")
	assert.False(t, deleted)
	assert.NotEmpty(t, hash)

	// 消费语义：一次性
	_, _, ok = f.ConsumeSelfWrite("a.md")
	assert.False(t, ok, "标记消费后即删（防 map 无界增长）")

	// 未打标路径
	_, _, ok = f.ConsumeSelfWrite("never-written.md")
	assert.False(t, ok)
}

func TestVaultFilerMoveToTrashMarksDeleted(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, f.WriteDoc(root, "sub/del.md", &VaultDoc{Body: "待删除"}))
	_, _, _ = f.ConsumeSelfWrite("sub/del.md") // 清掉 WriteDoc 标记

	_, err := f.MoveToTrash(root, "sub/del.md")
	require.NoError(t, err)
	_, deleted, ok := f.ConsumeSelfWrite("sub/del.md")
	assert.True(t, ok, "MoveToTrash 后应有自写标记")
	assert.True(t, deleted, "MoveToTrash 标记必须为 deleted=true（与写入区分）")
}

// ── G3-B4：Move（库内跨目录移动） ───────────────────────────────────────────

func TestVaultFilerMove_Basic(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte("# A\n"), 0o644))

	finalRel, err := f.Move(root, "notes/a.md", "archive/a.md", "")
	require.NoError(t, err)
	assert.Equal(t, "archive/a.md", finalRel)

	// 源消失、目标存在且内容一致；目标父目录自动创建
	_, err = os.Stat(filepath.Join(root, "notes", "a.md"))
	assert.True(t, os.IsNotExist(err), "源文件必须消失")
	data, err := os.ReadFile(filepath.Join(root, "archive", "a.md"))
	require.NoError(t, err)
	assert.Equal(t, "# A\n", string(data))

	// 自写标记：源 deleted=true，目标写入标记（hash 为内容 hash）
	_, srcDeleted, ok := f.ConsumeSelfWrite("notes/a.md")
	assert.True(t, ok, "源路径必须有自写标记")
	assert.True(t, srcDeleted, "源路径标记必须为 deleted")
	dstHash, dstDeleted, ok := f.ConsumeSelfWrite("archive/a.md")
	assert.True(t, ok, "目标路径必须有自写标记")
	assert.False(t, dstDeleted, "目标路径标记必须为写入")
	assert.Equal(t, HashContent("# A\n"), dstHash)
}

func TestVaultFilerMove_ConflictDefault(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "archive"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte("mine"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "archive", "a.md"), []byte("existing"), 0o644))

	_, err := f.Move(root, "notes/a.md", "archive/a.md", "")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeConflict), "默认策略同名必须 CodeConflict")

	// 双份均保持原样（前端弹 覆盖/改名/取消 再决策）
	data, _ := os.ReadFile(filepath.Join(root, "notes", "a.md"))
	assert.Equal(t, "mine", string(data))
	data, _ = os.ReadFile(filepath.Join(root, "archive", "a.md"))
	assert.Equal(t, "existing", string(data))
}

func TestVaultFilerMove_ConflictOverwrite(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "archive"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte("mine"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "archive", "a.md"), []byte("existing"), 0o644))

	finalRel, err := f.Move(root, "notes/a.md", "archive/a.md", "overwrite")
	require.NoError(t, err)
	assert.Equal(t, "archive/a.md", finalRel)

	// 目标被覆盖为源内容；目标旧版本抢救进 trash（R-6 不丢数据）
	data, _ := os.ReadFile(filepath.Join(root, "archive", "a.md"))
	assert.Equal(t, "mine", string(data))
	trashEntries, err := os.ReadDir(filepath.Join(root, ".aranea", "trash", "archive"))
	require.NoError(t, err, "目标旧版本必须进 trash")
	assert.NotEmpty(t, trashEntries)
}

func TestVaultFilerMove_ConflictRename(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "archive"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte("mine"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "archive", "a.md"), []byte("existing"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "archive", "a (2).md"), []byte("taken"), 0o644))

	finalRel, err := f.Move(root, "notes/a.md", "archive/a.md", "rename")
	require.NoError(t, err)
	assert.Equal(t, "archive/a (3).md", finalRel, "a.md 与 a (2).md 均被占时必须递增到 a (3).md")

	// 三份都在（保留两份语义）
	data, _ := os.ReadFile(filepath.Join(root, "archive", "a.md"))
	assert.Equal(t, "existing", string(data))
	data, _ = os.ReadFile(filepath.Join(root, "archive", "a (3).md"))
	assert.Equal(t, "mine", string(data))
	_, err = os.Stat(filepath.Join(root, "notes", "a.md"))
	assert.True(t, os.IsNotExist(err))
}

func TestVaultFilerMove_Rejects(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte("x"), 0o644))

	// 源/目标路径校验
	_, err := f.Move(root, "../escape.md", "b.md", "")
	require.Error(t, err)
	_, err = f.Move(root, "notes/a.md", ".aranea/b.md", "")
	require.Error(t, err)
	_, err = f.Move(root, "notes/a.md", "/abs/b.md", "")
	require.Error(t, err)
	// 源不存在 → NotFound
	_, err = f.Move(root, "notes/ghost.md", "b.md", "")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeNotFound))
	// 源是目录 → BadRequest
	_, err = f.Move(root, "notes", "archive", "")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeBadRequest))
	// 未知策略 → BadRequest（目标存在时才校验策略）
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.md"), []byte("y"), 0o644))
	_, err = f.Move(root, "notes/a.md", "b.md", "bogus")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeBadRequest))
}

func TestVaultFilerWriteTrashFromMirror(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	doc := &VaultDoc{
		Frontmatter: DocFrontmatter{Summary: "镜像摘要", Tags: []string{"t1"}, Type: "note"},
		Body:        "镜像正文内容",
	}
	trashPath, err := f.WriteTrashFromMirror(root, "dir/gone.md", doc)
	require.NoError(t, err)

	// 写入 trash 而非 vault 原路径
	assert.Contains(t, trashPath, filepath.Join(".aranea", "trash"))
	_, err = os.Stat(filepath.Join(root, "dir", "gone.md"))
	assert.True(t, os.IsNotExist(err), "原路径不得重建")

	// 内容可读回（frontmatter + body）
	data, err := os.ReadFile(trashPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "镜像摘要")
	assert.Contains(t, string(data), "镜像正文内容")

	// 不产生自写标记（trash 在 vault 事件域之外）
	_, _, ok := f.ConsumeSelfWrite("dir/gone.md")
	assert.False(t, ok)

	// 同名去重：再次抢救同路径产生不同 trash 文件
	trashPath2, err := f.WriteTrashFromMirror(root, "dir/gone.md", doc)
	require.NoError(t, err)
	assert.NotEqual(t, trashPath, trashPath2)
}

func TestSummaryStale(t *testing.T) {
	body := "正文内容"
	assert.True(t, SummaryStale(body, ""), "空 summary_hash 视为 stale（从未生成）")
	assert.True(t, SummaryStale(body, "sha1:other"), "hash 不匹配视为 stale")
	assert.False(t, SummaryStale(body, HashContent(body)), "hash 匹配不 stale")
	// 关键不变量：摘要对象仅 Body——frontmatter 变更（含 KB 写回摘要）不使摘要过期，
	// 避免「写回摘要 → 整文件 hash 变 → stale → 再生成」无限循环。
	assert.False(t, SummaryStale(body, HashContent(body)), "摘要写回后 body 不变则不 stale")
}

func TestVaultFilerReadDocWithHash(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	raw := "# 标题\n\n正文内容\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "h.md"), []byte(raw), 0o644))

	doc, hash, err := f.ReadDocWithHash(root, "h.md")
	require.NoError(t, err)
	assert.Equal(t, raw, doc.Body)
	assert.Equal(t, HashContent(raw), hash)
}

func TestVaultFilerWriteDocCASNoConflict(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, f.WriteDoc(root, "a.md", &VaultDoc{Body: "v1"}))
	_, hash1, err := f.ReadDocWithHash(root, "a.md")
	require.NoError(t, err)

	conflict, err := f.WriteDocCAS(root, "a.md", &VaultDoc{Body: "v2"}, hash1)
	require.NoError(t, err)
	assert.False(t, conflict, "hash 匹配不应标记冲突")

	cur, err := os.ReadFile(filepath.Join(root, "a.md"))
	require.NoError(t, err)
	assert.Contains(t, string(cur), "v2")

	// 覆盖即备份契约不变（R-6）：v1 进 trash
	trashDir := filepath.Join(root, ".aranea", "trash")
	entries, err := os.ReadDir(trashDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	backup, err := os.ReadFile(filepath.Join(trashDir, entries[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(backup), "v1")
}

func TestVaultFilerWriteDocCASConflictKeepsBothCopies(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, f.WriteDoc(root, "a.md", &VaultDoc{Body: "KB v1"}))
	_, hash1, err := f.ReadDocWithHash(root, "a.md")
	require.NoError(t, err)

	// 用户外部编辑（绕过 filer，模拟 KB 读取后文件被改）
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("用户 v2"), 0o644))

	conflict, err := f.WriteDocCAS(root, "a.md", &VaultDoc{Body: "KB v3"}, hash1)
	require.NoError(t, err)
	assert.True(t, conflict, "hash 不匹配必须标记冲突（R-1 写入前重读 hash）")

	// 冲突留双份：路径上是 KB v3，trash 里是用户 v2
	cur, err := os.ReadFile(filepath.Join(root, "a.md"))
	require.NoError(t, err)
	assert.Contains(t, string(cur), "KB v3")

	trashDir := filepath.Join(root, ".aranea", "trash")
	entries, err := os.ReadDir(trashDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	backup, err := os.ReadFile(filepath.Join(trashDir, entries[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(backup), "用户 v2")
}

func TestVaultFilerWriteDocCASNewFileAndExistsConflict(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	// 期望不存在 + 实际不存在 → 正常创建
	conflict, err := f.WriteDocCAS(root, "new.md", &VaultDoc{Body: "新建"}, "")
	require.NoError(t, err)
	assert.False(t, conflict)

	// 期望不存在 + 实际已存在 → 冲突（防并发创建覆盖用户同名文件），备份并写入
	conflict, err = f.WriteDocCAS(root, "new.md", &VaultDoc{Body: "并发创建"}, "")
	require.NoError(t, err)
	assert.True(t, conflict)
	cur, err := os.ReadFile(filepath.Join(root, "new.md"))
	require.NoError(t, err)
	assert.Contains(t, string(cur), "并发创建")

	trashDir := filepath.Join(root, ".aranea", "trash")
	entries, err := os.ReadDir(trashDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	backup, err := os.ReadFile(filepath.Join(trashDir, entries[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(backup), "新建")
}

func TestVaultFilerWriteDocCASDisappearedFile(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, f.WriteDoc(root, "gone.md", &VaultDoc{Body: "v1"}))
	_, hash1, err := f.ReadDocWithHash(root, "gone.md")
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(root, "gone.md")))

	// 期望存在的文件已消失：标记冲突，仍写入（保守：不丢 KB 产出）
	conflict, err := f.WriteDocCAS(root, "gone.md", &VaultDoc{Body: "v2"}, hash1)
	require.NoError(t, err)
	assert.True(t, conflict)
	cur, err := os.ReadFile(filepath.Join(root, "gone.md"))
	require.NoError(t, err)
	assert.Contains(t, string(cur), "v2")
}

// ── G1-B1：ListSubdirs 目录扫描（树节点目录来源）──────────────────────────────

func TestVaultFilerListSubdirs(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "beta", "gamma"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "alpha"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "top.md"), []byte("x"), 0o644))

	got, err := f.ListSubdirs(root, "")
	require.NoError(t, err)
	require.Len(t, got, 2, "只含目录：文件与点开头目录被排除")
	assert.Equal(t, "alpha", got[0].Name, "按名称排序")
	assert.Equal(t, "beta", got[1].Name)
	assert.False(t, got[0].ModTime.IsZero(), "目录必须带 mtime")

	// 嵌套前缀
	got, err = f.ListSubdirs(root, "beta")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "gamma", got[0].Name)
}

func TestVaultFilerListSubdirsMissingPrefixReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	got, err := f.ListSubdirs(root, "ghost")
	require.NoError(t, err, "前缀目录不存在不报错（外部删除竞态）")
	assert.Empty(t, got)
}

func TestVaultFilerListSubdirsRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	_, err := f.ListSubdirs(root, "../escape")
	require.Error(t, err)
	_, err = f.ListSubdirs(root, ".aranea")
	require.Error(t, err, ".aranea 为保留目录")
}

// ── G1-B2：Mkdir / SnapshotDoc ────────────────────────────────────────────────

func TestVaultFilerMkdir(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, f.Mkdir(root, "a/b/c"))
	info, err := os.Stat(filepath.Join(root, "a", "b", "c"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	require.NoError(t, f.Mkdir(root, "a/b/c"), "幂等")
	require.Error(t, f.Mkdir(root, "../escape"))
	require.Error(t, f.Mkdir(root, ".aranea/meta"))
}

func TestVaultFilerSnapshotDoc(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	content := "# 标题\n\n正文\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte(content), 0o644))

	snap, err := f.SnapshotDoc(root, "a.md")
	require.NoError(t, err)
	assert.Equal(t, "a.md", snap.RelPath)
	assert.Equal(t, int64(len(content)), snap.Size)
	assert.Equal(t, HashContent(content), snap.Hash)
	assert.False(t, snap.ModTime.IsZero())

	_, err = f.SnapshotDoc(root, "ghost.md")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeNotFound), "不存在必须 CodeNotFound（create 冲突判定依赖）")

	_, err = f.SnapshotDoc(root, "../escape.md")
	require.Error(t, err)
}

// ── G1-B3：WriteRaw（上传落盘，任意字节） ─────────────────────────────────────

func TestVaultFilerWriteRaw(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	raw := []byte{0x25, 0x50, 0x44, 0x46, 0x00, 0x01} // %PDF 二进制
	require.NoError(t, f.WriteRaw(root, "reports/q1/result.pdf", raw))

	got, err := os.ReadFile(filepath.Join(root, "reports", "q1", "result.pdf"))
	require.NoError(t, err, "嵌套父目录必须自动创建")
	assert.Equal(t, raw, got, "字节必须原样落盘（不经过 frontmatter 编组）")

	// 自写标记：watcher 必须能过滤（防回环）
	hash, deleted, ok := f.ConsumeSelfWrite("reports/q1/result.pdf")
	assert.True(t, ok, "写入必须登记自写标记")
	assert.False(t, deleted)
	assert.Equal(t, HashContent(string(raw)), hash)
}

func TestVaultFilerWriteRaw_ConflictKeepsOriginal(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	original := []byte("original")
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), original, 0o644))

	err := f.WriteRaw(root, "a.md", []byte("overwrite"))
	require.Error(t, err, "已存在必须报错（create 语义）")
	assert.True(t, apierror.IsCode(err, apierror.CodeConflict))

	got, _ := os.ReadFile(filepath.Join(root, "a.md"))
	assert.Equal(t, original, got, "原文件必须保持原样")
}

func TestVaultFilerWriteRaw_Rejects(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.Error(t, f.WriteRaw(root, "../escape.bin", []byte("x")), "穿越拒绝")
	require.Error(t, f.WriteRaw(root, ".aranea/meta.bin", []byte("x")), "点目录拒绝")
	require.Error(t, f.WriteRaw(root, "dir/", []byte("x")), "目录路径拒绝")
}

func TestVaultFilerRemoveDoc(t *testing.T) {
	root := t.TempDir()
	f := newTestFiler()

	require.NoError(t, f.WriteRaw(root, "up/a.bin", []byte("x")))
	require.NoError(t, f.RemoveDoc(root, "up/a.bin"))

	_, err := os.Stat(filepath.Join(root, "up", "a.bin"))
	assert.True(t, os.IsNotExist(err), "文件必须被移除")

	// 幂等：不存在不报错（补偿路径允许重入）
	require.NoError(t, f.RemoveDoc(root, "up/a.bin"))
	require.Error(t, f.RemoveDoc(root, "../escape"))
}
