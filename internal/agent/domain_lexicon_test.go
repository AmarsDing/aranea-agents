package agent

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// ---------------------------------------------------------------------------
// 领域词表（B.10.21.3 / B.10.21.9 lexicon 层）单元测试。
// ---------------------------------------------------------------------------

func TestNormalizeDomainPath_LexiconHit(t *testing.T) {
	cases := map[string]string{
		"创作/文学": "创作/文学",
		"软件/后端": "软件/后端",
		"其他":    "其他",
		// 一级域本身是合法归一目标。
		"创作": "创作",
		"软件": "软件",
	}
	for in, want := range cases {
		if got := NormalizeDomainPath(in); got != want {
			t.Errorf("NormalizeDomainPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeDomainPath_PrefixMerge(t *testing.T) {
	cases := map[string]string{
		// 词表外二级域归并一级域。
		"创作/诗歌": "创作",
		"软件/中台": "软件",
		// 命中词表内更长前缀。
		"创作/文学/现代诗": "创作/文学",
		"软件/后端/微服务": "软件/后端",
	}
	for in, want := range cases {
		if got := NormalizeDomainPath(in); got != want {
			t.Errorf("NormalizeDomainPath(%q) = %q, want %q（词表外路径归并最近已知域）", in, got, want)
		}
	}
}

func TestNormalizeDomainPath_UnknownFallsBackToOther(t *testing.T) {
	for _, in := range []string{"诗歌", "cooking", "xyz/abc"} {
		if got := NormalizeDomainPath(in); got != "其他" {
			t.Errorf("NormalizeDomainPath(%q) = %q, want 其他（完全无法归类）", in, got)
		}
	}
}

func TestNormalizeDomainPath_EmptyStaysEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", " / ", "//", " \\ "} {
		if got := NormalizeDomainPath(in); got != "" {
			t.Errorf("NormalizeDomainPath(%q) = %q, want 空（advisory 不变量 1）", in, got)
		}
	}
}

func TestNormalizeDomainPath_WhitespaceAndSeparatorCollapse(t *testing.T) {
	cases := map[string]string{
		" 创作 / 文学 ": "创作/文学",
		"创作\\文学":    "创作/文学",
		"创作//文学":    "创作/文学",
	}
	for in, want := range cases {
		if got := NormalizeDomainPath(in); got != want {
			t.Errorf("NormalizeDomainPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeDomainPath_Idempotent(t *testing.T) {
	for _, in := range []string{"创作/诗歌", "软件/后端", "诗歌", ""} {
		once := NormalizeDomainPath(in)
		if twice := NormalizeDomainPath(once); twice != once {
			t.Errorf("NormalizeDomainPath(%q) 非幂等: %q → %q", in, once, twice)
		}
	}
}

func TestTopLevelDomain(t *testing.T) {
	cases := map[string]string{
		"创作/文学": "创作",
		"创作/诗歌": "创作", // 归并后再取一级域
		"其他":    "其他",
		"":      "",
	}
	for in, want := range cases {
		if got := TopLevelDomain(in); got != want {
			t.Errorf("TopLevelDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDomainPathRelated(t *testing.T) {
	cases := []struct {
		a, b  string
		want  bool
		notes string
	}{
		{"创作/文学", "创作/文学", true, "相等"},
		{"创作", "创作/文学", true, "前缀（任一方向）"},
		{"创作/文学", "创作", true, "前缀反向"},
		{"创作/文学", "创作/文案", true, "归并后同一级域"},
		{"软件/后端", "创作/文学", false, "不同一级域"},
		{"", "创作", false, "空不相关"},
		{"创作", "", false, "空不相关（反向）"},
		{"软件", "软件/前端", true, "一级域与二级域"},
	}
	for _, c := range cases {
		if got := DomainPathRelated(c.a, c.b); got != c.want {
			t.Errorf("DomainPathRelated(%q, %q) = %v, want %v（%s）", c.a, c.b, got, c.want, c.notes)
		}
	}
}

func TestPrimaryDomainPath(t *testing.T) {
	// 首个非空 subtask 域为主导域。
	subs := []biz.SubTask{
		{ID: "st_1", DomainPath: ""},
		{ID: "st_2", DomainPath: "创作/文学"},
		{ID: "st_3", DomainPath: "软件/后端"},
	}
	if got := PrimaryDomainPath(subs); got != "创作/文学" {
		t.Errorf("PrimaryDomainPath = %q, want 创作/文学（首个非空）", got)
	}
	if got := PrimaryDomainPath([]biz.SubTask{{ID: "st_1"}}); got != "" {
		t.Errorf("PrimaryDomainPath(全空) = %q, want 空", got)
	}
	if got := PrimaryDomainPath(nil); got != "" {
		t.Errorf("PrimaryDomainPath(nil) = %q, want 空", got)
	}
}

func TestDomainLexiconPromptList(t *testing.T) {
	list := DomainLexiconPromptList()
	for _, entry := range []string{"创作/文学", "软件/后端", "其他"} {
		if !strings.Contains(list, entry) {
			t.Errorf("DomainLexiconPromptList 缺少词表项 %q: %q", entry, list)
		}
	}
}
