// shared_dir_test.go —— W3 shared_dir walker 钉阵（真 SyncEngine + t.TempDir：
// 出席全列/游标往返/防撞键/隐藏过滤/Fetch 守卫/三态确认/坏配置归仓）。
package sourcesync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
)

func writeSourceFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sharedDirConn(root string) bizsync.Connector {
	return bizsync.Connector{
		ID: "src:conn:sd", Kind: bizsync.KindSharedDir, CollectionID: "col1",
		ConfigJSON: `{"root": ` + jsonQuote(root) + `}`,
	}
}

func jsonQuote(s string) string { return `"` + filepath.ToSlash(s) + `"` }

func collectWalk(t *testing.T, w *SharedDirWalker, conn bizsync.Connector, prev CursorState) ([]Snapshot, CursorState) {
	t.Helper()
	var got []Snapshot
	next, err := w.Walk(context.Background(), conn, prev, func(s Snapshot) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk err = %v", err)
	}
	return got, next
}

// TestSharedDir_WalkEmitsSortedAndCursorRoundTrip 出席全列（sort by rel）+
// 游标编解码往返一致（mtime 预筛同输入同输出）。指纹=内容 sha1。
func TestSharedDir_WalkEmitsSortedAndCursorRoundTrip(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "b.md", "# B\n")
	writeSourceFile(t, root, "a/b.md", "# X\n")
	writeSourceFile(t, root, "a-b.md", "# AB\n")
	w := NewSharedDirWalker(nil, nil)
	conn := sharedDirConn(root)

	got, cur1 := collectWalk(t, w, conn, "")
	wantOrder := []string{"a-b.md", "a/b.md", "b.md"}
	if len(got) != 3 {
		t.Fatalf("emitted = %+v", got)
	}
	for i, s := range got {
		if s.ExternalID != wantOrder[i] {
			t.Fatalf("order[%d] = %q, want %q", i, s.ExternalID, wantOrder[i])
		}
		if s.Fingerprint != bizknowledge.HashContent("# "+map[string]string{"a-b.md": "AB", "a/b.md": "X", "b.md": "B"}[s.ExternalID]+"\n") {
			t.Fatalf("fingerprint 空间不符: %q → %q", s.ExternalID, s.Fingerprint)
		}
	}
	// 二轮：同游标→同输出（content-hash 稳定，出席全列不变）
	got2, cur2 := collectWalk(t, w, conn, cur1)
	if string(cur1) != string(cur2) {
		t.Fatalf("cursor 不幂等:\n%q\n%q", cur1, cur2)
	}
	if len(got2) != 3 {
		t.Fatalf("二轮出席 = %+v", got2)
	}
}

// TestSharedDir_SanitizeNoCollision 防撞键钉（W3 验收钉）：
// a/b.md 与 a-b.md 必须存活为两个独立 external_id（归一处=零映射逐字保留）。
func TestSharedDir_SanitizeNoCollision(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "a/b.md", "# nested\n")
	writeSourceFile(t, root, "a-b.md", "# dashed\n")
	w := NewSharedDirWalker(nil, nil)
	conn := sharedDirConn(root)

	got, _ := collectWalk(t, w, conn, "")
	seen := map[string]bool{}
	for _, s := range got {
		seen[s.ExternalID] = true
		// 零映射守卫：不得出现破坏性归一（斜杠被抹）
		clean, err := sanitizeExternalID(s.ExternalID)
		if err != nil || clean != s.ExternalID {
			t.Fatalf("external_id 应逐字自洽: %q → %q, %v", s.ExternalID, clean, err)
		}
	}
	if !seen["a/b.md"] || !seen["a-b.md"] {
		t.Fatalf("防撞键：两个 external_id 必须并存: %+v", seen)
	}
}

// TestSharedDir_HiddenAndNonIngestibleFiltered 隐藏家族 + 非白名单扩展过滤
// （下沉 SyncEngine 情形的直通钉：.h/、.aranea/、.exe 全不出席）。
func TestSharedDir_HiddenAndNonIngestibleFiltered(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, ".h/secret.md", "x")
	writeSourceFile(t, root, ".aranea/trash/old.md", "x")
	writeSourceFile(t, root, "tool.exe", "MZ")
	writeSourceFile(t, root, "ok.md", "# ok\n")
	w := NewSharedDirWalker(nil, nil)
	conn := sharedDirConn(root)

	got, _ := collectWalk(t, w, conn, "")
	if len(got) != 1 || got[0].ExternalID != "ok.md" {
		t.Fatalf("过滤出席 = %+v", got)
	}
}

// TestSharedDir_FetchAndConfirmDeleted Fetch 守卫（越界拒/缺失错/大小 MIME）+
// 三态确认：缺席=Confirmed（walk-absent 即真相）、存在=StillExists。
func TestSharedDir_FetchAndConfirmDeleted(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "ok.md", "# ok\n")
	w := NewSharedDirWalker(nil, nil)
	conn := sharedDirConn(root)

	p, err := w.Fetch(context.Background(), conn, "ok.md")
	if err != nil {
		t.Fatalf("Fetch err = %v", err)
	}
	_ = p.Body.Close() // 断言前先放句柄（Windows TempDir 清理红区）
	if p.MIME != "text/markdown" || p.SizeBytes != 5 {
		t.Fatalf("payload = %+v", p)
	}
	if _, err := w.Fetch(context.Background(), conn, "gone.md"); err == nil {
		t.Fatalf("缺失文件必定错（fetch_failed 桶凭）")
	}
	if _, err := w.Fetch(context.Background(), conn, "../escape.md"); err == nil {
		t.Fatalf("sanitize 守卫必拒越界")
	}
	if v := w.ConfirmDeleted(context.Background(), conn, "gone.md"); v != bizsync.VerdictConfirmed {
		t.Fatalf("walk-absent=Confirmed, got %d", v)
	}
	if v := w.ConfirmDeleted(context.Background(), conn, "ok.md"); v != bizsync.VerdictStillExists {
		t.Fatalf("存在=StillExists, got %d", v)
	}
}

// TestSharedDir_BadConfigAndCursorFallback 配置缺陷归 walk_failed（游标不动）+
// 坏游标降级全量重扫（不失真、不误删）。
func TestSharedDir_BadConfigAndCursorFallback(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "ok.md", "# ok\n")
	w := NewSharedDirWalker(nil, nil)

	bad := bizsync.Connector{ID: "c", Kind: bizsync.KindSharedDir, ConfigJSON: `{}`}
	if _, err := w.Walk(context.Background(), bad, "cur:prev", func(Snapshot) error { return nil }); err == nil {
		t.Fatalf("缺 root 必 abort")
	}
	notDir := sharedDirConn(filepath.Join(root, "ghost"))
	prevOut, err := w.Walk(context.Background(), notDir, "cur:prev", func(Snapshot) error { return nil })
	if err == nil || prevOut != "cur:prev" {
		t.Fatalf("root 缺席：abort 且游标不动: %v %q", err, prevOut)
	}
	// 坏游标：全量重扫不丢出席
	conn := sharedDirConn(root)
	got, _ := collectWalk(t, w, conn, CursorState("{badjson"))
	if len(got) != 1 {
		t.Fatalf("坏游标降级出席 = %+v", got)
	}
	// 敏感键拒（④ 凭据行表纪律：config_json 永不藏秘密）
	secretCfg := bizsync.Connector{ID: "c", Kind: bizsync.KindSharedDir,
		ConfigJSON: `{"root":"/tmp","access_token":"x"}`}
	if _, err := w.Walk(context.Background(), secretCfg, "", func(Snapshot) error { return nil }); err == nil {
		t.Fatalf("敏感键必拒")
	}
}

// TestSharedDir_SanitizeRejectsBackslash 评审修 #WALKER-2/#W3-F3：反斜杠/NUL
// 先验拒——biz SanitizeRelPath 首步是反斜杠→正斜杠改写而非拒绝，直接代理会让
// items 行坍缩张冠李戴；越界/绝对路径同闸。
func TestSharedDir_SanitizeRejectsBackslash(t *testing.T) {
	for name, bad := range map[string]string{
		"backslash": "docs\\evil.md",
		"nul":       "a\x00.md",
		"absolute":  "/etc/passwd",
		"dotdot":    "../escape.md",
	} {
		if _, err := sanitizeExternalID(bad); err == nil {
			t.Fatalf("%s 必拒: %q", name, bad)
		}
	}
}

// TestSharedDir_SymlinkContained 评审修 #WALKER-1：符号链接/特殊文件围栏——
// Scan 出席谓词（域级 regular-file 闸）不 emit、Fetch 持 Lstat 闸拒读、
// ConfirmDeleted 判 identity 已死=Confirmed（不锁权限平台 t.Skip 优雅降级）。
func TestSharedDir_SymlinkContained(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("平台不给 symlink（Windows 需权限）: %v", err)
	}
	writeSourceFile(t, root, "real.md", "# real\n")
	w := NewSharedDirWalker(nil, nil)
	conn := sharedDirConn(root)

	got, _ := collectWalk(t, w, conn, "")
	if len(got) != 1 || got[0].ExternalID != "real.md" {
		t.Fatalf("symlink 必不出席: %+v", got)
	}
	if _, err := w.Fetch(context.Background(), conn, "link.md"); err == nil {
		t.Fatal("Fetch 必拒 symlink（Lstat 正则文件闸）")
	}
	if v := w.ConfirmDeleted(context.Background(), conn, "link.md"); v != bizsync.VerdictConfirmed {
		t.Fatalf("symlink identity 已死=Confirmed, got %d", v)
	}
}

// TestSharedDir_ConfirmDeletedPredicateReplay 评审修 #W3-F2：StillExists 唯
// 「Scan 会出席」——白名单外扩展/点文件/超 MaxBytes 的对象 identity 已死
// =Confirmed（改名出白名单不 limbo）；坏配置/非目录 root/取消 ctx/守卫拒绝
// =Unknown 保守不删。
func TestSharedDir_ConfirmDeletedPredicateReplay(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "renamed.dat", "x")
	writeSourceFile(t, root, ".hidden.md", "x")
	writeSourceFile(t, root, "ok.md", "# ok\n")
	if err := os.WriteFile(filepath.Join(root, "big.md"), make([]byte, (32<<20)+1), 0o644); err != nil {
		t.Fatal(err)
	}
	w := NewSharedDirWalker(nil, nil)
	conn := sharedDirConn(root)
	ctx := context.Background()

	for name, c := range map[string]struct {
		id   string
		want bizsync.DeletionVerdict
	}{
		"改名出白名单":     {"renamed.dat", bizsync.VerdictConfirmed},
		"点文件":        {".hidden.md", bizsync.VerdictConfirmed},
		"超 MaxBytes": {"big.md", bizsync.VerdictConfirmed},
		"正常出席":       {"ok.md", bizsync.VerdictStillExists},
	} {
		if v := w.ConfirmDeleted(ctx, conn, c.id); v != c.want {
			t.Fatalf("%s: got %d want %d", name, v, c.want)
		}
	}

	// I/O 病/配置缺陷家族 = Unknown（保守不删，digest 不误级联）
	badJSON := bizsync.Connector{ID: "c", Kind: bizsync.KindSharedDir, ConfigJSON: `{"root":`}
	if v := w.ConfirmDeleted(ctx, badJSON, "ok.md"); v != bizsync.VerdictUnknown {
		t.Fatalf("坏配置=Unknown, got %d", v)
	}
	fileRoot := sharedDirConn(filepath.Join(root, "ok.md"))
	if v := w.ConfirmDeleted(ctx, fileRoot, "ok.md"); v != bizsync.VerdictUnknown {
		t.Fatalf("root 非目录=Unknown, got %d", v)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if v := w.ConfirmDeleted(cctx, conn, "ok.md"); v != bizsync.VerdictUnknown {
		t.Fatalf("取消 ctx=Unknown, got %d", v)
	}
	if v := w.ConfirmDeleted(ctx, conn, "../x.md"); v != bizsync.VerdictUnknown {
		t.Fatalf("守卫拒绝=Unknown, got %d", v)
	}
}

// TestSharedDir_MimeTableMirror 评审修 #W3-PIN-7：提交 MIME 表镜像 vault
// mimeTypeFor 静态表——表新增扩展必须双轨（漏同步即红）。
func TestSharedDir_MimeTableMirror(t *testing.T) {
	golden := map[string]string{
		"a.pdf":  "application/pdf",
		"a.doc":  "application/msword",
		"a.docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"a.xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"a.pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"a.png":  "image/png",
		"a.jpg":  "image/jpeg",
		"a.jpeg": "image/jpeg",
		"a.webp": "image/webp",
		"a.md":   "text/markdown",
		"a.txt":  "text/markdown",
		"a.dat":  "text/markdown",
		"A.PDF":  "application/pdf",
	}
	for rel, want := range golden {
		if got := mimeForRelPath(rel); got != want {
			t.Fatalf("mimeForRelPath(%q) = %q want %q", rel, got, want)
		}
	}
}
