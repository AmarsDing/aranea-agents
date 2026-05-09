package provider

import (
	"strings"
	"testing"
)

func TestVisibleStreamingDelta(t *testing.T) {
	t.Run("incremental_chunks", func(t *testing.T) {
		var b strings.Builder
		if got := VisibleStreamingDelta(&b, "你好"); got != "你好" || b.String() != "你好" {
			t.Fatalf("first chunk: got %q acc %q", got, b.String())
		}
		if got := VisibleStreamingDelta(&b, "，世界"); got != "，世界" || b.String() != "你好，世界" {
			t.Fatalf("second chunk: got %q acc %q", got, b.String())
		}
	})
	t.Run("cumulative_chunks", func(t *testing.T) {
		var b strings.Builder
		s := "好的，让我查看一下。"
		if got := VisibleStreamingDelta(&b, s); got != s || b.String() != s {
			t.Fatalf("first cumul: got %q acc %q", got, b.String())
		}
		if got := VisibleStreamingDelta(&b, s); got != "" || b.String() != s {
			t.Fatalf("repeat cumul: got %q acc %q", got, b.String())
		}
		ext := s + "完毕"
		if got := VisibleStreamingDelta(&b, ext); got != "完毕" || b.String() != ext {
			t.Fatalf("extend cumul: got %q acc %q", got, b.String())
		}
	})
}
