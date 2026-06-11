package service

import (
	"strings"

	"aranea-agents/internal/biz"
)

// PendingQueueWriter enqueues and modifies pending messages.
// Stability:evolving
type PendingQueueWriter interface {
	EnqueueUserMessage(sessionID, content string) (accepted, queued bool, pendingID, rejectReason string, err error)
	CancelPendingMessage(sessionID, pendingID string) bool
	UpdatePendingMessage(sessionID, pendingID, content string) bool
}

// PendingQueueReader reads pending message state.
// Stability:evolving
type PendingQueueReader interface {
	DequeuePendingMessage(sessionID string) (biz.PendingQueueEntry, bool)
	GetPendingMessages(sessionID string) []biz.PendingQueueEntry
	LastPendingMessageID(sessionID string) string
}

// MergeFollowupManager controls the merge-followup flag.
// Stability:evolving
type MergeFollowupManager interface {
	SetSessionPendingMergeFollowup(sessionID string, merge bool)
	SessionPendingMergeFollowup(sessionID string) bool
}

// pendingQueueManager is the composite interface.
// Stability:evolving
type pendingQueueManager interface {
	PendingQueueWriter
	PendingQueueReader
	MergeFollowupManager
	Sweep()
}

// chatPendingQueueManager implements pendingQueueManager.
//
// Part of the TECH-DEBT(BL8) resolution: extracting pending queue management
// from ChatOrchestrator to reduce cognitive complexity (AS-COG-01).
type chatPendingQueueManager struct {
	chatUC        *biz.ChatUsecase
	mergeFollowup *TypedSyncMap[string, bool]
}

func newChatPendingQueueManager(chatUC *biz.ChatUsecase) *chatPendingQueueManager {
	return &chatPendingQueueManager{
		chatUC:        chatUC,
		mergeFollowup: NewTypedSyncMap[string, bool](orchMapMaxIdle),
	}
}

// Compile-time interface check.
var _ pendingQueueManager = (*chatPendingQueueManager)(nil)

// EnqueueUserMessage enqueues a user message, respecting the merge-followup flag.
func (m *chatPendingQueueManager) EnqueueUserMessage(sessionID, content string) (accepted, queued bool, pendingID, rejectReason string, err error) {
	return m.chatUC.EnqueueUserMessage(sessionID, content, m.SessionPendingMergeFollowup(sessionID))
}

// DequeuePendingMessage dequeues the next pending message.
func (m *chatPendingQueueManager) DequeuePendingMessage(sessionID string) (biz.PendingQueueEntry, bool) {
	return m.chatUC.DequeuePendingMessage(sessionID)
}

// SetSessionPendingMergeFollowup toggles followup merge for pending queue enqueues (CH-BOR-01).
func (m *chatPendingQueueManager) SetSessionPendingMergeFollowup(sessionID string, merge bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || m == nil {
		return
	}
	if merge {
		m.mergeFollowup.Store(sessionID, true)
	} else {
		m.mergeFollowup.Delete(sessionID)
	}
}

// SessionPendingMergeFollowup returns whether the session has a pending merge followup.
func (m *chatPendingQueueManager) SessionPendingMergeFollowup(sessionID string) bool {
	if m == nil {
		return false
	}
	v, ok := m.mergeFollowup.Load(strings.TrimSpace(sessionID))
	if !ok {
		return false
	}
	return v
}

// LastPendingMessageID returns the most recently enqueued pending message id.
func (m *chatPendingQueueManager) LastPendingMessageID(sessionID string) string {
	if m == nil || m.chatUC == nil {
		return ""
	}
	entries := m.chatUC.GetPendingMessages(sessionID)
	if len(entries) == 0 {
		return ""
	}
	return entries[len(entries)-1].ID
}

// GetPendingMessages returns pending messages for a session.
func (m *chatPendingQueueManager) GetPendingMessages(sessionID string) []biz.PendingQueueEntry {
	return m.chatUC.GetPendingMessages(sessionID)
}

// CancelPendingMessage cancels a pending message.
func (m *chatPendingQueueManager) CancelPendingMessage(sessionID, pendingID string) bool {
	return m.chatUC.CancelPendingMessage(sessionID, pendingID)
}

// UpdatePendingMessage updates a pending message's content.
func (m *chatPendingQueueManager) UpdatePendingMessage(sessionID, pendingID, content string) bool {
	return m.chatUC.UpdatePendingMessage(sessionID, pendingID, content)
}

// Sweep removes expired entries from the merge-followup map.
func (m *chatPendingQueueManager) Sweep() {
	m.mergeFollowup.Sweep()
}
