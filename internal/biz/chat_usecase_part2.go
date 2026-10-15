package biz

import (
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"context"
	"strings"
	"time"
)

// ConsumeLeadingInjects dequeues head-of-queue inject entries so a newly
// started user turn can merge them as context (N2: normal message consumes
// stranded injects in one turn). Callers must hold the session lock.
func (uc *ChatUsecase) ConsumeLeadingInjects(sessionID string) []string {
	if uc == nil || uc.pending == nil {
		return nil
	}

	entries := uc.pending.DequeueLeadingInjects(sessionID)
	if len(entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Content)
	}
	return out
}

// SplitLeadingInjects 将队列头部连续的 inject 条目与其后的第一条 followup
// 切分出来：injects 为按序的上下文内容，followup 为承载它们的条目，leadCount
// 为头部 inject 条数（= 需要额外出队的条数）。无 followup（仅 inject 或空
// 队列）时 ok=false——inject 不单独唤醒 turn，保持排队。
//
// 无 Kind 的旧条目按 followup 处理（空 Kind 兼容）。
func SplitLeadingInjects(entries []PendingQueueEntry) (injects []string, followup PendingQueueEntry, leadCount int, ok bool) {
	for i, e := range entries {
		if e.Kind == ChatEnqueueKindInject {
			injects = append(injects, e.Content)
			continue
		}
		return injects, e, i, true
	}
	return nil, PendingQueueEntry{}, 0, false
}

// injectContextHeader 是 inject 上下文合入 turn 输入时的包裹标记。
const injectContextHeader = "[系统上下文补充]"

// MergeInjectContext 将 inject 上下文内容作为前缀合入 followup 输入。
func MergeInjectContext(injects []string, content string) string {
	if len(injects) == 0 {
		return content
	}
	return injectContextHeader + "\n" + strings.Join(injects, "\n") + "\n\n" + content
}

func (uc *ChatUsecase) RegisterAwaitChannel(sessionID string, ch AwaitChannel) {
	uc.RegisterAwaitChannelForTool(sessionID, "", ch)
}

// RegisterAwaitChannelForTool registers ch under the tool-scoped key.
// Re-registering the same scope closes the stale entry (interrupt semantics);
// distinct toolCallIDs coexist for parallel confirmations.
func (uc *ChatUsecase) RegisterAwaitChannelForTool(sessionID, toolCallID string, ch AwaitChannel) {
	key := awaitChanScopeKey(sessionID, toolCallID)
	uc.mu.Lock()
	if old, ok := uc.awaitChans[key]; ok {
		close(old.done)
	}
	uc.awaitChans[key] = awaitChanEntry{ch: ch, done: make(chan struct{}), createdAt: time.Now()}
	uc.mu.Unlock()
}

func (uc *ChatUsecase) DeleteAwaitChannel(sessionID string) {
	uc.DeleteAwaitChannelForTool(sessionID, "")
}

func (uc *ChatUsecase) DeleteAwaitChannelForTool(sessionID, toolCallID string) {
	key := awaitChanScopeKey(sessionID, toolCallID)
	uc.mu.Lock()
	if entry, ok := uc.awaitChans[key]; ok {
		close(entry.done)
		delete(uc.awaitChans, key)
	}
	uc.mu.Unlock()
}

// DeleteAwaitChannelsForSession removes every await entry of the session —
// the session-level slot plus all tool-scoped ones. Used on terminal run
// status so parallel-confirmation leftovers cannot leak until GC.
func (uc *ChatUsecase) DeleteAwaitChannelsForSession(sessionID string) {
	prefix := sessionID + "\x00"
	uc.mu.Lock()
	for key, entry := range uc.awaitChans {
		if key != sessionID && !strings.HasPrefix(key, prefix) {
			continue
		}
		close(entry.done)
		delete(uc.awaitChans, key)
	}
	uc.mu.Unlock()
}

func (uc *ChatUsecase) LoadAwaitChannel(sessionID string) (AwaitChannel, bool) {
	return uc.LoadAwaitChannelForTool(sessionID, "")
}

// LoadAwaitChannelForTool loads the entry for the tool-scoped key, falling
// back to the session-level slot when no tool-scoped entry exists (legacy
// registration + scoped delivery interop).
func (uc *ChatUsecase) LoadAwaitChannelForTool(sessionID, toolCallID string) (AwaitChannel, bool) {
	entry, ok := uc.lookupAwaitEntry(sessionID, toolCallID)
	if !ok {
		return nil, false
	}

	select {
	case <-entry.done:
		return nil, false
	default:
		return entry.ch, true
	}
}

// lookupAwaitEntry finds the registry entry: exact tool-scoped key first,
// then the session-level slot. Caller must NOT hold uc.mu (RLock taken inside).
func (uc *ChatUsecase) lookupAwaitEntry(sessionID, toolCallID string) (awaitChanEntry, bool) {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	if toolCallID != "" {
		if entry, ok := uc.awaitChans[awaitChanScopeKey(sessionID, toolCallID)]; ok {
			return entry, true
		}
	}
	entry, ok := uc.awaitChans[sessionID]
	return entry, ok
}

// TrySendAwaitChannel attempts to send msg to the await channel for sessionID.
// It holds a read lock while checking the entry and sending, which prevents
// the GC goroutine from closing the done channel concurrently.
func (uc *ChatUsecase) TrySendAwaitChannel(sessionID string, msg AwaitReplyMsg) bool {
	return uc.TrySendAwaitChannelForTool(sessionID, "", msg)
}

// TrySendAwaitChannelForTool sends to the tool-scoped channel (falling back
// to the session-level slot, same lookup as LoadAwaitChannelForTool).
func (uc *ChatUsecase) TrySendAwaitChannelForTool(sessionID, toolCallID string, msg AwaitReplyMsg) bool {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	var entry awaitChanEntry
	var ok bool
	if toolCallID != "" {
		entry, ok = uc.awaitChans[awaitChanScopeKey(sessionID, toolCallID)]
	}
	if !ok {
		entry, ok = uc.awaitChans[sessionID]
	}
	if !ok {
		return false
	}

	select {
	case <-entry.done:
		return false
	default:
	}
	select {
	case entry.ch <- msg:
		return true
	default:
		return false
	}
}

func (uc *ChatUsecase) PersistAwaitMarkers(ctx context.Context, sessionID, runID string, meta ChatAwaitMeta) {
	uc.persist.PersistAwaitMarkers(ctx, sessionID, runID, meta)
}

func (uc *ChatUsecase) ClearAwaitingRunState(ctx context.Context, sessionID string) {
	uc.persist.ClearAwaitingRunState(ctx, sessionID)
}

func (uc *ChatUsecase) Close() {
	if uc.bgCancel != nil {
		uc.bgCancel()
	}
	uc.pending.Close()
}

func (uc *ChatUsecase) StartBackgroundGoroutines() {
	ctx, cancel := context.WithCancel(context.Background())
	uc.bgCancel = cancel
	safego.Go(ctx, "chat-usecase-gc", func() {
		ticker := time.NewTicker(awaitChanGCInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				uc.mu.Lock()
				for sid, entry := range uc.awaitChans {
					if strings.TrimSpace(sid) == "" {
						close(entry.done)
						delete(uc.awaitChans, sid)
						continue
					}
					if now.Sub(entry.createdAt) > awaitChanMaxAge {

						sess := sid
						if i := strings.IndexByte(sid, 0); i >= 0 {
							sess = sid[:i]
						}
						if uc.runs.HasActive(sess) {
							continue
						}
						uc.lg.Warn("await channel expired, cleaning up", loggateway.StepID("session.compress"), loggateway.SessionID(sess), loggateway.Str("age", now.Sub(entry.createdAt).Round(time.Second).String()))
						close(entry.done)
						delete(uc.awaitChans, sid)
					}
				}
				uc.mu.Unlock()
			}
		}
	})
}
