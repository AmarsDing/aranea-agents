package knowledge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const knowledgeRAGFixtureDir = "../../docs/testing/agent-eval-20260818/03-knowledge-rag"

type retrievalGoldFile struct {
	Cases []struct {
		ID        string `json:"id"`
		SourceDoc string `json:"source_doc"`
		Question  string `json:"question"`
	} `json:"cases"`
}

var retrievalRewritePrefixes = []string{
	"",
	"请问",
	"根据现有文档，",
	"运维规范中，",
	"帮我查一下：",
	"知识库检索：",
	"请引用原文回答：",
	"关于这个问题：",
	"请确认：",
	"检索：",
}

// identifierGoldQueries are grounded exact-term lookups taken from the five
// eval documents. They expand unique information needs without inventing facts.
var identifierGoldQueries = []RetrievalGoldCase{
	{ID: "id-ins-ticket", RelevantDocIDs: []string{"sample-doc-inspection.md"}, Abstain: false},
	{ID: "id-sw-core", RelevantDocIDs: []string{"sample-doc-inspection.md"}},
	{ID: "id-emg-prefix", RelevantDocIDs: []string{"sample-doc-change.md"}},
	{ID: "id-policy-test", RelevantDocIDs: []string{"sample-doc-change.md"}},
	{ID: "id-tacacs", RelevantDocIDs: []string{"sample-doc-security.md"}},
	{ID: "id-mgmt-net", RelevantDocIDs: []string{"sample-doc-security.md"}},
	{ID: "id-postmortem", RelevantDocIDs: []string{"sample-doc-emergency.md"}},
	{ID: "id-duty-log", RelevantDocIDs: []string{"sample-doc-duty.md"}},
	{ID: "id-cn2", RelevantDocIDs: []string{"sample-doc-emergency.md"}},
	{ID: "id-ups-runtime", RelevantDocIDs: []string{"sample-doc-emergency.md"}},
}

var identifierGoldTexts = map[string]string{
	"id-ins-ticket":  "INS-YYYYMMDD-NN",
	"id-sw-core":     "SW-Core-01",
	"id-emg-prefix":  "EMG-",
	"id-policy-test": "POLICY-TEST",
	"id-tacacs":      "TACACS+",
	"id-mgmt-net":    "10.99.0.0/24",
	"id-postmortem":  "TPL-POSTMORTEM-V3",
	"id-duty-log":    "DUTY-YYYYMMDD-NN",
	"id-cn2":         "电信 CN2 专线",
	"id-ups-runtime": "UPS 满载 28 分钟",
}

var abstainGoldQueries = []RetrievalGoldCase{
	{ID: "abs-01", Abstain: true},
	{ID: "abs-02", Abstain: true},
	{ID: "abs-03", Abstain: true},
	{ID: "abs-04", Abstain: true},
	{ID: "abs-05", Abstain: true},
}

var abstainGoldTexts = map[string]string{
	"abs-01": "不存在的词语xx",
	"abs-02": "kubernetes operator 灰度发布比例",
	"abs-03": "如何申请年假和出差报销",
	"abs-04": "办公区咖啡机维修电话",
	"abs-05": "this query is not in the operations corpus zzz",
}

// LoadRetrievalGoldV1 loads the 30-query RAG fixture and expands it into a
// rewrite-robustness set (≥300 unique queries) plus identifier and abstain slices.
func LoadRetrievalGoldV1() ([]RetrievalGoldCase, map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join(knowledgeRAGFixtureDir, "sample-knowledge-qa.json"))
	if err != nil {
		return nil, nil, err
	}
	var file retrievalGoldFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, nil, err
	}
	cases := make([]RetrievalGoldCase, 0, len(file.Cases)*len(retrievalRewritePrefixes)+20)
	texts := make(map[string]string, cap(cases))
	seen := make(map[string]struct{}, cap(cases))
	add := func(id, query, source string, abstain bool) {
		query = strings.TrimSpace(query)
		if query == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		gold := RetrievalGoldCase{ID: id, Abstain: abstain}
		if source != "" && !abstain {
			gold.RelevantDocIDs = []string{source}
		}
		cases = append(cases, gold)
		texts[id] = query
	}
	for _, tc := range file.Cases {
		for i, prefix := range retrievalRewritePrefixes {
			id := tc.ID
			if i > 0 {
				id = tc.ID + "-rw" + strconv.Itoa(i)
			}
			add(id, prefix+tc.Question, tc.SourceDoc, false)
		}
	}
	for _, gold := range identifierGoldQueries {
		add(gold.ID, identifierGoldTexts[gold.ID], firstString(gold.RelevantDocIDs), false)
	}
	for _, gold := range abstainGoldQueries {
		add(gold.ID, abstainGoldTexts[gold.ID], "", true)
	}
	return cases, texts, nil
}

func firstString(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}
