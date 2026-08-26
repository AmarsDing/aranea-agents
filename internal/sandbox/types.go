// Package sandbox provides pooled, use-and-destroy isolated execution
// environments (M82). A Manager hands out exclusive Leases over pre-warmed
// containers; every Lease is destroyed on Release/TTL/idle/reconcile and is
// never recycled into the pool (ADR-82-2).
package sandbox

import (
	"errors"
	"fmt"
	"time"
)

// NetworkMode is the egress stance of a sandbox profile.
type NetworkMode string

const (
	NetworkNone   NetworkMode = "none"
	NetworkEgress NetworkMode = "egress" // P2: CONNECT proxy + domain allowlist
	NetworkFull   NetworkMode = "full"
)

// Profile is a static spec portrait for a class of sandboxes.
type Profile struct {
	Name           string
	Image          string
	CPUs           float64 // docker --cpus (default 0.5)
	MemoryBytes    int64   // container memory limit (default 256 MiB)
	PidsLimit      int64   // docker --pids-limit (default 256)
	Network        NetworkMode
	ReadOnlyRootfs bool   // default true
	TmpSize        string // tmpfs size for /tmp (default 128m)

	// RequiresConfirmation marks high-risk profiles (P2-4): Acquire rejects
	// with ErrConfirmationRequired unless AcquireReq.Confirmed is true.
	// normalize force-sets this on NetworkFull profiles (fail-closed).
	RequiresConfirmation bool

	// EgressNetwork / EgressProxy are resolved from Config.Egress by
	// ConfigFromProto for NetworkEgress profiles (P2-1): the per-sandbox
	// egress network name PREFIX (review 2026-08-26 #3: each instance gets a
	// dedicated internal network "<prefix>-<sandboxID>" shared only with the
	// proxy) and the CONNECT proxy URL injected as HTTP(S)_PROXY.
	// Empty for none/full.
	EgressNetwork string
	EgressProxy   string
}

func (p Profile) withDefaults() Profile {
	if p.CPUs <= 0 {
		p.CPUs = 0.5
	}
	if p.MemoryBytes <= 0 {
		p.MemoryBytes = 256 * 1024 * 1024
	}
	if p.PidsLimit <= 0 {
		p.PidsLimit = 256
	}
	if p.Network == "" {
		p.Network = NetworkNone
	}
	if p.TmpSize == "" {
		p.TmpSize = "128m"
	}
	return p
}

// LeaseState is the lifecycle state of a sandbox entry.
type LeaseState string

const (
	StateReady      LeaseState = "ready"      // warm-pool instance, unattributed
	StateLeased     LeaseState = "leased"     // exclusively held by a consumer
	StateDestroying LeaseState = "destroying" // terminal; teardown in flight
)

// AcquireReq is the input for Manager.Acquire.
type AcquireReq struct {
	Profile   string        // profile name; empty = default profile
	AgentKey  string        // attribution (required for quota)
	SessionID string        // attribution
	RunID     string        // optional: team run attribution (P2 quota)
	TTL       time.Duration // 0 = default; capped at TTL.Max
	// Confirmed must be true when acquiring a profile marked
	// RequiresConfirmation (P2-4); set only by callers that passed a
	// confirmation chain. Fail-closed: no in-tree consumer sets it yet.
	Confirmed bool
}

// LeaseView is a read-only snapshot of a live sandbox.
type LeaseView struct {
	SandboxID  string
	Profile    string
	AgentKey   string
	SessionID  string
	RunID      string
	State      LeaseState
	CreatedAt  time.Time
	Deadline   time.Time
	LastExecAt time.Time
	ExecCount  int64
}

// ExecSpec describes one command execution inside a sandbox.
type ExecSpec struct {
	Argv    []string      // executed directly (no shell wrapping by the engine)
	Stdin   string        // streamed to the process stdin
	Timeout time.Duration // 0 = engine default (30s)
	// StdoutLimit caps how many stdout bytes are retained in ExecResult.Stdout
	// (0 = unlimited, the historical behavior). Excess output is drained and
	// discarded so the process never blocks on a full pipe, while host memory
	// stays bounded regardless of in-sandbox output size (ReadFile contract).
	StdoutLimit int64
}

// ExecResult is the outcome of one Exec.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
	OOM      bool
}

// Destroy reasons (metric label values).
const (
	ReasonRelease   = "release"
	ReasonTTL       = "ttl"
	ReasonIdle      = "idle"
	ReasonPoolEvict = "pool_evict"
	ReasonReconcile = "reconcile"
	ReasonForce     = "force"
)

// Quota scopes (metric label values).
const (
	QuotaScopeGlobal = "global"
	QuotaScopeAgent  = "agent"
	QuotaScopeRun    = "run"
)

var (
	// ErrNotFound is returned when the sandbox id is unknown (or already destroyed).
	ErrNotFound = errors.New("sandbox: not found")
	// ErrDisabled is returned when the sandbox subsystem is disabled or the engine is unavailable.
	ErrDisabled = errors.New("sandbox: disabled")
	// ErrProfileUnknown is returned for an unconfigured profile name.
	ErrProfileUnknown = errors.New("sandbox: unknown profile")
	// ErrConfirmationRequired is returned when acquiring a profile marked
	// RequiresConfirmation without AcquireReq.Confirmed (P2-4 fail-closed).
	ErrConfirmationRequired = errors.New("sandbox: profile requires confirmation")
	// ErrNotRegular is returned by Lease.ReadFile when the path is not a
	// regular file (e.g. a directory).
	ErrNotRegular = errors.New("sandbox: not a regular file")
	// ErrTooLarge is returned by UntarFiles when the cumulative payload
	// exceeds the caller's byte budget.
	ErrTooLarge = errors.New("sandbox: payload exceeds byte budget")
)

// ReadFileMaxBytesDefault caps Lease.ReadFile when the caller passes
// maxBytes<=0 (matches the sandbox_fs read tool's hard ceiling).
const ReadFileMaxBytesDefault = 256 * 1024

// QuotaError reports a quota rejection at one scope.
type QuotaError struct {
	Scope string
	Limit int
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("sandbox: %s quota exceeded (limit %d)", e.Scope, e.Limit)
}

// IsQuotaError reports whether err is a QuotaError.
func IsQuotaError(err error) bool {
	var qe *QuotaError
	return errors.As(err, &qe)
}
