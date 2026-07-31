// Package patcherfs provides the 3 worktree-scoped tools assembled for the
// self-improvement Patcher agent (73-self-iteration-v3, design D5): read,
// write, and produce the unified diff of the sandbox worktree. Every file
// path is resolved and confined to the bound worktree root — absolute paths,
// ".." escapes, .git internals, and symlink breakouts are rejected.
package patcherfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	// defaultMaxReadBytes / maxReadBytes bounds fs_read payloads (mirrors the
	// memberfs 200KB convention).
	defaultMaxReadBytes = 204800
	maxReadBytes        = 204800
	// maxDiffBytes caps the git_diff payload returned to the LLM.
	maxDiffBytes = 256 * 1024
)

// toolset binds the worktree root shared by all patcher tools.
type toolset struct {
	root     string
	rootReal string // symlink-resolved root for escape checks
	lg       loggateway.Logger
}

// RegisterAll creates the patcher tools bound to worktreeRoot. Returns nil
// when the root is unusable (empty / nonexistent).
func RegisterAll(worktreeRoot string, lg loggateway.Logger) []trpctool.Tool {
	if strings.TrimSpace(worktreeRoot) == "" {
		return nil
	}
	abs, err := filepath.Abs(worktreeRoot)
	if err != nil {
		return nil
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil // worktree must already exist (PrepareWorktree ran first)
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	s := &toolset{root: abs, rootReal: real, lg: lg}
	return []trpctool.Tool{
		&fsReadTool{s: s},
		&fsWriteTool{s: s},
		&gitDiffTool{s: s},
	}
}

// resolve maps a caller-supplied relative path to an absolute path inside the
// worktree. mustExist=false additionally validates the nearest existing
// ancestor so writes cannot escape through a symlinked directory.
func (s *toolset) resolve(rel string, mustExist bool) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", errors.New("path 为必填项")
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", fmt.Errorf("拒绝绝对路径: %s", rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("拒绝越界路径: %s", rel)
	}
	if clean == ".git" || strings.HasPrefix(clean, ".git"+string(filepath.Separator)) {
		return "", fmt.Errorf("禁止访问 .git 内部: %s", rel)
	}
	abs := filepath.Join(s.root, clean)

	target := abs
	if mustExist {
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("路径不存在或不可读: %s", rel)
		}
		target = real
	} else {
		// Walk up to the nearest existing ancestor, resolve it, then re-attach
		// the not-yet-created suffix.
		dir := abs
		var suffix []string
		for {
			if _, err := os.Lstat(dir); err == nil {
				break
			}
			suffix = append([]string{filepath.Base(dir)}, suffix...)
			parent := filepath.Dir(dir)
			if parent == dir {
				return "", fmt.Errorf("路径无有效祖先目录: %s", rel)
			}
			dir = parent
		}
		realAncestor, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return "", fmt.Errorf("祖先目录不可解析: %s", rel)
		}
		parts := append([]string{realAncestor}, suffix...)
		target = filepath.Join(parts...)
	}

	if target != s.rootReal && !strings.HasPrefix(target, s.rootReal+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径逃逸出 worktree: %s", rel)
	}
	return abs, nil
}

func (s *toolset) logWarn(step, path string, err error) {
	s.lg.Warn("patcherfs 工具调用失败",
		loggateway.StepID("tool."+step),
		loggateway.Str("path", path),
		loggateway.Err(err))
}

// ---------------------------------------------------------------------------
// patcher_fs_read
// ---------------------------------------------------------------------------

type fsReadTool struct{ s *toolset }

type fsReadInput struct {
	Path     string `json:"path"`
	MaxBytes int64  `json:"max_bytes"`
}

type fsReadOutput struct {
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
	Path      string `json:"path"`
}

func (t *fsReadTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "patcher_fs_read",
		Description: "读取补丁沙盒（git worktree）内的文本文件。路径相对 worktree 根；" +
			"二进制、越界、.git 内部路径一律拒绝；超过 max_bytes 截断并标记 truncated。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]*trpctool.Schema{
				"path":      {Type: "string", Description: "文件相对路径（相对 worktree 根）。"},
				"max_bytes": {Type: "integer", Description: "最大读取字节数，默认/上限 204800（200KB）。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "文件文本内容（可能截断）。",
			Required:    []string{"content", "truncated", "path"},
			Properties: map[string]*trpctool.Schema{
				"content":   {Type: "string", Description: "文件文本内容。"},
				"truncated": {Type: "boolean", Description: "是否被截断到 max_bytes。"},
				"path":      {Type: "string", Description: "实际读取的相对路径。"},
			},
		},
	}
}

func (t *fsReadTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in fsReadInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	abs, err := t.s.resolve(in.Path, true)
	if err != nil {
		t.s.logWarn("patcher_fs_read", in.Path, err)
		return nil, err
	}
	maxBytes := in.MaxBytes
	if maxBytes <= 0 || maxBytes > maxReadBytes {
		maxBytes = defaultMaxReadBytes
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		t.s.logWarn("patcher_fs_read", in.Path, err)
		return nil, fmt.Errorf("读取失败: %w", err)
	}
	if isBinaryContent(data) {
		err := fmt.Errorf("拒绝二进制文件: %s", in.Path)
		t.s.logWarn("patcher_fs_read", in.Path, err)
		return nil, err
	}
	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	return fsReadOutput{Content: string(data), Truncated: truncated, Path: filepath.ToSlash(strings.TrimPrefix(in.Path, "./"))}, nil
}

// isBinaryContent reports whether data looks binary (NUL byte in the first
// 8KB, matching git's own heuristic).
func isBinaryContent(data []byte) bool {
	const sniff = 8192
	n := len(data)
	if n > sniff {
		n = sniff
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// patcher_fs_write
// ---------------------------------------------------------------------------

type fsWriteTool struct{ s *toolset }

type fsWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type fsWriteOutput struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

func (t *fsWriteTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "patcher_fs_write",
		Description: "在补丁沙盒（git worktree）内写入文本文件（不存在则创建，父目录自动创建）。" +
			"路径相对 worktree 根；越界、.git 内部、符号链接逃逸一律拒绝。写入后经 patcher_git_diff 产出补丁。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"path", "content"},
			Properties: map[string]*trpctool.Schema{
				"path":    {Type: "string", Description: "文件相对路径（相对 worktree 根）。"},
				"content": {Type: "string", Description: "完整文件内容（整体覆盖写）。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "写入结果。",
			Required:    []string{"path", "bytes"},
			Properties: map[string]*trpctool.Schema{
				"path":  {Type: "string", Description: "实际写入的相对路径。"},
				"bytes": {Type: "integer", Description: "写入字节数。"},
			},
		},
	}
}

func (t *fsWriteTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in fsWriteInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	abs, err := t.s.resolve(in.Path, false)
	if err != nil {
		t.s.logWarn("patcher_fs_write", in.Path, err)
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.s.logWarn("patcher_fs_write", in.Path, err)
		return nil, fmt.Errorf("创建父目录失败: %w", err)
	}
	if err := os.WriteFile(abs, []byte(in.Content), 0o644); err != nil {
		t.s.logWarn("patcher_fs_write", in.Path, err)
		return nil, fmt.Errorf("写入失败: %w", err)
	}
	return fsWriteOutput{Path: filepath.ToSlash(strings.TrimPrefix(in.Path, "./")), Bytes: len(in.Content)}, nil
}

// ---------------------------------------------------------------------------
// patcher_git_diff
// ---------------------------------------------------------------------------

type gitDiffTool struct{ s *toolset }

type gitDiffOutput struct {
	Diff      string        `json:"diff"`
	Truncated bool          `json:"truncated"`
	Stats     biz.DiffStats `json:"stats"`
}

func (t *gitDiffTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "patcher_git_diff",
		Description: "产出补丁沙盒当前的 unified diff（含未跟踪的新文件）。" +
			"这是 Patcher 的最终交付物，将交由验证沙盒执行 G1-G3 门禁。",
		InputSchema: &trpctool.Schema{
			Type:       "object",
			Properties: map[string]*trpctool.Schema{},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "unified diff 与统计。",
			Required:    []string{"diff", "truncated", "stats"},
			Properties: map[string]*trpctool.Schema{
				"diff":      {Type: "string", Description: "unified diff 全文（超过 256KB 截断）。"},
				"truncated": {Type: "boolean", Description: "diff 是否被截断。"},
				"stats":     {Type: "object", Description: "{files,additions,deletions}。"},
			},
		},
	}
}

func (t *gitDiffTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	// Intent-to-add makes untracked files visible to `git diff`. It mutates
	// the index without staging content, which is exactly what a patch
	// scratchpad needs.
	if out, err := t.runGit(ctx, "add", "-N", "."); err != nil {
		t.s.logWarn("patcher_git_diff", "add -N", fmt.Errorf("%w: %s", err, out))
		return nil, fmt.Errorf("git add -N 失败: %w", err)
	}
	diff, err := t.runGit(ctx, "diff")
	if err != nil {
		t.s.logWarn("patcher_git_diff", "diff", err)
		return nil, fmt.Errorf("git diff 失败: %w", err)
	}
	truncated := len(diff) > maxDiffBytes
	if truncated {
		diff = diff[:maxDiffBytes] + "\n...[truncated]"
	}
	return gitDiffOutput{Diff: diff, Truncated: truncated, Stats: biz.ComputeDiffStats(diff)}, nil
}

func (t *gitDiffTool) runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = t.s.root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// --- interface guards ---

var (
	_ trpctool.Tool         = (*fsReadTool)(nil)
	_ trpctool.CallableTool = (*fsReadTool)(nil)
	_ trpctool.Tool         = (*fsWriteTool)(nil)
	_ trpctool.CallableTool = (*fsWriteTool)(nil)
	_ trpctool.Tool         = (*gitDiffTool)(nil)
	_ trpctool.CallableTool = (*gitDiffTool)(nil)
)
