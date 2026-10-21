package biz

import "testing"

// TestLooksLikeConstraintStatement_EvalExt covers the AMC C3-A Q7a cue
// extensions (如果/若/每当/只要/按照/规定/规则/whenever/if…then/policy).
func TestLooksLikeConstraintStatement_EvalExt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want bool
	}{
		{"如果明天下雨我就取消露营", true},
		{"若客户再来电话就转给我", true},
		{"每当有人进门要说欢迎", true},
		{"只要订单超过一百就免运费", true},
		{"按照公司流程报销需要发票", true},
		{"公司有规定要打卡", true},
		{"他喜欢研究规则怪谈", true},
		{"Whenever it rains, take an umbrella", true},
		{"If it rains then stay home", true},
		{"Company policy forbids smoking here", true},
		{"I always drink coffee in the morning", false},
		{"我喜欢喝拿铁", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := LooksLikeConstraintStatement(tc.stmt); got != tc.want {
			t.Errorf("LooksLikeConstraintStatement(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}

// TestPreferenceSlotKey_EvalExt covers the AMC C3-A Q7c slot extensions
// (搬到/搬去/moved to→live, 换工作/入职/新工作→job, 手机号/电话号码→phone).
func TestPreferenceSlotKey_EvalExt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want string
	}{
		{"我搬到了上海", "live"},
		{"她搬去了北京", "live"},
		{"I moved to Berlin last month", "live"},
		{"我换工作了", "job"},
		{"我入职了新的互联网公司", "job"},
		{"我找到了一份新工作", "job"},
		{"我的手机号是13800001111", "phone"},
		{"我的电话号码是13800001111", "phone"},
		{"我最喜欢的颜色是红色", "favorite:颜色"},
		{"今天天气不错", ""},
	}
	for _, tc := range cases {
		if got := PreferenceSlotKey(tc.stmt); got != tc.want {
			t.Errorf("PreferenceSlotKey(%q) = %q, want %q", tc.stmt, got, tc.want)
		}
	}
}

// TestHasPreferenceUpdateCue_EvalExt covers 不喜欢了/没感觉/改为 added by
// AMC C3-A Q7c (existing cues remain covered by older tests).
func TestHasPreferenceUpdateCue_EvalExt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want bool
	}{
		{"我现在不喜欢蓝色了", true},
		{"我对咖啡没感觉", true},
		{"我的最爱改为红茶", true},
		{"最喜欢改成乌龙茶", true},
		{"换成冰美式了", true},
		{"I prefer tea now", true},
		{"我喜欢茉莉花茶", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := HasPreferenceUpdateCue(tc.stmt); got != tc.want {
			t.Errorf("HasPreferenceUpdateCue(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}
