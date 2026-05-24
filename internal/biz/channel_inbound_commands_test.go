package biz

import "testing"

func TestIsChannelStatusQuery(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"?", true},
		{"？", true},
		{" ? ", true},
		{"hello", false},
		{"??", false},
	}
	for _, tc := range cases {
		if got := IsChannelStatusQuery(tc.text); got != tc.want {
			t.Fatalf("IsChannelStatusQuery(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
