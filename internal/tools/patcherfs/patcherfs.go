// Package patcherfs provides worktree-scoped file tools for the platform
// self-improvement Analyst (read-only) and Patcher (read/write + git diff).
//
// All paths are repository-relative. Absolute paths, ".." traversal, and
// symlink escapes are rejected. Writes are blocked against the M73 protected
// file list. The package has no LLM or agent-runtime dependency so the
// service-layer tool loop can call it directly.
package patcherfs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// Tool names used in the LLM tool-call JSON contract.
const (
	ToolRead  = "patcher_fs_read"
	ToolWrite = "patcher_fs_write"
	ToolList  = "patcher_fs_list"
	ToolDiff  = "patcher_git_diff"
)

const (
	defaultReadLimit = 32 * 1024
	maxReadLimit     = 64 * 1024
	maxWriteBytes    = 256 * 1024
	maxListEntries   = 200
)

// Mode selects which tools a workspace may execute.
type Mode int

const (
	// ModeRead allows read and list only (Analyst).
	ModeRead Mode = iota
	// ModeReadWrite allows read, list, write, and git diff (Patcher).
	ModeReadWrite
)

// Workspace is a path-jailed view of one directory (repo root or SI worktree).
type Workspace struct {
	root string
	mode Mode
}

// New binds a workspace to an existing directory. root must exist.
func New(root string, mode Mode) (*Workspace, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "patcherfs root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainTool)
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return nil, apierror.BadRequest(apierror.DomainTool, "patcherfs root %q is not a directory", root)
	}
	return &Workspace{root: abs, mode: mode}, nil
}

// Root returns the absolute workspace root.
func (w *Workspace) Root() string {
	if w == nil {
		return ""
	}
	return w.root
}

// Request is the LLM tool-call JSON:
//
//	{"tool":"patcher_fs_read","path":"internal/biz/x.go"}
//	{"tool":"patcher_fs_write","path":"...","content":"..."}
type Request struct {
	Tool    string `json:"tool"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}

// ParseRequest reports whether text is a tool call (has a non-empty tool name).
// Final Diagnosis / PatcherOutput JSON that happens to include a "tool" field
// is still treated as a tool call — callers should parse the final contract first.
func ParseRequest(text string) (Request, bool) {
	raw := stripFence(text)
	if raw == "" {
		return Request{}, false
	}
	var req Request
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return Request{}, false
	}
	req.Tool = strings.TrimSpace(req.Tool)
	if req.Tool == "" {
		return Request{}, false
	}
	return req, true
}

// Exec runs one tool request and returns a prompt-safe result string.
func (w *Workspace) Exec(req Request) string {
	if w == nil {
		return "error: workspace not bound"
	}
	switch req.Tool {
	case ToolRead:
		out, err := w.Read(req.Path, defaultReadLimit)
		if err != nil {
			return "error: " + err.Error()
		}
		return out
	case ToolList:
		out, err := w.List(req.Path)
		if err != nil {
			return "error: " + err.Error()
		}
		return out
	case ToolWrite:
		if w.mode != ModeReadWrite {
			return "error: write is not allowed in this workspace"
		}
		if err := w.Write(req.Path, req.Content); err != nil {
			return "error: " + err.Error()
		}
		return "ok: wrote " + strings.TrimSpace(req.Path)
	case ToolDiff:
		if w.mode != ModeReadWrite {
			return "error: git diff is not allowed in this workspace"
		}
		out, err := w.Diff(req.Path)
		if err != nil {
			return "error: " + err.Error()
		}
		if strings.TrimSpace(out) == "" {
			return "(empty diff: worktree matches HEAD)"
		}
		return out
	default:
		return "error: unknown tool " + req.Tool
	}
}

// Read returns file contents (UTF-8 text only), truncated to limit bytes.
func (w *Workspace) Read(rel string, limit int) (string, error) {
	full, err := w.resolve(rel, false)
	if err != nil {
		return "", err
	}
	if limit <= 0 {
		limit = defaultReadLimit
	}
	if limit > maxReadLimit {
		limit = maxReadLimit
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", rel, err)
	}
	if isBinary(data) {
		return "", fmt.Errorf("%s is binary", rel)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%s is not valid utf-8", rel)
	}
	if len(data) > limit {
		return string(data[:limit]) + "\n…[truncated]", nil
	}
	return string(data), nil
}

// Write creates or overwrites a UTF-8 text file. Parent directories are created.
func (w *Workspace) Write(rel, content string) error {
	if w.mode != ModeReadWrite {
		return errors.New("write is not allowed in this workspace")
	}
	rel = strings.TrimSpace(rel)
	if err := rejectProtected(rel, fileExists(w, rel)); err != nil {
		return err
	}
	if len(content) > maxWriteBytes {
		return fmt.Errorf("write %s exceeds %d bytes", rel, maxWriteBytes)
	}
	if isBinary([]byte(content)) || !utf8.Valid([]byte(content)) {
		return fmt.Errorf("write %s: content must be utf-8 text", rel)
	}
	full, err := w.resolve(rel, true)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content), 0o644)
}

// List returns a newline-separated directory listing (non-recursive).
func (w *Workspace) List(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		rel = "."
	}
	full, err := w.resolve(rel, false)
	if err != nil {
		return "", err
	}
	ents, err := os.ReadDir(full)
	if err != nil {
		return "", fmt.Errorf("list %s: %w", rel, err)
	}
	var b strings.Builder
	n := 0
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if n >= maxListEntries {
			b.WriteString("…[truncated]\n")
			break
		}
		if e.IsDir() {
			b.WriteString(name)
			b.WriteString("/\n")
		} else {
			b.WriteString(name)
			b.WriteByte('\n')
		}
		n++
	}
	if n == 0 {
		return "(empty)", nil
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// Diff returns `git diff` of the workspace (optional path) against HEAD.
func (w *Workspace) Diff(rel string) (string, error) {
	args := []string{"diff", "--no-ext-diff", "--no-color"}
	if p := strings.TrimSpace(rel); p != "" && p != "." {
		if _, err := w.resolve(p, true); err != nil {
			return "", err
		}
		args = append(args, "--", filepath.ToSlash(p))
	}
	out, err := w.git(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

// Restore resets tracked and untracked worktree changes so the pipeline can
// re-apply the official PatcherOutput.Diff from a clean base.
func (w *Workspace) Restore() error {
	if w == nil || w.mode != ModeReadWrite {
		return nil
	}
	if _, err := os.Stat(filepath.Join(w.root, ".git")); err != nil {
		// Bare dirs used in unit tests are not git repos.
		if _, err := w.git("rev-parse", "--is-inside-work-tree"); err != nil {
			return nil
		}
	}
	if _, err := w.git("checkout", "--", "."); err != nil {
		return err
	}
	_, err := w.git("clean", "-fd", "-e", ".aranea-self-improve")
	return err
}

func (w *Workspace) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = w.root
	cmd.Env = []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_OPTIONAL_LOCKS=0"}
	if p := os.Getenv("PATH"); p != "" {
		cmd.Env = append(cmd.Env, "PATH="+p)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return string(out), nil
}

func (w *Workspace) resolve(rel string, allowMissing bool) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return w.root, nil
	}
	rel = filepath.ToSlash(rel)
	if filepath.IsAbs(rel) || strings.Contains(rel, `\`) {
		return "", fmt.Errorf("path %q must be a repo-relative slash path", rel)
	}
	clean := path.Clean("/" + rel)
	if clean == "/" || strings.HasPrefix(clean, "/..") {
		return "", fmt.Errorf("path %q escapes workspace", rel)
	}
	rel = strings.TrimPrefix(clean, "/")
	full := filepath.Join(w.root, filepath.FromSlash(rel))
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	root := w.root
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
		return "", fmt.Errorf("path %q escapes workspace", rel)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
		if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
			return "", fmt.Errorf("path %q escapes workspace via symlink", rel)
		}
	} else if !allowMissing && !os.IsNotExist(err) {
		if _, statErr := os.Stat(abs); statErr != nil && !os.IsNotExist(statErr) {
			return "", statErr
		}
	}
	if !allowMissing {
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("path %q not found", rel)
		}
	}
	return abs, nil
}

func rejectProtected(rel string, exists bool) error {
	kind := biz.PatchChangeAdded
	if exists {
		kind = biz.PatchChangeModified
	}
	hits := biz.CheckProtectedFiles([]biz.PatchFileChange{{
		Path: filepath.ToSlash(rel),
		Kind: kind,
	}}, biz.DefaultProtectedFileRules())
	if len(hits) > 0 {
		return fmt.Errorf("protected file %s (%s)", hits[0].Path, hits[0].Reason)
	}
	return nil
}

func fileExists(w *Workspace, rel string) bool {
	full, err := w.resolve(rel, true)
	if err != nil {
		return false
	}
	_, err = os.Stat(full)
	return err == nil
}

func isBinary(data []byte) bool {
	n := len(data)
	if n > 512 {
		n = 512
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}

func stripFence(text string) string {
	raw := strings.TrimSpace(text)
	if !strings.HasPrefix(raw, "```") {
		return raw
	}
	lines := strings.Split(raw, "\n")
	if len(lines) < 2 {
		return raw
	}
	lines = lines[1:]
	if n := len(lines); n > 0 && strings.TrimSpace(lines[n-1]) == "```" {
		lines = lines[:n-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
