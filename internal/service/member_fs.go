package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/skill/storage"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// M71: memberfs 的 service 层实现
//   - MemberDirResolver：agentKey → 工作目录（复刻 resolveAgentFilesystemDir 布局）
//   - MemberFileReader：只读文件操作（secureJoin + 二进制嗅探 + 截断）
// 设计：docs/development/71-agent-resource-sharing.design.md §7
// ---------------------------------------------------------------------------

// memberDirResolver implements biz.MemberDirResolver.
type memberDirResolver struct {
	sys biz.SystemSettingRepo
}

var _ biz.MemberDirResolver = (*memberDirResolver)(nil)

// NewMemberDirResolver creates the resolver.
func NewMemberDirResolver(sys biz.SystemSettingRepo) biz.MemberDirResolver {
	return &memberDirResolver{sys: sys}
}

// ResolveDir resolves {base}/workspace/{workspaceID}/{agentKey} without
// creating the directory (read-only access; missing dir is an error).
func (r *memberDirResolver) ResolveDir(ctx context.Context, agentKey string) (string, error) {
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return "", fmt.Errorf("agent key is empty")
	}
	base := memberWorkspaceBase(ctx, r.sys)
	wsID := workspace.IDFromContext(ctx)
	if strings.TrimSpace(wsID) == "" {
		wsID = workspace.DefaultWorkspaceID
	}
	dir := filepath.Join(base, "workspace", wsID, agentKey)
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("workspace dir not found for agent %s", agentKey)
	}
	return dir, nil
}

// memberWorkspaceBase mirrors toolWorkspaceBase (agent/tool_assembly.go):
// SystemSetting.RootDirectory → ARANEA_WORKSPACE_ROOT → WORKSPACE_ROOT → ".".
func memberWorkspaceBase(ctx context.Context, sys biz.SystemSettingRepo) string {
	base := "."
	if sys != nil {
		if st, err := sys.Get(ctx); err == nil && strings.TrimSpace(st.RootDirectory) != "" {
			base = storage.Absolute(st.RootDirectory)
		}
	}
	if v := strings.TrimSpace(os.Getenv("ARANEA_WORKSPACE_ROOT")); v != "" {
		return storage.Absolute(v)
	}
	if v := strings.TrimSpace(os.Getenv("WORKSPACE_ROOT")); v != "" {
		return storage.Absolute(v)
	}
	return storage.Absolute(base)
}

// teamInboxFS copies declared Bulk files into downstream agent workspaces.
type teamInboxFS struct {
	sys biz.SystemSettingRepo
}

var _ biz.TeamInboxFS = (*teamInboxFS)(nil)

// NewTeamInboxFS creates the inbox materializer (M78 ORGFAST-17).
func NewTeamInboxFS(sys biz.SystemSettingRepo) biz.TeamInboxFS {
	return &teamInboxFS{sys: sys}
}

func (f *teamInboxFS) agentDir(ctx context.Context, agentKey string) string {
	base := memberWorkspaceBase(ctx, f.sys)
	wsID := workspace.IDFromContext(ctx)
	if strings.TrimSpace(wsID) == "" {
		wsID = workspace.DefaultWorkspaceID
	}
	return filepath.Join(base, "workspace", wsID, strings.TrimSpace(agentKey))
}

func (f *teamInboxFS) MaterializeFile(ctx context.Context, spec biz.InboxCopySpec) error {
	rel := filepath.FromSlash(strings.TrimSpace(spec.RelPath))
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") {
		return fmt.Errorf("invalid rel_path")
	}
	destName := filepath.Base(strings.TrimSpace(spec.DestName))
	if destName == "" || destName == "." || destName == string(filepath.Separator) {
		return fmt.Errorf("invalid dest name")
	}
	var data []byte
	found := false
	for _, src := range spec.SrcAgentKeys {
		p := filepath.Join(f.agentDir(ctx, src), rel)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		data, found = b, true
		break
	}
	if !found {
		return fmt.Errorf("source file not found: %s", spec.RelPath)
	}
	upID := strings.TrimSpace(spec.UpstreamTeamID)
	if upID == "" {
		return fmt.Errorf("upstream team id is empty")
	}
	for _, dest := range spec.DestAgentKeys {
		dir := filepath.Join(f.agentDir(ctx, dest), "inbox", upID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, destName), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------

// memberFileReader implements biz.MemberFileReader (read-only).
type memberFileReader struct {
	lg loggateway.Logger
}

var _ biz.MemberFileReader = (*memberFileReader)(nil)

// NewMemberFileReader creates the reader.
func NewMemberFileReader(lg loggateway.Logger) biz.MemberFileReader {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &memberFileReader{lg: lg.With(loggateway.Domain("member_fs"))}
}

// maxListEntries caps the total entries returned by one List call.
const maxListEntries = 2000

// List returns the directory tree under root/subdir up to depth levels.
// Paths are slash-separated and relative to root.
func (r *memberFileReader) List(root, subdir string, depth int) ([]biz.FileEntry, error) {
	start, err := secureJoin(root, subdir)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(start)
	if err != nil {
		return nil, fmt.Errorf("path not found: %s", subdir)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", subdir)
	}
	var entries []biz.FileEntry
	walkErr := r.listWalk(root, start, depth, &entries)
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func (r *memberFileReader) listWalk(root, dir string, depth int, entries *[]biz.FileEntry) error {
	if depth < 0 || len(*entries) >= maxListEntries {
		return nil
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, item := range items {
		if len(*entries) >= maxListEntries {
			return nil
		}
		name := item.Name()
		// 跳过隐藏文件与符号链接（防逃逸 + 降噪）。
		if strings.HasPrefix(name, ".") || item.Type()&os.ModeSymlink != 0 {
			continue
		}
		full := filepath.Join(dir, name)
		rel, err := filepath.Rel(root, full)
		if err != nil {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		*entries = append(*entries, biz.FileEntry{
			Path:  filepath.ToSlash(rel),
			IsDir: item.IsDir(),
			Size:  info.Size(),
		})
		if item.IsDir() && depth > 0 {
			if err := r.listWalk(root, full, depth-1, entries); err != nil {
				return err
			}
		}
	}
	return nil
}

// ReadText reads a UTF-8 text file under root, rejecting binary content and
// truncating at maxBytes. Returns (content, truncated, err).
func (r *memberFileReader) ReadText(root, rel string, maxBytes int64) (string, bool, error) {
	full, err := secureJoin(root, rel)
	if err != nil {
		return "", false, err
	}
	st, err := os.Stat(full)
	if err != nil {
		return "", false, fmt.Errorf("file not found: %s", rel)
	}
	if st.IsDir() {
		return "", false, fmt.Errorf("not a file: %s", rel)
	}
	f, err := os.Open(full)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = f.Close() }()

	// 读 maxBytes+1 以判定截断。
	buf, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", false, err
	}
	truncated := int64(len(buf)) > maxBytes
	if truncated {
		buf = buf[:maxBytes]
		// 截断点可能落在多字节 UTF-8 字符中间；最多回退 3 字节到完整字符
		// 边界，否则下方 utf8.Valid 会把正常文本误判为非法 UTF-8 文件。
		// 上限 3 字节保证真正的非法文件（长串非法尾字节）仍被 utf8.Valid 拒绝。
		for i := 0; i < utf8.UTFMax-1 && len(buf) > 0; i++ {
			if r, size := utf8.DecodeLastRune(buf); r == utf8.RuneError && size <= 1 {
				buf = buf[:len(buf)-1]
			} else {
				break
			}
		}
	}
	if isBinaryContent(buf) {
		return "", false, fmt.Errorf("binary file rejected: %s", rel)
	}
	if !utf8.Valid(buf) {
		return "", false, fmt.Errorf("non-UTF-8 file rejected: %s", rel)
	}
	return string(buf), truncated, nil
}

// Search finds files whose root-relative path matches the glob pattern
// (matched against both the base name and the full relative path).
func (r *memberFileReader) Search(root, pattern string, limit int) ([]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, fmt.Errorf("pattern is empty")
	}
	var matches []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if len(matches) >= limit {
			return filepath.SkipAll
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if ok, _ := filepath.Match(pattern, name); ok {
			matches = append(matches, rel)
			return nil
		}
		if ok, _ := filepath.Match(pattern, rel); ok {
			matches = append(matches, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// secureJoin joins root+rel and guarantees the result stays inside root after
// Abs+EvalSymlinks. Rejects absolute rel, ".." escape and symlink escape.
func secureJoin(root, rel string) (string, error) {
	root = strings.TrimSpace(root)
	rel = strings.TrimSpace(rel)
	if root == "" {
		return "", fmt.Errorf("root is empty")
	}
	if rel == "" || rel == "." {
		return filepath.Clean(root), nil
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute path rejected: %s", rel)
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	if eval, err := filepath.EvalSymlinks(absRoot); err == nil && eval != "" {
		absRoot = eval
	}
	candidate := filepath.Join(absRoot, rel)
	absCand, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	// 已存在路径做符号链接求值；不存在的路径退化为词法包含检查。
	if eval, err := filepath.EvalSymlinks(absCand); err == nil && eval != "" {
		absCand = eval
	}
	rel2, err := filepath.Rel(absRoot, absCand)
	if err != nil || rel2 == ".." || strings.HasPrefix(rel2, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace root: %s", rel)
	}
	return absCand, nil
}

// isBinaryContent sniffs the leading bytes for binary content (NUL byte or
// http.DetectContentType reporting a non-text type). Callers additionally
// enforce utf8.Valid, so this is a first-line guard only.
func isBinaryContent(buf []byte) bool {
	sample := buf
	if len(sample) > 512 {
		sample = sample[:512]
	}
	for _, b := range sample {
		if b == 0 {
			return true
		}
	}
	ct := http.DetectContentType(sample)
	if strings.HasPrefix(ct, "text/") {
		return false
	}
	switch ct {
	case "application/json", "application/javascript", "application/xml",
		"application/x-javascript", "application/x-sh":
		return false
	}
	// DetectContentType 无法识别时保守放行（utf8.Valid 在调用侧兜底）。
	return ct != "application/octet-stream"
}
