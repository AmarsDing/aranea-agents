package biz

import (
	"context"
	"errors"
	"strings"
)

var ErrMemoryPolicyAudit = errors.New("memory: policy audit write failed")

const (
	PolicyVersionConsolidateV1 = "consolidate_v1"
	PolicyVersionCascadeV1     = "cascade_v1"
	PolicyVersionL2DecayV1     = "l2_decay_v1"
	PolicyVersionL3DecayV1     = "l3_decay_v1"
	PolicyVersionL3RecallV1    = "l3_recall_v1"
)

// PolicyStrictMode reports env-only strict mode (legacy helper; prefer ResolvePolicyStrict).
func PolicyStrictMode() bool {
	return envTruthy("MEMORY_POLICY_STRICT")
}

// MemoryPolicyRecord is one auditable memory mutation decision.
type MemoryPolicyRecord struct {
	Action         string
	TargetKind     string
	TargetID       string
	Reason         string
	PolicyVersion  string
	TurnID         string
	SourceEventIDs []string
	MetadataJSON   string
}

// MemoryActionLogWriter persists policy audit rows (implemented by sessionmemory.Store).
type MemoryActionLogWriter interface {
	WriteMemoryActionLog(ctx context.Context, rec MemoryPolicyRecord) error
}

// StrictModeResolver resolves whether policy audit failures must block writes (may read DB per call).
type StrictModeResolver func(ctx context.Context) bool

// MemoryPolicyEngine routes memory mutations through a single audit path.
type MemoryPolicyEngine struct {
	writer   MemoryActionLogWriter
	strict   bool
	strictFn StrictModeResolver
}

func NewMemoryPolicyEngine(writer MemoryActionLogWriter, strict StrictModeResolver) *MemoryPolicyEngine {
	if writer == nil {
		return nil
	}
	return &MemoryPolicyEngine{writer: writer, strictFn: strict}
}

// NewMemoryPolicyEngineStatic is for tests that need a fixed strict flag.
func NewMemoryPolicyEngineStatic(writer MemoryActionLogWriter, strict bool) *MemoryPolicyEngine {
	if writer == nil {
		return nil
	}
	return &MemoryPolicyEngine{writer: writer, strict: strict}
}

func (e *MemoryPolicyEngine) Strict() bool {
	return e != nil && (e.strict || e.strictFn != nil)
}

func (e *MemoryPolicyEngine) strictEnabled(ctx context.Context) bool {
	if e == nil {
		return false
	}
	if e.strictFn != nil {
		return e.strictFn(ctx)
	}
	return e.strict
}

// StrictEnabled reports whether audit write failures must block mutations.
func (e *MemoryPolicyEngine) StrictEnabled(ctx context.Context) bool {
	return e.strictEnabled(ctx)
}

// Record appends one action log row. Failures propagate to callers that require strict audit.
func (e *MemoryPolicyEngine) Record(ctx context.Context, rec MemoryPolicyRecord) error {
	if e == nil || e.writer == nil {
		return nil
	}
	rec.Action = strings.TrimSpace(rec.Action)
	rec.TargetKind = strings.TrimSpace(rec.TargetKind)
	rec.TargetID = strings.TrimSpace(rec.TargetID)
	if rec.Action == "" || rec.TargetKind == "" || rec.TargetID == "" {
		return nil
	}
	if strings.TrimSpace(rec.PolicyVersion) == "" {
		rec.PolicyVersion = PolicyVersionConsolidateV1
	}
	return e.writer.WriteMemoryActionLog(ctx, rec)
}

// RecordBestEffort logs policy actions; returns error when strict mode is enabled.
func (e *MemoryPolicyEngine) RecordBestEffort(ctx context.Context, rec MemoryPolicyRecord) error {
	if e == nil {
		return nil
	}
	err := e.Record(ctx, rec)
	if err != nil && e.strictEnabled(ctx) {
		return err
	}
	return nil
}
