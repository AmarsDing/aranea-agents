package office

import (
	"context"
	"net/mail"
	"sort"
	"strings"
	"testing"
	"time"
)

// ---------- fake 仓储补齐（第二轮新口） ----------

func (f *mailFakeRepos) ListThread(_ context.Context, userID, subjectKey string, limit int) ([]*MailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*MailMessage
	for _, m := range f.messages {
		if m.UserID == userID && m.TrashedAt == nil && !mailFolderIs(m, MailFolderDrafts) && threadKey(m.Subject) == subjectKey {
			out = append(out, cloneMessage(m))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReceivedAt.Before(*out[j].ReceivedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *mailFakeRepos) ListRecentAddressFields(_ context.Context, userID string, _ int) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, m := range f.messages {
		if m.UserID == userID && !m.Spam {
			for _, s := range []string{m.Sender, m.To, m.Cc} {
				if s != "" {
					out = append(out, s)
				}
			}
		}
	}
	return out, nil
}

func (f *mailFakeRepos) MarkViewSeen(ctx context.Context, userID, view, accountID string) ([]string, error) {
	rows, _, _ := f.QueryMailMessages(ctx, userID, MailQuery{View: view, AccountID: accountID})
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for _, r := range rows {
		if m := f.messages[r.ID]; m != nil && !m.Seen {
			m.Seen = true
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

func (f *mailFakeRepos) PurgeTrashedBefore(_ context.Context, before time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for id, m := range f.messages {
		if m.TrashedAt != nil && m.TrashedAt.Before(before) {
			delete(f.messages, id)
			n++
		}
	}
	return n, nil
}

func (f *mailFakeRepos) ListDueScheduledDrafts(_ context.Context, now time.Time, _ int) ([]*MailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*MailMessage
	for _, m := range f.messages {
		if mailFolderIs(m, MailFolderDrafts) && m.SendAt != nil && !m.SendAt.After(now) {
			out = append(out, cloneMessage(m))
		}
	}
	return out, nil
}

func (f *mailFakeRepos) ClearDraftSendAt(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m := f.messages[id]; m != nil {
		m.SendAt = nil
	}
	return nil
}

func (f *mailFakeRepos) ListBlockedSenders(_ context.Context, userID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.blocked[userID]...), nil
}

func (f *mailFakeRepos) BlockSender(_ context.Context, userID, address string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked == nil {
		f.blocked = map[string][]string{}
	}
	f.blocked[userID] = append(f.blocked[userID], address)
	var n int64
	for _, m := range f.messages {
		if m.UserID == userID && !m.Spam && strings.Contains(strings.ToLower(m.Sender), address) {
			m.Spam = true
			n++
		}
	}
	return n, nil
}

func (f *mailFakeRepos) UnblockSender(_ context.Context, userID, address string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keep []string
	for _, a := range f.blocked[userID] {
		if a != address {
			keep = append(keep, a)
		}
	}
	f.blocked[userID] = keep
	return nil
}

func (f *fakeEventRepo) ListRecurringReminders(_ context.Context, _ int) ([]*Event, error) {
	var out []*Event
	for _, ev := range f.byID {
		if ev.RRule != "" && ev.ReminderMinutes > 0 {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (f *fakeEventRepo) SearchEvents(_ context.Context, userID, keyword string, _ int) ([]*Event, error) {
	var out []*Event
	for _, ev := range f.byID {
		if ev.UserID == userID && strings.Contains(ev.Title+ev.Note+ev.Location, keyword) {
			out = append(out, ev)
		}
	}
	return out, nil
}

// ---------- 用例 ----------

func TestThreadKeyAndContacts(t *testing.T) {
	for in, want := range map[string]string{
		"Re: 回复：周报":       "周报",
		"FW: Fwd: Budget": "budget",
		"  转发: 项目 A  ":    "项目 a",
		"普通主题":            "普通主题",
	} {
		if got := threadKey(in); got != want {
			t.Errorf("threadKey(%q)=%q want %q", in, got, want)
		}
	}
	repo := newMailFakeRepos()
	now := time.Now()
	for i, m := range []struct{ subj, from, to string }{
		{"周报", "李卓 <li@x.cn>", "me@x.cn"},
		{"Re: 周报", "me@x.cn", "李卓 <li@x.cn>, wm@x.cn"},
		{"回复：周报", "li@x.cn", "me@x.cn"},
		{"别的", "hr@x.cn", "me@x.cn"},
	} {
		at := now.Add(time.Duration(i) * time.Minute)
		repo.messages[m.subj+m.from] = &MailMessage{ID: m.subj + m.from, UserID: "u", Subject: m.subj, Sender: m.from, To: m.to, ReceivedAt: &at}
	}
	uc := NewMailMessageUsecase(repo, repo, repo, nil, nil)
	th, err := uc.Thread(context.Background(), "u", "周报李卓 <li@x.cn>")
	if err != nil || len(th) != 3 {
		t.Fatalf("thread: n=%d err=%v", len(th), err)
	}
	hits, _ := uc.SuggestContacts(context.Background(), "u", "li", 5)
	if len(hits) != 1 || hits[0].Address != "li@x.cn" || hits[0].Name != "李卓" || hits[0].Count != 3 {
		t.Fatalf("contacts: %+v", hits)
	}
	if all, _ := uc.SuggestContacts(context.Background(), "u", "", 2); len(all) != 2 || all[0].Address != "me@x.cn" {
		t.Fatalf("top contacts: %+v", all)
	}
}

func TestMailLabelsAndBlock(t *testing.T) {
	repo := newMailFakeRepos()
	uc := NewMailMessageUsecase(repo, repo, repo, nil, nil)
	ctx := context.Background()
	if _, err := uc.Mark(ctx, "u", []string{"x"}, MailMark{AddLabel: "a,b"}); err != ErrMailLabelInvalid {
		t.Fatalf("comma label: %v", err)
	}
	if _, err := uc.Block(ctx, "u", "not an address"); err != ErrMailAddressInvalid {
		t.Fatalf("bad address: %v", err)
	}
	repo.messages["m1"] = &MailMessage{ID: "m1", UserID: "u", Sender: "Ad <Ad@Spam.cn>", Folder: MailFolderInbox}
	n, err := uc.Block(ctx, "u", "AD@spam.cn")
	if err != nil || n != 1 || !repo.messages["m1"].Spam {
		t.Fatalf("block: n=%d err=%v", n, err)
	}
	if list, _ := uc.ListBlocked(ctx, "u"); len(list) != 1 || list[0] != "ad@spam.cn" {
		t.Fatalf("blocked list: %v", list)
	}
	if senderAddr("Ad <Ad@Spam.cn>") != "ad@spam.cn" || senderAddr("bare@x.cn") != "bare@x.cn" {
		t.Fatal("senderAddr")
	}
}

// TestMailServerWriteBack 删除 / 恢复 / 举报回写服务器：按 Message-ID 在源文件夹里找到、挪过去；
// 挪回收件箱后再同步不会多出一封（按 Message-ID 去重）。
func TestMailServerWriteBack(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	msgs := NewMailMessageUsecase(repo, repo, repo, nil, nil)
	if _, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	inbox, _, _ := msgs.Query(ctx, "u-1", MailQuery{View: MailViewInbox})
	first := inbox[0]
	yes, no := true, false

	// 删除 → 服务器收件箱少一封、废件箱多一封
	if _, err := msgs.Mark(ctx, "u-1", []string{first.ID}, MailMark{Trashed: &yes}); err != nil {
		t.Fatal(err)
	}
	syncer.PushMoves(ctx, "u-1", []string{first.ID}, MailMark{Trashed: &yes})
	if srv.Mailbox().Live() != 2 || srv.Trash().Live() != 1 {
		t.Fatalf("after trash: inbox=%d trash=%d", srv.Mailbox().Live(), srv.Trash().Live())
	}
	// 恢复 → 回收件箱（新 UID）；再同步不重复
	if _, err := msgs.Mark(ctx, "u-1", []string{first.ID}, MailMark{Trashed: &no}); err != nil {
		t.Fatal(err)
	}
	syncer.PushMoves(ctx, "u-1", []string{first.ID}, MailMark{Trashed: &no})
	if srv.Mailbox().Live() != 3 || srv.Trash().Live() != 0 {
		t.Fatalf("after restore: inbox=%d trash=%d", srv.Mailbox().Live(), srv.Trash().Live())
	}
	if _, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil {
		t.Fatalf("resync: %v", err)
	}
	if inbox, _, _ = msgs.Query(ctx, "u-1", MailQuery{View: MailViewInbox}); len(inbox) != 3 {
		t.Fatalf("dedupe by message-id failed: inbox=%d", len(inbox))
	}
	// 举报 → 服务器垃圾箱
	if _, err := msgs.Mark(ctx, "u-1", []string{first.ID}, MailMark{Spam: &yes}); err != nil {
		t.Fatal(err)
	}
	junkBefore := srv.Junk().Live()
	syncer.PushMoves(ctx, "u-1", []string{first.ID}, MailMark{Spam: &yes})
	if srv.Junk().Live() != junkBefore+1 || srv.Mailbox().Live() != 2 {
		t.Fatalf("after spam: junk=%d inbox=%d", srv.Junk().Live(), srv.Mailbox().Live())
	}
}

// TestMailScheduledSendAndTrashPurge 定时草稿到点发出（发不出去退回普通草稿）；已删除放满 30 天清掉。
func TestMailScheduledSendAndTrashPurge(t *testing.T) {
	repo := newMailFakeRepos()
	ctx := context.Background()
	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	past := time.Now().Add(-time.Minute)
	// 账户不存在 → 发送失败 → 退回普通草稿
	repo.messages["d1"] = &MailMessage{ID: "d1", UserID: "u", AccountID: "nope", Folder: MailFolderDrafts, To: "a@b.cn", SendAt: &past}
	syncer.SendDueDrafts(ctx)
	if repo.messages["d1"].SendAt != nil {
		t.Fatal("failed scheduled draft should fall back to a normal draft")
	}
	old := time.Now().Add(-31 * 24 * time.Hour)
	recent := time.Now().Add(-time.Hour)
	repo.messages["t1"] = &MailMessage{ID: "t1", UserID: "u", TrashedAt: &old}
	repo.messages["t2"] = &MailMessage{ID: "t2", UserID: "u", TrashedAt: &recent}
	syncer.PurgeOldTrash(ctx)
	if _, ok := repo.messages["t1"]; ok {
		t.Fatal("old trash not purged")
	}
	if _, ok := repo.messages["t2"]; !ok {
		t.Fatal("recent trash purged")
	}
}

func TestBuildOutgoingMailHTML(t *testing.T) {
	from := &mail.Address{Address: "me@x.cn"}
	raw := string(buildOutgoingMail(from, []*mail.Address{{Address: "a@x.cn"}}, nil, "s", "plain", "<p><b>bold</b></p>", "<id@x>", "", "", nil, time.Now()))
	if !strings.Contains(raw, "multipart/alternative") || !strings.Contains(raw, "text/html") || !strings.Contains(raw, "text/plain") {
		t.Fatalf("html mail missing parts:\n%s", raw)
	}
	m := ParseMailRaw([]byte(raw))
	if !strings.Contains(m.HTML, "bold") || strings.TrimSpace(m.Body) != "plain" {
		t.Fatalf("round trip: body=%q html=%q", m.Body, m.HTML)
	}
}
