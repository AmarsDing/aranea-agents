package voice

import (
	"context"
	"strings"
)

// VoiceConfirmDecision 是语音确认词表的匹配结果（M74 V2-T5，需求 §5.3：
// 用户说「好的」确认 / 说「算了」取消）。
type VoiceConfirmDecision int

const (
	// VoiceConfirmNone 未命中词表（按普通语句进 Chat 管线）。
	VoiceConfirmNone VoiceConfirmDecision = iota
	// VoiceConfirmApprove 批准待决议的工具确认。
	VoiceConfirmApprove
	// VoiceConfirmDeny 拒绝待决议的工具确认。
	VoiceConfirmDeny
)

// Stability:evolving — 语音确认决议端口（service 层适配实现）。
//
// 仅当会话内存在待决议的工具确认 step（kind=confirm + tool_blocked）时
// 返回 resolved=true；此时语音语句被拦截，不再进入 Chat Turn 管线。
type ConfirmResolver interface {
	ResolvePendingConfirm(ctx context.Context, sessionID string, approved bool) (resolved bool, err error)
}

// 词表取「整句精确匹配」（归一化大小写/空白/首尾标点），有意保持保守：
// 命中但无待决议确认时会照常落入 Chat 管线，故误命中的代价只是一次 resolver 查询。
var approveWords = []string{
	"好的", "好", "好吧", "好呀", "嗯", "行", "可以", "确认", "同意", "批准", "允许", "执行",
	"打开吧", "开吧", "ok", "okay", "yes", "yeah", "sure",
}

var denyWords = []string{
	"算了", "取消", "拒绝", "不要", "别", "不行", "不用", "不用了", "先别",
	"no", "nope", "cancel",
}

// MatchVoiceConfirm 将 ASR 终稿匹配到确认决议；普通语句返回 VoiceConfirmNone。
func MatchVoiceConfirm(text string) VoiceConfirmDecision {
	w := normalizeConfirmWord(text)
	if w == "" {
		return VoiceConfirmNone
	}
	for _, a := range approveWords {
		if w == a {
			return VoiceConfirmApprove
		}
	}
	for _, d := range denyWords {
		if w == d {
			return VoiceConfirmDeny
		}
	}
	return VoiceConfirmNone
}

func normalizeConfirmWord(text string) string {
	w := strings.ToLower(strings.TrimSpace(text))
	w = strings.Trim(w, "。！？!?,.，、~～;；…")
	w = strings.ReplaceAll(w, " ", "")
	return w
}
