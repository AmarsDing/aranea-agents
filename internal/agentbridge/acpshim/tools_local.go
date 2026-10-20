package acpshim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// tools_local.go — 内置 fs/shell 本地工具（设计 §C/§D-5）。
//
//	cwd 锁：路径解析锁定 session/new 落定的注册项目目录；
//	逃逸拒绝：解析后路径必须仍在 cwd 内（Rel 不以 ".." 开头），
//	绝对路径仅在仍属 cwd 内时放行。残余风险（shell 命令自身漫游、
//	symlink TOCTOU）如实记录——「cwd 锁 ≠ 文件系统围墙」，远期挂 82-sandbox。

const (
	toolFSRead   = "fs_read"
	toolFSList   = "fs_list"
	toolFSWrite  = "fs_write"
	toolShell    = "shell_exec"
	maxReadBytes = 1 << 20 // fs_read 截断上限 1MiB
	maxShellOut  = 64 << 10
)

// ToolEscapeError 路径逃逸拒绝（P1-5 断言点）。
type ToolEscapeError struct{ Path string }

func (e *ToolEscapeError) Error() string {
	return fmt.Sprintf("acpshim: path escapes locked cwd: %s", e.Path)
}

// Registry 内置工具执行器（无状态；锁定根在每次调用由会话 cwd 传入）。
type Registry struct {
	lg           loggateway.Logger
	shellTimeout time.Duration // <=0 → 2min
}

// NewRegistry 构造。lg nil → NewNoop()。
func NewRegistry(lg loggateway.Logger, shellTimeout time.Duration) *Registry {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if shellTimeout <= 0 {
		shellTimeout = 2 * time.Minute
	}
	return &Registry{
		lg:           lg.With(loggateway.Str("component", "acpshim.tools")),
		shellTimeout: shellTimeout,
	}
}

// Defs 返回 OpenAI function schema 形态的工具声明。
func (r *Registry) Defs() []ToolDef {
	return []ToolDef{
		{
			Name:        toolFSRead,
			Description: "读取工作区内文本文件内容（>1MiB 截断）。参数 path 为相对 cwd 的路径。",
			ParametersJSON: json.RawMessage(`{
				"type":"object",
				"properties":{"path":{"type":"string","description":"相对工作区根的文件路径"}},
				"required":["path"]
			}`),
		},
		{
			Name:        toolFSList,
			Description: "列出工作区内目录内容（每行一项，目录以 / 结尾）。path 省略或为空 = 根。",
			ParametersJSON: json.RawMessage(`{
				"type":"object",
				"properties":{"path":{"type":"string","description":"相对工作区根的目录路径，可空"}}
			}`),
		},
		{
			Name:        toolFSWrite,
			Description: "在工作区内写入文本文件（自动创建父目录；存在即覆盖）。需用户审批。",
			ParametersJSON: json.RawMessage(`{
				"type":"object",
				"properties":{
					"path":{"type":"string","description":"相对工作区根的文件路径"},
					"content":{"type":"string","description":"完整文件内容"}
				},
				"required":["path","content"]
			}`),
		},
		{
			Name:        toolShell,
			Description: "在工作区根执行 shell 命令（Windows cmd /C，其他 sh -c；2 分钟超时；输出截断 64KiB）。需用户审批。",
			ParametersJSON: json.RawMessage(`{
				"type":"object",
				"properties":{"command":{"type":"string","description":"要执行的命令行"}},
				"required":["command"]
			}`),
		},
	}
}

// ReadOnly 免批判定（Q2 终值：读类免批 / 写与执行强制批）。
func (r *Registry) ReadOnly(name string) bool {
	return name == toolFSRead || name == toolFSList
}

// Execute 分发执行。cwd 为锁定根；未知工具报错。
func (r *Registry) Execute(ctx context.Context, cwd, name string, args json.RawMessage) (string, error) {
	switch name {
	case toolFSRead:
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Path == "" {
			return "", errors.New("fs_read: invalid args (path required)")
		}
		return r.fsRead(cwd, a.Path)
	case toolFSList:
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &a) // path 可空
		return r.fsList(cwd, a.Path)
	case toolFSWrite:
		var a struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Path == "" {
			return "", errors.New("fs_write: invalid args (path/content required)")
		}
		return r.fsWrite(cwd, a.Path, a.Content)
	case toolShell:
		var a struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Command) == "" {
			return "", errors.New("shell_exec: invalid args (command required)")
		}
		return r.shellExec(ctx, cwd, a.Command)
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// resolveLocked 解析并校验路径归属（cwd 锁）。
func resolveLocked(cwd, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.New("empty path")
	}
	root := filepath.Clean(cwd)
	target := p
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", &ToolEscapeError{Path: p}
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", &ToolEscapeError{Path: p}
	}
	return target, nil
}

func (r *Registry) fsRead(cwd, p string) (string, error) {
	fp, err := resolveLocked(cwd, p)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(fp)
	if err != nil {
		return "", err
	}
	if len(data) > maxReadBytes {
		return string(data[:maxReadBytes]) + "\n... [truncated at 1MiB]", nil
	}
	return string(data), nil
}

func (r *Registry) fsList(cwd, p string) (string, error) {
	dir := cwd
	if strings.TrimSpace(p) != "" {
		var err error
		dir, err = resolveLocked(cwd, p)
		if err != nil {
			return "", err
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		sb.WriteString(name)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func (r *Registry) fsWrite(cwd, p, content string) (string, error) {
	fp, err := resolveLocked(cwd, p)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		return "", err
	}
	r.lg.Info("acpshim: fs_write", loggateway.Str("path", fp), loggateway.Int("bytes", len(content)))
	return fmt.Sprintf("written %d bytes to %s", len(content), p), nil
}

func (r *Registry) shellExec(ctx context.Context, cwd, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.shellTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = cwd
	var buf bytes.Buffer
	cmd.Stdout = &cappedWriter{w: &buf, max: maxShellOut}
	cmd.Stderr = &cappedWriter{w: &buf, max: maxShellOut}
	runErr := cmd.Run()
	out := buf.String()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("shell_exec: timeout after %s", r.shellTimeout)
	}
	if runErr != nil {
		return out, fmt.Errorf("shell_exec: %w", runErr)
	}
	return out, nil
}

// cappedWriter 截断写（超出丢弃，防失控输出撑爆内存）。
type cappedWriter struct {
	w   *bytes.Buffer
	max int
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	room := c.max - c.w.Len()
	if room > 0 {
		if len(p) > room {
			c.w.Write(p[:room])
		} else {
			c.w.Write(p)
		}
	}
	return len(p), nil
}
