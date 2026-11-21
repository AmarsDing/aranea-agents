package office

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDecodeIMAPUTF7(t *testing.T) {
	for in, want := range map[string]string{
		"&V4NXPpCuTvY-":   "垃圾邮件",
		"INBOX":           "INBOX",
		"Tom &- Jerry":    "Tom & Jerry",
		"[Gmail]/Spam":    "[Gmail]/Spam",
		"&broken!-x":      "&broken!-x",
		"&V4NXPpCuTvY-/a": "垃圾邮件/a",
	} {
		if got := decodeIMAPUTF7(in); got != want {
			t.Errorf("decodeIMAPUTF7(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestParseImapListLineAndFindJunk(t *testing.T) {
	lines := []string{
		`* LIST (\HasNoChildren) "/" "INBOX"`,
		`* LIST () "/" "&V4NXPpCuTvY-"`,
		`* LIST (\HasNoChildren \Junk) "/" "[Gmail]/Spam"`,
		`* LIST (\Noselect) "." Junk`,
		`* LIST () NIL "Archive"`,
		`* LIST (\HasNoChildren) "\\" "Folder"`,
		`* OK LIST done`,
	}
	var boxes []MailImapMailbox
	for _, l := range lines {
		if mb, ok := parseImapListLine(l); ok {
			boxes = append(boxes, mb)
		}
	}
	if len(boxes) != 6 || boxes[2].Name != "[Gmail]/Spam" || boxes[3].Name != "Junk" || boxes[5].Name != "Folder" {
		t.Fatalf("parsed boxes: %+v", boxes)
	}
	// \Junk 属性优先于名字。
	if name, ok := findJunkMailbox(boxes); !ok || name != "[Gmail]/Spam" {
		t.Fatalf("find junk with attr: %q %v", name, ok)
	}
	// 网易：没有 \Junk，按解码后的名字认；\Noselect 的不选。
	netease := []MailImapMailbox{boxes[0], boxes[1], boxes[3]}
	if name, ok := findJunkMailbox(netease); !ok || name != "&V4NXPpCuTvY-" {
		t.Fatalf("find junk by name: %q %v", name, ok)
	}
	if _, ok := findJunkMailbox([]MailImapMailbox{boxes[0], boxes[4]}); ok {
		t.Fatal("no junk folder should be found")
	}
}

// TestMailSync_JunkFolder 收件箱同步后顺带同步服务器垃圾箱：进「垃圾邮件」视图、不进收件箱，
// 二次同步不重复；网易账号（文件夹不带 \Junk、名字是修改版 UTF-7）也认得出。
func TestMailSync_JunkFolder(t *testing.T) {
	for _, username := range []string{"test@local", "me@126.com"} {
		t.Run(username, func(t *testing.T) {
			srv := startMockIMAPServer(t)
			repo := newMailFakeRepos()
			ctx := context.Background()
			acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
			repo.accounts[acc.ID].Username = username
			syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
			msgs := NewMailMessageUsecase(repo, repo, repo, nil, nil)

			if n, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil || n != 3 {
				t.Fatalf("sync: imported=%d err=%v (junk 不计入收件箱导入数)", n, err)
			}
			spam, _, err := msgs.Query(ctx, "u-1", MailQuery{View: MailViewSpam})
			if err != nil || len(spam) != 1 || !strings.Contains(spam[0].Subject, "恭喜中奖") || spam[0].Folder != MailFolderJunk {
				t.Fatalf("spam view: %+v err=%v", spam, err)
			}
			inbox, _, _ := msgs.Query(ctx, "u-1", MailQuery{View: MailViewInbox})
			if len(inbox) != 3 {
				t.Fatalf("inbox should not list junk: n=%d", len(inbox))
			}
			if cur, _ := repo.GetMailAccount(ctx, acc.ID, "u-1"); cur.JunkLastUID != 1 {
				t.Fatalf("junk cursor=%d, want 1", cur.JunkLastUID)
			}

			// 二刷不重复；服务器垃圾箱新来一封，增量取到。
			srv.Junk().Append([]byte("From: x@spam.example\r\nSubject: second ad\r\n\r\nbuy\r\n"))
			if _, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil {
				t.Fatalf("resync: %v", err)
			}
			spam, _, _ = msgs.Query(ctx, "u-1", MailQuery{View: MailViewSpam})
			if len(spam) != 2 {
				t.Fatalf("spam after resync: n=%d, want 2", len(spam))
			}

			// 删到「已删除」→ 恢复 → 彻底删除；不是垃圾邮件 → 回收件箱。
			yes, no := true, false
			if _, err := msgs.Mark(ctx, "u-1", []string{spam[0].ID}, MailMark{Spam: &no}); err != nil {
				t.Fatalf("not spam: %v", err)
			}
			if inbox, _, _ = msgs.Query(ctx, "u-1", MailQuery{View: MailViewInbox}); len(inbox) != 4 {
				t.Fatalf("rescued mail should be in inbox: n=%d", len(inbox))
			}
			if _, err := msgs.Mark(ctx, "u-1", []string{spam[1].ID}, MailMark{Trashed: &yes}); err != nil {
				t.Fatalf("trash: %v", err)
			}
			trash, _, _ := msgs.Query(ctx, "u-1", MailQuery{View: MailViewTrash})
			if len(trash) != 1 {
				t.Fatalf("trash view: n=%d", len(trash))
			}
			if n, err := msgs.Purge(ctx, "u-1", MailViewTrash, nil); err != nil || n != 1 {
				t.Fatalf("empty trash: n=%d err=%v", n, err)
			}
			if _, err := msgs.Purge(ctx, "u-1", MailViewInbox, nil); !errors.Is(err, ErrMailViewInvalid) {
				t.Fatalf("purge inbox must be refused: %v", err)
			}
		})
	}
}
