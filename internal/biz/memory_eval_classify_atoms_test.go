package biz

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitEvalStatements_TableDriven(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		content  string
		maxAtoms int
		want     []string
	}{
		{"empty passthrough", "", 8, []string{""}},
		{"single sentence unchanged", "我喜欢咖啡", 8, []string{"我喜欢咖啡"}},
		{"single sentence keeps trailing punct", "我最喜欢的颜色是红色。", 8, []string{"我最喜欢的颜色是红色。"}},
		{"chinese sentence delimiters", "我喜欢咖啡。我住在杭州。我的工号是9527。", 8, []string{"我喜欢咖啡。", "我住在杭州。", "我的工号是9527。"}},
		{"english period-space-uppercase", "I like coffee. She lives in Rome", 8, []string{"I like coffee.", "She lives in Rome"}},
		{"english period-space-lowercase no split", "I like coffee. then she left", 8, []string{"I like coffee. then she left"}},
		{"newline split", "我今天去了北京。她昨天回了上海", 8, []string{"我今天去了北京。", "她昨天回了上海"}},
		{"exclamation question semicolon", "你今天好吗？我很好啊！我们一起去吃饭吧；就这么定了。", 8, []string{"你今天好吗？", "我很好啊！", "我们一起去吃饭吧；", "就这么定了。"}},
		{"short fragments dropped", "好的。我最喜欢的颜色是红色。", 8, []string{"我最喜欢的颜色是红色。"}},
		{"all fragments fallback to trimmed whole", "啊。哦。嗯。", 8, []string{"啊。哦。嗯。"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SplitEvalStatements(tc.content, tc.maxAtoms)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitEvalStatements(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

func TestSplitEvalStatements_MaxAtomsMergesTail(t *testing.T) {
	t.Parallel()
	sents := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		sents = append(sents, "这是第"+string(rune('零'+i+1))+"句比较长的话。")
	}
	content := strings.Join(sents, "")
	got := SplitEvalStatements(content, 8)
	if len(got) != 8 {
		t.Fatalf("atoms=%d, want 8 (overflow merged into last atom)", len(got))
	}
	if !strings.HasSuffix(got[7], sents[9]) {
		t.Fatalf("last atom should contain merged tail, got %q", got[7])
	}
	if !strings.Contains(got[7], sents[7]) {
		t.Fatalf("last atom should start from atom 8, got %q", got[7])
	}
}

func TestSplitEvalStatements_ZeroMaxUsesDefault(t *testing.T) {
	t.Parallel()
	got := SplitEvalStatements("我喜欢咖啡。我住在杭州。", 0)
	if len(got) != 2 {
		t.Fatalf("atoms=%d, want 2 with default cap", len(got))
	}
}
