package biz

import "testing"

func TestIsChannelCancelCommand(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"取消", true},
		{"  Cancel  ", true},
		{"/cancel", true},
		{"停止", true},
		{"hello", false},
		{"", false},
		{"取消任务", false},
	}
	for _, tc := range cases {
		if got := IsChannelCancelCommand(tc.text); got != tc.want {
			t.Fatalf("IsChannelCancelCommand(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
