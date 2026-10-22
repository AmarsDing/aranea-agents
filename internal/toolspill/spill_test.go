package toolspill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

func newTestStore(t *testing.T, mutate func(*Options)) (*LocalFS, string) {
	t.Helper()
	ws := t.TempDir()
	opts := Options{WorkspaceRoot: ws, Logger: loggateway.NewNoop()}
	if mutate != nil {
		mutate(&opts)
	}
	s, err := NewLocalFS(opts)
	if err != nil {
		t.Fatalf("NewLocalFS: %v", err)
	}
	return s, ws
}

func mkGet(t *testing.T, s *LocalFS, session, loc string) Record {
	t.Helper()
	rec, err := s.Get(context.Background(), session, loc)
	if err != nil {
		t.Fatalf("Get(%q): %v", loc, err)
	}
	return rec
}

// ---------------------------------------------------------------------------
// put/get round trip & locator grammar
// ---------------------------------------------------------------------------

func TestPutGetRoundTrip(t *testing.T) {
	s, _ := newTestStore(t, nil)
	payload := []byte(strings.Repeat("hello-世界-", 2048)) // multi-byte + ASCII
	loc, err := s.Put(context.Background(), Record{SessionID: "sess-1", Tool: "web_search", Content: payload})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !strings.HasPrefix(loc.String(), Scheme+StoreFS+"/") {
		t.Fatalf("locator %q missing scheme prefix", loc.String())
	}
	if loc.Bytes != int64(len(payload)) {
		t.Fatalf("locator bytes = %d, want %d", loc.Bytes, len(payload))
	}
	parsed, err := ParseLocator(loc.String())
	if err != nil {
		t.Fatalf("ParseLocator: %v", err)
	}
	if parsed.Store != StoreFS || parsed.ID != loc.ID {
		t.Fatalf("round-trip mismatch: %+v vs %+v", parsed, loc)
	}

	// File landed at <ws>/.aranea_spill/<sha16>/<id>.spill.json
	wantDir := filepath.Join(s.Root(), SessionDirName("sess-1"))
	if _, err := os.Stat(wantDir); err != nil {
		t.Fatalf("session dir missing: %v", err)
	}

	rec := mkGet(t, s, "sess-1", loc.String())
	if string(rec.Content) != string(payload) {
		t.Fatalf("content mismatch: got %d bytes, want %d", len(rec.Content), len(payload))
	}
	if rec.Tool != "web_search" || rec.SessionID != "sess-1" {
		t.Fatalf("metadata mismatch: %+v", rec)
	}

	fi, err := s.Stat(context.Background(), "sess-1", loc.String())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.ByteSize != int64(len(payload)) || fi.Tool != "web_search" {
		t.Fatalf("Stat mismatch: %+v", fi)
	}
}

func TestParseLocatorShapes(t *testing.T) {
	good := "spill://fs/" + SessionDirName("s") + "/" + strings.Repeat("a", 32) + ".spill.json"
	if _, err := ParseLocator(good); err != nil {
		t.Fatalf("good locator rejected: %v", err)
	}
	bad := []string{
		"",
		"spill://",
		"blob_12345",
		"artifact://x/y",
		"spill:///",
		"spill://fs/",
		"spill://FS/x",
		"spill://fs/../etc",
		"spill://fs/a\\b",
		"spill://fs//double",
	}
	for _, tc := range bad {
		if _, err := ParseLocator(tc); err == nil {
			t.Fatalf("bad locator %q accepted", tc)
		}
	}
	// A future pg locator parses structurally here (per-store rules live in
	// the store, not the parser).
	if _, err := ParseLocator("spill://pg/abc123"); err != nil {
		t.Fatalf("future pg locator must parse at the grammar level: %v", err)
	}
}

// ---------------------------------------------------------------------------
// quotas
// ---------------------------------------------------------------------------

func TestPutFileSizeLimit(t *testing.T) {
	s, _ := newTestStore(t, func(o *Options) { o.MaxFileBytes = 128 })
	exact := make([]byte, 128)
	if _, err := s.Put(context.Background(), Record{SessionID: "s", Content: exact}); err != nil {
		t.Fatalf("payload at the limit must be accepted: %v", err)
	}
	over := make([]byte, 129)
	if _, err := s.Put(context.Background(), Record{SessionID: "s", Content: over}); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("payload 1 byte over must fail with ErrFileTooLarge, got %v", err)
	}
}

func TestSessionQuota(t *testing.T) {
	// Session cap counts on-disk file bytes (envelope included).
	s, _ := newTestStore(t, func(o *Options) {
		o.MaxFileBytes = 256
		o.MaxSessionBytes = 450
	})
	payload := make([]byte, 200)
	if _, err := s.Put(context.Background(), Record{SessionID: "s", Content: payload}); err != nil {
		t.Fatalf("first put must fit the quota: %v", err)
	}
	// The first envelope alone (~410B on disk) plus 200 more payload bytes
	// exceeds the 450B session cap.
	if _, err := s.Put(context.Background(), Record{SessionID: "s", Content: payload}); !errors.Is(err, ErrSessionQuota) {
		t.Fatalf("quota overflow error = %v, want ErrSessionQuota", err)
	}
	// A different session has an independent quota on the same store.
	if _, err := s.Put(context.Background(), Record{SessionID: "other-session", Content: payload}); err != nil {
		t.Fatalf("other session unaffected by quota: %v", err)
	}
}

// ---------------------------------------------------------------------------
// concurrency: idempotent, never overwriting
// ---------------------------------------------------------------------------

func TestConcurrentPuts(t *testing.T) {
	s, _ := newTestStore(t, nil)
	const workers = 16
	const putsEach = 4
	locs := make([]Locator, 0, workers*putsEach)
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make(chan error, workers*putsEach)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < putsEach; i++ {
				payload := []byte(fmt.Sprintf("worker-%d-put-%d-%s", w, i, strings.Repeat("x", 256)))
				loc, err := s.Put(context.Background(), Record{SessionID: "shared", Tool: "conc_tool", Content: payload})
				if err != nil {
					errs <- err
					return
				}
				mu.Lock()
				locs = append(locs, loc)
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent put: %v", err)
	}
	if len(locs) != workers*putsEach {
		t.Fatalf("got %d locators, want %d", len(locs), workers*putsEach)
	}
	seen := map[string]bool{}
	for _, loc := range locs {
		if seen[loc.ID] {
			t.Fatalf("duplicate id %q — O_EXCL uniqueness violated", loc.ID)
		}
		seen[loc.ID] = true
		rec := mkGet(t, s, "shared", loc.String())
		if !strings.HasPrefix(string(rec.Content), "worker-") {
			t.Fatalf("corrupt payload at %q", loc.String())
		}
	}
	// No temp files left behind.
	files, err := os.ReadDir(filepath.Join(s.Root(), SessionDirName("shared")))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f.Name(), tmpSuffix) {
			t.Fatalf("leftover temp file %q", f.Name())
		}
	}
}

// ---------------------------------------------------------------------------
// symlink attacks
// ---------------------------------------------------------------------------

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation unsupported in this environment: %v", err)
	}
}

func TestPutRejectsPrePlantedSessionDirSymlink(t *testing.T) {
	s, _ := newTestStore(t, nil)
	sessionDir := filepath.Join(s.Root(), SessionDirName("victim"))
	outside := t.TempDir()
	if err := os.MkdirAll(s.Root(), 0o700); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	symlinkOrSkip(t, outside, sessionDir)
	_, err := s.Put(context.Background(), Record{SessionID: "victim", Content: []byte("data")})
	if !errors.Is(err, ErrSecurity) {
		t.Fatalf("pre-planted symlink session dir must fail with ErrSecurity, got %v", err)
	}
	// The outside directory must remain untouched.
	entries, rerr := os.ReadDir(outside)
	if rerr != nil || len(entries) != 0 {
		t.Fatalf("payload escaped into symlink target: entries=%v err=%v", entries, rerr)
	}
}

func TestGetRejectsSymlinkedFile(t *testing.T) {
	s, _ := newTestStore(t, nil)
	loc, err := s.Put(context.Background(), Record{SessionID: "s", Content: []byte("legit")})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	_, file, err := splitFSID(loc.ID)
	if err != nil {
		t.Fatalf("splitFSID: %v", err)
	}
	victim := filepath.Join(s.Root(), SessionDirName("s"), file)
	if err := os.Remove(victim); err != nil {
		t.Fatalf("remove legit file: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("top-secret"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	symlinkOrSkip(t, outside, victim)
	_, err = s.Get(context.Background(), "s", loc.String())
	if !errors.Is(err, ErrSecurity) {
		t.Fatalf("symlinked file read must fail with ErrSecurity, got %v", err)
	}
}

func TestNewLocalFSRejectsSymlinkRoot(t *testing.T) {
	s, ws := newTestStore(t, nil)
	if s == nil {
		t.Fatal("nil store")
	}
	// Replace the created store root with a symlink and try to open it.
	if err := os.RemoveAll(filepath.Join(ws, DefaultDirName)); err != nil {
		t.Fatalf("remove root: %v", err)
	}
	symlinkOrSkip(t, t.TempDir(), filepath.Join(ws, DefaultDirName))
	if _, err := NewLocalFS(Options{WorkspaceRoot: ws, Logger: loggateway.NewNoop()}); !errors.Is(err, ErrSecurity) {
		t.Fatalf("symlinked store root must fail with ErrSecurity, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// get: traversal & ownership
// ---------------------------------------------------------------------------

func TestGetRejectsTraversalAndBadIDs(t *testing.T) {
	s, _ := newTestStore(t, nil)
	sha := SessionDirName("s")
	id32 := strings.Repeat("ab", 16)
	cases := map[string]error{
		"spill://fs/../" + id32 + ".spill.json":                              ErrInvalidLocator,
		"spill://fs/./x":                                                     ErrInvalidLocator,
		"spill://fs/" + sha + "/../../etc/passwd":                            ErrInvalidLocator,
		"spill://fs//etc/passwd":                                             ErrInvalidLocator,
		"spill://fs/C:/Windows/win.ini":                                      ErrInvalidLocator,
		"spill://fs/" + sha + "/" + id32 + ".json":                           ErrInvalidLocator, // wrong suffix
		"spill://fs/" + sha + "/" + id32 + ".spill.json/extra":               ErrInvalidLocator,
		"spill://fs/XYZW/" + id32 + ".spill.json":                            ErrInvalidLocator,
		"spill://fs/" + sha + "/" + id32:                                     ErrInvalidLocator,
		"spill://pg/some-blob-id":                                            ErrUnsupportedStore,
		"spill://fs/" + SessionDirName("other") + "/" + id32 + ".spill.json": ErrNotFound, // cross-session
	}
	for loc, want := range cases {
		_, err := s.Get(context.Background(), "s", loc)
		if !errors.Is(err, want) {
			t.Errorf("Get(%q) error = %v, want %v", loc, err, want)
		}
	}
}

func TestGetCrossSessionDenied(t *testing.T) {
	s, _ := newTestStore(t, nil)
	loc, err := s.Put(context.Background(), Record{SessionID: "owner", Content: []byte("mine")})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.Get(context.Background(), "intruder", loc.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-session read must be ErrNotFound, got %v", err)
	}
	if _, err := s.Get(context.Background(), "", loc.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty session read must be ErrNotFound, got %v", err)
	}
}

// TestGetMissingSessionDirReturnsNotFound pins the resolveOwned contract:
// a structurally valid locator whose session directory does not exist (never
// spilled, or already reclaimed by GC) is a plain miss — ErrNotFound, never
// a raw *os.PathError leaking filesystem detail.
func TestGetMissingSessionDirReturnsNotFound(t *testing.T) {
	s, _ := newTestStore(t, nil)
	loc := "spill://fs/" + SessionDirName("ghost") + "/" + strings.Repeat("ef", 16) + ".spill.json"
	if _, err := s.Get(context.Background(), "ghost", loc); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on missing session dir = %v, want ErrNotFound", err)
	}
	if _, err := s.Stat(context.Background(), "ghost", loc); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Stat on missing session dir = %v, want ErrNotFound", err)
	}
}

// TestPutWithRetry pins the "at most one retry" contract from design §3.5.
func TestPutWithRetry(t *testing.T) {
	// flaky fails the first N Puts, then succeeds.
	flaky := func(failures int) Store {
		var calls int
		return StoreFunc{
			put: func(ctx context.Context, rec Record) (Locator, error) {
				calls++
				if calls <= failures {
					return Locator{}, errors.New("transient disk error")
				}
				return Locator{Store: StoreFS, ID: "ok/ok.spill.json", Bytes: 1}, nil
			},
		}
	}
	loc, err := PutWithRetry(context.Background(), flaky(1), Record{SessionID: "s", Content: []byte("x")})
	if err != nil {
		t.Fatalf("single transient failure must be retried to success: %v", err)
	}
	if loc.ID == "" {
		t.Fatal("locator must come from the successful attempt")
	}
	if _, err := PutWithRetry(context.Background(), flaky(2), Record{SessionID: "s", Content: []byte("x")}); err == nil {
		t.Fatal("two consecutive failures must surface (no third attempt)")
	}
	if _, err := PutWithRetry(context.Background(), flaky(0), Record{SessionID: "s", Content: []byte("x")}); err != nil {
		t.Fatalf("healthy store must succeed on first attempt: %v", err)
	}
	// Context cancellation is never retried: an error with a dead context
	// returns on the first attempt instead of burning a second write.
	ctx, cancel := context.WithCancel(context.Background())
	var attempts int
	store := StoreFunc{put: func(context.Context, Record) (Locator, error) {
		attempts++
		cancel()
		return Locator{}, errors.New("disk died mid-write")
	}}
	if _, err := PutWithRetry(ctx, store, Record{SessionID: "s", Content: []byte("x")}); err == nil {
		t.Fatal("failing store must surface its error")
	}
	if attempts != 1 {
		t.Fatalf("cancelled context must not be retried: attempts = %d, want 1", attempts)
	}
}

// StoreFunc is a minimal Store test double: only Put is exercised by
// PutWithRetry; the remaining methods fail loudly if reached.
type StoreFunc struct {
	put  func(ctx context.Context, rec Record) (Locator, error)
	get  func(ctx context.Context, sessionID, locator string) (Record, error)
	stat func(ctx context.Context, sessionID, locator string) (FileInfo, error)
	gc   func(ctx context.Context, now time.Time) (GCStats, error)
}

func (f StoreFunc) Put(ctx context.Context, rec Record) (Locator, error) { return f.put(ctx, rec) }
func (f StoreFunc) Get(ctx context.Context, sessionID, locator string) (Record, error) {
	return f.get(ctx, sessionID, locator)
}
func (f StoreFunc) Stat(ctx context.Context, sessionID, locator string) (FileInfo, error) {
	return f.stat(ctx, sessionID, locator)
}
func (f StoreFunc) GC(ctx context.Context, now time.Time) (GCStats, error) {
	return f.gc(ctx, now)
}

// ---------------------------------------------------------------------------
// GC
// ---------------------------------------------------------------------------

func ageFile(t *testing.T, path string, age time.Duration) {
	t.Helper()
	old := time.Now().Add(-age)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
}

func TestGCExpiresOldKeepsNewAndPrunesEmptyDir(t *testing.T) {
	s, _ := newTestStore(t, func(o *Options) { o.TTL = time.Hour })
	oldLoc, err := s.Put(context.Background(), Record{SessionID: "old-sess", Content: []byte("old")})
	if err != nil {
		t.Fatalf("put old: %v", err)
	}
	newLoc, err := s.Put(context.Background(), Record{SessionID: "new-sess", Content: []byte("new")})
	if err != nil {
		t.Fatalf("put new: %v", err)
	}
	_, oldFile, _ := splitFSID(oldLoc.ID)
	ageFile(t, filepath.Join(s.Root(), SessionDirName("old-sess"), oldFile), 2*time.Hour)

	// Stale temp file should also be collected.
	tmpStale := filepath.Join(s.Root(), SessionDirName("old-sess"), strings.Repeat("cd", 16)+tmpSuffix)
	if err := os.WriteFile(tmpStale, []byte("junk"), 0o600); err != nil {
		t.Fatalf("write stale tmp: %v", err)
	}
	ageFile(t, tmpStale, 2*time.Hour)

	stats, err := s.GC(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if stats.Removed != 2 {
		t.Fatalf("GC removed = %d, want 2", stats.Removed)
	}
	// Old session dir pruned (empty after collection).
	if _, err := os.Stat(filepath.Join(s.Root(), SessionDirName("old-sess"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty session dir must be pruned, stat err=%v", err)
	}
	// New file intact and readable.
	rec := mkGet(t, s, "new-sess", newLoc.String())
	if string(rec.Content) != "new" {
		t.Fatalf("fresh payload corrupted by GC")
	}
	// GC is idempotent.
	stats2, err := s.GC(context.Background(), time.Now())
	if err != nil || stats2.Removed != 0 {
		t.Fatalf("second GC must be a no-op: stats=%+v err=%v", stats2, err)
	}
}

func TestGCEnforcesTotalCapOldestFirst(t *testing.T) {
	s, _ := newTestStore(t, func(o *Options) {
		o.TTL = 24 * time.Hour // nothing expires by age in this test
		o.MaxTotalBytes = 700
	})
	put := func(session string, age time.Duration) Locator {
		loc, err := s.Put(context.Background(), Record{SessionID: session, Content: make([]byte, 300)})
		if err != nil {
			t.Fatalf("put %s: %v", session, err)
		}
		_, f, _ := splitFSID(loc.ID)
		ageFile(t, filepath.Join(s.Root(), SessionDirName(session), f), age)
		return loc
	}
	oldest := put("a", 5*time.Hour)
	middle := put("b", 3*time.Hour)
	newest := put("c", time.Hour)
	_ = middle

	stats, err := s.GC(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	// Three ~300B payloads produce ~530B envelopes each (~1590B total).
	// Cap 700B -> the two oldest must be evicted, the newest survives.
	if stats.Removed != 2 {
		t.Fatalf("total-cap eviction removed %d files, want 2", stats.Removed)
	}
	if _, err := s.Get(context.Background(), "a", oldest.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("oldest spill must be evicted, err=%v", err)
	}
	if _, err := s.Get(context.Background(), "c", newest.String()); err != nil {
		t.Fatalf("newest spill must survive eviction: %v", err)
	}
}

// ---------------------------------------------------------------------------
// permission bits (unix only)
// ---------------------------------------------------------------------------

func TestPermissions0700And0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows does not honor unix mode bits")
	}
	s, _ := newTestStore(t, nil)
	loc, err := s.Put(context.Background(), Record{SessionID: "perm", Content: []byte("x")})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	assertMode := func(path string, want os.FileMode) {
		t.Helper()
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %q: %v", path, err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Fatalf("mode(%q) = %o, want %o", path, got, want)
		}
	}
	assertMode(s.Root(), 0o700)
	assertMode(filepath.Join(s.Root(), SessionDirName("perm")), 0o700)
	_, file, _ := splitFSID(loc.ID)
	assertMode(filepath.Join(s.Root(), SessionDirName("perm"), file), 0o600)
}

// ---------------------------------------------------------------------------
// helpers contract
// ---------------------------------------------------------------------------

func TestDepsKillSwitchAndDedupeKey(t *testing.T) {
	var d *Deps
	if d.Enabled() {
		t.Fatal("nil Deps must be disabled")
	}
	if (&Deps{}).Enabled() {
		t.Fatal("nil Store must be disabled")
	}
	ts := time.Unix(1_700_000_123, 0)
	key := SpillFailedDedupeKey("sess", ts)
	want := fmt.Sprintf("spill_failed:sess:%d", ts.Unix()/300)
	if key != want {
		t.Fatalf("dedupe key = %q, want %q", key, want)
	}
	var hooked string
	deps := &Deps{
		Now:   func() time.Time { return ts },
		Hooks: Hooks{OnSpillFailed: func(_ context.Context, k string, _ error) { hooked = k }},
	}
	deps.ReportSpillFailed(context.Background(), loggateway.NewNoop(), "step", "sess", errors.New("disk gone"))
	if hooked != key {
		t.Fatalf("hook received %q, want %q", hooked, key)
	}
}

func TestPointerTextStableShape(t *testing.T) {
	loc := Locator{Store: StoreFS, ID: "abc/def.spill.json", Bytes: 42}
	p1 := PointerText("web_search", 42, loc)
	p2 := PointerText("web_search", 42, loc)
	if p1 != p2 {
		t.Fatalf("pointer text must be deterministic: %q vs %q", p1, p2)
	}
	if !strings.Contains(p1, "42") || !strings.Contains(p1, "web_search") || !strings.Contains(p1, loc.String()) {
		t.Fatalf("pointer text missing fields: %q", p1)
	}
	if !strings.Contains(p1, "read_tool_result") {
		t.Fatalf("pointer text must direct the model to read_tool_result: %q", p1)
	}
}
