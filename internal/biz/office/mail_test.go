package office

// W1d 邮件域 biz 单测（mock IMAP 驱动 + 全语义内存 fake；验收 #1-#6 单测面）。
//
// fake 语义合同（TESTING.md）：fake 实现接口完整语义——(account_id,uid)
// 部分唯一索引（account_id='' 豁免）、received_at DESC NULLS LAST +
// created_at DESC 双键排序、分页、属主隔离、转任务同事务回写、故障注入
// （指定 uid 前 N 次插入失败 → 验证整体回滚可重试）。

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/testutil/mockimap"
)

// ============================================================================
// 全语义内存 fake（四仓储 + 信封加解密 + 审计/知识库/总台窄口）
// ============================================================================

type mailFakeRepos struct {
	mu          sync.Mutex
	accounts    map[string]*MailAccount
	messages    map[string]*MailMessage
	attachments map[string]*MailAttachment
	tasks       map[string]*OfficeTask
	taskFiles   map[string]*OfficeTaskFile
	taskExecs   []*OfficeTaskExecution
	failUID     map[int64]int // uid → 剩余注入失败次数
	insertedSeq []int64       // 成功插入的 uid 轨迹（故障注入断言用）
	blocked     map[string][]string
}

func newMailFakeRepos() *mailFakeRepos {
	return &mailFakeRepos{
		accounts:    map[string]*MailAccount{},
		messages:    map[string]*MailMessage{},
		attachments: map[string]*MailAttachment{},
		tasks:       map[string]*OfficeTask{},
		taskFiles:   map[string]*OfficeTaskFile{},
		failUID:     map[int64]int{},
	}
}

func cloneAccount(a *MailAccount) *MailAccount { c := *a; return &c }

func cloneMessage(m *MailMessage) *MailMessage {
	c := *m
	c.Attachments = nil
	return &c
}

// ---------- MailAccountRepo ----------

func (f *mailFakeRepos) CreateMailAccount(_ context.Context, a *MailAccount) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[a.ID] = cloneAccount(a)
	return nil
}

func (f *mailFakeRepos) UpdateMailAccount(_ context.Context, a *MailAccount) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.accounts[a.ID]
	if !ok {
		return ErrMailAccountNotFound
	}
	_ = cur
	f.accounts[a.ID] = cloneAccount(a)
	return nil
}

func (f *mailFakeRepos) DeleteMailAccount(_ context.Context, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.accounts[id]
	if !ok || cur.UserID != userID {
		return ErrMailAccountNotFound
	}
	delete(f.accounts, id)
	return nil
}

func (f *mailFakeRepos) GetMailAccount(_ context.Context, id, userID string) (*MailAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.accounts[id]
	if !ok || cur.UserID != userID {
		return nil, ErrMailAccountNotFound
	}
	return cloneAccount(cur), nil
}

func (f *mailFakeRepos) ListMailAccounts(_ context.Context, userID string) ([]*MailAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*MailAccount, 0)
	for _, a := range f.accounts {
		if a.UserID == userID {
			out = append(out, cloneAccount(a))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *mailFakeRepos) ListAllMailAccounts(_ context.Context) ([]*MailAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*MailAccount, 0, len(f.accounts))
	for _, a := range f.accounts {
		out = append(out, cloneAccount(a))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *mailFakeRepos) UpdateMailSyncCursor(_ context.Context, id string, lastUID int64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.accounts[id]
	if !ok {
		return ErrMailAccountNotFound
	}
	cur.LastUID = lastUID
	cur.LastSyncAt = &at
	return nil
}

// ---------- MailMessageRepo ----------

// InsertMessageWithAttachmentsTx 单事务语义：失败整封回滚（附件不落）；
// (account_id,uid) 部分唯一（account_id=” 豁免）。
func (f *mailFakeRepos) InsertMessageWithAttachmentsTx(_ context.Context, m *MailMessage) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n := f.failUID[m.UID]; n > 0 {
		f.failUID[m.UID] = n - 1
		return false, fmt.Errorf("injected insert failure uid=%d", m.UID)
	}
	if m.AccountID != "" && m.MessageIDHdr != "" && (mailFolderIs(m, MailFolderInbox) || mailFolderIs(m, MailFolderJunk)) {
		for _, cur := range f.messages {
			if cur.AccountID == m.AccountID && cur.MessageIDHdr == m.MessageIDHdr &&
				(mailFolderIs(cur, MailFolderInbox) || mailFolderIs(cur, MailFolderJunk)) {
				return false, nil // 同一 Message-ID 已有（挪过文件夹换了 UID）
			}
		}
	}
	for _, folder := range []string{MailFolderInbox, MailFolderJunk} {
		if m.AccountID == "" || !mailFolderIs(m, folder) {
			continue
		}
		for _, cur := range f.messages {
			if cur.AccountID == m.AccountID && cur.UID == m.UID && mailFolderIs(cur, folder) {
				return false, nil // 幂等命中
			}
		}
	}
	f.messages[m.ID] = cloneMessage(m)
	for _, a := range m.Attachments {
		ac := *a
		f.attachments[a.ID] = &ac
	}
	f.insertedSeq = append(f.insertedSeq, m.UID)
	return true, nil
}

func (f *mailFakeRepos) fillHasAttachmentsLocked(m *MailMessage) {
	for _, a := range f.attachments {
		if a.MessageID == m.ID {
			m.HasAttachments = true
			return
		}
	}
	m.HasAttachments = false
}

func (f *mailFakeRepos) ListMailMessages(_ context.Context, userID, accountID, quadrant string, page, pageSize int) ([]*MailMessage, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pool := make([]*MailMessage, 0)
	for _, m := range f.messages {
		if m.UserID != userID || m.Spam || m.TrashedAt != nil {
			continue
		}
		if accountID != "" && m.AccountID != accountID {
			continue
		}
		if quadrant != "" && m.Quadrant != quadrant {
			continue
		}
		pool = append(pool, cloneMessage(m))
	}
	// received_at DESC NULLS LAST，再 created_at DESC。
	sort.Slice(pool, func(i, j int) bool {
		a, b := pool[i].ReceivedAt, pool[j].ReceivedAt
		if a == nil && b == nil {
			return pool[i].CreatedAt.After(pool[j].CreatedAt)
		}
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		if !a.Equal(*b) {
			return a.After(*b)
		}
		return pool[i].CreatedAt.After(pool[j].CreatedAt)
	})
	total := int64(len(pool))
	start := (page - 1) * pageSize
	if start >= len(pool) {
		return []*MailMessage{}, total, nil
	}
	end := start + pageSize
	if end > len(pool) {
		end = len(pool)
	}
	out := pool[start:end]
	for _, m := range out {
		f.fillHasAttachmentsLocked(m)
	}
	return out, total, nil
}

func (f *mailFakeRepos) GetMailMessage(_ context.Context, id, userID string) (*MailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.messages[id]
	if !ok || cur.UserID != userID {
		return nil, ErrMailMessageNotFound
	}
	out := cloneMessage(cur)
	f.fillHasAttachmentsLocked(out)
	return out, nil
}

func (f *mailFakeRepos) DeleteMailMessage(_ context.Context, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.messages[id]
	if !ok || cur.UserID != userID {
		return ErrMailMessageNotFound
	}
	delete(f.messages, id)
	for aid, a := range f.attachments { // ON DELETE CASCADE
		if a.MessageID == id {
			delete(f.attachments, aid)
		}
	}
	return nil
}

func (f *mailFakeRepos) SetMailTaskRef(_ context.Context, id, userID, taskRef string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.messages[id]
	if !ok || cur.UserID != userID {
		return ErrMailMessageNotFound
	}
	cur.TaskRef = taskRef
	return nil
}

func (f *mailFakeRepos) ListMailMessagesSince(_ context.Context, userID string, since time.Time, limit int) ([]*MailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*MailMessage, 0)
	for _, m := range f.messages {
		if m.UserID != userID || m.ReceivedAt == nil || m.ReceivedAt.Before(since) || m.Spam || m.TrashedAt != nil {
			continue
		}
		out = append(out, cloneMessage(m))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReceivedAt.Before(*out[j].ReceivedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *mailFakeRepos) ListMailDigestUsers(_ context.Context, since time.Time) ([]*MailDigestCandidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	type key struct{ u, w string }
	acc := map[key]int{}
	for _, m := range f.messages {
		if m.ReceivedAt == nil || m.ReceivedAt.Before(since) || m.Spam || m.TrashedAt != nil {
			continue
		}
		acc[key{m.UserID, m.WorkspaceID}]++
	}
	out := make([]*MailDigestCandidate, 0, len(acc))
	for k, n := range acc {
		out = append(out, &MailDigestCandidate{UserID: k.u, WorkspaceID: k.w, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out, nil
}

func (f *mailFakeRepos) SearchMailMessages(_ context.Context, userID, query string, limit int) ([]*MailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := strings.ToLower(query)
	out := make([]*MailMessage, 0)
	for _, m := range f.messages {
		if m.UserID != userID {
			continue
		}
		if !strings.Contains(strings.ToLower(m.Subject), q) &&
			!strings.Contains(strings.ToLower(m.Sender), q) &&
			!strings.Contains(strings.ToLower(m.Body), q) {
			continue
		}
		out = append(out, cloneMessage(m))
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].ReceivedAt, out[j].ReceivedAt
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		return a.After(*b)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---------- MailAttachmentRepo ----------

func (f *mailFakeRepos) ListMailAttachments(_ context.Context, messageID string) ([]*MailAttachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*MailAttachment, 0)
	for _, a := range f.attachments {
		if a.MessageID == messageID {
			c := *a
			c.Content = nil // 列表不含字节
			out = append(out, &c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *mailFakeRepos) GetMailAttachment(_ context.Context, id, userID string) (*MailAttachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.attachments[id]
	if !ok {
		return nil, ErrMailAttachmentNotFound
	}
	m, ok := f.messages[a.MessageID] // JOIN 属主隔离
	if !ok || m.UserID != userID {
		return nil, ErrMailAttachmentNotFound
	}
	c := *a
	return &c, nil
}

func (f *mailFakeRepos) SetMailAttachmentKnowledgeDoc(_ context.Context, id, docID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.attachments[id]
	if !ok {
		return ErrMailAttachmentNotFound
	}
	a.KnowledgeDocID = docID
	return nil
}

// ---------- OfficeTaskRepo ----------

func (f *mailFakeRepos) CreateOfficeTask(_ context.Context, t *OfficeTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := *t
	f.tasks[t.ID] = &c
	return nil
}

func (f *mailFakeRepos) GetOfficeTask(_ context.Context, id, userID string) (*OfficeTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.tasks[id]
	if !ok || cur.UserID != userID {
		return nil, ErrMailMessageNotFound
	}
	c := *cur
	return &c, nil
}

func (f *mailFakeRepos) ListOfficeTasks(_ context.Context, userID string, filter OfficeTaskFilter, limit int) ([]*OfficeTask, error) {
	in := func(list []string, v string) bool {
		if len(list) == 0 {
			return true
		}
		for _, s := range list {
			if s == v {
				return true
			}
		}
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*OfficeTask, 0)
	for _, t := range f.tasks {
		assigned := t.AssigneeKind != ""
		if t.UserID != userID || !in(filter.Statuses, t.Status) || !in(filter.Sources, t.Source) ||
			(filter.Assigned > 0 && !assigned) || (filter.Assigned < 0 && assigned) {
			continue
		}
		c := *t
		for _, file := range f.taskFiles {
			if file.TaskID == t.ID {
				c.FileCount++
			}
		}
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *mailFakeRepos) SetOfficeTaskStatus(_ context.Context, id, userID, status string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.tasks[id]
	if !ok || cur.UserID != userID {
		return ErrOfficeTaskNotFound
	}
	cur.Status, cur.UpdatedAt = status, at
	return nil
}

// CreateTaskFromMailTx 同事务语义：建任务 + 回写 task_ref（邮件不存在整体失败）。
func (f *mailFakeRepos) CreateTaskFromMailTx(_ context.Context, t *OfficeTask, mailID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.messages[mailID]
	if !ok || cur.UserID != userID {
		return ErrMailMessageNotFound
	}
	c := *t
	f.tasks[t.ID] = &c
	cur.TaskRef = t.ID
	return nil
}

// ---------- 信封加解密 fake（可逆；断言密文不含明文） ----------

type mailFakeCrypto struct{}

func (mailFakeCrypto) EncryptSecret(_ context.Context, plain string) (string, error) {
	return "enc:" + base64.StdEncoding.EncodeToString([]byte(plain)), nil
}

func (mailFakeCrypto) DecryptSecret(_ context.Context, enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, "enc:"))
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(raw), nil
}

// ---------- 审计/知识库窄口 fake ----------

type mailAuditRecord struct{ action, resourceID, summary string }

type mailFakeAudit struct {
	mu      sync.Mutex
	records []mailAuditRecord
}

func (a *mailFakeAudit) RecordMailAudit(_ context.Context, action, resourceID, summary string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.records = append(a.records, mailAuditRecord{action, resourceID, summary})
}

func (a *mailFakeAudit) snapshot() []mailAuditRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]mailAuditRecord(nil), a.records...)
}

type mailFakeKnowledge struct {
	mu      sync.Mutex
	ingests []MailKnowledgeIngest
	nextID  int
}

func (k *mailFakeKnowledge) IngestAttachment(_ context.Context, in MailKnowledgeIngest) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.nextID++
	k.ingests = append(k.ingests, in)
	return fmt.Sprintf("doc-%d", k.nextID), nil
}

// ---------- 测试装配辅助 ----------

// newMailTestAccount 指向 mock 服务的账号（tls=none；密码经 fake 信封加密）。
func newMailTestAccount(t *testing.T, ctx context.Context, repo *mailFakeRepos, srv *mockimap.Server, userID string) *MailAccount {
	t.Helper()
	host, portStr, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("split mock addr: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse mock port: %v", err)
	}
	uc := NewMailAccountUsecase(repo, mailFakeCrypto{}, nil)
	acc, err := uc.Create(ctx, &MailAccount{
		WorkspaceID: "ws-test", UserID: userID, Name: "mock 邮箱",
		Host: host, Port: port, TLSMode: MailTLSNone, Username: "test@local",
	}, "test-password-01")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return acc
}

func startMockIMAPServer(t *testing.T) *mockimap.Server {
	t.Helper()
	srv, err := mockimap.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("start mock imap: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

// ============================================================================
// 用例
// ============================================================================

// TestMailSync_FirstSyncAndIdempotent 验收 #1+#2：首同步导入 3 封（encoded-word
// 中文主题解码、附件字节保真）；游标 last_uid=3。邮件 P1 起按 UID 增量：
// 二次同步只问游标之后的 UID，没有新信就什么都不抓（imported=0 skipped=0）。
func TestMailSync_FirstSyncAndIdempotent(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	audit := &mailFakeAudit{}
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")

	// 验收 #6（单测面）：落库 PasswordEnc 不含明文。
	stored := repo.accounts[acc.ID]
	if strings.Contains(stored.PasswordEnc, "test-password-01") {
		t.Fatalf("password_enc leaks plaintext: %q", stored.PasswordEnc)
	}

	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, audit, nil)
	imported, skipped, err := syncer.Sync(ctx, acc.ID, "u-1")
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if imported != 3 || skipped != 0 {
		t.Fatalf("first sync imported=%d skipped=%d, want 3/0", imported, skipped)
	}

	// 游标推进。
	cur, _ := repo.GetMailAccount(ctx, acc.ID, "u-1")
	if cur.LastUID != 3 || cur.LastSyncAt == nil {
		t.Fatalf("cursor last_uid=%d last_sync_at=%v, want 3/non-nil", cur.LastUID, cur.LastSyncAt)
	}

	// 验收 #2：encoded-word 中文主题渲染。
	msgs, total, err := repo.ListMailMessages(ctx, "u-1", "", "", 1, 50)
	if err != nil || total != 3 {
		t.Fatalf("list after sync: total=%d err=%v", total, err)
	}
	var fund, hr *MailMessage
	for _, m := range msgs {
		switch {
		case strings.Contains(m.Subject, "项目资金方案与排期表"):
			fund = m
		case strings.Contains(m.Subject, "团建报名截止提醒"):
			hr = m
		}
	}
	if fund == nil || hr == nil {
		t.Fatalf("encoded-word subjects not decoded: %+v", msgs)
	}
	if !fund.HasAttachments {
		t.Fatal("fund mail should carry attachment")
	}

	// 验收 #3（附件字节保真）：budget.txt base64 解码后与源一致。
	atts, err := repo.ListMailAttachments(ctx, fund.ID)
	if err != nil || len(atts) != 1 {
		t.Fatalf("list attachments: n=%d err=%v", len(atts), err)
	}
	if atts[0].Filename != "budget.txt" {
		t.Fatalf("attachment filename %q", atts[0].Filename)
	}
	full, err := repo.GetMailAttachment(ctx, atts[0].ID, "u-1")
	if err != nil {
		t.Fatalf("get attachment: %v", err)
	}
	wantBody := "项目资金方案 v1：总预算 80 万，凤凰计划三期专款。"
	if string(full.Content) != wantBody {
		t.Fatalf("attachment bytes mismatch: %q", string(full.Content))
	}

	// 属主隔离：他用户取不到。
	if _, err := repo.GetMailAttachment(ctx, atts[0].ID, "u-2"); !errors.Is(err, ErrMailAttachmentNotFound) {
		t.Fatalf("owner isolation: err=%v", err)
	}

	// 审计至少一条 ok=true。
	var sawOK bool
	for _, r := range audit.snapshot() {
		if r.action == "sync.mail_account" && strings.Contains(r.summary, "ok=true") {
			sawOK = true
		}
	}
	if !sawOK {
		t.Fatal("missing ok=true sync audit")
	}

	// 幂等二刷。
	imported, skipped, err = syncer.Sync(ctx, acc.ID, "u-1")
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if imported != 0 || skipped != 0 {
		t.Fatalf("second sync imported=%d skipped=%d, want 0/0 (incremental)", imported, skipped)
	}
}

// TestMailSync_FailureInjectionRollback 验收 #3 故障注入：seq=2 首次插入
// 失败 → 整封回滚（邮件+附件同生共死），本轮 imported=1 报错；重试恢复，
// seq=2 成功且附件字节齐全，终态 3 封无重复。
func TestMailSync_FailureInjectionRollback(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
	repo.failUID[2] = 1 // seq=2（带附件邮件）首插失败

	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	imported, _, err := syncer.Sync(ctx, acc.ID, "u-1")
	if err == nil {
		t.Fatal("expected injected failure")
	}
	if imported != 1 {
		t.Fatalf("imported=%d, want 1（seq=1 已落，seq=2 回滚）", imported)
	}
	// 回滚面无半成品：messages 仅 seq=1，无 seq=2 的附件。
	_, total, _ := repo.ListMailMessages(ctx, "u-1", "", "", 1, 50)
	if total != 1 {
		t.Fatalf("after rollback total=%d, want 1", total)
	}
	for _, a := range repo.attachments {
		t.Fatalf("orphan attachment after rollback: %s", a.ID)
	}

	// 重试恢复：seq=1 幂等命中，seq=2/3 新导入。
	imported, skipped, err := syncer.Sync(ctx, acc.ID, "u-1")
	if err != nil {
		t.Fatalf("retry sync: %v", err)
	}
	if imported != 2 || skipped != 1 {
		t.Fatalf("retry imported=%d skipped=%d, want 2/1", imported, skipped)
	}
	_, total, _ = repo.ListMailMessages(ctx, "u-1", "", "", 1, 50)
	if total != 3 {
		t.Fatalf("after retry total=%d, want 3", total)
	}
	// seq=2 附件字节齐全。
	msgs, _, _ := repo.ListMailMessages(ctx, "u-1", "", "", 1, 50)
	for _, m := range msgs {
		if !strings.Contains(m.Subject, "项目资金方案与排期表") {
			continue
		}
		atts, _ := repo.ListMailAttachments(ctx, m.ID)
		if len(atts) != 1 {
			t.Fatalf("recovered mail attachments=%d, want 1", len(atts))
		}
		full, _ := repo.GetMailAttachment(ctx, atts[0].ID, "u-1")
		if !strings.Contains(string(full.Content), "总预算 80 万") {
			t.Fatalf("recovered attachment bytes broken: %q", string(full.Content))
		}
	}
}

// TestMailSync_UpstreamError 验收 #7（单测面）：端口不可达 → ErrMailUpstream
// 哨兵（service 映 502），审计 ok=false。
func TestMailSync_UpstreamError(t *testing.T) {
	repo := newMailFakeRepos()
	audit := &mailFakeAudit{}
	ctx := context.Background()
	uc := NewMailAccountUsecase(repo, mailFakeCrypto{}, nil)
	acc, err := uc.Create(ctx, &MailAccount{
		WorkspaceID: "ws-test", UserID: "u-1", Host: "127.0.0.1", Port: 1, // 不可达
		TLSMode: MailTLSNone, Username: "test@local",
	}, "test-password-01")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, audit, nil)
	_, _, err = syncer.Sync(ctx, acc.ID, "u-1")
	if !errors.Is(err, ErrMailUpstream) {
		t.Fatalf("err=%v, want ErrMailUpstream", err)
	}
	var sawFail bool
	for _, r := range audit.snapshot() {
		if r.action == "sync.mail_account" && strings.Contains(r.summary, "ok=false") {
			sawFail = true
		}
		if strings.Contains(r.summary, "test-password-01") {
			t.Fatal("audit leaks password")
		}
	}
	if !sawFail {
		t.Fatal("missing ok=false sync audit")
	}
}

// TestMailIdle_PushTriggersSync 验收 #4（单测面）：IDLE 推送到达 →
// idleOnce 重连同步，新邮件入库。
func TestMailIdle_PushTriggersSync(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	// 先同步掉三封种子。
	if _, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil {
		t.Fatalf("seed sync: %v", err)
	}

	sup := NewMailIdleSupervisor(repo, syncer, nil)
	sup.idleWait = 8 * time.Second // 同包测试缩等待窗

	done := make(chan error, 1)
	go func() { done <- sup.idleOnce(ctx, acc) }()
	time.Sleep(300 * time.Millisecond) // 等 IDLE 进入 idling
	srv.Mailbox().Append([]byte("From: pm@example.com\r\nTo: test@local\r\nSubject: idle push hello\r\nDate: Mon, 02 Nov 2026 12:00:00 +0800\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nIDLE 推送验证邮件。\r\n"))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("idleOnce: %v", err)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("idleOnce did not return after push")
	}

	msgs, total, err := repo.ListMailMessages(ctx, "u-1", "", "", 1, 50)
	if err != nil || total != 4 {
		t.Fatalf("after idle push total=%d err=%v, want 4", total, err)
	}
	var pushed bool
	for _, m := range msgs {
		if m.Subject == "idle push hello" {
			pushed = true
		}
	}
	if !pushed {
		t.Fatal("pushed mail not synced after IDLE notify")
	}
}

// TestMailImapIdleWaitTimeout 无推送时 IdleWait 超时正常退出（gotNew=false）。
func TestMailImapIdleWaitTimeout(t *testing.T) {
	srv := startMockIMAPServer(t)
	host, portStr, _ := net.SplitHostPort(srv.Addr())
	port, _ := strconv.Atoi(portStr)
	cli, err := DialMailIMAP(context.Background(), host, port, MailTLSNone)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close()
	if err := cli.Login("test@local", "x"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if n, err := cli.SelectInbox(); err != nil || n != 3 {
		t.Fatalf("select: n=%d err=%v", n, err)
	}
	_, gotNew, err := cli.IdleWait(1200 * time.Millisecond)
	if err != nil {
		t.Fatalf("idle wait: %v", err)
	}
	if gotNew {
		t.Fatal("gotNew should be false on timeout")
	}
}

// TestMailImapLoginSendsID 网易邮箱登录后要先报 ID 才能 SELECT；口令错要在登录这步就报错。
func TestMailImapLoginSendsID(t *testing.T) {
	srv := startMockIMAPServer(t)
	host, portStr, _ := net.SplitHostPort(srv.Addr())
	port, _ := strconv.Atoi(portStr)
	cli, err := DialMailIMAP(context.Background(), host, port, MailTLSNone)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close()
	if err := cli.Login("someone@126.com", `pa"ss\word`); err != nil {
		t.Fatalf("login: %v", err)
	}
	if n, err := cli.SelectInbox(); err != nil || n != 3 {
		t.Fatalf("select after ID: n=%d err=%v", n, err)
	}

	bad, err := DialMailIMAP(context.Background(), host, port, MailTLSNone)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer bad.Close()
	if err := bad.Login("someone@126.com", "wrong-password"); err == nil {
		t.Fatal("wrong password should fail at LOGIN")
	}
}

// TestMailImapFetchKeepsUnread 同步取信不能把服务器上的信标成已读；FLAGS 在原文后面也要读到。
func TestMailImapFetchKeepsUnread(t *testing.T) {
	srv := startMockIMAPServer(t)
	host, portStr, _ := net.SplitHostPort(srv.Addr())
	port, _ := strconv.Atoi(portStr)
	cli, err := DialMailIMAP(context.Background(), host, port, MailTLSNone)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close()
	if err := cli.Login("test@local", "x"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := cli.SelectInbox(); err != nil {
		t.Fatalf("select: %v", err)
	}
	for i := 0; i < 2; i++ {
		raw, flags, err := cli.FetchUID(1)
		if err != nil || !strings.Contains(string(raw), "Weekly sync notes") {
			t.Fatalf("fetch #%d: err=%v raw=%q", i, err, raw)
		}
		if strings.Contains(flags, `\Seen`) || srv.Mailbox().Seen(1) {
			t.Fatalf("fetch #%d marked the message read on the server (flags=%q)", i, flags)
		}
	}
	if err := cli.StoreFlags(2, true, `\Seen`); err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, flags, err := cli.FetchUID(2); err != nil || flags != `\Seen` {
		t.Fatalf("flags after the literal not parsed: flags=%q err=%v", flags, err)
	}
}

// TestMailAccount_CRUD 验收 #6（账号面）：密码加密落库 + 轮换 + 属主隔离
// 删除；非法 tls_mode 400；默认端口补齐。
func TestMailAccount_CRUD(t *testing.T) {
	repo := newMailFakeRepos()
	ctx := context.Background()
	uc := NewMailAccountUsecase(repo, mailFakeCrypto{}, nil)

	if _, err := uc.Create(ctx, &MailAccount{UserID: "u-1", Username: "x"}, "pw"); !errors.Is(err, ErrMailHostRequired) {
		t.Fatalf("missing host: err=%v", err)
	}
	if _, err := uc.Create(ctx, &MailAccount{UserID: "u-1", Host: "h", Username: "x", TLSMode: "bogus"}, "pw"); !errors.Is(err, ErrMailTLSModeInvalid) {
		t.Fatalf("bad tls mode: err=%v", err)
	}
	acc, err := uc.Create(ctx, &MailAccount{UserID: "u-1", Host: "imap.example.com", Username: "a@b.c"}, "pw-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if acc.Port != 993 || acc.TLSMode != MailTLSTLS {
		t.Fatalf("defaults port=%d mode=%s, want 993/tls", acc.Port, acc.TLSMode)
	}
	if repo.accounts[acc.ID].PasswordEnc == "pw-1" || !strings.HasPrefix(repo.accounts[acc.ID].PasswordEnc, "enc:") {
		t.Fatalf("password not encrypted: %q", repo.accounts[acc.ID].PasswordEnc)
	}

	// 轮换密码。
	if _, err := uc.Update(ctx, acc.ID, "u-1", &MailAccount{}, "pw-2"); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	enc1 := repo.accounts[acc.ID].PasswordEnc
	if !strings.Contains(enc1, base64.StdEncoding.EncodeToString([]byte("pw-2"))) {
		t.Fatalf("rotation not applied: %q", enc1)
	}

	// 属主隔离：u-2 不可见不可删。
	if _, err := uc.Get(ctx, acc.ID, "u-2"); !errors.Is(err, ErrMailAccountNotFound) {
		t.Fatalf("cross-user get: err=%v", err)
	}
	if err := uc.Delete(ctx, acc.ID, "u-2"); !errors.Is(err, ErrMailAccountNotFound) {
		t.Fatalf("cross-user delete: err=%v", err)
	}
	if err := uc.Delete(ctx, acc.ID, "u-1"); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
}

// TestMailToTask_Idempotent 验收 #5（单测面）：转任务落 office_tasks（正文
// 含回链）+ task_ref 同事务回写；二次调用返回同一引用不重复建。
func TestMailToTask_Idempotent(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	if _, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	msgs, _, _ := repo.ListMailMessages(ctx, "u-1", "", "", 1, 50)
	target := msgs[0]

	msgUC := NewMailMessageUsecase(repo, repo, repo, nil, nil)
	taskRef, err := msgUC.MailToTask(ctx, target.ID, "u-1", "")
	if err != nil {
		t.Fatalf("mail to task: %v", err)
	}
	if !strings.HasPrefix(taskRef, "otk_") {
		t.Fatalf("task ref %q", taskRef)
	}
	task, err := repo.GetOfficeTask(ctx, taskRef, "u-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !strings.Contains(task.Body, "来自邮件："+target.ID) {
		t.Fatalf("task body missing backlink: %q", task.Body)
	}
	if task.Source != "mail" || task.SourceRef != target.ID || task.Status != "open" {
		t.Fatalf("task fields: %+v", task)
	}
	// task_ref 回写。
	after, _ := repo.GetMailMessage(ctx, target.ID, "u-1")
	if after.TaskRef != taskRef {
		t.Fatalf("task_ref=%q, want %q", after.TaskRef, taskRef)
	}
	// 幂等。
	again, err := msgUC.MailToTask(ctx, target.ID, "u-1", "")
	if err != nil || again != taskRef {
		t.Fatalf("idempotent re-run: ref=%q err=%v", again, err)
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("tasks=%d, want 1（幂等不重复建）", len(repo.tasks))
	}
	// 标题覆盖。
	t2 := msgs[1]
	ref2, err := msgUC.MailToTask(ctx, t2.ID, "u-1", "自定义标题")
	if err != nil {
		t.Fatalf("override to task: %v", err)
	}
	task2, _ := repo.GetOfficeTask(ctx, ref2, "u-1")
	if task2.Title != "自定义标题" {
		t.Fatalf("override title %q", task2.Title)
	}
}

// TestMailEmlImportExportRoundTrip .eml 导入（含附件）→ 导出 → 重解析
// 字节级对齐（主题/正文/附件名/附件内容）。
func TestMailEmlImportExportRoundTrip(t *testing.T) {
	repo := newMailFakeRepos()
	ctx := context.Background()
	msgUC := NewMailMessageUsecase(repo, repo, repo, nil, nil)

	src := mockimap.SeedMails()[1] // 带 budget.txt 附件的中文主题邮件
	msg, err := msgUC.ImportEml(ctx, "u-1", "ws-test", src)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if msg.AccountID != "" || msg.UID != 0 {
		t.Fatalf("import row account=%q uid=%d, want ''/0", msg.AccountID, msg.UID)
	}
	if !strings.Contains(msg.Subject, "项目资金方案与排期表") {
		t.Fatalf("import subject %q", msg.Subject)
	}
	atts, _ := msgUC.ListAttachments(ctx, msg.ID, "u-1")
	if len(atts) != 1 || atts[0].Filename != "budget.txt" {
		t.Fatalf("import attachments: %+v", atts)
	}

	// 导入幂等面：同 account_id='' 重复导入不冲突（部分唯一索引豁免）。
	if _, err := msgUC.ImportEml(ctx, "u-1", "ws-test", src); err != nil {
		t.Fatalf("re-import: %v", err)
	}

	raw, filename, err := msgUC.ExportEml(ctx, msg.ID, "u-1")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.HasSuffix(filename, ".eml") {
		t.Fatalf("export filename %q", filename)
	}
	reparsed := ParseMailRaw(raw)
	if reparsed.Subject != msg.Subject {
		t.Fatalf("roundtrip subject %q, want %q", reparsed.Subject, msg.Subject)
	}
	if !strings.Contains(reparsed.Body, "请查收项目资金方案") {
		t.Fatalf("roundtrip body %q", reparsed.Body)
	}
	if len(reparsed.Attachments) != 1 || reparsed.Attachments[0].Filename != "budget.txt" {
		t.Fatalf("roundtrip attachments: %+v", reparsed.Attachments)
	}
	if !strings.Contains(string(reparsed.Attachments[0].Content), "总预算 80 万") {
		t.Fatalf("roundtrip attachment bytes broken")
	}

	// 卡口：空 eml 400。
	if _, err := msgUC.ImportEml(ctx, "u-1", "ws-test", nil); !errors.Is(err, ErrMailEmlEmpty) {
		t.Fatalf("empty eml: err=%v", err)
	}
}

// TestMailAttachmentToKnowledge 附件入知识库：密级随行透传 + 幂等 +
// 非法密级 400 + 未装配 sink 503。
func TestMailAttachmentToKnowledge(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	knowledge := &mailFakeKnowledge{}
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	if _, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	msgUC := NewMailMessageUsecase(repo, repo, repo, knowledge, nil)
	msgs, _, _ := repo.ListMailMessages(ctx, "u-1", "", "", 1, 50)
	var fund *MailMessage
	for _, m := range msgs {
		if m.HasAttachments {
			fund = m
		}
	}
	atts, _ := msgUC.ListAttachments(ctx, fund.ID, "u-1")
	attID := atts[0].ID

	docID, err := msgUC.AttachmentToKnowledge(ctx, attID, "u-1", 3)
	if err != nil {
		t.Fatalf("to knowledge: %v", err)
	}
	if docID == "" {
		t.Fatal("empty doc id")
	}
	ing := knowledge.ingests[0]
	if ing.Classification != 3 || ing.Filename != "budget.txt" || ing.SourceMailID != fund.ID || ing.WorkspaceID != "ws-test" {
		t.Fatalf("ingest payload: %+v", ing)
	}
	// 幂等：回写 knowledge_doc_id 后二次调用返回同一 doc。
	again, err := msgUC.AttachmentToKnowledge(ctx, attID, "u-1", 3)
	if err != nil || again != docID {
		t.Fatalf("idempotent: doc=%q err=%v", again, err)
	}
	if len(knowledge.ingests) != 1 {
		t.Fatalf("ingests=%d, want 1", len(knowledge.ingests))
	}
	// 密级越界。
	if _, err := msgUC.AttachmentToKnowledge(ctx, attID, "u-1", 6); !errors.Is(err, ErrMailClassificationInvalid) {
		t.Fatalf("classification: err=%v", err)
	}
	// sink 未装配 → 503 哨兵（另起无附件邮件路径）。
	msgUC2 := NewMailMessageUsecase(repo, repo, repo, nil, nil)
	for _, m := range msgs {
		if m.HasAttachments {
			continue
		}
		list, _ := msgUC2.ListAttachments(ctx, m.ID, "u-1")
		_ = list
	}
	// 直接对已知附件 id 走 nil-sink 分支需未写过 doc——新建一封导入邮件。
	msg3, err := msgUC.ImportEml(ctx, "u-1", "ws-test", mockimap.SeedMails()[1])
	if err != nil {
		t.Fatalf("import for nil-sink: %v", err)
	}
	atts3, _ := msgUC.ListAttachments(ctx, msg3.ID, "u-1")
	if _, err := msgUC2.AttachmentToKnowledge(ctx, atts3[0].ID, "u-1", 0); !errors.Is(err, ErrKnowledgeUnavailable) {
		t.Fatalf("nil sink: err=%v", err)
	}
}

// TestMailSearch 关键词检索（subject/sender/body ILIKE 语义；agent
// mail.search 工具读口的 biz 底座）。
func TestMailSearch(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	if _, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	hits, err := repo.SearchMailMessages(ctx, "u-1", "资金", 10)
	if err != nil || len(hits) == 0 {
		t.Fatalf("search subject: n=%d err=%v", len(hits), err)
	}
	hits, _ = repo.SearchMailMessages(ctx, "u-1", "hr@example.com", 10)
	if len(hits) != 1 {
		t.Fatalf("search sender: n=%d", len(hits))
	}
	hits, _ = repo.SearchMailMessages(ctx, "u-1", "不存在的词xyz", 10)
	if len(hits) != 0 {
		t.Fatalf("search miss: n=%d", len(hits))
	}
	// 他用户不可见。
	hits, _ = repo.SearchMailMessages(ctx, "u-2", "资金", 10)
	if len(hits) != 0 {
		t.Fatalf("cross-user search leak: n=%d", len(hits))
	}
}

// ============================================================================
// 摘要三腿（验收 #8 单测面）
// ============================================================================

type mailFakeNotify struct {
	mu    sync.Mutex
	items []*Notification
}

func (f *mailFakeNotify) CreateNotification(_ context.Context, n *Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := *n
	f.items = append(f.items, &c)
	return nil
}

func (f *mailFakeNotify) ListNotifications(_ context.Context, userID string, unreadOnly bool, page, pageSize int) ([]*Notification, int64, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pool := make([]*Notification, 0)
	var unread int64
	for _, n := range f.items {
		if n.UserID != userID {
			continue
		}
		if !n.Read {
			unread++
		}
		if unreadOnly && n.Read {
			continue
		}
		c := *n
		pool = append(pool, &c)
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].CreatedAt.After(pool[j].CreatedAt) })
	total := int64(len(pool))
	start := (page - 1) * pageSize
	if start >= len(pool) {
		return []*Notification{}, total, unread, nil
	}
	end := start + pageSize
	if end > len(pool) {
		end = len(pool)
	}
	return pool[start:end], total, unread, nil
}

func (f *mailFakeNotify) DismissNotification(_ context.Context, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.items {
		if n.ID == id && n.UserID == userID {
			n.Read = true
			return nil
		}
	}
	return ErrNotificationNotFound
}

func (f *mailFakeNotify) MarkAllNotificationsRead(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.items {
		if n.UserID == userID {
			n.Read = true
		}
	}
	return nil
}

type mailIMSent struct {
	sink ReminderSink
	text string
	key  string
}

type mailFakeIM struct {
	mu    sync.Mutex
	sinks []ReminderSink
	sent  []mailIMSent
}

func (f *mailFakeIM) ListSinks(_ context.Context, _ string) ([]ReminderSink, error) {
	return append([]ReminderSink(nil), f.sinks...), nil
}

func (f *mailFakeIM) Enqueue(_ context.Context, sink ReminderSink, text, idempotencyKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, mailIMSent{sink: sink, text: text, key: idempotencyKey})
	return nil
}

type mailCeoboardEvent struct {
	userID, title, body string
	at                  time.Time
}

type mailFakeCeoboard struct {
	mu     sync.Mutex
	events []mailCeoboardEvent
}

func (c *mailFakeCeoboard) EmitMailDigest(_ context.Context, userID, title, body string, occurredAt time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, mailCeoboardEvent{userID: userID, title: title, body: body, at: occurredAt})
	return nil
}

// TestMailDigest_ThreeLegs 验收 #8：Trigger 三腿各落一条（通知中心
// kind=mail_digest / IM 幂等键含日期+通道 / 作战总台一条），摘要含计数与
// 主题行。
func TestMailDigest_ThreeLegs(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	if _, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	notify := &mailFakeNotify{}
	im := &mailFakeIM{sinks: []ReminderSink{{ChannelID: "ch-1", Platform: "wecom", Recipient: "u-1-im"}}}
	cb := &mailFakeCeoboard{}
	digest := NewMailDigestUsecase(repo, notify, im, cb, nil)
	// 种子邮件日期 2026-11-02 09:00-11:00 +0800（01:00-03:00 UTC）——固定
	// now 让默认 24h 窗口罩住三封。
	digest.now = func() time.Time { return time.Date(2026, 11, 2, 16, 0, 0, 0, time.FixedZone("CST", 8*3600)) }

	count, summary, err := digest.Trigger(ctx, "u-1", "ws-test", time.Time{})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if count != 3 {
		t.Fatalf("count=%d, want 3", count)
	}
	if !strings.Contains(summary, "共 3 封") || !strings.Contains(summary, "项目资金方案与排期表") {
		t.Fatalf("summary missing count/subjects: %q", summary)
	}

	// 腿 1：通知中心。
	if len(notify.items) != 1 {
		t.Fatalf("notifications=%d, want 1", len(notify.items))
	}
	n := notify.items[0]
	if n.Kind != NotifyKindMailDigest || n.Link != "/mail" || n.UserID != "u-1" {
		t.Fatalf("notification: %+v", n)
	}

	// 腿 2：IM（幂等键 mail-digest-<user>-<date>-<channel>）。
	if len(im.sent) != 1 {
		t.Fatalf("im sent=%d, want 1", len(im.sent))
	}
	dayKey := digest.now().Format("2006-01-02")
	wantKey := "mail-digest-u-1-" + dayKey + "-ch-1"
	if im.sent[0].key != wantKey || im.sent[0].sink.ChannelID != "ch-1" {
		t.Fatalf("im enqueue: %+v", im.sent[0])
	}

	// 腿 3：作战总台。
	if len(cb.events) != 1 || cb.events[0].userID != "u-1" {
		t.Fatalf("ceoboard events: %+v", cb.events)
	}
}

// TestMailDigestWorker_TickDedup 调度面：命中时刻 tick 触达窗口内有邮件的
// 用户；当日重 tick 去重不重复投递。
func TestMailDigestWorker_TickDedup(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	if _, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	notify := &mailFakeNotify{}
	digest := NewMailDigestUsecase(repo, notify, nil, nil, nil)
	worker := NewMailDigestWorker(repo, digest, nil)
	// 固定触发时刻：08:00（默认 hour=8）。
	fireAt := time.Date(2026, 11, 2, 8, 0, 0, 0, time.Local)
	worker.now = func() time.Time { return fireAt }
	digest.now = worker.now

	worker.tick(ctx)
	if len(notify.items) != 1 {
		t.Fatalf("after first tick notifications=%d, want 1", len(notify.items))
	}
	worker.tick(ctx) // 当日去重
	if len(notify.items) != 1 {
		t.Fatalf("after dedup tick notifications=%d, want 1", len(notify.items))
	}
	// 非触发时刻不动作。
	worker.now = func() time.Time { return time.Date(2026, 11, 3, 9, 0, 0, 0, time.Local) }
	worker.tick(ctx) // hour=9 ≠ 8，且窗口外无新邮件
	if len(notify.items) != 1 {
		t.Fatalf("off-hour tick notifications=%d, want 1", len(notify.items))
	}
}
