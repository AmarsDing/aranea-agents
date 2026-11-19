package fleet

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	gitTimeout       = 2 * time.Minute
	maxDiffFiles     = 400
	maxLinesPerFile  = 4000
	maxUntrackedSize = 256 * 1024
)

// runGit 在 dir 下执行 git；返回 stdout，失败时错误里带 stderr。
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	full := append([]string{"-c", "core.quotepath=false", "-c", "color.ui=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C.UTF-8")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), errors.New(msg)
	}
	return stdout.String(), nil
}

// runGitCombined 返回合并的 stdout+stderr（提交、推送要把钩子输出原样给人看）。
func runGitCombined(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	full := append([]string{"-c", "core.quotepath=false", "-c", "color.ui=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func isGitRepo(ctx context.Context, path string) bool {
	out, err := runGit(ctx, path, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// defaultBranch：优先本地默认分支（不依赖网络），其次 origin/HEAD。
func defaultBranch(ctx context.Context, repo string) string {
	if out, err := runGit(ctx, repo, "symbolic-ref", "--short", "HEAD"); err == nil {
		if b := strings.TrimSpace(out); b != "" {
			return b
		}
	}
	if out, err := runGit(ctx, repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimSpace(out)
	}
	return "HEAD"
}

func listRefs(ctx context.Context, repo string) (local, remote []string, err error) {
	out, err := runGit(ctx, repo, "for-each-ref", "--sort=-committerdate", "--format=%(refname)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, nil, err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "refs/heads/"):
			local = append(local, strings.TrimPrefix(line, "refs/heads/"))
		case strings.HasPrefix(line, "refs/remotes/") && !strings.HasSuffix(line, "/HEAD"):
			remote = append(remote, strings.TrimPrefix(line, "refs/remotes/"))
		}
	}
	return local, remote, nil
}

func branchExists(ctx context.Context, repo, branch string) bool {
	_, err := runGit(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

var slugInvalid = regexp.MustCompile(`[^a-zA-Z0-9._/-]+`)

// slugify 把任务名变成合法分支/目录名；全是中文等非 ASCII 时退化为 task-<时间>。
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugInvalid.ReplaceAllString(s, "-")
	s = strings.Trim(strings.ReplaceAll(s, "--", "-"), "-./")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if s == "" {
		s = "task-" + time.Now().Format("0102-1504")
	}
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-./")
	}
	return s
}

// addWorktree 建 worktree：分支已存在就直接检出，否则从 startFrom 新建。
func addWorktree(ctx context.Context, repo, path, branch, startFrom string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var err error
	if branchExists(ctx, repo, branch) {
		_, err = runGit(ctx, repo, "worktree", "add", path, branch)
	} else {
		args := []string{"worktree", "add", "-b", branch, path}
		if startFrom != "" {
			args = append(args, startFrom)
		}
		_, err = runGit(ctx, repo, args...)
	}
	return err
}

func removeWorktree(ctx context.Context, repo, path string) error {
	if _, err := runGit(ctx, repo, "worktree", "remove", "--force", path); err != nil {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			_, _ = runGit(ctx, repo, "worktree", "prune")
			return nil
		}
		return err
	}
	_, _ = runGit(ctx, repo, "worktree", "prune")
	return nil
}

// deleteBranch：force=false 时用 -d，git 认为有未合并提交会拒绝——这时保留分支。
func deleteBranch(ctx context.Context, repo, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := runGit(ctx, repo, "branch", flag, branch)
	return err
}

// copyWorktreeIncludes 把 .worktreeinclude 列出的被忽略文件从主检出复制进新 worktree
// （.env 之类）。只支持字面路径；不存在或被 git 跟踪的跳过。
func copyWorktreeIncludes(ctx context.Context, repo, dst string) []string {
	raw, err := os.ReadFile(filepath.Join(repo, ".worktreeinclude"))
	if err != nil {
		return nil
	}
	var copied []string
	for _, line := range strings.Split(string(raw), "\n") {
		rel := strings.TrimSpace(line)
		if rel == "" || strings.HasPrefix(rel, "#") || strings.ContainsAny(rel, "*?[!") || strings.Contains(rel, "..") {
			continue
		}
		if _, err := runGit(ctx, repo, "ls-files", "--error-unmatch", rel); err == nil {
			continue // 被跟踪的文件 worktree 里本来就有
		}
		src := filepath.Join(repo, filepath.FromSlash(rel))
		info, err := os.Stat(src)
		if err != nil || info.IsDir() {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if os.MkdirAll(filepath.Dir(target), 0o755) == nil && os.WriteFile(target, data, info.Mode().Perm()) == nil {
			copied = append(copied, rel)
		}
	}
	return copied
}

// ---------- 状态 ----------

// GitFile 是 porcelain 状态里的一行。
type GitFile struct {
	Path      string
	Index     string
	Worktree  string
	Staged    bool
	Untracked bool
}

// GitStatus 是 worktree 的源码管理状态。
type GitStatus struct {
	Branch    string
	Upstream  string
	Ahead     int
	Behind    int
	Head      string
	BaseRef   string
	Files     []GitFile
	Additions int
	Deletions int
}

var aheadBehindRe = regexp.MustCompile(`\[(?:ahead (\d+))?(?:, )?(?:behind (\d+))?\]`)

func gitStatus(ctx context.Context, dir string) (GitStatus, error) {
	out, err := runGit(ctx, dir, "status", "--porcelain=v1", "-b", "--untracked-files=all")
	if err != nil {
		return GitStatus{}, err
	}
	var st GitStatus
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			parseBranchLine(strings.TrimPrefix(line, "## "), &st)
			continue
		}
		if len(line) < 4 {
			continue
		}
		x, y, path := line[0:1], line[1:2], unquoteGitPath(line[3:])
		if i := strings.Index(path, " -> "); i >= 0 {
			path = unquoteGitPath(path[i+4:])
		}
		f := GitFile{Path: path, Index: x, Worktree: y}
		f.Untracked = x == "?"
		f.Staged = x != " " && x != "?"
		st.Files = append(st.Files, f)
	}
	if head, err := runGit(ctx, dir, "rev-parse", "--short", "HEAD"); err == nil {
		st.Head = strings.TrimSpace(head)
	}
	return st, nil
}

func parseBranchLine(s string, st *GitStatus) {
	if strings.HasPrefix(s, "No commits yet on ") {
		st.Branch = strings.TrimPrefix(s, "No commits yet on ")
		return
	}
	rest := s
	if i := strings.Index(rest, " ["); i >= 0 {
		if m := aheadBehindRe.FindStringSubmatch(rest[i+1:]); m != nil {
			st.Ahead, _ = strconv.Atoi(m[1])
			st.Behind, _ = strconv.Atoi(m[2])
		}
		rest = rest[:i]
	}
	if i := strings.Index(rest, "..."); i >= 0 {
		st.Branch, st.Upstream = rest[:i], rest[i+3:]
	} else {
		st.Branch = rest
	}
}

func unquoteGitPath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
		if u, err := strconv.Unquote(p); err == nil {
			return u
		}
	}
	return p
}

func mergeBase(ctx context.Context, dir, base string) string {
	if base == "" {
		return "HEAD"
	}
	if out, err := runGit(ctx, dir, "merge-base", base, "HEAD"); err == nil {
		if mb := strings.TrimSpace(out); mb != "" {
			return mb
		}
	}
	return base
}

func diffTotals(ctx context.Context, dir, against string) (adds, dels int) {
	out, err := runGit(ctx, dir, "diff", "--numstat", against)
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		a, _ := strconv.Atoi(parts[0])
		d, _ := strconv.Atoi(parts[1])
		adds += a
		dels += d
	}
	return adds, dels
}

// ---------- diff ----------

// DiffLine / DiffHunk / DiffFile 是解析后的统一 diff。
type DiffLine struct {
	Kind  string // ctx | add | del
	OldNo int
	NewNo int
	Text  string
}

type DiffHunk struct {
	Header string
	Lines  []DiffLine
}

type DiffFile struct {
	Path      string
	OldPath   string
	Status    string
	Additions int
	Deletions int
	Binary    bool
	Truncated bool
	Hunks     []DiffHunk
}

// DiffResult 是 worktree 相对起点的全部改动（已提交 + 未提交 + 未跟踪）。
type DiffResult struct {
	BaseRef   string
	MergeBase string
	Files     []DiffFile
	Additions int
	Deletions int
}

func gitDiff(ctx context.Context, dir, base string) (DiffResult, error) {
	mb := mergeBase(ctx, dir, base)
	out, err := runGit(ctx, dir, "diff", "--no-ext-diff", "-M", "-U3", mb)
	if err != nil {
		return DiffResult{}, err
	}
	res := DiffResult{BaseRef: base, MergeBase: mb, Files: parseUnifiedDiff(out)}
	if untracked, err := runGit(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z"); err == nil {
		for _, p := range strings.Split(untracked, "\x00") {
			if p == "" || len(res.Files) >= maxDiffFiles {
				continue
			}
			res.Files = append(res.Files, untrackedFile(dir, p))
		}
	}
	for _, f := range res.Files {
		res.Additions += f.Additions
		res.Deletions += f.Deletions
	}
	return res, nil
}

func untrackedFile(dir, rel string) DiffFile {
	f := DiffFile{Path: rel, Status: "untracked"}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return f
	}
	if len(data) > maxUntrackedSize {
		data, f.Truncated = data[:maxUntrackedSize], true
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		f.Binary = true
		return f
	}
	text := strings.TrimSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > maxLinesPerFile {
		lines, f.Truncated = lines[:maxLinesPerFile], true
	}
	h := DiffHunk{Header: fmt.Sprintf("@@ -0,0 +1,%d @@", len(lines))}
	for i, l := range lines {
		h.Lines = append(h.Lines, DiffLine{Kind: "add", NewNo: i + 1, Text: l})
	}
	f.Additions = len(lines)
	f.Hunks = []DiffHunk{h}
	return f
}

var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// parseUnifiedDiff 解析 git diff 输出。
func parseUnifiedDiff(out string) []DiffFile {
	var files []DiffFile
	var cur *DiffFile
	var hunk *DiffHunk
	oldNo, newNo, lineCount := 0, 0, 0
	oldLeft, newLeft := 0, 0 // hunk 头声明的剩余行数，用完即 hunk 结束
	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
		}
		hunk = nil
	}
	flushFile := func() {
		flushHunk()
		if cur != nil && len(files) < maxDiffFiles {
			files = append(files, *cur)
		}
		cur = nil
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushFile()
			cur = &DiffFile{Status: "modified"}
			lineCount = 0
			if a, b, ok := splitDiffGitPaths(strings.TrimPrefix(line, "diff --git ")); ok {
				cur.OldPath, cur.Path = a, b
			}
		case cur == nil:
			continue
		case hunk == nil && strings.HasPrefix(line, "new file mode"):
			cur.Status = "added"
		case hunk == nil && strings.HasPrefix(line, "deleted file mode"):
			cur.Status = "deleted"
		case hunk == nil && strings.HasPrefix(line, "rename from "):
			cur.Status = "renamed"
			cur.OldPath = unquoteGitPath(strings.TrimPrefix(line, "rename from "))
		case hunk == nil && strings.HasPrefix(line, "rename to "):
			cur.Path = unquoteGitPath(strings.TrimPrefix(line, "rename to "))
		case hunk == nil && strings.HasPrefix(line, "Binary files "):
			cur.Binary = true
		case hunk == nil && strings.HasPrefix(line, "--- "):
			if p := strings.TrimPrefix(line, "--- "); p != "/dev/null" {
				cur.OldPath = strings.TrimPrefix(unquoteGitPath(p), "a/")
			}
		case hunk == nil && strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p != "/dev/null" {
				cur.Path = strings.TrimPrefix(unquoteGitPath(p), "b/")
			}
		case strings.HasPrefix(line, "@@"):
			flushHunk()
			m := hunkHeaderRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			oldNo, _ = strconv.Atoi(m[1])
			newNo, _ = strconv.Atoi(m[3])
			oldLeft, newLeft = hunkCount(m[2]), hunkCount(m[4])
			hunk = &DiffHunk{Header: line}
		case hunk != nil && (oldLeft > 0 || newLeft > 0):
			if strings.HasPrefix(line, `\ `) {
				continue
			}
			kind := byte(' ')
			text := ""
			if line != "" {
				kind, text = line[0], line[1:]
			}
			switch kind {
			case '+':
				newLeft--
			case '-':
				oldLeft--
			default:
				oldLeft--
				newLeft--
			}
			if lineCount >= maxLinesPerFile {
				cur.Truncated = true
				continue
			}
			switch kind {
			case '+':
				hunk.Lines = append(hunk.Lines, DiffLine{Kind: "add", NewNo: newNo, Text: text})
				newNo++
				cur.Additions++
			case '-':
				hunk.Lines = append(hunk.Lines, DiffLine{Kind: "del", OldNo: oldNo, Text: text})
				oldNo++
				cur.Deletions++
			default:
				hunk.Lines = append(hunk.Lines, DiffLine{Kind: "ctx", OldNo: oldNo, NewNo: newNo, Text: text})
				oldNo++
				newNo++
			}
			lineCount++
		}
	}
	flushFile()
	for i := range files {
		if files[i].Status == "deleted" && files[i].Path == "" {
			files[i].Path = files[i].OldPath
		}
		if files[i].Status == "modified" && files[i].OldPath == files[i].Path {
			files[i].OldPath = ""
		}
	}
	return files
}

// hunkCount 解析 hunk 头里的行数；省略时为 1。
func hunkCount(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}

// splitDiffGitPaths 解析 "a/x b/y"（可能带引号）。
func splitDiffGitPaths(s string) (string, string, bool) {
	if strings.HasPrefix(s, `"`) {
		end := strings.Index(s[1:], `" `)
		if end < 0 {
			return "", "", false
		}
		a := unquoteGitPath(s[:end+2])
		b := unquoteGitPath(strings.TrimSpace(s[end+3:]))
		return strings.TrimPrefix(a, "a/"), strings.TrimPrefix(b, "b/"), true
	}
	if i := strings.Index(s, " b/"); i >= 0 {
		return strings.TrimPrefix(s[:i], "a/"), unquoteGitPath(s[i+3:]), true
	}
	return "", "", false
}
