package turn

import (
	"strings"

	"aranea-agents/internal/biz"
)

// AdmissionAction classifies the admission gate outcome before turn body execution.
type AdmissionAction int

const (
	AdmissionProceed AdmissionAction = iota
	AdmissionQueued
	AdmissionRejectBusy
	AdmissionRejectEnqueue
)

// AdmissionVerdict is the structured result of a pre-turn admission check.
type AdmissionVerdict struct {
	Action       AdmissionAction
	PendingID    string
	RejectReason string
	Err          error
}

// AdmissionGateDeps wires registry, lock, and enqueue dependencies.
type AdmissionGateDeps struct {
	Runs    ActiveRunRegistry
	Lock    SessionLocker
	Enqueue MessageEnqueuer
}

// AdmissionGate performs session-locked admission before turn body execution.
type AdmissionGate struct {
	runs    ActiveRunRegistry
	lock    SessionLocker
	enqueue MessageEnqueuer
}

// NewAdmissionGate constructs an admission gate from dependencies.
func NewAdmissionGate(deps AdmissionGateDeps) *AdmissionGate {
	return &AdmissionGate{
		runs:    deps.Runs,
		lock:    deps.Lock,
		enqueue: deps.Enqueue,
	}
}

// Check evaluates whether a turn should proceed, enqueue, or reject.
// It mirrors the pre-body admission block previously inlined in ChatOrchestrator.
func (g *AdmissionGate) Check(input biz.TurnInput) AdmissionVerdict {
	if g == nil {
		return AdmissionVerdict{Action: AdmissionProceed}
	}

	sessionID := strings.TrimSpace(input.SessionID)
	content := strings.TrimSpace(input.Content)
	if sessionID == "" || content == "" {
		return AdmissionVerdict{Action: AdmissionProceed}
	}

	// 锁内仅做决策读（活跃运行快照），不得持锁调用 Enqueue：生产装配下
	// gate 的 SessionLocker 与 ChatUsecase.EnqueueUserMessage 内部的 locker
	// 是同一把非可重入互斥锁，持锁 enqueue 会同 goroutine 重入自死锁
	// （澄清续跑路径曾复现：resume → Execute → Check → enqueue 永久阻塞）。
	// 变更路径原子性由 EnqueueUserMessage 内部持锁并复核 HasActive 保证。
	unlock := g.lockSession(sessionID)
	hasActive := g.runs != nil && g.runs.HasActive(sessionID)
	hasRunner := hasActive && g.runs.HasActiveRunner(sessionID)
	if unlock != nil {
		unlock()
	}

	if !hasActive {
		return AdmissionVerdict{Action: AdmissionProceed}
	}

	switch EvaluateAdmission(true, hasRunner, input.AllowPendingQueue()) {
	case biz.AdmitEnqueue:
		return g.tryEnqueue(sessionID, content)
	case biz.AdmitReject:
		return AdmissionVerdict{Action: AdmissionRejectBusy}
	default:
		return AdmissionVerdict{Action: AdmissionProceed}
	}
}

func (g *AdmissionGate) lockSession(sessionID string) func() {
	if g.lock == nil {
		return func() {}
	}
	return g.lock.LockSession(sessionID)
}

func (g *AdmissionGate) tryEnqueue(sessionID, content string) AdmissionVerdict {
	if g.enqueue == nil {
		return AdmissionVerdict{Action: AdmissionRejectEnqueue, RejectReason: "enqueue unavailable"}
	}
	accepted, pendingID, rejectReason, err := g.enqueue.EnqueueUserMessage(sessionID, content)
	if err != nil {
		return AdmissionVerdict{Action: AdmissionRejectEnqueue, Err: err}
	}
	if !accepted {
		return AdmissionVerdict{
			Action:       AdmissionRejectEnqueue,
			RejectReason: rejectReason,
		}
	}
	return AdmissionVerdict{
		Action:    AdmissionQueued,
		PendingID: pendingID,
	}
}
