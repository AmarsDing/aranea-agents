package office

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/loggateway"
)

// ============================================================================
// 每日邮件摘要（需求文档 §1「信息收集+提示」；验收 #8：作战总台+IM 各一条）
//
// 投递三腿（与提醒 K-4 同构）：
//   1. 通知中心落表（kind=mail_digest，收件箱可见）；
//   2. IM 腿：复用 reminder_sink 声明式接收点（幂等键 mail-digest-<user>-
//      <date>-<channelID>）；
//   3. 作战总台卡片：ceoboard 事件（UID aranea:mail-digest-<user>-<date>，
//      幂等）。
// 调度：MailDigestWorker 每分钟 tick，本地时间命中摘要时刻（默认 08:00，
// ARANEA_MAIL_DIGEST_HOUR 可配）且当日未发则触发；TriggerDigest RPC 提供
// 手动触发（验收 #8 入口）。
// ============================================================================

// MailDigestCandidate 摘要候选人（窗口内有邮件的用户）。
type MailDigestCandidate struct {
	UserID      string
	WorkspaceID string
	Count       int
}

// MailDigestUsecase 摘要生成与投递。
type MailDigestUsecase struct {
	messages MailMessageRepo
	notify   NotificationRepo
	im       ReminderDelivery  // nil=仅通知中心+作战总台
	ceoboard MailCeoboardSink  // nil=仅通知中心+IM
	lg       loggateway.Logger
	now      func() time.Time
}

func NewMailDigestUsecase(messages MailMessageRepo, notify NotificationRepo, im ReminderDelivery, ceoboard MailCeoboardSink, lg loggateway.Logger) *MailDigestUsecase {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MailDigestUsecase{messages: messages, notify: notify, im: im, ceoboard: ceoboard, lg: lg, now: time.Now}
}

// Trigger 手动/调度触发一人摘要：窗口内邮件聚合成一条摘要，三腿投递。
// since 零值=默认最近 24h（「昨夜今晨」窗口）。返回 (邮件数, 摘要文本)。
func (u *MailDigestUsecase) Trigger(ctx context.Context, userID, workspaceID string, since time.Time) (int, string, error) {
	if since.IsZero() {
		since = u.now().Add(-24 * time.Hour)
	}
	msgs, err := u.messages.ListMailMessagesSince(ctx, userID, since, 200)
	if err != nil {
		return 0, "", err
	}
	summary := mailDigestSummary(msgs, since, u.now())
	now := u.now()
	dayKey := now.Format("2006-01-02")

	// IM 腿（幂等键含日期+通道，同日重触不重复打扰）。
	if u.im != nil {
		sinks, err := u.im.ListSinks(ctx, workspaceID)
		if err != nil {
			u.lg.Warn("mail digest: list sinks failed", loggateway.Err(err))
		}
		for _, s := range sinks {
			key := "mail-digest-" + userID + "-" + dayKey + "-" + s.ChannelID
			if err := u.im.Enqueue(ctx, s, summary, key); err != nil {
				u.lg.Warn("mail digest: enqueue failed",
					loggateway.Str("channel_id", s.ChannelID), loggateway.Err(err))
			}
		}
	}

	// 作战总台卡片（UID 幂等）。
	if u.ceoboard != nil {
		if err := u.ceoboard.EmitMailDigest(ctx, userID, "邮件摘要（"+dayKey+"）", summary, now); err != nil {
			u.lg.Warn("mail digest: ceoboard emit failed", loggateway.Err(err))
		}
	}

	// 通知中心落表（收件箱腿，最后做——前两腿失败可重触不丢收件箱）。
	n := &Notification{
		ID:     "ntf_" + uuid.NewString(),
		UserID: userID,
		Kind:   NotifyKindMailDigest,
		Title:  "邮件摘要：" + dayKey,
		Body:   summary,
		Link:   "/mail",
	}
	if err := u.notify.CreateNotification(ctx, n); err != nil {
		return len(msgs), summary, fmt.Errorf("create digest notification: %w", err)
	}
	return len(msgs), summary, nil
}

// mailDigestSummary 摘要文本：计数 + 前 5 封主题行（等宽可读）。
func mailDigestSummary(msgs []*MailMessage, since, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "窗口 %s ~ %s 共 %d 封邮件。", since.Format("01-02 15:04"), now.Format("01-02 15:04"), len(msgs))
	const top = 5
	for i, m := range msgs {
		if i >= top {
			break
		}
		from := m.Sender
		if from == "" {
			from = "(unknown)"
		}
		fmt.Fprintf(&b, "\n· %s — %s", m.Subject, from)
	}
	if len(msgs) > top {
		fmt.Fprintf(&b, "\n…另有 %d 封，见 /mail。", len(msgs)-top)
	}
	return b.String()
}

// MailDigestWorker 每日摘要调度（1min tick 命中时刻触发，当日去重）。
type MailDigestWorker struct {
	messages MailMessageRepo
	digest   *MailDigestUsecase
	lg       loggateway.Logger
	hour     int           // 本地触发时刻（时），默认 8
	lastFiredDay string    // 当日去重闸
	now      func() time.Time
}

// NewMailDigestWorker hour 缺省 8；ARANEA_MAIL_DIGEST_HOUR（0-23）可配。
func NewMailDigestWorker(messages MailMessageRepo, digest *MailDigestUsecase, lg loggateway.Logger) *MailDigestWorker {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	hour := 8
	if v := strings.TrimSpace(os.Getenv("ARANEA_MAIL_DIGEST_HOUR")); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h >= 0 && h <= 23 {
			hour = h
		}
	}
	return &MailDigestWorker{messages: messages, digest: digest, lg: lg, hour: hour, now: time.Now}
}

// Start 阻塞运行直到 ctx 取消（workers.go goAfterReady 承载）。
func (w *MailDigestWorker) Start(ctx context.Context) {
	tk := time.NewTicker(time.Minute)
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

func (w *MailDigestWorker) tick(ctx context.Context) {
	now := w.now()
	dayKey := now.Format("2006-01-02")
	if now.Hour() != w.hour || w.lastFiredDay == dayKey {
		return
	}
	w.lastFiredDay = dayKey
	since := now.Add(-24 * time.Hour)
	cands, err := w.messages.ListMailDigestUsers(ctx, since)
	if err != nil {
		w.lg.Warn("mail digest: list candidates failed", loggateway.Err(err))
		return
	}
	for _, c := range cands {
		if _, _, err := w.digest.Trigger(ctx, c.UserID, c.WorkspaceID, since); err != nil {
			w.lg.Warn("mail digest: trigger failed",
				loggateway.Str("user_id", c.UserID), loggateway.Err(err))
		}
	}
}
