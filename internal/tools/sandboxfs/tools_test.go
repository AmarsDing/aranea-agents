package sandboxfs

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"path"
	"strings"
	"sync"
	"testing"

	"aranea-agents/internal/sandbox"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// memFSEngine 是带内存文件系统的 sandbox.Engine 测试桩：写入走 exec
// `sh -c 'cat > "$1"'`（stdin 落 map，与 r2 #2 的 Lease.WriteFile 一致），
// CopyFrom 按 docker cp 语义对缺失路径返回空流（Lease.ReadFile 因此映射为
// ErrNotFound），Exec 同时记录 argv（mkdir -p 断言用）。
type memFSEngine struct {
	mu      sync.Mutex
	created []string
	execs   [][]string
	files   map[string][]byte
}

func (e *memFSEngine) Create(ctx context.Context, id string, p sandbox.Profile, labels map[string]string) (sandbox.Handle, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.created = append(e.created, id)
	return sandbox.Handle{ID: id, SandboxID: id}, nil
}

func (e *memFSEngine) Exec(ctx context.Context, h sandbox.Handle, spec sandbox.ExecSpec) (sandbox.ExecResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.execs = append(e.execs, append([]string(nil), spec.Argv...))
	// Lease.WriteFile: sh -c 'cat > "$1"' sh <path>，stdin 为文件内容。
	if len(spec.Argv) >= 5 && spec.Argv[0] == "sh" && spec.Argv[1] == "-c" && spec.Argv[2] == `cat > "$1"` {
		e.files[spec.Argv[4]] = []byte(spec.Stdin)
	}
	return sandbox.ExecResult{ExitCode: 0}, nil
}

func (e *memFSEngine) CopyFrom(ctx context.Context, h sandbox.Handle, p string) (io.ReadCloser, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, ok := e.files[p]
	if !ok {
		// docker cp 缺失路径：空 stdout 流 → Lease.ReadFile tar EOF → ErrNotFound。
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: path.Base(p), Mode: 0o644, Size: int64(len(b)), Typeflag: tar.TypeReg}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(b); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return io.NopCloser(&buf), nil
}

func (e *memFSEngine) Destroy(ctx context.Context, h sandbox.Handle) error { return nil }

func (e *memFSEngine) ListByLabels(ctx context.Context, labels map[string]string) ([]sandbox.Handle, error) {
	return nil, nil
}

func (e *memFSEngine) createdCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.created)
}

func (e *memFSEngine) mkdirCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, argv := range e.execs {
		if len(argv) >= 3 && argv[0] == "mkdir" && argv[1] == "-p" {
			n++
		}
	}
	return n
}

func newTestStore(t *testing.T) (*sandbox.SessionLeases, *memFSEngine) {
	t.Helper()
	eng := &memFSEngine{files: map[string][]byte{}}
	cfg := sandbox.DefaultConfig()
	cfg.Pool.MinReady = 0 // 测试显式驱动租约，不起热池
	mgr := sandbox.NewManager(cfg, eng, nil)
	t.Cleanup(mgr.Close)
	return sandbox.NewSessionLeases(mgr), eng
}

func TestCleanAbsPath(t *testing.T) {
	for _, bad := range []string{"", "  ", "tmp/a", "../etc/passwd", "/..", "/"} {
		if _, err := cleanAbsPath(bad); err == nil {
			t.Errorf("cleanAbsPath(%q) should reject", bad)
		}
	}
	// 绝对路径中的 .. 由 path.Clean 在根边界内收敛（不会逃逸出沙箱命名空间），
	// 归一后仍是合法绝对路径；是否可写由 cleanWritablePath 的前缀门禁裁决。
	if got, err := cleanAbsPath("/../etc"); err != nil || got != "/etc" {
		t.Errorf("cleanAbsPath(/../etc) = %q, %v, want /etc", got, err)
	}
	if got, err := cleanAbsPath("/tmp/../workspace/out/a.txt"); err != nil || got != "/workspace/out/a.txt" {
		t.Errorf("cleanAbsPath normalize = %q, %v", got, err)
	}
}

func TestCleanWritablePath(t *testing.T) {
	for _, bad := range []string{"/etc/passwd", "/workspace/in/a", "/root/x"} {
		if _, err := cleanWritablePath(bad); err == nil {
			t.Errorf("cleanWritablePath(%q) should reject (rootfs read-only)", bad)
		}
	}
	for _, ok := range []string{"/tmp/a.txt", "/workspace/out/b.bin"} {
		if _, err := cleanWritablePath(ok); err != nil {
			t.Errorf("cleanWritablePath(%q) rejected: %v", ok, err)
		}
	}
}

func TestNewToolsetPrunesOnNilStore(t *testing.T) {
	if ts := NewToolset(nil); ts != nil {
		t.Fatalf("nil store must prune toolset, got %d tools", len(ts))
	}
	store, _ := newTestStore(t)
	ts := NewToolset(store)
	if len(ts) != 2 || ts[0].Declaration().Name != ToolWrite || ts[1].Declaration().Name != ToolRead {
		t.Fatalf("toolset = %+v, want [%s %s]", ts, ToolWrite, ToolRead)
	}
}

// 同 key 写后读全链路：utf8 文本与 base64 二进制双路回环；全程只创建一个
// 沙箱（共享 SessionLeases），写入经 mkdir -p + exec stdin（r2 #2）。
func TestWriteReadRoundtrip(t *testing.T) {
	store, eng := newTestStore(t)
	ctx := context.Background()
	key := "app/u/s1"

	wout, err := writeFn(ctx, store, key, []byte(`{"path":"/tmp/note.txt","content":"hello 沙箱"}`))
	if err != nil {
		t.Fatalf("writeFn utf8: %v", err)
	}
	rout, err := readFn(ctx, store, key, []byte(`{"path":"/tmp/note.txt"}`))
	if err != nil {
		t.Fatalf("readFn utf8: %v", err)
	}
	rm := rout.(map[string]any)
	if rm["encoding"] != "utf8" || rm["content"] != "hello 沙箱" || rm["truncated"] != false {
		t.Errorf("utf8 roundtrip = %+v", rm)
	}
	if rm["sandbox_id"] != wout.(map[string]any)["sandbox_id"] {
		t.Errorf("write/read sandbox_id differ: %v vs %v", wout, rm)
	}

	raw := []byte{0xff, 0x00, 0x01, 0x02}
	args := []byte(`{"path":"/workspace/out/bin.dat","encoding":"base64","content":"` + base64.StdEncoding.EncodeToString(raw) + `"}`)
	if _, err := writeFn(ctx, store, key, args); err != nil {
		t.Fatalf("writeFn base64: %v", err)
	}
	rout2, err := readFn(ctx, store, key, []byte(`{"path":"/workspace/out/bin.dat"}`))
	if err != nil {
		t.Fatalf("readFn base64: %v", err)
	}
	rm2 := rout2.(map[string]any)
	if rm2["encoding"] != "base64" || rm2["content"] != base64.StdEncoding.EncodeToString(raw) {
		t.Errorf("base64 roundtrip = %+v", rm2)
	}

	if n := eng.createdCount(); n != 1 {
		t.Errorf("created = %d, want 1 (shared session lease)", n)
	}
	if n := eng.mkdirCount(); n < 2 {
		t.Errorf("mkdir -p execs = %d, want >= 2 (per write)", n)
	}
}

// 活租约内读缺失文件 → 结构化 file-not-found（可被模型纠正），不触发
// withLease 的 Evict+重建重试（created 保持 1）。
func TestReadMissingFileInLiveLease(t *testing.T) {
	store, eng := newTestStore(t)
	ctx := context.Background()
	key := "app/u/s1"
	if _, err := writeFn(ctx, store, key, []byte(`{"path":"/tmp/a","content":"x"}`)); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	_, err := readFn(ctx, store, key, []byte(`{"path":"/tmp/missing"}`))
	if err == nil || !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("readFn missing = %v, want file-not-found", err)
	}
	if n := eng.createdCount(); n != 1 {
		t.Errorf("created = %d, want 1 (no evict/retry on live-lease miss)", n)
	}
}

func TestWriteRejectsBadArgs(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	for _, args := range []string{
		`{"path":"/etc/x","content":"a"}`,   // 只读前缀
		`{"path":"tmp/x","content":"a"}`,    // 相对路径
		`{"path":"/tmp/x","content":"a","encoding":"base64"}`,   // 长度非 4 倍数，非法 base64
		`{"path":"/tmp/x","content":"!!!","encoding":"base64"}`, // 非法字符
		`{"path":"/tmp/x","content":"a","encoding":"hex"}`,      // 未知编码
		`{}`, // 缺 path
	} {
		if _, err := writeFn(ctx, store, "k", []byte(args)); err == nil {
			t.Errorf("writeFn(%s) should fail", args)
		}
	}
}

func TestSessionKeyFromCtx(t *testing.T) {
	if _, err := sessionKeyFromCtx(context.Background()); err == nil {
		t.Fatal("no invocation → error expected")
	}
	inv := trpcagent.NewInvocation()
	inv.Session = &trpcsession.Session{ID: "s1", AppName: "app", UserID: "u"}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	key, err := sessionKeyFromCtx(ctx)
	if err != nil || key != "app/u/s1" {
		t.Fatalf("sessionKeyFromCtx = %q, %v", key, err)
	}
}

// 经 Declaration/Call 的端到端：write 与 read 工具在同一 invocation 上下文
// 下共享一个沙箱租约（与 execute_code 会话粘性的同一派生键）。
func TestToolCallSharesSessionSandbox(t *testing.T) {
	store, eng := newTestStore(t)
	ts := NewToolset(store)
	writeT, readT := ts[0], ts[1]

	inv := trpcagent.NewInvocation()
	inv.Session = &trpcsession.Session{ID: "s9", AppName: "app", UserID: "u"}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	if _, err := writeT.Call(ctx, []byte(`{"path":"/tmp/shared.txt","content":"via-call"}`)); err != nil {
		t.Fatalf("write Call: %v", err)
	}
	out, err := readT.Call(ctx, []byte(`{"path":"/tmp/shared.txt"}`))
	if err != nil {
		t.Fatalf("read Call: %v", err)
	}
	if out.(map[string]any)["content"] != "via-call" {
		t.Errorf("call roundtrip = %+v", out)
	}
	if n := eng.createdCount(); n != 1 {
		t.Errorf("created = %d, want 1 (write/read tools share session sandbox)", n)
	}
}
