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
