package data

// 复现 mem-sh-04 召回排序：用真实 DB dump（agent+user scope 全量事实、含 embedding）
// + 真实 Ollama bge-m3 查询向量，离线重放 scoreFactRow 打分，定位为何
// 「FW-Edge-02 管理 IP」事实未进入 top-5 注入。
// 数据文件（评测证据目录）：
//   sh04-facts-full.jsonl  — psql json_build_object 导出的候推行
//   sh04-query-embed.json  — Ollama /api/embed 返回的查询向量
// 运行：go test ./internal/data/ -run TestSh04RecallRepro -v

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"
)

const (
	sh04FactsFile = `..\..\docs\testing\agent-eval-20260818\02-memory-recall\evidence\sh04-facts-full.jsonl`
	sh04EmbedFile = `..\..\docs\testing\agent-eval-20260818\02-memory-recall\evidence\sh04-query-embed.json`
	sh04Query     = "FW-Edge-02 的管理 IP 是多少？"
)

type sh04Scored struct {
	id    string
	scope string
	stmt  string
	bd    recallScoreBreakdown
}

func TestSh04RecallRepro(t *testing.T) {
	embRaw, err := os.ReadFile(sh04EmbedFile)
	if err != nil {
		t.Skipf("query embedding dump missing: %v", err)
	}
	var embResp struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(embRaw, &embResp); err != nil || len(embResp.Embeddings) == 0 {
		t.Fatalf("parse query embedding: %v", err)
	}
	qvec := embResp.Embeddings[0]
	tokens := tokenizeQuery(sh04Query)
	t.Logf("query tokens (%d): %v", len(tokens), tokens)

	f, err := os.Open(sh04FactsFile)
	if err != nil {
		t.Skipf("facts dump missing: %v", err)
	}
	defer f.Close()

	now := time.Now().UTC()
	var scored []sh04Scored
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatalf("bad row: %v", err)
		}
		bd := scoreFactRow(row, tokens, qvec, nil, now)
		id, _ := row["id"].(string)
		scope, _ := row["scope_type"].(string)
		stmt, _ := row["statement"].(string)
		scored = append(scored, sh04Scored{id: id[:8], scope: scope, stmt: stmt, bd: bd})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].bd.Total > scored[j].bd.Total })

	t.Logf("total candidates: %d", len(scored))
	for i, s := range scored {
		if i >= 15 {
			break
		}
		t.Logf("#%02d total=%.4f kw=%.3f vec=%.3f imp=%.3f rec=%.3f q=%.2f [%s/%s] %s",
			i+1, s.bd.Total, s.bd.Keyword, s.bd.Vector, s.bd.Importance, s.bd.Recency, s.bd.QualityScore,
			s.scope, s.id, s.stmt)
	}
	// 定位两条关键事实的排名
	for i, s := range scored {
		if s.id == "b02bffb9" || s.id == "00739f43" || s.id == "3d26b3a2" || s.id == "c9e8a50a" || s.id == "deb4d608" {
			t.Logf("TRACK rank=%d total=%.4f kw=%.3f vec=%.3f imp=%.3f rec=%.3f [%s/%s] %s",
				i+1, s.bd.Total, s.bd.Keyword, s.bd.Vector, s.bd.Importance, s.bd.Recency, s.scope, s.id, s.stmt)
		}
	}
}
