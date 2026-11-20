package office

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ICalUsecase ics 只读订阅（K-3）：token-in-path 公开 feed，令牌即凭据。
// 安全口径：token 明文只在 CreateICalToken 返回值出现一次；库内只存
// SHA-256 哈希；吊销即 ResolveICalToken 失败 → feed 401。
type ICalUsecase struct {
	tokens ICalTokenRepo
	events EventRepo
}

func NewICalUsecase(tokens ICalTokenRepo, events EventRepo) *ICalUsecase {
	return &ICalUsecase{tokens: tokens, events: events}
}

// HashICalToken 令牌哈希（feed 端点与建库共用同一口径）。
func HashICalToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// FeedPath 返回相对 feed 路径（前端拼 origin 组成可订阅 URL）。
func FeedPath(token string) string { return "/v1/office/calendar/feeds/" + token }

// CreateToken 生成令牌（32B 随机 hex=64 字符）并落哈希；返回明文 token
// 仅此一次（调用方负责展示，不回读）。
func (u *ICalUsecase) CreateToken(ctx context.Context, userID, name string) (*ICalToken, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, "", err
	}
	plain := hex.EncodeToString(buf)
	t := &ICalToken{
		ID:        "ics_" + uuid.NewString(),
		UserID:    userID,
		TokenHash: HashICalToken(plain),
		Name:      strings.TrimSpace(name),
	}
	if err := u.tokens.CreateICalToken(ctx, t); err != nil {
		return nil, "", err
	}
	return t, plain, nil
}

func (u *ICalUsecase) RevokeToken(ctx context.Context, id, userID string) error {
	return u.tokens.RevokeICalToken(ctx, id, userID)
}

func (u *ICalUsecase) ListTokens(ctx context.Context, userID string) ([]*ICalToken, error) {
	return u.tokens.ListICalTokens(ctx, userID)
}

// ics 输出窗口：回看 31 天、前瞻 366 天（个人日程体量下的合理边界；
// 全量导出属 F2 CalDAV 双向同步范围）。
const (
	icalLookback  = 31 * 24 * time.Hour
	icalLookahead = 366 * 24 * time.Hour
)

// RenderFeed 解析令牌 → 渲染属主事件的 text/calendar 快照。令牌无效/吊销
// 返回 ErrICalTokenInvalid（service 映 401）。DTSTART/DTEND 一律 UTC
// （RFC 5545 基本形式，免 VTIMEZONE 复杂度，客户端普适）。
func (u *ICalUsecase) RenderFeed(ctx context.Context, token string, now time.Time) (string, error) {
	userID, err := u.tokens.ResolveICalToken(ctx, HashICalToken(strings.TrimSpace(token)))
	if err != nil {
		return "", err
	}
	events, err := u.events.ListEventsRange(ctx, userID, now.Add(-icalLookback), now.Add(icalLookahead))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//aranea//office calendar//CN\r\nCALSCALE:GREGORIAN\r\n")
	for _, ev := range events {
		// 订阅到手机的是「要做的」：工作记录和已取消的计划不导出。
		if ev.Kind == KindLog || ev.Status == StatusCancelled {
			continue
		}
		b.WriteString("BEGIN:VEVENT\r\n")
		fmt.Fprintf(&b, "UID:%s@aranea\r\n", ev.ID)
		fmt.Fprintf(&b, "DTSTAMP:%s\r\n", ev.CreatedAt.UTC().Format("20060102T150405Z"))
		if ev.AllDay {
			// 全天按事件自己的时区取日期，写成 DATE 值（结束不含），手机上显示为全天。
			loc, lerr := time.LoadLocation(ev.Timezone)
			if lerr != nil {
				loc = time.UTC
			}
			fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", ev.StartAt.In(loc).Format("20060102"))
			fmt.Fprintf(&b, "DTEND;VALUE=DATE:%s\r\n", ev.EndAt.In(loc).Format("20060102"))
		} else {
			fmt.Fprintf(&b, "DTSTART:%s\r\n", ev.StartAt.UTC().Format("20060102T150405Z"))
			fmt.Fprintf(&b, "DTEND:%s\r\n", ev.EndAt.UTC().Format("20060102T150405Z"))
		}
		fmt.Fprintf(&b, "SUMMARY:%s\r\n", icalEscape(ev.Title))
		if ev.Note != "" {
			fmt.Fprintf(&b, "DESCRIPTION:%s\r\n", icalEscape(ev.Note))
		}
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String(), nil
}

// icalEscape RFC 5545 §3.3.11 文本转义（反斜杠/分号/逗号/换行）。
func icalEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		";", `\;`,
		",", `\,`,
		"\r\n", `\n`,
		"\n", `\n`,
		"\r", `\n`,
	)
	return r.Replace(s)
}
