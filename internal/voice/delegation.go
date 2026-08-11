package voice

import (
	"sync"
	"sync/atomic"

	"aranea-agents/pkg/loggateway"
)

// DelegationStatus 委派条目状态。
type DelegationStatus string

const (
	// DelegationPending 已登记待绑定：工具已提交，TaskCreated 尚未到达。
	DelegationPending DelegationStatus = "pending"
	// DelegationBound 已绑定 taskID（TaskCreated 内容匹配完成），等待终态。
	DelegationBound DelegationStatus = "bound"
)

// DelegationEntry 一条语音委派记录：voice session → spirit session 的任务映射。
type DelegationEntry struct {
	// RegID 登记序号（Register 返回值，单调递增）。MarkSubmitFailed 按 RegID
	// 精确移除，避免同内容并发委派的误删。
	RegID           int64
	VoiceSessionID  string
	SpiritSessionID string
	// Content 提交给精灵的任务原文（= TurnInput.Content =
	// TaskCreatedEvent.Task.UserMessage，内容匹配绑定的依据）。
	Content string
	TaskID  string // pending 时空置；BindTask 后填入
	Status  DelegationStatus
}

// DelegationNotice 是 registry → voice Session 的带外通知（watcher 回调载荷）。
// 覆盖「提交同步失败」这类不产生总线事件的路径（turn 未受理即无 TaskCreated）。
type DelegationNotice struct {
	Kind    string // NoticeDelegationSubmitFailed
	Message string // 口播文本（已按 TTS 口语约束组织）
}

// NoticeDelegationSubmitFailed 委派提交同步失败（准入拒绝 / DB 错误）。
const NoticeDelegationSubmitFailed = "submit_failed"

// delegationMaxEntries 是条目上限。正常路径条目由 voice eventLoop 终态消费
// 移除；非语音入口（文本直聊语音助手）无 eventLoop 消费，上限兜底防堆积，
// 满时淘汰最旧条目（该委派的终态播报随之放弃，结果仍在 chat 页可见）。
const delegationMaxEntries = 64

// DelegationRegistry 是进程级委派登记表（M74 V9，设计 74 §15）。
//
// Wire 单例双向注入：工具（service 层 agent build）Register/MarkSubmitFailed；
// voice Session（server 层）三路分流 BindTask/CompleteTask/SetWatcher。
// 纯内存实现：进程重启后条目丢失，委派结果退化为在 chat 页查看（设计决策）。
//
// 数据量级：桌面单用户，同时在途委派个位数，slice 线性扫描足够。
type DelegationRegistry struct {
	mu       sync.Mutex
	nextID   atomic.Int64
	entries  []DelegationEntry
	watchers map[string]func(DelegationNotice)
	lg       loggateway.Logger
}

func NewDelegationRegistry(lg loggateway.Logger) *DelegationRegistry {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &DelegationRegistry{
		watchers: map[string]func(DelegationNotice){},
		lg:       lg.With(loggateway.Domain("voice_delegation")),
	}
}

// Register 登记一条 pending 委派，返回 RegID。必须先注册后提交（工具保证），
// 消除 TaskCreated 先于登记到达的漏绑窗口。签名用原始参数：*DelegationRegistry
// 借此直接满足工具侧窄端口（service 注入零 adapter）。
func (r *DelegationRegistry) Register(voiceSessionID, spiritSessionID, content string) int64 {
	if r == nil {
		return 0
	}
	id := r.nextID.Add(1)
	r.mu.Lock()
	if len(r.entries) >= delegationMaxEntries {
		r.entries = r.entries[1:]
	}
	r.entries = append(r.entries, DelegationEntry{
		RegID:           id,
		VoiceSessionID:  voiceSessionID,
		SpiritSessionID: spiritSessionID,
		Content:         content,
		Status:          DelegationPending,
	})
	r.mu.Unlock()
	return id
}

// BindTask 按 (spirit_session_id + 内容精确匹配) 绑定最早的 pending 条目
// （FIFO：同内容重复委派时，TaskCreated 顺序与提交顺序一致）。返回拥有该
// 委派的 voice session id。
func (r *DelegationRegistry) BindTask(spiritSessionID, content, taskID string) (string, bool) {
	if r == nil || taskID == "" {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.entries {
		e := &r.entries[i]
		if e.Status != DelegationPending {
			continue
		}
		if e.SpiritSessionID == spiritSessionID && e.Content == content {
			e.TaskID = taskID
			e.Status = DelegationBound
			return e.VoiceSessionID, true
		}
	}
	return "", false
}

// CompleteTask 终态（TaskCompleted/TaskFailed）按 (voice_session_id +
// spirit_session_id + taskID) 取出并移除条目。voiceSessionID 限定所有者：
// 事件总线全量广播（V2Bus.Subscribe 忽略过滤参数），非所有者会话的
// eventLoop 调用不得消费条目（防截胡导致所有者播报丢失）。
// ok=false = 非本会话委派任务（调用方丢弃事件）。
func (r *DelegationRegistry) CompleteTask(voiceSessionID, spiritSessionID, taskID string) (DelegationEntry, bool) {
	if r == nil || voiceSessionID == "" || taskID == "" {
		return DelegationEntry{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.entries {
		e := r.entries[i]
		if e.Status == DelegationBound && e.VoiceSessionID == voiceSessionID &&
			e.SpiritSessionID == spiritSessionID && e.TaskID == taskID {
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			return e, true
		}
	}
	return DelegationEntry{}, false
}

// OwnerOf 报告 spiritSessionID 是否为某 voice session 的委派目标
// （eventLoop 三路分流的第二路判定）。
func (r *DelegationRegistry) OwnerOf(spiritSessionID string) (voiceSessionID string, ok bool) {
	if r == nil || spiritSessionID == "" {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.entries {
		if r.entries[i].SpiritSessionID == spiritSessionID {
			return r.entries[i].VoiceSessionID, true
		}
	}
	return "", false
}

// MarkSubmitFailed 提交同步失败（永无 TaskCreated）：按 RegID 移除条目并
// 触发该 voice session 的 watcher 口播失败原因，防 delegation 泄漏空等。
// watcher 在回调外执行（不持锁），回调侧自行保证线程安全。
func (r *DelegationRegistry) MarkSubmitFailed(regID int64, message string) {
	if r == nil || regID == 0 {
		return
	}
	var cb func(DelegationNotice)
	var voiceSessionID string
	r.mu.Lock()
	for i := range r.entries {
		if r.entries[i].RegID == regID {
			voiceSessionID = r.entries[i].VoiceSessionID
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			break
		}
	}
	if voiceSessionID != "" {
		cb = r.watchers[voiceSessionID]
	}
	r.mu.Unlock()
	if cb != nil {
		cb(DelegationNotice{Kind: NoticeDelegationSubmitFailed, Message: message})
	}
}

// SetWatcher 注册 voice session 的通知回调（Session.Start 时调用）。
// 同 session 重复注册覆盖旧回调（单会话单语音连接，重连即新 Session）。
func (r *DelegationRegistry) SetWatcher(voiceSessionID string, cb func(DelegationNotice)) {
	if r == nil || voiceSessionID == "" || cb == nil {
		return
	}
	r.mu.Lock()
	r.watchers[voiceSessionID] = cb
	r.mu.Unlock()
}

// ClearVoiceSession voice session 关闭即清条目与 watcher（设计决策：
// 进程内委派跟随会话生命周期；会话没了，终态播报无接收方）。
func (r *DelegationRegistry) ClearVoiceSession(voiceSessionID string) {
	if r == nil || voiceSessionID == "" {
		return
	}
	r.mu.Lock()
	delete(r.watchers, voiceSessionID)
	kept := r.entries[:0]
	for _, e := range r.entries {
		if e.VoiceSessionID != voiceSessionID {
			kept = append(kept, e)
		}
	}
	r.entries = kept
	r.mu.Unlock()
}
