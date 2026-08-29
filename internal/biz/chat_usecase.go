package biz

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// awaitChanMaxAge is the maximum lifetime of an await channel entry before GC
// reclaims it. Reduced from 30m to 10m as a safety net: the primary cleanup
// mechanism is event-driven (SetRunStatus deletes on terminal status), and the
// GC ticker serves as a fallback for edge cases.
const awaitChanMaxAge = 10 * time.Minute

// awaitChanGCInterval is the interval at which the background GC ticker scans
// for stale await channel entries.
const awaitChanGCInterval = 5 * time.Minute

// AwaitReplyMsg is the message sent over an await channel when a user replies.
type AwaitReplyMsg struct {
	RunID string
	Reply string
	// ToolCallID routes the reply to one specific tool-confirmation await when
	// parallel tool calls block on the same session (BUG-02, chat-e2e-20260823).
	// Empty = session-level slot (free-text await_user_reply / clarification).
	ToolCallID string
}

// AwaitChannel is the concrete channel type used for await-reply coordination.
type AwaitChannel = chan AwaitReplyMsg

type awaitChanEntry struct {
	ch        AwaitChannel
	done      chan struct{}
	createdAt time.Time
}

// awaitChanScopeKey builds the awaitChans registry key. An empty toolCallID
// uses the session-level slot (legacy paths never run in parallel); a non-empty
// toolCallID gets its own slot so parallel tool-confirmation awaits no longer
// overwrite each other (previously a single per-session slot was closed and
// replaced by each new registration, breaking concurrent confirms).
func awaitChanScopeKey(sessionID, toolCallID string) string {
	if toolCallID == "" {
		return sessionID
	}
	return sessionID + "\x00" + toolCallID
}

type ChatRunStatus struct {
	RunID     string
	Status    string
	ErrMsg    string
	UpdatedAt time.Time
}

// Stability:evolving
type ChatRunGateway interface {
	HasActive(sessionID string) bool
	Cancel(sessionID, reason string) (stopped bool, runID string)
	EnqueueUserMessage(sessionID, content string) (bool, error)
	SetStatus(sessionID, runID, status, errMsg string)
	GetStatus(sessionID string) (ChatRunStatus, bool)
}

// Stability:evolving
type ChatSessionLocker interface {
	Lock(sessionID string) func()
}

// Stability:evolving
type ChatPendingQueue interface {
	List(sessionID string) []PendingQueueEntry
	Enqueue(sessionID, content string) string
	EnqueueFollowup(sessionID, content string) string
	// EnqueueInject 追加静默上下文条目（P2-3 inject 级）：不单独唤醒 turn，
	// 仅随下一条 followup 作为上下文前缀合入。
	EnqueueInject(sessionID, content string) string
	// FlushLeadingInjects 仅在整队全为 inject 条目时原子清空并返回它们
	// （N2 满队死锁冲刷）；否则返回 nil、不动队列。
	FlushLeadingInjects(sessionID string) []PendingQueueEntry
	Dequeue(sessionID string) (PendingQueueEntry, bool)
	Peek(sessionID string) (PendingQueueEntry, bool)
	Remove(sessionID, entryID string) bool
	Update(sessionID, entryID, newContent string) bool
	PromoteToFront(sessionID, pendingID string) error
	SetPriority(sessionID, pendingID string, priority int) error
	Close()
}

type PendingQueueEntry struct {
	ID        string
	Content   string
	Status    string
	CreatedAt string
	// Kind 注入级别（P2-3）："" / ChatEnqueueKindFollowup = 追问；
	// ChatEnqueueKindInject = 静默上下文。空值兼容旧快照。
	Kind string
}

type ChatAwaitMeta struct {
	Kind       string
	ToolKey    string
	ToolCallID string
}

const (
	ChatAwaitKindReply       = "reply"
	ChatAwaitKindToolConfirm = "tool_confirm"
)

const (
	ChatEnqueueRejectNone        = ""
	ChatEnqueueRejectNoActiveRun = "no_active_run"
	ChatEnqueueRejectQueueFull   = "queue_full"
)

// ChatPendingQueueCap is the per-session pending depth (must match
// runtime.MaxPendingPerSession). When the queue is at this cap, new
// SendChatMessage/steer attempts are rejected with CHAT_QUEUE_FULL
// instead of silently steering into the active turn (G1 / S12 dual-null).
const ChatPendingQueueCap = 32

// P2-3 Inbox 三级注入语义（DSH followup/steer/inject 对齐）。
const (
	// ChatEnqueueKindSteer 用户插话（默认）：优先经框架 steer 队列在下一
	// step 边界消费；活动 runner 不支持 steer 时回落为 followup 排队。
	ChatEnqueueKindSteer = "steer"
	// ChatEnqueueKindFollowup 显式追问：跳过 steer 直接入 pending 队列，
	// 当前 turn 结束后作为新 turn 输入。
	ChatEnqueueKindFollowup = "followup"
	// ChatEnqueueKindInject 系统上下文静默排队：不要求活动 run、不尝试
	// steer、不单独唤醒 turn，仅随下一条 followup 作为上下文前缀合入。
	ChatEnqueueKindInject = "inject"
)

// Stability:evolving
type ChatRunStatusPersister interface {
	PersistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) error
	PersistAwaitMarkers(ctx context.Context, sessionID, runID string, meta ChatAwaitMeta)
	ClearAwaitingRunState(ctx context.Context, sessionID string)
}

// Stability:evolving
type ChatEventPublisher interface {
	PublishRunStatus(sessionID, runID, status, errMsg string)
	PublishMessageQueued(sessionID string)
}

type ChatUsecase struct {
	runs       ChatRunGateway
	locker     ChatSessionLocker
	pending    ChatPendingQueue
	persist    ChatRunStatusPersister
	publisher  ChatEventPublisher
	mu         sync.RWMutex
	awaitChans map[string]awaitChanEntry
	bgCancel   context.CancelFunc
	lg         loggateway.Logger

	// Optional ports for provider/model resolution (BA4). Nil-safe: when not
	// wired, ResolveProviderModel and SyncSessionProviderModel degrade to no-ops.
	refineLLM      RefineLLMLookup
	modelLister    TeamModelCatalog
	sessionUpdater SessionCRUDPort
}

func NewChatUsecase(
	runs ChatRunGateway,
	locker ChatSessionLocker,
	pending ChatPendingQueue,
	persist ChatRunStatusPersister,
	publisher ChatEventPublisher,
	lg loggateway.Logger,
) *ChatUsecase {
	return &ChatUsecase{
		runs:       runs,
		locker:     locker,
		pending:    pending,
		persist:    persist,
		publisher:  publisher,
		awaitChans: make(map[string]awaitChanEntry),
		lg:         lg,
	}
}

func (uc *ChatUsecase) LockSession(sessionID string) func() {
	return uc.locker.Lock(sessionID)
}

func (uc *ChatUsecase) HasActiveRun(sessionID string) bool {
	return uc.runs.HasActive(sessionID)
}

func (uc *ChatUsecase) CancelRun(sessionID string) (bool, string) {
	return uc.runs.Cancel(sessionID, "user_cancel")
}

// InterruptAndSendMessage promotes a pending message to the front of the queue,
// marks it as high priority, and cancels the current turn so the pending queue
// processor picks it up next.
func (uc *ChatUsecase) InterruptAndSendMessage(ctx context.Context, sessionID, pendingEntryID string) error {
	if err := uc.pending.PromoteToFront(sessionID, pendingEntryID); err != nil {
		return err
	}
	if err := uc.pending.SetPriority(sessionID, pendingEntryID, 1); err != nil {
		return err
	}
	if stopped, runID := uc.runs.Cancel(sessionID, "user_interrupt"); !stopped {
		uc.lg.Warn("interrupt cancel found no active run; pending message stays queued",
			loggateway.StepID("chat.interrupt_cancel"),
			loggateway.SessionID(sessionID),
			loggateway.Str("run_id", runID))
	}
	return nil
}

// SetRunStatusWithError updates the run status with WBPF
// (Write-Before-Publish-Fire) semantics and returns the persistence error so
// callers in critical paths can react (e.g., retry or fail the parent
// operation). State-machine rejection is also surfaced as an error.
//
// Stability:evolving
func (uc *ChatUsecase) SetRunStatusWithError(ctx context.Context, sessionID, runID, status, errMsg string) error {
	// ── 1. State machine validation (AS-FSM-01 / FSM2) ──────────────────────
	current, hasCurrent := uc.runs.GetStatus(sessionID)
	fromState := RunStateNone
	if hasCurrent {
		fromState = ParseRunState(current.Status)
	}
	toState := ParseRunState(status)
	// Skip validation when there is no prior record (bootstrap/crash recovery).
	if hasCurrent && fromState != toState {
		sm := NewRunStateMachine()
		if !sm.CanTransition(fromState, toState) {
			uc.lg.Warn("invalid run status transition rejected",
				loggateway.StepID("chat.set_run_status"),
				loggateway.SessionID(sessionID),
				loggateway.Str("run_id", runID),
				loggateway.Str("from", string(fromState)),
				loggateway.Str("to", string(toState)))
			return apierror.BadRequest("CHAT", "invalid run status transition: %s → %s", fromState, toState)
		}
	}

	// ── 2. WBPF: persist first, then update memory and publish (BD1) ────────
	if err := uc.persist.PersistRunStatus(ctx, sessionID, runID, status, errMsg); err != nil {
		uc.lg.Error("persist run status failed; skipping in-memory and event update to maintain DB/memory consistency",
			loggateway.StepID("chat.persist_run_status"),
			loggateway.SessionID(sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
		return err
	}

	// ── 3. DB write succeeded → propagate to memory and event bus ──────────
	uc.runs.SetStatus(sessionID, runID, status, errMsg)
	uc.publisher.PublishRunStatus(sessionID, runID, status, errMsg)

	// Proactively clean up await channels when run reaches a terminal status.
	// This prevents memory leaks when runs end without going through the normal
	// await reply flow (e.g., hard budget, cancellation, unexpected failure).
	// Session-wide sweep: parallel tool confirmations each hold a tool-scoped
	// entry (BUG-02), a single-slot delete would leak them until GC.
	switch strings.ToLower(strings.TrimSpace(status)) {
	case SessionRunPhaseCompleted, SessionRunPhaseFailed, SessionRunPhaseCancelled:
		uc.DeleteAwaitChannelsForSession(sessionID)
	}
	return nil
}

func (uc *ChatUsecase) GetRunStatus(sessionID string) (ChatRunStatus, bool) {
	return uc.runs.GetStatus(sessionID)
}

func (uc *ChatUsecase) GetPendingMessages(sessionID string) []PendingQueueEntry {
	return uc.pending.List(sessionID)
}

func (uc *ChatUsecase) EnqueuePendingMessage(sessionID, content string) string {
	return uc.pending.Enqueue(sessionID, content)
}

func (uc *ChatUsecase) CancelPendingMessage(sessionID, pendingID string) bool {
	return uc.pending.Remove(sessionID, pendingID)
}

func (uc *ChatUsecase) UpdatePendingMessage(sessionID, pendingID, content string) bool {
	return uc.pending.Update(sessionID, pendingID, content)
}

func (uc *ChatUsecase) DequeuePendingMessage(sessionID string) (PendingQueueEntry, bool) {
	return uc.pending.Dequeue(sessionID)
}

// PeekPendingMessage returns the head of the pending queue without removing it.
// Used by processPendingQueue to perform an atomic check-and-dequeue under the
// session lock, eliminating the TOCTOU race between Dequeue and HasActive.
func (uc *ChatUsecase) PeekPendingMessage(sessionID string) (PendingQueueEntry, bool) {
	return uc.pending.Peek(sessionID)
}

func (uc *ChatUsecase) EnqueueUserMessage(sessionID, content string, mergeFollowup bool) (accepted, queued bool, pendingID, rejectReason string, err error) {
	return uc.EnqueueUserMessageWithKind(sessionID, content, ChatEnqueueKindSteer, mergeFollowup)
}

// EnqueueUserMessageWithKind 按注入级别（P2-3）路由入队：
//   - steer（默认）：活动 run 优先经框架 steer（下一 step 边界消费），不支持
//     时回落 followup 排队；
//   - followup：跳过 steer 直接排队，当前 turn 结束后作为新 turn 输入；
//   - inject：静默上下文——不要求活动 run、不尝试 steer，仅排队等待随下一条
//     followup 合入（见 SplitLeadingInjects / MergeInjectContext）。
func (uc *ChatUsecase) EnqueueUserMessageWithKind(sessionID, content, kind string, mergeFollowup bool) (accepted, queued bool, pendingID, rejectReason string, err error) {
	unlock := uc.locker.Lock(sessionID)
	defer unlock()

	// inject 级：静默排队，无活动 run 也接受（不唤醒语义本身即"不打扰"）。
	if kind == ChatEnqueueKindInject {
		pid := uc.pending.EnqueueInject(sessionID, content)
		if pid == "" {
			return false, false, "", ChatEnqueueRejectQueueFull, nil
		}
		uc.publisher.PublishMessageQueued(sessionID)
		return true, true, pid, "", nil
	}

	if !uc.runs.HasActive(sessionID) {
		return false, false, "", ChatEnqueueRejectNoActiveRun, nil
	}

	// G1：队列已满时禁止再走 steer 成功空回执（S12 队满期 POST /v1/chat/messages
	// 返回 userMessage/agentMessage 双 null）。满员全 inject 仍走冲刷合入。
	if len(uc.pending.List(sessionID)) >= ChatPendingQueueCap {
		if mergedPid, rescued := uc.rescueFullInjectQueue(sessionID, content); rescued {
			uc.publisher.PublishMessageQueued(sessionID)
			return true, true, mergedPid, "", nil
		}
		return false, false, "", ChatEnqueueRejectQueueFull, nil
	}

	// followup 级：显式追问，跳过 steer 保证成为独立新 turn。
	if kind != ChatEnqueueKindFollowup {
		enqueued, enqueueErr := uc.runs.EnqueueUserMessage(sessionID, content)
		if enqueueErr != nil {
			return false, false, "", "", enqueueErr
		}
		if enqueued {
			uc.publisher.PublishMessageQueued(sessionID)
			return true, false, "", "", nil
		}
	}

	var pid string
	if mergeFollowup {
		pid = uc.pending.EnqueueFollowup(sessionID, content)
	} else {
		pid = uc.pending.Enqueue(sessionID, content)
	}
	if pid == "" {
		// N2 满队 inject 死锁（session-eval-20260827 S12 / C5-③）：队列被
		// inject 占满时新 followup 永远入不了队、inject 永远等不到合入载体
		// （「已接受但永滞留」）。冲刷合入：滞留 inject 全部出队，作为上下文
		// 前缀并入本条消息入队——内容不丢失、队列恢复流通。
		if mergedPid, rescued := uc.rescueFullInjectQueue(sessionID, content); rescued {
			uc.publisher.PublishMessageQueued(sessionID)
			return true, true, mergedPid, "", nil
		}
		return false, false, "", ChatEnqueueRejectQueueFull, nil
	}
	uc.publisher.PublishMessageQueued(sessionID)
	return true, true, pid, "", nil
}

// rescueFullInjectQueue 处理满队全 inject 死锁（N2）：FlushLeadingInjects
// 仅在整队全 inject 时原子出队全部条目（含任何 followup 时既有
// processPendingQueue 循环会正常冲刷，无死锁，不 rescue）。会话锁已持有，
// 同会话入队串行，冲刷后队列必有空位，重入队不会再次满员。
func (uc *ChatUsecase) rescueFullInjectQueue(sessionID, content string) (string, bool) {
	flushed := uc.pending.FlushLeadingInjects(sessionID)
	if len(flushed) == 0 {
		return "", false
	}
	contents := make([]string, 0, len(flushed))
	for _, e := range flushed {
		contents = append(contents, e.Content)
	}
	merged := MergeInjectContext(contents, content)
	pid := uc.pending.Enqueue(sessionID, merged)
	if pid == "" {
		// 理论不可达（见上注释）；若发生，inject 内容仅存于本条被拒绝的
		// 消息中——打 Error 留痕，调用方收到 queue_full 可重试。
		uc.lg.Error("rescueFullInjectQueue: re-enqueue failed after flush, inject contents stranded",
			loggateway.StepID("chat.enqueue.rescue_reenqueue_failed"),
			loggateway.Str("session_id", sessionID),
			loggateway.Int("flushed_count", len(flushed)),
			loggateway.Str("merged_content", merged))
		return "", false
	}
	uc.lg.Info("full inject queue rescued: stranded injects merged into new message",
		loggateway.StepID("chat.enqueue.rescue_full_injects"),
		loggateway.Str("session_id", sessionID),
		loggateway.Int("flushed_count", len(flushed)))
	return pid, true
}

// ConsumeLeadingInjects dequeues head-of-queue inject entries so a newly
// started user turn can merge them as context (N2: normal message consumes
// stranded injects in one turn). Callers must hold the session lock.
func (uc *ChatUsecase) ConsumeLeadingInjects(sessionID string) []string {
	if uc == nil || uc.pending == nil {
		return nil
	}
	var out []string
	for {
		peek, ok := uc.pending.Peek(sessionID)
		if !ok || peek.Kind != ChatEnqueueKindInject {
			return out
		}
		e, ok := uc.pending.Dequeue(sessionID)
		if !ok {
			return out
		}
		if e.Kind != ChatEnqueueKindInject {
			return out
		}
		out = append(out, e.Content)
	}
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
	// Entry has been logically deleted (done closed) but not yet removed from map.
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
	// If done is already closed, the entry has been logically deleted.
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
						// 活跃 run 的 await channel 是合法在用（HITL 等待用户回复
						// 可远超 maxAge），交给 SetRunStatus 终态时的事件驱动清理；
						// GC 只回收 run 已不存在/已结束的泄漏条目。
						// tool 作用域 key（sessionID\x00toolCallID，BUG-02）按会话部分判定。
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
