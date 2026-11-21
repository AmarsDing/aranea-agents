package office

// 服务器垃圾邮件文件夹同步：126 / Gmail 网页上「垃圾邮件」里的信，这边也能在「垃圾邮件」视图看到，
// 被误判的信点「不是垃圾邮件」就回收件箱（只改本地分类，不动服务器）。

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"aranea-agents/pkg/loggateway"
)

// junkFirstSyncMax 首次同步垃圾邮件文件夹最多取最近这么多封（垃圾箱常年堆着几千封广告）。
const junkFirstSyncMax = 200

// junkMailboxNames 服务器没标 \Junk 时按名字认（比较前已转小写、解码修改版 UTF-7）。
// 网易 126/163 的「垃圾邮件」不带 \Junk 属性，只能靠名字。
var junkMailboxNames = map[string]bool{
	"junk": true, "spam": true, "junk e-mail": true, "junk email": true, "bulk mail": true,
	"垃圾邮件": true, "垃圾箱": true,
}

// syncJunk 同步垃圾邮件文件夹（尽力而为：找不到或出错只记日志，不影响收件箱同步结果）。
// 游标 junk_last_uid 与收件箱各自独立；入库时 folder=junk、spam=true。
func (u *MailSyncUsecase) syncJunk(ctx context.Context, cli MailImapClient, acc *MailAccount) {
	warn := func(msg string, err error) {
		u.lg.Warn(msg, loggateway.Str("account_id", acc.ID), loggateway.Err(err))
	}
	boxes, err := cli.ListMailboxes()
	if err != nil {
		warn("mail junk: list mailboxes failed", err)
		return
	}
	name, ok := findJunkMailbox(boxes)
	if !ok {
		return
	}
	if _, err := cli.Select(name); err != nil {
		warn("mail junk: select failed", err)
		return
	}
	criteria := "ALL"
	switch {
	case acc.JunkLastUID > 0:
		criteria = fmt.Sprintf("UID %d:*", acc.JunkLastUID+1)
	case acc.SyncSince != nil:
		criteria = "SINCE " + acc.SyncSince.Format("2-Jan-2006")
	}
	uids, err := cli.SearchUIDs(criteria)
	if err != nil {
		warn("mail junk: search failed", err)
		return
	}
	sort.Ints(uids)
	if acc.JunkLastUID == 0 && len(uids) > junkFirstSyncMax {
		uids = uids[len(uids)-junkFirstSyncMax:]
	}
	maxUID := acc.JunkLastUID
	for _, uid := range uids {
		if int64(uid) <= acc.JunkLastUID {
			continue
		}
		raw, flags, err := cli.FetchUID(uid)
		if err != nil {
			warn("mail junk: fetch failed", err)
			break
		}
		m := ParseMailRaw(raw)
		m.Seen = strings.Contains(flags, `\Seen`)
		m.Flagged = strings.Contains(flags, `\Flagged`)
		msg := mailMessageFromImap(acc, uid, m, u.now)
		msg.Folder = MailFolderJunk
		msg.Spam = true
		if _, err := u.messages.InsertMessageWithAttachmentsTx(ctx, msg); err != nil {
			warn("mail junk: insert failed", err)
			break
		}
		maxUID = int64(uid)
	}
	if maxUID > acc.JunkLastUID {
		if err := u.accounts.UpdateMailJunkCursor(ctx, acc.ID, maxUID); err != nil {
			warn("mail junk: update cursor failed", err)
			return
		}
		acc.JunkLastUID = maxUID
	}
}

// findJunkMailbox 先认 RFC 6154 的 \Junk（老 Gmail XLIST 是 \Spam），再按常见名字认。
func findJunkMailbox(boxes []MailImapMailbox) (string, bool) {
	for _, mb := range boxes {
		for _, a := range strings.Fields(mb.Attrs) {
			if strings.EqualFold(a, `\Junk`) || strings.EqualFold(a, `\Spam`) {
				return mb.Name, true
			}
		}
	}
	for _, mb := range boxes {
		if strings.Contains(strings.ToLower(mb.Attrs), `\noselect`) {
			continue
		}
		n := strings.ToLower(decodeIMAPUTF7(mb.Name))
		segs := strings.FieldsFunc(n, func(r rune) bool { return r == '/' || r == '.' })
		if junkMailboxNames[n] || (len(segs) > 0 && junkMailboxNames[segs[len(segs)-1]]) {
			return mb.Name, true
		}
	}
	return "", false
}

// parseImapListLine 解析 `* LIST (\HasNoChildren \Junk) "/" "Junk"`；名字是字面量（{n}）的不认。
func parseImapListLine(line string) (MailImapMailbox, bool) {
	if len(line) < 8 || !strings.EqualFold(line[:7], "* LIST ") {
		return MailImapMailbox{}, false
	}
	rest := line[7:]
	end := strings.IndexByte(rest, ')')
	if !strings.HasPrefix(rest, "(") || end < 0 {
		return MailImapMailbox{}, false
	}
	attrs := rest[1:end]
	rest = strings.TrimSpace(rest[end+1:])
	switch {
	case strings.HasPrefix(rest, `"\\"`): // 分隔符是反斜杠
		rest = rest[4:]
	case len(rest) >= 3 && rest[0] == '"' && rest[2] == '"':
		rest = rest[3:]
	case len(rest) >= 3 && strings.EqualFold(rest[:3], "NIL"):
		rest = rest[3:]
	default:
		return MailImapMailbox{}, false
	}
	name := strings.TrimSpace(rest)
	if len(name) >= 2 && name[0] == '"' && name[len(name)-1] == '"' {
		name = strings.ReplaceAll(strings.ReplaceAll(name[1:len(name)-1], `\"`, `"`), `\\`, `\`)
	}
	if name == "" || strings.HasPrefix(name, "{") {
		return MailImapMailbox{}, false
	}
	return MailImapMailbox{Name: name, Attrs: attrs}, true
}

// decodeIMAPUTF7 修改版 UTF-7（RFC 3501 §5.1.3）：`&-` 是 `&`，`&...-` 是 base64（`,` 代 `/`）的 UTF-16BE。
// 解不开的段原样保留。
func decodeIMAPUTF7(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '&' {
			b.WriteByte(s[i])
			continue
		}
		j := strings.IndexByte(s[i+1:], '-')
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		enc := s[i+1 : i+1+j]
		i += j + 1
		if enc == "" {
			b.WriteByte('&')
			continue
		}
		raw, err := base64.RawStdEncoding.DecodeString(strings.ReplaceAll(enc, ",", "/"))
		if err != nil || len(raw)%2 != 0 {
			b.WriteString("&" + enc + "-")
			continue
		}
		u := make([]uint16, len(raw)/2)
		for k := range u {
			u[k] = uint16(raw[2*k])<<8 | uint16(raw[2*k+1])
		}
		b.WriteString(string(utf16.Decode(u)))
	}
	return b.String()
}
