package office

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/loggateway"
)

// ============================================================================
// MailSyncUsecase —— IMAP 同步（自 workbench misc.go syncOneAccount 平移）
//
// 平移语义：
//   - seq 1..EXISTS 全扫，(account_id,uid=seq) 幂等去重命中即跳过；
//   - 每封邮件一个事务（InsertMessageWithAttachmentsTx：邮件+附件同生共死，
//     附件失败整体回滚可重试）；
//   - 同步成功后 last_uid=EXISTS、last_sync_at=now；
//   - 上游故障（连接/登录/选箱/抓取）→ ErrMailUpstream（service 映 502，
//     明细 WithCause 进日志与审计摘要，不回传客户端原文）。
//
// 变更（aranea 化）：密码经 MailSecretCrypto 解密（仅拨号前内存瞬态）；
// 审计经 MailAuditSink（mail_sync/mail_idle_notify 动作，验收 #7）。
// ============================================================================

// MailAuditSink 邮件域审计窄口（wire 适配 monitor.Usecase.RecordAdminAudit）。
// Stability:evolving
type MailAuditSink interface {
	// RecordMailAudit 记一条审计（action 规范 "<verb>.mail_account"）。
	RecordMailAudit(ctx context.Context, action, resourceID, summary string)
}

// MailSyncUsecase 同步编排。
type MailSyncUsecase struct {
	accounts    MailAccountRepo
	messages    MailMessageRepo
	attachments MailAttachmentRepo // 转发带原附件用；nil 时转发不带附件
	crypto      MailSecretCrypto
	dialer      MailImapDialer
	smtpDialer  MailSmtpDialer
	mxLookup    MailMXLookup
	audit       MailAuditSink // nil=不记审计
	lg          loggateway.Logger
	now         func() time.Time
}

func NewMailSyncUsecase(accounts MailAccountRepo, messages MailMessageRepo, crypto MailSecretCrypto, dialer MailImapDialer, audit MailAuditSink, lg loggateway.Logger) *MailSyncUsecase {
	if dialer == nil {
		dialer = DialMailIMAP
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MailSyncUsecase{accounts: accounts, messages: messages, crypto: crypto, dialer: dialer, smtpDialer: DialMailSMTP, audit: audit, lg: lg, now: time.Now}
}

// WithAttachments 装上附件仓储（转发时带原附件）。
func (u *MailSyncUsecase) WithAttachments(repo MailAttachmentRepo) *MailSyncUsecase {
	u.attachments = repo
	return u
}

// Sync 手动/兜底同步一个账号（属主隔离：id+userID）。返回 (新导入, 幂等命中)。
func (u *MailSyncUsecase) Sync(ctx context.Context, accountID, userID string) (int, int, error) {
	acc, err := u.accounts.GetMailAccount(ctx, accountID, userID)
	if err != nil {
		return 0, 0, err
	}
	imported, skipped, err := u.syncOne(ctx, acc)
	action := "sync.mail_account"
	if err != nil {
		u.recordAuditCtx(ctx, action, accountID, fmt.Sprintf("ok=false imported=%d skipped=%d err=%s", imported, skipped, mailErrSummary(err)))
		return imported, skipped, err
	}
	u.recordAuditCtx(ctx, action, accountID, fmt.Sprintf("ok=true imported=%d skipped=%d", imported, skipped))
	return imported, skipped, nil
}

// mailConnect 解密口令 → 建连 → 登录；失败时返回错误类别（用来记账号状态）。
func (u *MailSyncUsecase) mailConnect(ctx context.Context, acc *MailAccount, password string) (MailImapClient, string, error) {
	cli, err := u.dialer(ctx, acc.Host, acc.Port, acc.TLSMode)
	if err != nil {
		return nil, classifyMailDialErr(err), ErrMailUpstream.WithCause(fmt.Errorf("dial: %w", err))
	}
	if err := cli.Login(acc.Username, password); err != nil {
		cli.Close()
		return nil, MailErrKindAuth, ErrMailUpstream.WithCause(fmt.Errorf("login: %w", err))
	}
	return cli, "", nil
}

func (u *MailSyncUsecase) markAccountStatus(ctx context.Context, acc *MailAccount, kind string) {
	status, lastErr := MailStatusOK, ""
	if kind != "" {
		status, lastErr = mailStatusForKind(kind), kind
	}
	if acc.Status == status && acc.LastError == lastErr {
		return
	}
	if err := u.accounts.UpdateMailAccountStatus(ctx, acc.ID, status, lastErr); err != nil {
		u.lg.Warn("mail account status update failed", loggateway.Err(err))
		return
	}
	acc.Status, acc.LastError = status, lastErr
}

// syncOne 进程内同步（IDLE 推送后复用；属主已由调用方解析）。
// 按 IMAP UID 增量：首次按 sync_since（空 = 全部），之后只取游标之后的 UID；
// 抓取时带回 \Seen / \Flagged，手机上读过的信进来就是已读。
// shortcut: 服务器换了 UIDVALIDITY（邮箱重建）没处理，出现时清掉账号游标重同步即可。
func (u *MailSyncUsecase) syncOne(ctx context.Context, acc *MailAccount) (int, int, error) {
	password, err := u.crypto.DecryptSecret(ctx, acc.PasswordEnc)
	if err != nil {
		return 0, 0, ErrMailUpstream.WithCause(fmt.Errorf("decrypt account password: %w", err))
	}
	cli, kind, err := u.mailConnect(ctx, acc, password)
	if err != nil {
		u.markAccountStatus(ctx, acc, kind)
		return 0, 0, err
	}
	defer cli.Close()
	if _, err := cli.SelectInbox(); err != nil {
		u.markAccountStatus(ctx, acc, MailErrKindProtocol)
		return 0, 0, ErrMailUpstream.WithCause(fmt.Errorf("select inbox: %w", err))
	}
	criteria := "ALL"
	switch {
	case acc.LastUID > 0:
		criteria = fmt.Sprintf("UID %d:*", acc.LastUID+1)
	case acc.SyncSince != nil:
		criteria = "SINCE " + acc.SyncSince.Format("2-Jan-2006")
	}
	uids, err := cli.SearchUIDs(criteria)
	if err != nil {
		u.markAccountStatus(ctx, acc, MailErrKindProtocol)
		return 0, 0, ErrMailUpstream.WithCause(fmt.Errorf("search: %w", err))
	}
	sort.Ints(uids)
	imported, skipped := 0, 0
	maxUID := acc.LastUID
	blocked := u.blockedSet(ctx, acc.UserID)
	for _, uid := range uids {
		if int64(uid) <= acc.LastUID {
			continue // "n:*" 在没有新信时也会回最大的那个 UID
		}
		raw, flags, err := cli.FetchUID(uid)
		if err != nil {
			return imported, skipped, ErrMailUpstream.WithCause(fmt.Errorf("fetch uid %d: %w", uid, err))
		}
		m := ParseMailRaw(raw)
		m.Seen = strings.Contains(flags, `\Seen`)
		m.Flagged = strings.Contains(flags, `\Flagged`)
		msg := mailMessageFromImap(acc, uid, m, u.now)
		if blocked[senderAddr(msg.Sender)] {
			msg.Spam = true // 拒收名单里的发件人：直接进垃圾邮件
		}
		inserted, err := u.messages.InsertMessageWithAttachmentsTx(ctx, msg)
		if err != nil {
			// 邮件+附件同生共死：整体未提交，下个同步周期可重试。
			return imported, skipped, fmt.Errorf("insert message uid=%d: %w", uid, err)
		}
		if int64(uid) > maxUID {
			maxUID = int64(uid)
		}
		if !inserted {
			skipped++
		} else {
			imported++
		}
		// 每 20 封存一次游标：大邮箱首次同步被打断，下次接着来，不用从头再下
		if (imported+skipped)%20 == 0 {
			if err := u.accounts.UpdateMailSyncCursor(ctx, acc.ID, maxUID, u.now()); err != nil {
				return imported, skipped, fmt.Errorf("update sync cursor: %w", err)
			}
		}
	}
	if err := u.accounts.UpdateMailSyncCursor(ctx, acc.ID, maxUID, u.now()); err != nil {
		return imported, skipped, fmt.Errorf("update sync cursor: %w", err)
	}
	u.syncJunk(ctx, cli, acc)
	u.markAccountStatus(ctx, acc, "")
	return imported, skipped, nil
}

// mailMessageFromImap 装配落库实体（Snippet 写时物化：正文前 200 字符）。
func mailMessageFromImap(acc *MailAccount, seq int, m *MailImapMessage, now func() time.Time) *MailMessage {
	body := strings.TrimSpace(m.Body)
	receivedAt := m.Date
	if receivedAt.IsZero() {
		receivedAt = now()
	}
	msg := &MailMessage{
		ID:          "omm_" + uuid.NewString(),
		WorkspaceID: acc.WorkspaceID,
		UserID:      acc.UserID,
		AccountID:   acc.ID,
		UID:         int64(seq),
		Subject:     m.Subject,
		Sender:      m.Sender,
		Body:        body,
		Snippet:     mailSnippet(body),
		ReceivedAt:  &receivedAt,
		CreatedAt:   now(),
		To:            m.To,
		Cc:            m.Cc,
		Seen:          m.Seen,
		Flagged:       m.Flagged,
		Folder:        MailFolderInbox,
		BodyHTML:      m.HTML,
		MessageIDHdr:  m.MessageID,
		ReferencesHdr: m.References,
	}
	for _, a := range m.Attachments {
		msg.Attachments = append(msg.Attachments, &MailAttachment{
			ID:          "omat_" + uuid.NewString(),
			MessageID:   msg.ID,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        int64(len(a.Content)),
			Content:     a.Content,
			CreatedAt:   now(),
		})
	}
	msg.HasAttachments = len(msg.Attachments) > 0
	return msg
}

// mailSnippet 列表摘要：正文前 200 字符（按 rune 截断保 UTF-8 完整）。
func mailSnippet(body string) string {
	rs := []rune(strings.TrimSpace(body))
	if len(rs) > 200 {
		rs = rs[:200]
	}
	return string(rs)
}

func (u *MailSyncUsecase) recordAuditCtx(ctx context.Context, action, resourceID, summary string) {
	if u.audit != nil {
		u.audit.RecordMailAudit(ctx, action, resourceID, summary)
	}
}

// mailErrSummary 审计摘要用的错误简述（截断防上游长响应刷屏；不含凭据——
// IMAP 错误响应不回显口令）。
func mailErrSummary(err error) string {
	s := err.Error()
	const cap = 200
	rs := []rune(s)
	if len(rs) > cap {
		rs = rs[:cap]
	}
	return string(rs)
}

// ============================================================================
// MailIdleSupervisor —— IDLE 推送为主 + 轮询兜底（E-5，workers.go 承载）
//
// 平移 idleMailAccount 语义：IDLE 等推送 → gotNew 后关连接重连同步（IDLE
// 连接状态复杂，重连最稳）。aranea 化：
//   - 进程内 supervisor：每账号一个 goroutine 循环 IDLE（25s 窗，对齐
//     workbench 默认 timeout），推送即同步；
//   - 轮询兜底：全局 ticker（默认 5min）全账号强制同步一轮，覆盖服务端
//     不支持 IDLE / 推送丢失的场景；
//   - 账号集合按周期重扫（30s），新建/删除账号无需重启进程；
//   - 单账号故障指数退避（max 5min），不阻断其他账号。
// ============================================================================

// MailIdleSupervisor IDLE 主轮询兜底 supervisor。
type MailIdleSupervisor struct {
	accounts       MailAccountRepo
	syncer         *MailSyncUsecase
	lg             loggateway.Logger
	idleWait       time.Duration // 单次 IDLE 等待窗
	pollInterval   time.Duration // 轮询兜底周期
	rescanInterval time.Duration // 账号集合重扫周期
}

func NewMailIdleSupervisor(accounts MailAccountRepo, syncer *MailSyncUsecase, lg loggateway.Logger) *MailIdleSupervisor {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MailIdleSupervisor{
		accounts: accounts, syncer: syncer, lg: lg,
		idleWait: 25 * time.Second, pollInterval: 5 * time.Minute, rescanInterval: 30 * time.Second,
	}
}

// Start 阻塞运行直到 ctx 取消。
func (s *MailIdleSupervisor) Start(ctx context.Context) {
	running := map[string]context.CancelFunc{}
	rescan := func() {
		accs, err := s.accounts.ListAllMailAccounts(ctx)
		if err != nil {
			s.lg.Warn("mail idle: list accounts failed", loggateway.Err(err))
			return
		}
		seen := map[string]bool{}
		for _, acc := range accs {
			seen[acc.ID] = true
			if _, ok := running[acc.ID]; ok {
				continue
			}
			a := acc
			cctx, stop := context.WithCancel(ctx)
			running[acc.ID] = stop
			go s.idleLoop(cctx, a)
		}
		for id, stop := range running {
			if !seen[id] {
				stop()
				delete(running, id)
			}
		}
	}
	rescan()
	go s.housekeeping(ctx)
	rescanTk := time.NewTicker(s.rescanInterval)
	pollTk := time.NewTicker(s.pollInterval)
	defer rescanTk.Stop()
	defer pollTk.Stop()
	for {
		select {
		case <-ctx.Done():
			for _, stop := range running {
				stop()
			}
			return
		case <-rescanTk.C:
			rescan()
		case <-pollTk.C:
			s.pollAll(ctx)
		}
	}
}

// housekeeping 每分钟发到点的定时草稿；每小时清一次「已删除」里放满 30 天的信。
func (s *MailIdleSupervisor) housekeeping(ctx context.Context) {
	tk := time.NewTicker(time.Minute)
	defer tk.Stop()
	s.syncer.PurgeOldTrash(ctx)
	for n := 1; ; n++ {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			s.syncer.SendDueDrafts(ctx)
			if n%60 == 0 {
				s.syncer.PurgeOldTrash(ctx)
			}
		}
	}
}

// idleLoop 单账号 IDLE 循环：推送 → 关连接重连同步；故障退避重连。
func (s *MailIdleSupervisor) idleLoop(ctx context.Context, acc *MailAccount) {
	backoff := 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		err := s.idleOnce(ctx, acc)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errMailIdleUnsupported) {
			// 不再反复登录试 IDLE：pollAll 每 5 分钟收一次
			s.lg.Info("mail idle: server has no IDLE, polling only", loggateway.Str("account_id", acc.ID))
			return
		}
		if err != nil {
			s.lg.Warn("mail idle: cycle failed",
				loggateway.Str("account_id", acc.ID), loggateway.Err(err))
			s.recordIdleAudit(acc, fmt.Sprintf("ok=false err=%s", mailErrSummary(err)))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 5*time.Minute {
				backoff *= 2
			}
			continue
		}
		backoff = 5 * time.Second
	}
}

// idleOnce 单轮：拨号登录选箱 → IDLE 等待 → 推送则重连同步。
func (s *MailIdleSupervisor) idleOnce(ctx context.Context, acc *MailAccount) error {
	password, err := s.syncer.crypto.DecryptSecret(ctx, acc.PasswordEnc)
	if err != nil {
		return fmt.Errorf("decrypt account password: %w", err)
	}
	cli, err := s.syncer.dialer(ctx, acc.Host, acc.Port, acc.TLSMode)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer cli.Close()
	if err := cli.Login(acc.Username, password); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if _, err := cli.SelectInbox(); err != nil {
		return fmt.Errorf("select inbox: %w", err)
	}
	_, gotNew, err := cli.IdleWait(s.idleWait)
	if err != nil {
		return fmt.Errorf("idle: %w", err)
	}
	if !gotNew {
		return nil
	}
	// 推送后关连接重连同步（defer Close 幂等）。
	cli.Close()
	imported, _, err := s.syncer.syncOne(ctx, acc)
	if err != nil {
		return fmt.Errorf("sync after notify: %w", err)
	}
	s.recordIdleAudit(acc, fmt.Sprintf("ok=true notified=true imported=%d", imported))
	return nil
}

// pollAll 轮询兜底：全账号强制同步一轮（单账号失败不阻断）。
func (s *MailIdleSupervisor) pollAll(ctx context.Context) {
	accs, err := s.accounts.ListAllMailAccounts(ctx)
	if err != nil {
		s.lg.Warn("mail idle: poll list accounts failed", loggateway.Err(err))
		return
	}
	for _, acc := range accs {
		if _, _, err := s.syncer.syncOne(ctx, acc); err != nil {
			s.lg.Warn("mail idle: fallback poll sync failed",
				loggateway.Str("account_id", acc.ID), loggateway.Err(err))
		}
	}
}

func (s *MailIdleSupervisor) recordIdleAudit(acc *MailAccount, summary string) {
	if s.syncer.audit != nil {
		s.syncer.audit.RecordMailAudit(context.Background(), "idle.mail_account", acc.ID, summary)
	}
}
