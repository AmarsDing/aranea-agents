package knowledge

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// 检索金标（US-40）：证明「编译期成链 + 一跳 GraphExpander」相对「仅种子/混合命中」
// 能召回邻文档事实。语料与查询内嵌，不依赖 Postgres。

type goldDoc struct {
	id, title, body string
}

type goldQuery struct {
	id       string
	query    string
	seedDoc  string
	needDoc  string
	needText string
}

func goldCorpus() []goldDoc {
	return []goldDoc{
		{id: "ops", title: "运维手册", body: "日常请遵守值班制度，并参考 Escalation Policy 处理夜间告警。"},
		{id: "duty", title: "值班制度", body: "夜班必须双人值守，禁止单人进入机房。"},
		{id: "esc", title: "Escalation Policy", body: "Page the secondary oncall after 15 minutes of silence."},
		{id: "net", title: "通信协议", body: "MQTT 心跳超时后应切换备用通道，详见 值班制度。"},
		{id: "play", title: "Oncall Playbook", body: "Follow the Escalation Policy before declaring an incident."},
		{id: "sec", title: "安全基线", body: "生产变更必须走灰度，约束见 通信协议。"},
	}
}

func goldQueries() []goldQuery {
	return []goldQuery{
		{id: "zh-duty-1", query: "夜班能不能一个人值守", seedDoc: "ops", needDoc: "duty", needText: "双人值守"},
		{id: "zh-duty-2", query: "机房单人禁入", seedDoc: "ops", needDoc: "duty", needText: "禁止单人"},
		{id: "zh-duty-3", query: "值班制度怎么写夜班", seedDoc: "net", needDoc: "duty", needText: "双人值守"},
		{id: "zh-proto-1", query: "MQTT 心跳超时怎么办", seedDoc: "sec", needDoc: "net", needText: "备用通道"},
		{id: "zh-proto-2", query: "生产变更灰度依据哪份协议", seedDoc: "sec", needDoc: "net", needText: "MQTT"},
		{id: "en-esc-1", query: "when to page the secondary", seedDoc: "ops", needDoc: "esc", needText: "15 minutes"},
		{id: "en-esc-2", query: "15 minutes of silence", seedDoc: "play", needDoc: "esc", needText: "secondary oncall"},
		{id: "en-esc-3", query: "declare incident after escalation", seedDoc: "play", needDoc: "esc", needText: "15 minutes"},
		{id: "zh-mix-1", query: "夜间告警按哪份 Escalation", seedDoc: "ops", needDoc: "esc", needText: "secondary"},
		{id: "zh-mix-2", query: "备用通道在通信协议哪一节", seedDoc: "sec", needDoc: "net", needText: "心跳"},
		{id: "en-mix-1", query: "oncall playbook escalation wait", seedDoc: "play", needDoc: "esc", needText: "Page the secondary"},
		{id: "zh-mix-3", query: "运维手册引用的值班规则", seedDoc: "ops", needDoc: "duty", needText: "机房"},
	}
}

func TestGoldRecall_AutolinkPlusOneHopBeatsHybridSeed(t *testing.T) {
	corpus := goldCorpus()
	titles := make([]string, 0, len(corpus))
	for _, d := range corpus {
		titles = append(titles, d.title)
	}

	linked := map[string]string{}
	links := map[string][]bizknowledge.Link{}
	chunks := map[string][]biz.KnowledgeChunk{}
	for _, d := range corpus {
		body, n := bizknowledge.AutolinkWikiMentions(d.body, d.title, titles)
		linked[d.id] = body
		for _, title := range parseWikiTitles(body) {
			tid := titleToID(corpus, title)
			if tid == "" || tid == d.id {
				continue
			}
			links[d.id] = append(links[d.id], bizknowledge.Link{
				DocID: d.id, TargetDocID: tid, LinkType: bizknowledge.LinkTypeExplicit, Weight: 1,
			})
		}
		chunks[d.id] = []biz.KnowledgeChunk{{
			ID: d.id + "-c0", DocID: d.id, CollectionID: "gold", Content: body, Score: 0.9,
		}}
		if n == 0 && (d.id == "ops" || d.id == "net" || d.id == "play" || d.id == "sec") {
			t.Fatalf("doc %s should autolink, body=%q", d.id, body)
		}
	}

	exp := NewGraphExpander(fakeLinkReader{byDoc: links}, fakeChunkLister{byDoc: chunks}, loggateway.NewNoop())
	missedHybrid, missedExpand := 0, 0
	for _, q := range goldQueries() {
		seed := chunks[q.seedDoc]
		if len(seed) == 0 {
			t.Fatalf("no seed chunks for %s", q.seedDoc)
		}
		hybridHit := chunkHas(seed, q.needText)
		expanded := exp.Expand(context.Background(), biz.KnowledgeSearchQuery{
			CollectionID: "gold", Query: q.query, TopK: 8,
		}, seed)
		expandHit := chunkHas(expanded, q.needText) || chunkHasDoc(expanded, q.needDoc)
		if !hybridHit {
			missedHybrid++
		}
		if !expandHit {
			missedExpand++
			t.Errorf("%s: expander missed %q (need doc %s) linked seed=%q", q.id, q.needText, q.needDoc, linked[q.seedDoc])
		}
		if hybridHit {
			t.Errorf("%s: gold query should require 1-hop (seed already contains %q)", q.id, q.needText)
		}
	}
	if missedExpand > 0 {
		t.Fatalf("expander misses=%d hybrid_misses=%d (want expander 0)", missedExpand, missedHybrid)
	}
	if missedHybrid != len(goldQueries()) {
		t.Fatalf("gold set contaminated: hybrid already hits %d/%d", len(goldQueries())-missedHybrid, len(goldQueries()))
	}
}

func parseWikiTitles(body string) []string {
	var out []string
	for {
		i := strings.Index(body, "[[")
		if i < 0 {
			break
		}
		j := strings.Index(body[i+2:], "]]")
		if j < 0 {
			break
		}
		out = append(out, body[i+2:i+2+j])
		body = body[i+2+j+2:]
	}
	return out
}

func titleToID(corpus []goldDoc, title string) string {
	for _, d := range corpus {
		if strings.EqualFold(d.title, title) {
			return d.id
		}
	}
	return ""
}

func chunkHas(chunks []biz.KnowledgeChunk, needle string) bool {
	n := strings.ToLower(needle)
	for _, c := range chunks {
		if strings.Contains(strings.ToLower(c.Content), n) {
			return true
		}
	}
	return false
}

func chunkHasDoc(chunks []biz.KnowledgeChunk, docID string) bool {
	for _, c := range chunks {
		if c.DocID == docID {
			return true
		}
	}
	return false
}
