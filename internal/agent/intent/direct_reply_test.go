package intent

import "testing"

func TestSkipForDirectReply(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"你好，请用两三句话介绍你自己：你是谁、主要职责是什么、现在能帮我做什么。不要调用工具。", true},
		{"请记住：我的工号是 DIAG-20260818-A7", true},
		{"刚才我说的工号是什么？请直接回答，不要调用工具。", true},
		{"请调用工具查询当前时间，然后告诉我现在几点。", true},
		{"我刚才让你记住的工号和职责是什么？", true},
		{"who are you", true},
		{"明天天气怎么样", true},
		{"今天天气如何", true},
		{"帮我查一下北京明天的天气", true},
		{"what's the weather tomorrow", true},
		{"帮我做个应用", false},
		{"帮我做一个巡检看板", false},
		{"请排查杭州滨江机房核心交换机告警的根因", false},
		{"", false},
		{"  ", false},
	}
	for _, tc := range cases {
		if got := SkipForDirectReply(tc.in); got != tc.want {
			t.Errorf("SkipForDirectReply(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLooksLikeUnderspecifiedTask(t *testing.T) {
	if !LooksLikeUnderspecifiedTask("帮我做个应用") {
		t.Fatal("帮我做个应用 must keep the clarification gate")
	}
	if LooksLikeUnderspecifiedTask("现在能帮我做什么") {
		t.Fatal("能帮我做什么 must not look like an underspecified task")
	}
	if LooksLikeUnderspecifiedTask("请记住我的工号") {
		t.Fatal("remember turns are not underspecified tasks")
	}
}
