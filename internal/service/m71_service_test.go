package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizsession "aranea-agents/internal/biz/session"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// secureJoin：路径逃逸防护（NFR-02 / FR-02 边界）
// ---------------------------------------------------------------------------

func TestSecureJoin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	t.Run("normal relative path", func(t *testing.T) {
		t.Parallel()
		got, err := secureJoin(root, "sub/dir/file.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rel, err := filepath.Rel(root, got)
		if err != nil || strings.HasPrefix(rel, "..") {
			t.Fatalf("result escapes root: %q", got)
		}
	})

	t.Run("empty rel returns root", func(t *testing.T) {
		t.Parallel()
		got, err := secureJoin(root, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if filepath.Clean(got) != filepath.Clean(root) {
			t.Fatalf("got %q, want root %q", got, root)
		}
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		t.Parallel()
		abs := filepath.Join(root, "other", "x.txt")
		if _, err := secureJoin(root, abs); err == nil {
			t.Fatal("expected rejection of absolute path")
		}
	})

	t.Run("dotdot escape rejected", func(t *testing.T) {
		t.Parallel()
		for _, rel := range []string{"../x.txt", "sub/../../x.txt", "..\\x.txt"} {
			if _, err := secureJoin(root, rel); err == nil {
				t.Fatalf("expected escape rejection for %q", rel)
			}
		}
	})

	t.Run("empty root rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := secureJoin("  ", "a.txt"); err == nil {
			t.Fatal("expected rejection of empty root")
		}
	})

	t.Run("symlink escape rejected", func(t *testing.T) {
		t.Parallel()
		outside := t.TempDir()
		// 穿透后的目标必须真实存在，EvalSymlinks 才能解析出逃逸路径。
		writeMemberFile(t, filepath.Join(outside, "secret.txt"), []byte("top secret"))
		link := filepath.Join(root, "link")
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("symlink not permitted: %v", err)
		}
		if _, err := secureJoin(root, "link/secret.txt"); err == nil {
			t.Fatal("expected symlink escape rejection")
		}
	})
}

// ---------------------------------------------------------------------------
// memberFileReader：只读文件操作（FR-02~FR-04 边界）
// ---------------------------------------------------------------------------

func writeMemberFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMemberFileReader_ReadText(t *testing.T) {
	t.Parallel()
	r := NewMemberFileReader(nil)
	root := t.TempDir()

	writeMemberFile(t, filepath.Join(root, "note.txt"), []byte("hello world"))
	writeMemberFile(t, filepath.Join(root, "bin.dat"), []byte{'A', 0, 'B', 'C'})
	writeMemberFile(t, filepath.Join(root, "gbk.txt"), []byte{0xC4, 0xE3, 0xBA, 0xC3}) // GBK, 非 UTF-8
	writeMemberFile(t, filepath.Join(root, "long.txt"), []byte(strings.Repeat("x", 100)))

	t.Run("read normal file", func(t *testing.T) {
		content, truncated, err := r.ReadText(root, "note.txt", 1024)
		if err != nil || content != "hello world" || truncated {
			t.Fatalf("content=%q truncated=%v err=%v", content, truncated, err)
		}
	})

	t.Run("truncation flag", func(t *testing.T) {
		content, truncated, err := r.ReadText(root, "long.txt", 10)
		if err != nil || !truncated || len(content) != 10 {
			t.Fatalf("content len=%d truncated=%v err=%v", len(content), truncated, err)
		}
	})

	t.Run("multibyte truncation backs off to rune boundary", func(t *testing.T) {
		// "中" 3 字节；maxBytes=10 落在第 4 个字符中间。截断必须回退到完整字符
		// 边界（9 字节），而非把正常文本误判为非法 UTF-8。
		writeMemberFile(t, filepath.Join(root, "zh.txt"), []byte(strings.Repeat("中", 100)))
		content, truncated, err := r.ReadText(root, "zh.txt", 10)
		if err != nil {
			t.Fatalf("valid UTF-8 text must not be rejected on truncation boundary: %v", err)
		}
		if !truncated || content != "中中中" {
			t.Fatalf("content=%q truncated=%v, want 9 bytes backed off to rune boundary", content, truncated)
		}
	})

	t.Run("binary rejected", func(t *testing.T) {
		if _, _, err := r.ReadText(root, "bin.dat", 1024); err == nil {
			t.Fatal("expected binary rejection")
		}
	})

	t.Run("non-UTF-8 rejected", func(t *testing.T) {
		if _, _, err := r.ReadText(root, "gbk.txt", 1024); err == nil {
			t.Fatal("expected non-UTF-8 rejection")
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, _, err := r.ReadText(root, "ghost.txt", 1024); err == nil {
			t.Fatal("expected not-found error")
		}
	})

	t.Run("directory rejected", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(root, "adir"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, err := r.ReadText(root, "adir", 1024); err == nil {
			t.Fatal("expected not-a-file error")
		}
	})

	t.Run("escape rejected", func(t *testing.T) {
		if _, _, err := r.ReadText(root, "../outside.txt", 1024); err == nil {
			t.Fatal("expected escape rejection")
		}
	})
}

func TestMemberFileReader_List(t *testing.T) {
	t.Parallel()
	r := NewMemberFileReader(nil)
	root := t.TempDir()

	writeMemberFile(t, filepath.Join(root, "a.txt"), []byte("a"))
	writeMemberFile(t, filepath.Join(root, "sub", "b.txt"), []byte("bb"))
	writeMemberFile(t, filepath.Join(root, "sub", "deep", "c.txt"), []byte("ccc"))
	writeMemberFile(t, filepath.Join(root, ".hidden"), []byte("h"))
	if err := os.MkdirAll(filepath.Join(root, ".hdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("depth 0 lists top level only", func(t *testing.T) {
		entries, err := r.List(root, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		var paths []string
		for _, e := range entries {
			paths = append(paths, e.Path)
		}
		joined := strings.Join(paths, ",")
		if !strings.Contains(joined, "a.txt") || !strings.Contains(joined, "sub") {
			t.Fatalf("missing expected entries: %v", paths)
		}
		if strings.Contains(joined, "b.txt") || strings.Contains(joined, "hidden") {
			t.Fatalf("depth-0 leaked nested/hidden entries: %v", paths)
		}
	})

	t.Run("deep traversal includes nested, skips hidden", func(t *testing.T) {
		entries, err := r.List(root, "", 5)
		if err != nil {
			t.Fatal(err)
		}
		var paths []string
		for _, e := range entries {
			paths = append(paths, e.Path)
		}
		joined := strings.Join(paths, ",")
		if !strings.Contains(joined, "sub/deep/c.txt") {
			t.Fatalf("missing nested entry: %v", paths)
		}
		if strings.Contains(joined, "hidden") {
			t.Fatalf("hidden entry leaked: %v", paths)
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, err := r.List(root, "ghost", 1); err == nil {
			t.Fatal("expected not-found error")
		}
	})

	t.Run("file as subdir rejected", func(t *testing.T) {
		if _, err := r.List(root, "a.txt", 1); err == nil {
			t.Fatal("expected not-a-directory error")
		}
	})

	t.Run("escape rejected", func(t *testing.T) {
		if _, err := r.List(root, "../", 1); err == nil {
			t.Fatal("expected escape rejection")
		}
	})
}

func TestMemberFileReader_Search(t *testing.T) {
	t.Parallel()
	r := NewMemberFileReader(nil)
	root := t.TempDir()

	writeMemberFile(t, filepath.Join(root, "main.go"), []byte("package x"))
	writeMemberFile(t, filepath.Join(root, "sub", "util.go"), []byte("package y"))
	writeMemberFile(t, filepath.Join(root, "sub", "README.md"), []byte("# doc"))

	t.Run("glob by base name", func(t *testing.T) {
		matches, err := r.Search(root, "*.go", 10)
		if err != nil || len(matches) != 2 {
			t.Fatalf("matches=%v err=%v", matches, err)
		}
	})

	t.Run("glob by relative path", func(t *testing.T) {
		matches, err := r.Search(root, "sub/*.md", 10)
		if err != nil || len(matches) != 1 || matches[0] != "sub/README.md" {
			t.Fatalf("matches=%v err=%v", matches, err)
		}
	})

	t.Run("empty pattern rejected", func(t *testing.T) {
		if _, err := r.Search(root, "  ", 10); err == nil {
			t.Fatal("expected empty-pattern error")
		}
	})

	t.Run("limit enforced", func(t *testing.T) {
		matches, err := r.Search(root, "*.go", 1)
		if err != nil || len(matches) != 1 {
			t.Fatalf("matches=%v err=%v", matches, err)
		}
	})
}

// ---------------------------------------------------------------------------
// mailboxWaker：会话解析 + Turn 提交（FR-07 / NFR-05）
// ---------------------------------------------------------------------------

type m71StubSessionReader struct {
	res biz.SessionListResult
	err error
}

func (s *m71StubSessionReader) SearchSessions(context.Context, biz.SessionSearchQuery) (biz.SessionListResult, error) {
	return s.res, s.err
}
func (s *m71StubSessionReader) GetSessionByID(context.Context, string) (biz.Session, error) {
	return biz.Session{}, shared.ErrNotFound
}
func (s *m71StubSessionReader) GetSessionRevision(context.Context, string) (int64, error) {
	return 0, nil
}
func (s *m71StubSessionReader) ListSessionsForBatch(context.Context, biz.SessionSearchQuery) ([]biz.Session, error) {
	return nil, nil
}
func (s *m71StubSessionReader) ListSessionsByIDs(context.Context, []string) ([]biz.Session, error) {
	return nil, nil
}
func (s *m71StubSessionReader) ListActiveAgentUserKeys(context.Context, int) ([]bizsession.AgentUserKey, error) {
	return nil, nil
}

type m71StubSessionWriter struct {
	created biz.Session
	err     error
	calls   int
	// done, when non-nil, is signalled after each CreateSession returns so
	// tests can synchronise with the asynchronous wake delivery.
	done chan struct{}
}

func (s *m71StubSessionWriter) CreateSession(_ context.Context, in biz.Session) (biz.Session, error) {
	s.calls++
	s.created = in
	defer func() {
		if s.done != nil {
			s.done <- struct{}{}
		}
	}()
	if s.err != nil {
		return biz.Session{}, s.err
	}
	return in, nil
}
func (s *m71StubSessionWriter) UpdateSessionTitle(_ context.Context, id, _ string) (biz.Session, error) {
	return biz.Session{ID: id}, nil
}
func (s *m71StubSessionWriter) UpdateSession(_ context.Context, id string, _ biz.SessionUpdateFields) (biz.Session, error) {
	return biz.Session{ID: id}, nil
}
func (s *m71StubSessionWriter) UpdateSessionMetadataKey(context.Context, string, string, string) error {
	return nil
}
func (s *m71StubSessionWriter) RestoreSession(_ context.Context, id string) (biz.Session, error) {
	return biz.Session{ID: id}, nil
}
func (s *m71StubSessionWriter) BumpSessionRevision(context.Context, string) (int64, error) {
	return 1, nil
}

type m71StubTurnGateway struct {
	gotInput biz.TurnInput
	err      error
	calls    int
	// done, when non-nil, is signalled after each ExecuteTurn returns so tests
	// can synchronise with the asynchronous wake delivery.
	done chan struct{}
}

func (s *m71StubTurnGateway) ExecuteTurn(_ context.Context, in biz.TurnInput) (biz.TurnResult, error) {
	s.calls++
	s.gotInput = in
	defer func() {
		if s.done != nil {
			s.done <- struct{}{}
		}
	}()
	return biz.TurnResult{}, s.err
}
func (s *m71StubTurnGateway) RunNativeTurn(context.Context, biz.TurnInput) (biz.ChatMessage, biz.ChatMessage, error) {
	return biz.ChatMessage{}, biz.ChatMessage{}, nil
}
func (s *m71StubTurnGateway) RunNativeTurnWithOutcome(context.Context, biz.TurnInput) (biz.TurnResult, error) {
	return biz.TurnResult{}, nil
}

func newM71Waker(reader biz.SessionReader, writer biz.SessionWriter, gw biz.TurnExecutorGateway) biz.MailboxWaker {
	w := NewMailboxWaker(reader, writer, loggateway.NewNoop())
	if setter, ok := w.(interface{ SetTurnGateway(biz.TurnExecutorGateway) }); ok {
		setter.SetTurnGateway(gw)
	}
	return w
}

func TestMailboxWaker_NilGatewayNoop(t *testing.T) {
	t.Parallel()
	w := NewMailboxWaker(&m71StubSessionReader{}, &m71StubSessionWriter{}, loggateway.NewNoop())
	if err := w.WakeDeptLead(context.Background(), "lead-a", "hint"); err != nil {
		t.Fatalf("nil gateway must be a no-op, got %v", err)
	}
}

func TestMailboxWaker_ReusesActiveSession(t *testing.T) {
	t.Parallel()
	reader := &m71StubSessionReader{res: biz.SessionListResult{
		Items: []biz.Session{{ID: "sess-existing"}},
	}}
	writer := &m71StubSessionWriter{}
	gw := &m71StubTurnGateway{done: make(chan struct{}, 1)}
	w := newM71Waker(reader, writer, gw)

	if err := w.WakeDeptLead(context.Background(), "lead-a", "有新消息"); err != nil {
		t.Fatal(err)
	}
	waitM71Signal(t, gw.done)
	if writer.calls != 0 {
		t.Fatalf("must not create session when active one exists, got %d creates", writer.calls)
	}
	if gw.calls != 1 || gw.gotInput.SessionID != "sess-existing" || gw.gotInput.Content != "有新消息" {
		t.Fatalf("unexpected turn input: %+v", gw.gotInput)
	}
}

func TestMailboxWaker_CreatesSessionWhenNone(t *testing.T) {
	t.Parallel()
	writer := &m71StubSessionWriter{}
	gw := &m71StubTurnGateway{done: make(chan struct{}, 1)}
	w := newM71Waker(&m71StubSessionReader{}, writer, gw)

	if err := w.WakeDeptLead(context.Background(), "lead-a", "hint"); err != nil {
		t.Fatal(err)
	}
	waitM71Signal(t, gw.done)
	if writer.calls != 1 {
		t.Fatalf("expected 1 session create, got %d", writer.calls)
	}
	created := writer.created
	if created.AgentID != "lead-a" || created.OwnerType != "agent" || created.Title == "" {
		t.Fatalf("unexpected created session: %+v", created)
	}
	if gw.gotInput.SessionID != created.ID {
		t.Fatalf("turn must target the created session %q, got %q", created.ID, gw.gotInput.SessionID)
	}
}

func TestMailboxWaker_SearchErrorFallsBackToCreate(t *testing.T) {
	t.Parallel()
	reader := &m71StubSessionReader{err: errors.New("db busy")}
	writer := &m71StubSessionWriter{}
	gw := &m71StubTurnGateway{done: make(chan struct{}, 1)}
	w := newM71Waker(reader, writer, gw)

	if err := w.WakeDeptLead(context.Background(), "lead-a", "hint"); err != nil {
		t.Fatal(err)
	}
	waitM71Signal(t, gw.done)
	if writer.calls != 1 {
		t.Fatalf("search failure must fall back to create, got %d creates", writer.calls)
	}
}

func TestMailboxWaker_CreateFailureBestEffort(t *testing.T) {
	t.Parallel()
	writer := &m71StubSessionWriter{err: errors.New("disk full"), done: make(chan struct{}, 1)}
	gw := &m71StubTurnGateway{}
	w := newM71Waker(&m71StubSessionReader{}, writer, gw)

	// Wake delivery is asynchronous and best-effort (NFR-05): a session-create
	// failure is logged, never propagated to the caller.
	if err := w.WakeDeptLead(context.Background(), "lead-a", "hint"); err != nil {
		t.Fatalf("wake must be best-effort, got %v", err)
	}
	waitM71Signal(t, writer.done)
	if gw.calls != 0 {
		t.Fatal("no turn expected when session resolution fails")
	}
}

func TestMailboxWaker_TurnFailureBestEffort(t *testing.T) {
	t.Parallel()
	reader := &m71StubSessionReader{res: biz.SessionListResult{
		Items: []biz.Session{{ID: "sess-1"}},
	}}
	gw := &m71StubTurnGateway{err: errors.New("turn busy"), done: make(chan struct{}, 1)}
	w := newM71Waker(reader, &m71StubSessionWriter{}, gw)

	// Turn failure is logged inside the wake goroutine, never propagated.
	if err := w.WakeDeptLead(context.Background(), "lead-a", "hint"); err != nil {
		t.Fatalf("wake must be best-effort, got %v", err)
	}
	waitM71Signal(t, gw.done)
	if gw.calls != 1 {
		t.Fatalf("expected 1 turn attempt, got %d", gw.calls)
	}
}

// waitM71Signal waits for one stub signal, failing the test on timeout.
func waitM71Signal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for asynchronous wake delivery")
	}
}
