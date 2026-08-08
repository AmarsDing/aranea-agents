package voice

import "testing"

func TestMatchVoiceConfirm_ApproveWords(t *testing.T) {
	for _, w := range []string{
		"好的", "好", "好吧", "好呀", "嗯", "行", "可以", "确认", "同意", "批准", "允许", "执行",
		"打开吧", "开吧", "ok", "OK", "okay", "Yes", "yeah", "sure",
	} {
		if got := MatchVoiceConfirm(w); got != VoiceConfirmApprove {
			t.Errorf("MatchVoiceConfirm(%q) = %v, want approve", w, got)
		}
	}
}

func TestMatchVoiceConfirm_DenyWords(t *testing.T) {
	for _, w := range []string{
		"算了", "取消", "拒绝", "不要", "别", "不行", "不用", "不用了", "先别",
		"no", "No", "nope", "cancel",
	} {
		if got := MatchVoiceConfirm(w); got != VoiceConfirmDeny {
			t.Errorf("MatchVoiceConfirm(%q) = %v, want deny", w, got)
		}
	}
}

func TestMatchVoiceConfirm_NormalizesPunctuationAndCase(t *testing.T) {
	cases := map[string]VoiceConfirmDecision{
		"好的。":  VoiceConfirmApprove,
		" 好的 ": VoiceConfirmApprove,
		"OK!":  VoiceConfirmApprove,
		"可以，":  VoiceConfirmApprove,
		"算了…":  VoiceConfirmDeny,
		"No。":  VoiceConfirmDeny,
		"好 吧":  VoiceConfirmApprove,
	}
	for in, want := range cases {
		if got := MatchVoiceConfirm(in); got != want {
			t.Errorf("MatchVoiceConfirm(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMatchVoiceConfirm_NoneForOrdinaryUtterances(t *testing.T) {
	for _, w := range []string{
		"", "   ", "帮我打开微信", "打开微信", "今天天气怎么样", "好的帮我查一下天气", "确认一下明天日程", "不想要", "别走",
	} {
		if got := MatchVoiceConfirm(w); got != VoiceConfirmNone {
			t.Errorf("MatchVoiceConfirm(%q) = %v, want none", w, got)
		}
	}
}
