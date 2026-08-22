package biz

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTrimPrefixBudgetCapsAndDropsKnowledgeFirst(t *testing.T) {
	t.Parallel()
	in := PrefixBudget{
		Brief:     strings.Repeat("结", BriefBudgetRunes+50),
		Knowledge: strings.Repeat("知", KnowledgeBudgetRunes),
		Memory:    strings.Repeat("记", MemoryBudgetRunes),
		Protocol:  strings.Repeat("协", ProtocolBudgetRunes),
	}
	out := TrimPrefixBudget(in)
	if utf8.RuneCountInString(out.Brief) != BriefBudgetRunes {
		t.Fatalf("brief=%d", utf8.RuneCountInString(out.Brief))
	}
	if out.totalRunes() > MemberPrefixBudgetRunes {
		t.Fatalf("total=%d", out.totalRunes())
	}
	tight := trimPrefixBudgetWithLimit(in, 4500)
	if utf8.RuneCountInString(tight.Brief) != BriefBudgetRunes {
		t.Fatal("brief must survive overflow trim")
	}
	if utf8.RuneCountInString(tight.Knowledge) >= KnowledgeBudgetRunes {
		t.Fatal("overflow must shrink knowledge before brief")
	}
	if tight.totalRunes() > 4500 {
		t.Fatalf("tight total=%d", tight.totalRunes())
	}
}

func TestTrimPrefixBudgetSmallStays(t *testing.T) {
	t.Parallel()
	in := PrefixBudget{Brief: "短结论", Knowledge: "一条知识"}
	out := TrimPrefixBudget(in)
	if out.Brief != in.Brief || out.Knowledge != in.Knowledge {
		t.Fatalf("got %+v", out)
	}
}
