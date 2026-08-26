// Package sandboxfs 提供会话沙箱文件工具族（sandbox_fs_write / sandbox_fs_read）。
// 工具为薄壳层：参数解析 + 路径校验 + 委托 sandbox.SessionLeases（P1-1 共享
// 会话租约池），与同会话的 execute_code 落在同一个沙箱内——代码产物可读出、
// 写入的文件可被后续代码执行消费（M82 US-2）。
package sandboxfs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/sandbox"
	"aranea-agents/pkg/apierror"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Tool 名常量（与种子 tool_key 一致）。
const (
	ToolWrite = "sandbox_fs_write"
	ToolRead  = "sandbox_fs_read"
)

// sessionRenewExtend slides the session-pinned lease forward after each
// successful fs op (same cadence as the codeexecutor backend).
const sessionRenewExtend = 30 * time.Minute

// readMaxBytes caps sandbox_fs_read output (default and hard ceiling).
const (
	readDefaultMaxBytes = 64 * 1024
	readHardMaxBytes    = 256 * 1024
)

// writablePrefixes mirrors the P0 sandbox mounts: /tmp is tmpfs,
// /workspace/out is the artifact volume; the rootfs is read-only.
var writablePrefixes = []string{"/tmp/", "/workspace/out/"}

// NewToolset 构建 2 个 sandbox_fs 工具；store 为 nil 返回空（装配层裁剪——
// 沙箱子系统禁用或 docker daemon 不可用时 agent 看不到必然失败的工具）。
func NewToolset(store *sandbox.SessionLeases) []trpctool.CallableTool {
	if store == nil {
		return nil
	}
	return []trpctool.CallableTool{
		&tool{name: ToolWrite, store: store, fn: writeFn, schema: schemaOf(`{"type":"object","properties":{
			"path":{"type":"string","description":"沙箱内绝对路径；仅可写 /tmp/ 与 /workspace/out/ 前缀（rootfs 只读）"},
			"content":{"type":"string","description":"文件内容"},
			"encoding":{"type":"string","enum":["utf8","base64"],"description":"content 编码，默认 utf8；二进制内容用 base64"}},
			"required":["path","content"]}`), desc: "在会话沙箱中写文件（自动创建父目录）。沙箱与本会话的 execute_code 共享：写入的文件可被后续代码执行读取；/workspace/out/ 下的文件会作为代码执行产物被收集。路径仅允许 /tmp/ 与 /workspace/out/ 前缀。沙箱用完即毁，会话结束后文件不保留。"},
		&tool{name: ToolRead, store: store, fn: readFn, schema: schemaOf(`{"type":"object","properties":{
			"path":{"type":"string","description":"沙箱内绝对路径"},
			"max_bytes":{"type":"integer","description":"最大返回字节数，默认 65536，上限 262144；超出截断"}},
			"required":["path"]}`), desc: "读取会话沙箱中的文件内容（与本会话的 execute_code 同一沙箱）。文本按 UTF-8 返回，二进制自动 base64（encoding 字段标识）；超长按 max_bytes 截断（truncated=true）。"},
	}
}

type tool struct {
	name   string
	desc   string
	store  *sandbox.SessionLeases
	fn     func(context.Context, *sandbox.SessionLeases, string, []byte) (any, error)
	schema *trpctool.Schema
}

func (t *tool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: t.name, Description: t.desc, InputSchema: t.schema}
}

func (t *tool) Call(ctx context.Context, args []byte) (any, error) {
	key, err := sessionKeyFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	return t.fn(ctx, t.store, key, args)
}

func schemaOf(raw string) *trpctool.Schema {
	var s trpctool.Schema
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		panic("sandboxfs tool schema: " + err.Error())
	}
	return &s
}

// sessionKeyFromCtx 从调用上下文派生会话键（app/user/session 格式，与
// codeexecutor pooledAdapter 的 invocation 兜底派生一致——同会话的 code_exec
// 与 sandbox_fs 因此共享同一沙箱租约）。无会话上下文时报错：fs 工具在
// 无归属租约下无意义（用完即毁语义要求会话级回收）。
func sessionKeyFromCtx(ctx context.Context) (string, error) {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil || inv.Session.ID == "" {
		return "", apierror.Internal(apierror.DomainTool, "sandbox_fs: no session in invocation context")
	}
	s := inv.Session
	return s.AppName + "/" + s.UserID + "/" + s.ID, nil
}

// agentKeyFromCtx 派生 per-agent 配额归因键（review 2026-08-26 #1：
// SessionLeases.Acquire 此前不传 AgentKey，per-agent 并发闸在生产空转）。
// 无 invocation 上下文时返回 ""（跳过 per-agent 闸，global 闸仍兜底）。
func agentKeyFromCtx(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	return inv.AgentName
}

func decode(args []byte, v any) error {
	if err := json.Unmarshal(args, v); err != nil {
		return apierror.BadRequest(apierror.DomainTool, "invalid args: "+err.Error())
	}
	return nil
}

// cleanAbsPath 归一并校验沙箱内绝对路径（拒相对路径与 .. 逃逸）。
func cleanAbsPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || !strings.HasPrefix(p, "/") {
		return "", apierror.BadRequest(apierror.DomainTool, "path must be an absolute in-sandbox path")
	}
	clean := path.Clean(p)
	if clean == "/" || strings.HasPrefix(clean, "/../") || clean == "/.." {
		return "", apierror.BadRequest(apierror.DomainTool, "path escapes sandbox root")
	}
	return clean, nil
}

func cleanWritablePath(p string) (string, error) {
	clean, err := cleanAbsPath(p)
	if err != nil {
		return "", err
	}
	for _, prefix := range writablePrefixes {
		if strings.HasPrefix(clean, prefix) {
			return clean, nil
		}
	}
	return "", apierror.BadRequest(apierror.DomainTool,
		"path not writable: sandbox rootfs is read-only; allowed prefixes: "+strings.Join(writablePrefixes, ", "))
}

// withLease 执行一次租约操作；租约被管理侧 GC（idle/TTL）后 Evict 并重试一次
// （新租约是全新空状态，符合用完即毁契约）。
func withLease(ctx context.Context, store *sandbox.SessionLeases, key string, op func(*sandbox.Lease) (any, error)) (any, error) {
	for attempt := 0; attempt < 2; attempt++ {
		lease, err := store.Acquire(ctx, key, agentKeyFromCtx(ctx))
		if err != nil {
			return nil, fmt.Errorf("sandbox_fs acquire: %w", err)
		}
		out, err := op(lease)
		if err == nil {
			store.Renew(ctx, key, sessionRenewExtend)
			return out, nil
		}
		if errors.Is(err, sandbox.ErrNotFound) && attempt == 0 {
			store.Evict(key, lease)
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("sandbox_fs: unreachable")
}

func writeFn(ctx context.Context, store *sandbox.SessionLeases, key string, args []byte) (any, error) {
	var in struct {
		Path     string `json:"path"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	clean, err := cleanWritablePath(in.Path)
	if err != nil {
		return nil, err
	}
	var content []byte
	switch in.Encoding {
	case "", "utf8":
		content = []byte(in.Content)
	case "base64":
		raw, err := base64.StdEncoding.DecodeString(in.Content)
		if err != nil {
			return nil, apierror.BadRequest(apierror.DomainTool, "content is not valid base64: "+err.Error())
		}
		content = raw
	default:
		return nil, apierror.BadRequest(apierror.DomainTool, "encoding must be utf8 or base64")
	}

	return withLease(ctx, store, key, func(lease *sandbox.Lease) (any, error) {
		// WriteFile 要求父目录已存在（docker cp 语义）——先 mkdir -p。
		if dir := path.Dir(clean); dir != "/" {
			res, err := lease.Exec(ctx, sandbox.ExecSpec{Argv: []string{"mkdir", "-p", dir}, Timeout: 10 * time.Second})
			if err != nil {
				return nil, fmt.Errorf("sandbox_fs mkdir: %w", err)
			}
			if res.ExitCode != 0 {
				return nil, fmt.Errorf("sandbox_fs mkdir %s: exit %d: %s", dir, res.ExitCode, strings.TrimSpace(res.Stderr))
			}
		}
		if err := lease.WriteFile(ctx, clean, content); err != nil {
			return nil, fmt.Errorf("sandbox_fs write: %w", err)
		}
		return map[string]any{
			"ok":         true,
			"path":       clean,
			"bytes":      len(content),
			"sandbox_id": lease.SandboxID(),
		}, nil
	})
}

func readFn(ctx context.Context, store *sandbox.SessionLeases, key string, args []byte) (any, error) {
	var in struct {
		Path     string `json:"path"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	clean, err := cleanAbsPath(in.Path)
	if err != nil {
		return nil, err
	}
	maxBytes := in.MaxBytes
	if maxBytes <= 0 {
		maxBytes = readDefaultMaxBytes
	}
	if maxBytes > readHardMaxBytes {
		maxBytes = readHardMaxBytes
	}

	return withLease(ctx, store, key, func(lease *sandbox.Lease) (any, error) {
		content, truncated, err := lease.ReadFile(ctx, clean, int64(maxBytes))
		if err != nil {
			// 目录路径：模型可纠正（r2 #6），结构化报错，不触发重试。
			if errors.Is(err, sandbox.ErrNotRegular) {
				return nil, apierror.BadRequest(apierror.DomainTool, "sandbox_fs: not a regular file: "+clean)
			}
			// ErrNotFound 歧义拆分：租约存活 = 文件不存在（模型可纠正，
			// 结构化错误，不触发 withLease 重试）；租约已毁 = 原样上抛
			// （withLease Evict 并重试一次）。
			if errors.Is(err, sandbox.ErrNotFound) && lease.Alive() {
				return nil, apierror.NotFound(apierror.DomainTool, "sandbox_fs: file not found: "+clean)
			}
			return nil, fmt.Errorf("sandbox_fs read: %w", err)
		}
		out := map[string]any{
			"path":       clean,
			"size":       len(content),
			"truncated":  truncated,
			"sandbox_id": lease.SandboxID(),
		}
		if utf8.Valid(content) {
			out["encoding"] = "utf8"
			out["content"] = string(content)
		} else {
			out["encoding"] = "base64"
			out["content"] = base64.StdEncoding.EncodeToString(content)
		}
		return out, nil
	})
}
