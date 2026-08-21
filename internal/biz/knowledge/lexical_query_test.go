package knowledge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

const lexicalGoldDir = "../../../docs/testing/agent-eval-20260818/03-knowledge-rag"

// P1-3（2026-08-21）：CJK bigram 抽取——2 字中文词是问句与正文的核心重叠信号。
func TestCJKBigrams(t *testing.T) {
	t.Parallel()

	t.Run("问句提取 2 字词信号", func(t *testing.T) {
		got := CJKBigrams("夜班需要几个人同时值守？")
		for _, want := range []string{"夜班", "时值", "值守"} {
			if !containsString(got, want) {
				t.Errorf("bigrams %v missing %q", got, want)
			}
		}
		// 「需要」是 filler，清洗后不应残留其 bigram
		if containsString(got, "需要") {
			t.Errorf("filler bigram 需要 leaked: %v", got)
		}
	})

	t.Run("分隔符切 run 不跨词界", func(t *testing.T) {
		got := CJKBigrams("生产环境发布变更前必须做什么？")
		for _, want := range []string{"生产", "变更"} {
			if !containsString(got, want) {
				t.Errorf("bigrams %v missing %q", got, want)
			}
		}
		// 「前」是 splitter：不得产生跨 run 的「更前」
		if containsString(got, "更前") {
			t.Errorf("splitter boundary crossed: %v", got)
		}
	})

	t.Run("stopNeedle bigram 被剔除", func(t *testing.T) {
		got := CJKBigrams("机房里可以只留一个人吗？")
		if containsString(got, "可以") {
			t.Errorf("stopNeedle bigram 可以 leaked: %v", got)
		}
		if !containsString(got, "机房") {
			t.Errorf("bigrams %v missing 机房", got)
		}
	})

	t.Run("纯英文查询无 bigram", func(t *testing.T) {
		if got := CJKBigrams("secondary oncall"); len(got) != 0 {
			t.Errorf("english query bigrams = %v, want empty", got)
		}
	})
}

func TestLexicalSearchQueries_ShortKeywordUnchanged(t *testing.T) {
	for _, q := range []string{"斑马线", "TACACS+", "SW-Core-01", "INS-YYYYMMDD-NN", "DUTY-YYYYMMDD-NN"} {
		got := LexicalSearchQueries(q)
		if len(got) != 1 || got[0] != q {
			t.Fatalf("query %q variants = %v, want exactly [%q]", q, got, q)
		}
	}
}

func TestLexicalSearchQueries_StripsQuestionFillers(t *testing.T) {
	got := LexicalSearchQueries("请问核心机房的巡检周期是多久一次，周几几点开始？")
	joined := strings.Join(got, " ")
	for _, needle := range []string{"核心机房", "巡检周期"} {
		if !containsString(got, needle) && !strings.Contains(joined, needle) {
			t.Fatalf("variants %v missing content needle %q", got, needle)
		}
	}
}

func TestLexicalSearchQueries_GoldQuestionsHaveCorpusNeedle(t *testing.T) {
	dataset := loadLexicalGold(t)
	for _, tc := range dataset.Cases {
		body, err := os.ReadFile(filepath.Join(lexicalGoldDir, tc.SourceDoc))
		if err != nil {
			t.Fatal(err)
		}
		content := string(body)
		variants := LexicalSearchQueries(tc.Question)
		if len(variants) < 2 {
			t.Errorf("%s query=%q produced no content needles", tc.ID, tc.Question)
			continue
		}
		if !needleHitsCorpus(variants[1:], content) {
			t.Errorf("%s query=%q variants=%v have no substring in %s",
				tc.ID, tc.Question, variants, tc.SourceDoc)
		}
	}
}

func TestLexicalSearchQueries_AbstainNeedlesMissCorpus(t *testing.T) {
	corpus := loadAllLexicalGoldDocs(t)
	abstain := []string{
		"不存在的词语xx",
		"kubernetes operator 灰度发布比例",
		"如何申请年假和出差报销",
		"办公区咖啡机维修电话",
		"this query is not in the operations corpus zzz",
	}
	for _, q := range abstain {
		for _, needle := range LexicalSearchQueries(q) {
			if needle == q {
				continue
			}
			if utf8.RuneCountInString(needle) < minCJKNeedleRunes && !hasASCII(needle) {
				continue
			}
			if strings.Contains(corpus, needle) {
				t.Errorf("abstain %q needle %q leaked into eval corpus", q, needle)
			}
		}
	}
}

func TestLexicalFillersLongestFirst(t *testing.T) {
	for i := 1; i < len(lexicalFillers); i++ {
		if utf8.RuneCountInString(lexicalFillers[i-1]) < utf8.RuneCountInString(lexicalFillers[i]) {
			t.Fatalf("lexicalFillers not longest-first: %q before %q", lexicalFillers[i-1], lexicalFillers[i])
		}
	}
}

type lexicalGoldFile struct {
	Cases []struct {
		ID        string `json:"id"`
		SourceDoc string `json:"source_doc"`
		Question  string `json:"question"`
	} `json:"cases"`
}

func loadLexicalGold(t *testing.T) lexicalGoldFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(lexicalGoldDir, "sample-knowledge-qa.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file lexicalGoldFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Cases) < 30 {
		t.Fatalf("gold cases = %d, want 30", len(file.Cases))
	}
	return file
}

func loadAllLexicalGoldDocs(t *testing.T) string {
	t.Helper()
	dataset := loadLexicalGold(t)
	var b strings.Builder
	seen := map[string]struct{}{}
	for _, tc := range dataset.Cases {
		if _, ok := seen[tc.SourceDoc]; ok {
			continue
		}
		seen[tc.SourceDoc] = struct{}{}
		body, err := os.ReadFile(filepath.Join(lexicalGoldDir, tc.SourceDoc))
		if err != nil {
			t.Fatal(err)
		}
		b.Write(body)
		b.WriteByte('\n')
	}
	return b.String()
}

func needleHitsCorpus(needles []string, content string) bool {
	for _, needle := range needles {
		if utf8.RuneCountInString(needle) < minCJKNeedleRunes && !hasASCII(needle) {
			continue
		}
		if strings.Contains(content, needle) {
			return true
		}
	}
	return false
}

func containsString(vals []string, want string) bool {
	for _, v := range vals {
		if v == want {
			return true
		}
	}
	return false
}

func hasASCII(s string) bool {
	for _, r := range s {
		if r < 128 && ((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return true
		}
	}
	return false
}
