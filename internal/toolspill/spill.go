// Package toolspill implements the unified "truncation point = preservation
// point" contract for oversized tool results (P0-2,
// docs/reports/2026-10-21-review-harness-p0-gap-solutions.md §3).
//
// A SpillStore persists full tool payloads and hands back a SpillLocator
// (spill://<store>/<id>) that the model can resolve later through
// read_tool_result. The first implementation is LocalFS, which writes into a
// workspace-private directory with a strict security posture:
//
//   - store root "<workspace>/.aranea_spill" created with mode 0700 (best
//     effort on Windows ACLs; Unix mode bits asserted in tests);
//   - one subdirectory per session named sha256(sessionID)[:16] — the hash
//     scopes reads (cross-session Get is denied) and keeps raw session IDs
//     off disk;
//   - files "<id>.spill.json" written O_WRONLY|O_CREATE|O_EXCL to a random
//     "<id>.tmp-spill" sibling and atomically renamed, so a reader never
//     observes a partial file and a pre-planted name can never be
//     overwritten;
//   - symlink defence in depth: the root is canonicalized
//     (Abs+EvalSymlinks) at construction, every Lstat-visible component
//     (root / session dir / final file) must not be a symlink, and the open
//     handle is cross-checked against Lstat with os.SameFile;
//   - per-file and per-session byte quotas; TTL-based GC plus an optional
//     total-cap oldest-first eviction.
//
// Event levels (AS-EVT-01; see review doc §9.3 — channels are wired in
// batch B, this package only classifies and reports through Hooks):
//
//	tool_result.spilled      — Informational: high-frequency, log-only,
//	                           never placed on the bus.
//	tool_result.spill_failed — Important: BlockUpTo + async persistence
//	                           (decision record). Batch A logs at Error and
//	                           invokes Hooks.OnSpillFailed with a 5-minute
//	                           per-session dedupe key.
//	tool_result.gc_failed    — Important: one aggregated report per GC
//	                           cycle; GC returns the first error seen while
//	                           continuing through the walk.
package toolspill

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// Scheme is the locator URI scheme every store implementation shares.
// read_tool_result dispatches on this prefix.
const Scheme = "spill://"

// StoreFS names the local-filesystem store kind encoded in locators.
const StoreFS = "fs"

// StorePG names the PG row store kind encoded in locators (C-5 立项，
// 91-multi-replica-spill-store）：实现位 internal/data/tool_spill_pg_store.go，
// locator 形态 spill://pg/<id32>（会话所有权走行 session_hash 列过滤，
// locator 不携带会话前缀）。
const StorePG = "pg"

// EnvToolSpillStore 选择 Store 实现的 env flag（C-5）：缺省/空/非法值=fs
// 档字节级旧行为；pg=PG 行存储档。
const EnvToolSpillStore = "ARANEA_TOOLSPILL_STORE"

const (
	// DefaultDirName is the workspace-private spill directory name.
	DefaultDirName = ".aranea_spill"
	// DefaultMaxFileBytes caps a single spilled payload (design §3.4).
	DefaultMaxFileBytes = 10 << 20 // 10 MiB
	// DefaultMaxSessionBytes caps a session's total spill footprint on disk.
	DefaultMaxSessionBytes = 64 << 20 // 64 MiB
	// DefaultTTL is how long spilled files live before GC reclaims them.
	// 24h is deliberately conservative: GC never deletes anything younger.
	DefaultTTL = 24 * time.Hour

	fileSuffix  = ".spill.json"
	tmpSuffix   = ".tmp-spill"
	sessionHash = 16 // hex chars of sha256(sessionID) used as the session dir name
	idLen       = 32 // hex chars: 8B ms timestamp + 8B crypto/rand
)

// Sentinel errors surfaced by the store. Callers (read_tool_result and the
// integration points) map these to user/model-visible outcomes:
// ErrInvalidLocator -> BadRequest; ErrNotFound -> found:false;
// ErrUnsupportedStore -> BadRequest with a migration hint.
var (
	ErrInvalidLocator   = errors.New("toolspill: invalid locator")
	ErrNotFound         = errors.New("toolspill: result not found")
	ErrFileTooLarge     = errors.New("toolspill: payload exceeds per-file limit")
	ErrSessionQuota     = errors.New("toolspill: session quota exceeded")
	ErrUnsupportedStore = errors.New("toolspill: unsupported store kind")
	ErrSecurity         = errors.New("toolspill: path security check failed")
)

// PartialPreservedNote is appended to an oversized payload that could neither
// be offloaded nor spilled: the fail-soft contract keeps the content whole and
// makes the enforcement=partial posture explicit instead of silently
// truncating (design §3.4/§3.5). Downstream gates (BeforeModel RunResultGate
// at 50K chars, assembly budget) still bound what reaches the model window;
// nothing is dropped without a trace.
const PartialPreservedNote = "\n\n[spill_failed: output preserved untruncated (enforcement=partial); spill persistence unavailable — do NOT treat this content as lost]"

// Record is a single spilled payload.
type Record struct {
	// ID is the store-assigned file id (populated on writes/reads).
	ID string
	// SessionID owns the record; required on Put, echoed back on Get.
	SessionID string
	// Tool is the producing tool's name (metadata only).
	Tool string
	// Content is the full payload, round-tripped byte-for-byte.
	Content []byte
	// ByteSize is len(Content), recorded for cheap Stat answers.
	ByteSize int64
	// CreatedAt stamps the record (UTC).
	CreatedAt time.Time
}

// FileInfo is the metadata returned by Stat.
type FileInfo struct {
	Locator   Locator
	Tool      string
	ByteSize  int64
	CreatedAt time.Time
}

// GCStats summarizes one garbage-collection cycle.
type GCStats struct {
	Scanned        int
	Removed        int
	ReclaimedBytes int64
}

// Locator identifies one spilled payload. String() renders the canonical
// "spill://<store>/<id>" form; ID is a store-specific relative identifier
// (for LocalFS "<sha16>/<id32>.spill.json", forward slashes, never absolute).
type Locator struct {
	Store string
	ID    string
	Bytes int64
}

// String renders the canonical locator URI.
func (l Locator) String() string { return Scheme + l.Store + "/" + l.ID }

// RetrievalHint is the model-facing instruction for reading the payload back.
// Deliberately routed through read_tool_result's existing blob_id field so
// the tool schema stays unchanged and dispatch happens on the scheme prefix
// (design §9.2: "保全类引用统一用 read_tool_result，按 scheme 分派").
func (l Locator) RetrievalHint() string {
	return "Full payload preserved. Call read_tool_result with blob_id=\"" + l.String() + "\" (use offset/limit to paginate)."
}

// ParseLocator validates and splits a locator string. It accepts any
// well-formed "<scheme><store>/<id>" shape; per-store ID rules are enforced
// by the store itself so future schemes (spill://pg/) parse cleanly here.
func ParseLocator(s string) (Locator, error) {
	if !strings.HasPrefix(s, Scheme) {
		return Locator{}, fmt.Errorf("%w: missing %q scheme", ErrInvalidLocator, Scheme)
	}
	rest := strings.TrimPrefix(s, Scheme)
	if strings.Contains(rest, "\\") {
		return Locator{}, fmt.Errorf("%w: backslash not allowed", ErrInvalidLocator)
	}
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 || slash == len(rest)-1 {
		return Locator{}, fmt.Errorf("%w: want <store>/<id>", ErrInvalidLocator)
	}
	store, id := rest[:slash], rest[slash+1:]
	if !isLowerAlnum(store) || id == "" || strings.Contains(id, "..") || strings.HasPrefix(id, "/") {
		return Locator{}, fmt.Errorf("%w: malformed store or id", ErrInvalidLocator)
	}
	return Locator{Store: store, ID: id}, nil
}

// ResolveToolSpillStoreKind 纯函数解析 env 档位（无副作用，nil getenv 回落
// os.Getenv；镜像 board lease ResolveBoardRunLeaseMode 形态）。返回
// (kind, malformed)：缺省/空=fs 字节级旧行为；非法值亦回落 fs 且
// malformed=true 供调用方 Warn（fail-safe：不认得的档位=旧行为）。
func ResolveToolSpillStoreKind(getenv func(string) string) (string, bool) {
	if getenv == nil {
		getenv = os.Getenv
	}
	switch strings.ToLower(strings.TrimSpace(getenv(EnvToolSpillStore))) {
	case "", StoreFS:
		return StoreFS, false
	case StorePG:
		return StorePG, false
	default:
		return StoreFS, true
	}
}

// PointerText renders the message-body stub that replaces an oversized tool
// result once it has been spilled. It mirrors biz.ToolResultPrunePointer's
// format so the model already knows how to consume it, and it is a pure
// function of (tool, size, locator) with no timestamps so prompt-prefix
// stability is preserved.
func PointerText(tool string, byteSize int, loc Locator) string {
	return fmt.Sprintf("[已保全 tool_result｜原 %d 字节｜tool=%s｜locator=%s｜取回: 调用 read_tool_result(blob_id=%s)]",
		byteSize, tool, loc.String(), loc.String())
}

// SpillFailedDedupeKey groups Important-level spill failure reports per
// session within a 5-minute wall-clock bucket
// ("spill_failed:<session>:<floor(ts,300)>", review doc §12.2 P0-2⑤).
func SpillFailedDedupeKey(sessionID string, now time.Time) string {
	return fmt.Sprintf("spill_failed:%s:%d", sessionID, now.Unix()/300)
}

// Hooks lets integration points report classified events without importing
// the decision/bus layers in this batch (batch B wires the channels).
type Hooks struct {
	// OnSpillFailed, when non-nil, receives Important-level
	// (AS-EVT-01) spill failure reports keyed by SpillFailedDedupeKey.
	OnSpillFailed func(ctx context.Context, dedupeKey string, err error)
}

// Deps bundles everything an integration point (decorator / size limiter /
// gate hook / read_tool_result) needs. A nil *Deps — or one with a nil
// Store — disables spilling entirely (default OFF: every legacy fallback
// stays byte-identical). This is the batch-A grey-release gate; batch B
// wires a concrete instance through wire.
type Deps struct {
	Store Store
	Hooks Hooks
	// Now overrides the clock (tests); nil means time.Now.
	Now func() time.Time
}

// Enabled reports whether this Deps actually spills (kill switch).
func (d *Deps) Enabled() bool { return d != nil && d.Store != nil }

// NowTime returns the current time through the configured clock.
func (d *Deps) NowTime() time.Time {
	if d != nil && d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// ReportSpillFailed emits the Important-level failure signal for a failed
// Put: an Error log (always) plus the optional hook.
func (d *Deps) ReportSpillFailed(ctx context.Context, lg loggateway.Logger, stepID, sessionID string, err error) {
	// 对抗评审 #6：ctx 已取消的 Put 失败是取消噪声（PutWithRetry 本就放弃重试），
	// 不属于 AS-EVT-01「真实存储失败」Important 档——降级 Warn 且不触发 hook
	// （否则取消风暴会把决策闸门刷满噪声）。
	if ctx != nil && ctx.Err() != nil {
		if lg != nil {
			lg.Warn("tool_result.spill_failed on canceled ctx (downgraded, not Important)",
				loggateway.StepID(stepID),
				loggateway.SessionID(sessionID),
				loggateway.Err(err),
			)
		}
		return
	}
	key := SpillFailedDedupeKey(sessionID, d.NowTime())
	if lg != nil {
		lg.Error("tool_result.spill_failed (Important)",
			loggateway.StepID(stepID),
			loggateway.SessionID(sessionID),
			loggateway.Str("dedupe_key", key),
			loggateway.Err(err),
		)
	}
	if d != nil && d.Hooks.OnSpillFailed != nil {
		d.Hooks.OnSpillFailed(ctx, key, err)
	}
}

// PutWithRetry persists rec through st, retrying the write exactly once on
// failure (design §3.5 mitigation 2: the fail-soft bypass may only trigger
// after "spill 写失败……且只重试一次"). Context cancellation is never retried.
func PutWithRetry(ctx context.Context, st Store, rec Record) (Locator, error) {
	loc, err := st.Put(ctx, rec)
	if err == nil || ctx.Err() != nil {
		return loc, err
	}
	return st.Put(ctx, rec)
}

// Store is the spill persistence contract. LocalFS is the first
// implementation; the PG store (C-5) serves multi-replica deployments.
// Cross-scheme locators are hard rejects, not silent 404s: a store answers
// ErrUnsupportedStore for a locator outside its scheme, and the consumer
// (read_tool_result) maps that to BadRequest with a migration hint — the
// deliberate env-flip migration signal (C-f wire-env-01 pins this wording
// to the shipped behavior).
type Store interface {
	// Put persists rec and returns its locator. Concurrent Puts are
	// idempotent: each succeeds with its own file, never overwriting.
	Put(ctx context.Context, rec Record) (Locator, error)
	// Get reads the record named by locator, enforcing session ownership:
	// a locator minted for another session answers ErrNotFound.
	Get(ctx context.Context, sessionID, locator string) (Record, error)
	// Stat returns metadata for a locator with the same ownership rules.
	Stat(ctx context.Context, sessionID, locator string) (FileInfo, error)
	// GC reclaims expired files (TTL) and enforces any total cap
	// (oldest-first). It is safe to run periodically and is idempotent.
	// err != nil 时 stats 语义=实际**已生效**的部分计数：PG 实现事务整包
	// 回滚故恒零值（D2-f-3），FS 为已生效的部分删除计数。消费方（Q7 GC
	// 调度器的审计 Extra）不得把 err 路径的计数当真实吞吐。
	GC(ctx context.Context, now time.Time) (GCStats, error)
}

// Options configure a LocalFS store. Go-Options is the batch-A configuration
// surface; conf.proto wiring is deferred to batch B (default OFF by absence
// of a constructed store at the integration points).
type Options struct {
	// WorkspaceRoot is the tenant/agent workspace root; the store lives at
	// <root>/<DirName>. Required. Canonicalized (Abs + EvalSymlinks) at
	// construction; an existing symlink inside the store tree is rejected.
	WorkspaceRoot string
	// DirName overrides DefaultDirName (empty = default).
	DirName string
	// MaxFileBytes caps one payload (<=0 = DefaultMaxFileBytes).
	MaxFileBytes int64
	// MaxSessionBytes caps one session's total on-disk footprint
	// (<=0 = DefaultMaxSessionBytes).
	MaxSessionBytes int64
	// MaxTotalBytes caps the whole store; 0 (default) disables the cap.
	// Enforced by GC evicting the oldest files first.
	MaxTotalBytes int64
	// TTL is the file retention horizon (<=0 = DefaultTTL).
	TTL time.Duration
	// Logger receives structured store logs (nil = noop).
	Logger loggateway.Logger
	// Now overrides the clock (tests); nil means time.Now.
	Now func() time.Time
}

// LocalFS is the workspace-local filesystem SpillStore.
type LocalFS struct {
	root            string
	maxFileBytes    int64
	maxSessionBytes int64
	maxTotalBytes   int64
	ttl             time.Duration
	lg              loggateway.Logger
	now             func() time.Time
	newID           func() (string, error)
}

// NewLocalFS creates (or opens) the workspace-private spill store.
func NewLocalFS(opts Options) (*LocalFS, error) {
	if strings.TrimSpace(opts.WorkspaceRoot) == "" {
		return nil, fmt.Errorf("toolspill: workspace root is empty")
	}
	absRoot, err := filepath.Abs(filepath.Clean(opts.WorkspaceRoot))
	if err != nil {
		return nil, fmt.Errorf("toolspill: resolve workspace root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absRoot)
	if err == nil && resolved != "" {
		absRoot = resolved
	}
	dirName := opts.DirName
	if dirName == "" {
		dirName = DefaultDirName
	}
	if strings.ContainsAny(dirName, `/\`) || dirName == "." || dirName == ".." {
		return nil, fmt.Errorf("toolspill: bad dir name %q", dirName)
	}
	root := filepath.Join(absRoot, dirName)
	if err := ensurePrivateDir(root); err != nil {
		return nil, err
	}
	chmodBestEffort(root, 0o700)

	s := &LocalFS{
		root:            root,
		maxFileBytes:    opts.MaxFileBytes,
		maxSessionBytes: opts.MaxSessionBytes,
		maxTotalBytes:   opts.MaxTotalBytes,
		ttl:             opts.TTL,
		lg:              opts.Logger,
		now:             opts.Now,
		newID:           defaultID,
	}
	if s.maxFileBytes <= 0 {
		s.maxFileBytes = DefaultMaxFileBytes
	}
	if s.maxSessionBytes <= 0 {
		s.maxSessionBytes = DefaultMaxSessionBytes
	}
	if s.ttl <= 0 {
		s.ttl = DefaultTTL
	}
	if s.lg == nil {
		s.lg = loggateway.NewNoop()
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s, nil
}

// Root exposes the store root (diagnostics/tests).
func (s *LocalFS) Root() string { return s.root }

// SessionDirName maps a raw session ID to its on-disk directory name:
// lowercase hex of sha256(sessionID) truncated to 16 chars. It is exported
// so integration tests and batch-B wiring can locate a session's directory
// without duplicating the hashing rule.
func SessionDirName(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])[:sessionHash]
}

// Put implements Store.
func (s *LocalFS) Put(ctx context.Context, rec Record) (Locator, error) {
	if err := ctx.Err(); err != nil {
		return Locator{}, err
	}
	if strings.TrimSpace(rec.SessionID) == "" {
		return Locator{}, fmt.Errorf("toolspill: session id is empty")
	}
	size := int64(len(rec.Content))
	if size > s.maxFileBytes {
		return Locator{}, fmt.Errorf("%w (%d > %d)", ErrFileTooLarge, size, s.maxFileBytes)
	}
	if err := rejectSymlink(s.root); err != nil {
		return Locator{}, err
	}
	dir := filepath.Join(s.root, SessionDirName(rec.SessionID))
	if err := ensurePrivateDir(dir); err != nil {
		return Locator{}, err
	}
	used, err := dirUsage(dir)
	if err != nil {
		return Locator{}, fmt.Errorf("toolspill: session usage scan: %w", err)
	}
	if used+size > s.maxSessionBytes {
		return Locator{}, fmt.Errorf("%w (%d + %d > %d)", ErrSessionQuota, used, size, s.maxSessionBytes)
	}

	id, err := s.newID()
	if err != nil {
		return Locator{}, fmt.Errorf("toolspill: id generation: %w", err)
	}
	envBytes, err := json.Marshal(envelope{
		Version:   1,
		ID:        id,
		Session:   SessionDirName(rec.SessionID),
		Tool:      rec.Tool,
		CreatedAt: s.now().UTC().Format(time.RFC3339Nano),
		ByteSize:  size,
		Content:   rec.Content,
	})
	if err != nil {
		return Locator{}, fmt.Errorf("toolspill: envelope marshal: %w", err)
	}

	final := filepath.Join(dir, id+fileSuffix)
	tmp := filepath.Join(dir, id+tmpSuffix)
	if err := writeExclusiveThenRename(tmp, final, envBytes); err != nil {
		return Locator{}, err
	}
	loc := Locator{Store: StoreFS, ID: SessionDirName(rec.SessionID) + "/" + id + fileSuffix, Bytes: size}
	// tool_result.spilled — Informational (AS-EVT-01): high-frequency,
	// log-only, never placed on the bus.
	s.lg.Info("tool result spilled",
		loggateway.StepID("tool_result.spilled"),
		loggateway.SessionID(rec.SessionID),
		loggateway.Str("tool", rec.Tool),
		loggateway.Int64("bytes", size),
		loggateway.Str("locator", loc.String()),
	)
	return loc, nil
}

// Get implements Store.
func (s *LocalFS) Get(ctx context.Context, sessionID, locator string) (Record, error) {
	path, _, err := s.resolveOwned(sessionID, locator)
	if err != nil {
		return Record{}, err
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	env, err := s.readEnvelope(path)
	if err != nil {
		return Record{}, err
	}
	return Record{
		ID:        env.ID,
		SessionID: sessionID,
		Tool:      env.Tool,
		Content:   env.Content,
		ByteSize:  env.ByteSize,
		CreatedAt: env.createdAt(),
	}, nil
}

// Stat implements Store.
func (s *LocalFS) Stat(ctx context.Context, sessionID, locator string) (FileInfo, error) {
	path, loc, err := s.resolveOwned(sessionID, locator)
	if err != nil {
		return FileInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return FileInfo{}, err
	}
	env, err := s.readEnvelope(path)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Locator: loc, Tool: env.Tool, ByteSize: env.ByteSize, CreatedAt: env.createdAt()}, nil
}

// resolveOwned validates a locator against this store and the calling
// session, returning the absolute file path. All path-traversal resistance
// lives here: the id grammar admits nothing but lowercase-hex segments under
// the store root, the caller's session hash must match the locator's session
// segment (cross-session -> ErrNotFound), and every component is Lstat-ed
// for symlinks. containmentUnderRoot mirrors the
// internal/agent containPathUnderRoot (Abs + EvalSymlinks + prefix)
// precedent without importing the agent package.
func (s *LocalFS) resolveOwned(sessionID, locator string) (string, Locator, error) {
	loc, err := ParseLocator(locator)
	if err != nil {
		return "", Locator{}, err
	}
	if loc.Store != StoreFS {
		return "", Locator{}, fmt.Errorf("%w %q: this deployment only resolves spill://%s/ locators; the locator scheme migration is pending", ErrUnsupportedStore, loc.Store, StoreFS)
	}
	dirPart, filePart, err := splitFSID(loc.ID)
	if err != nil {
		return "", Locator{}, err
	}
	if strings.TrimSpace(sessionID) == "" || dirPart != SessionDirName(sessionID) {
		// Cross-session reads are denied by default (same posture as the
		// blob reader): report not-found, never the reason.
		return "", Locator{}, ErrNotFound
	}
	candidate := filepath.Join(s.root, dirPart, filePart)
	if err := containmentUnderRoot(candidate, s.root); err != nil {
		return "", Locator{}, err
	}
	if err := rejectSymlink(s.root); err != nil {
		return "", Locator{}, err
	}
	if err := rejectSymlink(filepath.Join(s.root, dirPart)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Session dir missing (never spilled, or GC already reclaimed it):
			// a plain miss, indistinguishable from any other not-found.
			return "", Locator{}, ErrNotFound
		}
		return "", Locator{}, err
	}
	if err := rejectSymlink(candidate); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", Locator{}, ErrNotFound
		}
		return "", Locator{}, err
	}
	return candidate, loc, nil
}

// readEnvelope opens path, verifies handle-vs-Lstat identity, and decodes
// the envelope with a bounded reader so a corrupt/hostile file can neither
// exhaust memory nor fool the identity checks.
func (s *LocalFS) readEnvelope(path string) (envelope, error) {
	fh, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return envelope{}, ErrNotFound
		}
		return envelope{}, fmt.Errorf("toolspill: open: %w", err)
	}
	defer fh.Close()
	fiStat, err := fh.Stat()
	if err != nil {
		return envelope{}, fmt.Errorf("toolspill: fstat: %w", err)
	}
	fiLstat, err := os.Lstat(path)
	if err != nil {
		return envelope{}, fmt.Errorf("toolspill: lstat: %w", err)
	}
	if fiLstat.Mode()&os.ModeSymlink != 0 || !os.SameFile(fiStat, fiLstat) {
		return envelope{}, fmt.Errorf("%w: handle/lstat mismatch on %q", ErrSecurity, path)
	}
	// Bound: content maxFileBytes -> base64 ~4/3x + JSON overhead.
	const envOverhead = 4 << 20
	limited := io.LimitReader(fh, s.maxFileBytes*2+envOverhead)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return envelope{}, fmt.Errorf("toolspill: read: %w", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return envelope{}, fmt.Errorf("toolspill: envelope decode: %w", err)
	}
	if env.ByteSize != int64(len(env.Content)) {
		return envelope{}, fmt.Errorf("toolspill: byte size mismatch (%d != %d)", env.ByteSize, len(env.Content))
	}
	return env, nil
}

// GC implements Store: TTL expiry, stale-temp cleanup, empty-dir pruning,
// and optional total-cap oldest-first eviction. Individual file errors are
// logged and skipped; the first error is returned (aggregated report once
// per cycle — tool_result.gc_failed is Important, one entry per cycle).
func (s *LocalFS) GC(ctx context.Context, now time.Time) (GCStats, error) {
	var stats GCStats
	var firstErr error
	remember := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return stats, fmt.Errorf("toolspill: gc read root: %w", err)
	}
	now = now.UTC()
	expireBefore := now.Add(-s.ttl)

	type candidate struct {
		path string
		mod  time.Time
		size int64
	}
	var survivors []candidate
	var total int64

	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		sessionDir := filepath.Join(s.root, e.Name())
		// Never follow a symlinked session dir during GC.
		if rejectSymlink(sessionDir) != nil {
			s.lg.Warn("spill gc: skipping symlink entry", loggateway.StepID("tool_result.gc"), loggateway.Str("path", sessionDir))
			continue
		}
		if !e.IsDir() {
			continue
		}
		files, ferr := os.ReadDir(sessionDir)
		if ferr != nil {
			remember(ferr)
			continue
		}
		for _, fe := range files {
			if fe.IsDir() {
				continue
			}
			p := filepath.Join(sessionDir, fe.Name())
			info, ierr := fe.Info()
			if ierr != nil {
				remember(ierr)
				continue
			}
			isTmp := strings.HasSuffix(fe.Name(), tmpSuffix)
			isSpill := strings.HasSuffix(fe.Name(), fileSuffix)
			if !isTmp && !isSpill {
				continue
			}
			stats.Scanned++
			if info.ModTime().UTC().Before(expireBefore) {
				stats.ReclaimedBytes += info.Size()
				stats.Removed += lenOrZero(os.Remove(p))
			} else if isSpill {
				survivors = append(survivors, candidate{path: p, mod: info.ModTime().UTC(), size: info.Size()})
				total += info.Size()
			}
		}
		// Prune now-empty session dirs.
		if rest, rerr := os.ReadDir(sessionDir); rerr == nil && len(rest) == 0 {
			_ = os.Remove(sessionDir)
		}
	}

	// Total-cap eviction: oldest first.
	if s.maxTotalBytes > 0 && total > s.maxTotalBytes {
		sort.Slice(survivors, func(i, j int) bool { return survivors[i].mod.Before(survivors[j].mod) })
		for _, c := range survivors {
			if total <= s.maxTotalBytes {
				break
			}
			if err := ctx.Err(); err != nil {
				return stats, err
			}
			if os.Remove(c.path) == nil {
				total -= c.size
				stats.Removed++
				stats.ReclaimedBytes += c.size
			}
		}
		// Second prune pass for dirs emptied by eviction.
		for _, e := range entries {
			sessionDir := filepath.Join(s.root, e.Name())
			if !e.IsDir() {
				continue
			}
			if rest, rerr := os.ReadDir(sessionDir); rerr == nil && len(rest) == 0 {
				_ = os.Remove(sessionDir)
			}
		}
	}

	if firstErr != nil {
		s.lg.Error("tool_result.gc_failed (Important)",
			loggateway.StepID("tool_result.gc_failed"),
			loggateway.Err(firstErr),
		)
		return stats, firstErr
	}
	return stats, nil
}

func lenOrZero(err error) int {
	if err == nil {
		return 1
	}
	return 0
}

// envelope is the on-disk JSON document. Content is []byte so encoding/json
// applies base64, guaranteeing a byte-faithful round trip for any payload
// (including non-UTF-8 edge cases).
type envelope struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Session   string `json:"session_hash"`
	Tool      string `json:"tool,omitempty"`
	CreatedAt string `json:"created_at"`
	ByteSize  int64  `json:"byte_size"`
	Content   []byte `json:"content"`
}

func (e envelope) createdAt() time.Time {
	t, err := time.Parse(time.RFC3339Nano, e.CreatedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

// defaultID generates a collision-resistant file id: 8-byte big-endian
// millisecond timestamp + 8-byte crypto/rand, hex-encoded (32 chars).
// Modelled on biz.newAgentCatalogID's crypto/rand pattern with a millisecond
// time prefix.
func defaultID() (string, error) {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixMilli()))
	if _, err := rand.Read(b[8:]); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// writeExclusiveThenRename creates tmp with O_WRONLY|O_CREATE|O_EXCL (no
// pre-existing name can ever be clobbered), writes data, then atomically
// renames to final so readers only ever see complete files. The open handle
// is cross-checked against Lstat (fstat re-verify of the same inode).
func writeExclusiveThenRename(tmp, final string, data []byte) error {
	fh, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("toolspill: exclusive create: %w", err)
	}
	fiStat, statErr := fh.Stat()
	fiLstat, lstatErr := os.Lstat(tmp)
	if statErr != nil || lstatErr != nil || !os.SameFile(fiStat, fiLstat) || fiLstat.Mode()&os.ModeSymlink != 0 {
		fh.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: temp file identity check failed", ErrSecurity)
	}
	if _, werr := fh.Write(data); werr != nil {
		fh.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("toolspill: write: %w", werr)
	}
	if cerr := fh.Close(); cerr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("toolspill: close: %w", cerr)
	}
	if rerr := os.Rename(tmp, final); rerr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("toolspill: rename: %w", rerr)
	}
	chmodBestEffort(final, 0o600)
	return nil
}

// ensurePrivateDir creates dir (mode 0700) when missing and rejects a
// pre-planted symlink or non-directory at the path.
func ensurePrivateDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err == nil {
		if err := rejectSymlink(dir); err != nil {
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("toolspill: %q exists and is not a directory", dir)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("toolspill: stat %q: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("toolspill: mkdir %q: %w", dir, err)
	}
	// An attacker could have swapped in a symlink between MkdirAll and use.
	return rejectSymlink(dir)
}

// rejectSymlink returns ErrSecurity when path itself is a symlink.
func rejectSymlink(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %q is a symlink", ErrSecurity, path)
	}
	return nil
}

// dirUsage sums the sizes of spill files directly under dir (quota
// accounting is on-disk bytes — what the tenant actually consumes).
func dirUsage(dir string) (int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, fileSuffix) && !strings.HasSuffix(name, tmpSuffix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

// containmentUnderRoot resolves candidate and root (Abs + EvalSymlinks
// best-effort, mirroring internal/agent's containPathUnderRoot precedent)
// and requires the resolved candidate to stay under the resolved root.
func containmentUnderRoot(candidate, root string) error {
	absCand, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return fmt.Errorf("toolspill: resolve candidate: %w", err)
	}
	if eval, evalErr := filepath.EvalSymlinks(absCand); evalErr == nil && eval != "" {
		absCand = eval
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return fmt.Errorf("toolspill: resolve root: %w", err)
	}
	if eval, evalErr := filepath.EvalSymlinks(absRoot); evalErr == nil && eval != "" {
		absRoot = eval
	}
	sep := string(filepath.Separator)
	rootWithSep := absRoot
	if !strings.HasSuffix(rootWithSep, sep) {
		rootWithSep += sep
	}
	candWithSep := absCand
	if !strings.HasSuffix(candWithSep, sep) {
		candWithSep += sep
	}
	if filepath.Separator == '\\' {
		if strings.HasPrefix(strings.ToLower(candWithSep), strings.ToLower(rootWithSep)) ||
			strings.EqualFold(absCand, absRoot) {
			return nil
		}
	} else if strings.HasPrefix(candWithSep, rootWithSep) || absCand == absRoot {
		return nil
	}
	return fmt.Errorf("%w: %q escapes store root %q", ErrSecurity, absCand, absRoot)
}

// splitFSID validates the LocalFS id grammar
// "<16-hex session dir>/<32-hex id>.spill.json". The grammar itself already
// excludes every traversal shape (".", "..", drive letters, UNC, separators)
// because admitted characters are lowercase hex plus exactly one slash.
func splitFSID(id string) (dir, file string, err error) {
	parts := strings.Split(id, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: id must be <session>/<file>", ErrInvalidLocator)
	}
	dir, file = parts[0], parts[1]
	if len(dir) != sessionHash || !isHex(dir) {
		return "", "", fmt.Errorf("%w: bad session segment", ErrInvalidLocator)
	}
	base := strings.TrimSuffix(file, fileSuffix)
	if base == file || len(base) != idLen || !isHex(base) {
		return "", "", fmt.Errorf("%w: bad file segment", ErrInvalidLocator)
	}
	return dir, file, nil
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') {
			continue
		}
		return false
	}
	return len(s) > 0
}

// IsSpillID reports whether id satisfies the canonical spill id grammar
// (idLen lowercase hex chars). Both stores enforce it on read paths so a
// well-formed-but-malformed-id locator answers ErrInvalidLocator rather
// than a misleading ErrNotFound (C-f C5-PARITY-1: mirrors the FS splitFSID
// sentinel classification for the PG store).
func IsSpillID(id string) bool { return len(id) == idLen && isHex(id) }

func isLowerAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if ('a' <= c && c <= 'z') || ('0' <= c && c <= '9') {
			continue
		}
		return false
	}
	return len(s) > 0
}

// chmodBestEffort applies Unix mode bits where the platform honors them;
// on Windows it is effectively a no-op (only the read-only bit maps) and the
// ACL posture comes from workspace-directory inheritance. Tests asserting
// mode bits skip on Windows.
func chmodBestEffort(path string, mode os.FileMode) {
	_ = os.Chmod(path, mode)
}
