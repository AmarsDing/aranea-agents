package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

// US-47：生产词法路径（tsvector simple + pg_trgm word_similarity）的中英金标。
// 依赖 aranea_test Postgres（与 knowledge_search_path_test 相同）。不是 50 条、
// 不走 GraphExpander、不声称混合检索；只锁住「正文里有的词能被 SearchChunksBM25 找回」。

type goldBM25Case struct {
	query  string
	wantID string
}

func seedGoldBM25Corpus(t *testing.T, repo *knowledgeRepo) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "gold-bm25", Name: "gold"}); err != nil {
		t.Fatal(err)
	}
	docs := []struct {
		id, rel, body string
	}{
		{"duty", "duty.md", "夜班必须双人值守，禁止单人进入机房。"},
		{"esc", "esc.md", "Page the secondary oncall after 15 minutes of silence."},
		{"net", "net.md", "MQTT 心跳超时后应切换备用通道。"},
		{"play", "play.md", "Follow the Escalation Policy before declaring an incident."},
		{"sec", "sec.md", "生产变更必须走灰度，保留回滚开关。"},
		{"ops", "ops.md", "日常请遵守值班制度，并参考 oncall 手册。"},
	}
	for i, d := range docs {
		if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
			ID: d.id, CollectionID: "gold-bm25", RelPath: d.rel, Source: d.rel, Status: "indexed",
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
			ID: "g" + d.id, DocID: d.id, CollectionID: "gold-bm25", Content: d.body, ChunkIndex: i,
		}}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestKnowledgeRepo_SearchChunksBM25_GoldBilingual(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	seedGoldBM25Corpus(t, repo)
	ctx := context.Background()

	cases := []goldBM25Case{
		{"双人值守", "duty"},
		{"单人进入机房", "duty"},
		{"secondary oncall", "esc"},
		{"15 minutes of silence", "esc"},
		{"MQTT", "net"},
		{"备用通道", "net"},
		{"declaring an incident", "play"},
		{"Escalation Policy", "play"},
		{"灰度", "sec"},
		{"必须走灰度", "sec"},
		{"回滚开关", "sec"},
		{"值班制度", "ops"},
		{"oncall 手册", "ops"},
	}
	for _, tc := range cases {
		got, err := repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{
			CollectionID: "gold-bm25", Query: tc.query, TopK: 5,
		})
		if err != nil {
			t.Fatalf("query %q: %v", tc.query, err)
		}
		if !chunkHasDoc(got, tc.wantID) {
			ids := make([]string, 0, len(got))
			for _, c := range got {
				ids = append(ids, c.DocID)
			}
			t.Errorf("query %q: docs=%v want %s", tc.query, ids, tc.wantID)
		}
	}

	miss, err := repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{
		CollectionID: "gold-bm25", Query: "不存在的词语xx", TopK: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(miss) != 0 {
		t.Errorf("negative query hit %d chunks", len(miss))
	}
}

// P1-3（2026-08-21）：完整问句形态金标——评测 40% 短板的真实场景。
// 查询不是正文子串，而是带疑问包装的完整句子；词法路径必须靠问句针
// 抽取（LexicalSearchQueries）命中共享内容词。
func TestKnowledgeRepo_SearchChunksBM25_GoldQuestions(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	seedGoldBM25Corpus(t, repo)
	ctx := context.Background()

	cases := []goldBM25Case{
		{"夜班需要几个人同时值守？", "duty"},
		{"机房里可以只留一个人吗？", "duty"},
		{"主 oncall 没有响应超过 15 分钟该怎么办？", "esc"},
		{"MQTT 心跳超时之后要做什么？", "net"},
		{"什么时候需要切换到备用通道？", "net"},
		{"宣布 incident 之前要走什么流程？", "play"},
		{"生产环境发布变更前必须做什么？", "sec"},
		{"回滚开关需要保留吗？", "sec"},
		{"值班制度一般是怎么规定的？", "ops"},
	}
	for _, tc := range cases {
		got, err := repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{
			CollectionID: "gold-bm25", Query: tc.query, TopK: 5,
		})
		if err != nil {
			t.Fatalf("query %q: %v", tc.query, err)
		}
		if !chunkHasDoc(got, tc.wantID) {
			ids := make([]string, 0, len(got))
			for _, c := range got {
				ids = append(ids, c.DocID)
			}
			t.Errorf("query %q: docs=%v want %s", tc.query, ids, tc.wantID)
		}
	}
}

func chunkHasDoc(chunks []biz.KnowledgeChunk, docID string) bool {
	for _, c := range chunks {
		if c.DocID == docID {
			return true
		}
	}
	return false
}
