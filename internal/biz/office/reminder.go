package office

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	loggateway "aranea-agents/pkg/loggateway"
)

// ============================================================================
// ReminderWorker —— K-4 提醒扫描（每分钟窗口）→ 通知中心落表 + IM 通道投递
//
// 投递语义：
//   - at-least-once：IM 幂等入队在先（键 sched-rem-<eventID>-<channelID>），
//     通知落表+事件置标事务在后；进程崩溃于两腿之间时下一 tick 重走，IM 侧
//     幂等键去重、库侧 reminded_at 置标拦截；
//   - 迟到补投：停机错过的窗口在恢复后首个 tick 补发（ListDueReminders 的
//     due 判定不含上界）；
//   - IM 接收点为声明式：channel 元数据 reminder_sink=true +
//     reminder_recipient=<平台接收方 id>（装配期解析，见 wire_office.go）；
//     无声明=仅通知中心，IM 腿静默跳过。
// 审计：通知行（收件箱）+ channel_delivery 行（IM 侧）即投递留痕。
// ============================================================================

// ReminderSink 提醒 IM 接收点。
type ReminderSink struct {
	ChannelID string
	Platform  string
	Recipient string
}

// ReminderDelivery 提醒 IM 投递窄口（wire 适配 channel 域实现）。
// Stability:evolving
type ReminderDelivery interface {
	// ListSinks 返回工作区声明的提醒接收点（无声明返回空集，非错误）。
	ListSinks(ctx context.Context, workspaceID string) ([]ReminderSink, error)
	// Enqueue 幂等入队一条文本投递。
	Enqueue(ctx context.Context, sink ReminderSink, text, idempotencyKey string) error
}

// ReminderStore 提醒投递事务口（通知落表 + 事件置标原子完成）。
// Stability:evolving
type ReminderStore interface {
	// DeliverReminderTx 同事务：插入 schedule_reminder 通知 + 置事件
	// reminded_at。事件已被并发 tick 置标时返回 delivered=false（此刻 IM 腿
	// 可能已重入，靠幂等键去重；收件箱腿由置标保证恰好一次）。
	DeliverReminderTx(ctx context.Context, ev *Event, n *Notification, at time.Time) (bool, error)
}

// ReminderWorker 周期扫描到期提醒。
type ReminderWorker struct {
	events   EventRepo
	store    ReminderStore
	im       ReminderDelivery // nil = 仅通知中心
	lg       loggateway.Logger
	interval time.Duration
	now      func() time.Time // 测试注入
}

func NewReminderWorker(events EventRepo, store ReminderStore, im ReminderDelivery, lg loggateway.Logger) *ReminderWorker {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ReminderWorker{events: events, store: store, im: im, lg: lg, interval: time.Minute, now: time.Now}
}

// reminderScanBatch 单 tick 拾取上限（个人日程体量下远超实际）。
const reminderScanBatch = 200

// Start 阻塞运行直到 ctx 取消（workers.go goAfterReady 承载）。
func (w *ReminderWorker) Start(ctx context.Context) {
	w.tick(ctx)
	tk := time.NewTicker(w.interval)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			w.tick(ctx)
		}
	}
}

func (w *ReminderWorker) tick(ctx context.Context) {
	due, err := w.events.ListDueReminders(ctx, w.now(), reminderScanBatch)
	if err != nil {
		w.lg.Warn("office reminder scan failed", loggateway.Err(err))
		return
	}
	for _, ev := range due {
		if err := w.deliver(ctx, ev); err != nil {
			// 单条失败不阻断批次；下一 tick 重试（reminded_at 未置）。
			w.lg.Warn("office reminder deliver failed", loggateway.Err(err), loggateway.Str("event_id", ev.ID))
		}
	}
}

// deliver 单条投递：IM 幂等入队在先 → 通知落表+置标事务在后。
func (w *ReminderWorker) deliver(ctx context.Context, ev *Event) error {
	text := reminderText(ev)
	if w.im != nil {
		sinks, err := w.im.ListSinks(ctx, ev.WorkspaceID)
		if err != nil {
			return fmt.Errorf("list reminder sinks: %w", err)
		}
		for _, s := range sinks {
			key := "sched-rem-" + ev.ID + "-" + s.ChannelID
			if err := w.im.Enqueue(ctx, s, text, key); err != nil {
				return fmt.Errorf("enqueue reminder to channel %s: %w", s.ChannelID, err)
			}
		}
	}
	n := &Notification{
		ID:     "ntf_" + uuid.NewString(),
		UserID: ev.UserID,
		Kind:   NotifyKindScheduleReminder,
		Title:  "日程提醒：" + ev.Title,
		Body:   text,
		Link:   "/schedule",
	}
	delivered, err := w.store.DeliverReminderTx(ctx, ev, n, w.now())
	if err != nil {
		return err
	}
	if !delivered {
		return nil // 并发 tick 已投递
	}
	return nil
}

// reminderText 提醒正文（事件时区呈现起止；等宽可读格式）。
func reminderText(ev *Event) string {
	loc := locOrUTC(ev.Timezone)
	start := ev.StartAt.In(loc)
	end := ev.EndAt.In(loc)
	return fmt.Sprintf("%s\n时间：%s ~ %s", ev.Title, start.Format("2006-01-02 15:04"), end.Format("15:04"))
}

func locOrUTC(tz string) *time.Location {
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	if loc, err := time.LoadLocation(DefaultTimezone); err == nil {
		return loc
	}
	return time.UTC
}
