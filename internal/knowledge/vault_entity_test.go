package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

func newEntityExtractorWithModel(llm biz.LLMCaller, uc *bizknowledge.Usecase) *VaultEntityExtractor {
	e := NewVaultEntityExtractor(llm,
		stubSysGetter{s: biz.SystemSetting{
			DefaultRefineLLM: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o-mini"},
		}},
		nil, uc, loggateway.NewNoop())
	e.retryAfter = 0 // 测试默认禁节流
	return e
}

func seedDoc(repo *vaultSyncMemRepo, collectionID, id, relPath, content string) {
	repo.documents[id] = bizknowledge.Document{
		ID: id, CollectionID: collectionID, RelPath: relPath,
		Status: "indexed", ContentText: content,
		ContentHash: bizknowledge.HashContent(content),
	}
}

func TestVaultEntityExtractAndLink(t *testing.T) {
	repo := newVaultSyncMemRepo()
	repo.collections["col1"] = bizknowledge.Collection{ID: "col1"}
	seedDoc(repo, "col1", "docA", "a.md", "动量策略历史回测。")
	seedDoc(repo, "col1", "docB", "b.md", "动量策略与双均线的组合研究。")
	// docA 已完成实体抽取（历史）
	repo.docEntities["docA"] = []bizknowledge.DocEntity{{Name: "动量策略", EntityType: "concept", Mentions: 2}}

	uc := bizknowledge.NewUsecaseFromRepo(repo)
	uc.SetLinkRepos(repo, repo)
	llm := &stubSummaryLLM{resp: `{"entities":[{"name":"动量策略","type":"concept","mentions":3},{"name":"双均线","type":"indicator","mentions":1}]}`}
	e := newEntityExtractorWithModel(llm, uc)

	applied, err := e.ExtractAndLink(context.Background(), "col1", "docB")
	require.NoError(t, err)
	assert.True(t, applied)
	assert.Equal(t, 1, llm.calls)

	// 实体落库
	ents := repo.docEntities["docB"]
	require.Len(t, ents, 2)
	assert.Equal(t, "动量策略", ents[0].Name)

	// 共现建链：docB → docA（共享「动量策略」，不含「双均线」）
	links, err := repo.ListLinks(context.Background(), "col1", "docB", bizknowledge.LinkTypeEntity)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, "docB", links[0].DocID)
	assert.Equal(t, "docA", links[0].TargetDocID)
	assert.Equal(t, bizknowledge.LinkTypeEntity, links[0].LinkType)
	assert.Contains(t, links[0].Context, "动量策略")
	assert.NotContains(t, links[0].Context, "双均线")
}

func TestVaultEntityStopwordsFiltered(t *testing.T) {
	repo := newVaultSyncMemRepo()
	repo.collections["col1"] = bizknowledge.Collection{ID: "col1"}
	seedDoc(repo, "col1", "docA", "a.md", "正文内容。")

	uc := bizknowledge.NewUsecaseFromRepo(repo)
	uc.SetLinkRepos(repo, repo)
	// 「文档」「the」为内置停用实体；「x」过短；仅「财报」保留
	llm := &stubSummaryLLM{resp: `{"entities":[{"name":"文档","type":"x","mentions":9},{"name":"the","type":"x","mentions":2},{"name":"x","type":"x","mentions":1},{"name":"财报","type":"topic","mentions":1}]}`}
	e := newEntityExtractorWithModel(llm, uc)

	applied, err := e.ExtractAndLink(context.Background(), "col1", "docA")
	require.NoError(t, err)
	assert.True(t, applied, "抽取调用成功即 applied（实体为空也是有效结果）")
	require.Len(t, repo.docEntities["docA"], 1)
	assert.Equal(t, "财报", repo.docEntities["docA"][0].Name)
}

func TestVaultEntityFrequencyFilter(t *testing.T) {
	repo := newVaultSyncMemRepo()
	repo.collections["col1"] = bizknowledge.Collection{ID: "col1"}
	seedDoc(repo, "col1", "docA", "a.md", "高频词文档 A。")
	seedDoc(repo, "col1", "docB", "b.md", "高频词文档 B。")
	seedDoc(repo, "col1", "docC", "c.md", "新文档。")
	// 「高频词」已出现在 2 个文档；maxDocFreq=2 → 达上限视为噪声
	repo.docEntities["docA"] = []bizknowledge.DocEntity{{Name: "高频词", Mentions: 1}}
	repo.docEntities["docB"] = []bizknowledge.DocEntity{{Name: "高频词", Mentions: 1}}

	uc := bizknowledge.NewUsecaseFromRepo(repo)
	uc.SetLinkRepos(repo, repo)
	llm := &stubSummaryLLM{resp: `{"entities":[{"name":"高频词","type":"topic","mentions":1}]}`}
	e := newEntityExtractorWithModel(llm, uc)
	e.maxDocFreq = 2

	applied, err := e.ExtractAndLink(context.Background(), "col1", "docC")
	require.NoError(t, err)
	assert.True(t, applied)
	links, err := repo.ListLinks(context.Background(), "col1", "docC", bizknowledge.LinkTypeEntity)
	require.NoError(t, err)
	assert.Empty(t, links, "超频次上限的实体不得建链（R-3）")
}

func TestVaultEntityNoLLMDegrades(t *testing.T) {
	repo := newVaultSyncMemRepo()
	repo.collections["col1"] = bizknowledge.Collection{ID: "col1"}
	seedDoc(repo, "col1", "docA", "a.md", "正文。")

	uc := bizknowledge.NewUsecaseFromRepo(repo)
	uc.SetLinkRepos(repo, repo)
	e := NewVaultEntityExtractor(nil, nil, nil, uc, loggateway.NewNoop())

	applied, err := e.ExtractAndLink(context.Background(), "col1", "docA")
	require.NoError(t, err)
	assert.False(t, applied, "无 LLM 降级不阻塞")
	assert.Empty(t, repo.docEntities["docA"])
}

func TestVaultEntityBadJSONDegrades(t *testing.T) {
	repo := newVaultSyncMemRepo()
	repo.collections["col1"] = bizknowledge.Collection{ID: "col1"}
	seedDoc(repo, "col1", "docA", "a.md", "正文。")

	uc := bizknowledge.NewUsecaseFromRepo(repo)
	uc.SetLinkRepos(repo, repo)
	llm := &stubSummaryLLM{resp: "这不是 JSON"}
	e := newEntityExtractorWithModel(llm, uc)

	applied, err := e.ExtractAndLink(context.Background(), "col1", "docA")
	require.NoError(t, err)
	assert.False(t, applied)

	// LLM 错误同样降级
	llm2 := &stubSummaryLLM{err: errors.New("timeout")}
	e2 := newEntityExtractorWithModel(llm2, uc)
	applied2, err2 := e2.ExtractAndLink(context.Background(), "col1", "docA")
	require.NoError(t, err2)
	assert.False(t, applied2)
}

func TestVaultEntityIdempotentByHash(t *testing.T) {
	repo := newVaultSyncMemRepo()
	repo.collections["col1"] = bizknowledge.Collection{ID: "col1"}
	seedDoc(repo, "col1", "docA", "a.md", "正文内容。")

	uc := bizknowledge.NewUsecaseFromRepo(repo)
	uc.SetLinkRepos(repo, repo)
	llm := &stubSummaryLLM{resp: `{"entities":[{"name":"财报","type":"topic","mentions":1}]}`}
	e := newEntityExtractorWithModel(llm, uc)

	applied, err := e.ExtractAndLink(context.Background(), "col1", "docA")
	require.NoError(t, err)
	assert.True(t, applied)

	// 同 content hash 再次调用短路（不重复 LLM）
	applied2, err := e.ExtractAndLink(context.Background(), "col1", "docA")
	require.NoError(t, err)
	assert.False(t, applied2)
	assert.Equal(t, 1, llm.calls)

	// 内容变更（hash 变）→ 重新抽取
	seedDoc(repo, "col1", "docA", "a.md", "正文内容已更新。")
	applied3, err := e.ExtractAndLink(context.Background(), "col1", "docA")
	require.NoError(t, err)
	assert.True(t, applied3)
	assert.Equal(t, 2, llm.calls)
}

func TestVaultEntityEmptyContentSkipped(t *testing.T) {
	repo := newVaultSyncMemRepo()
	repo.collections["col1"] = bizknowledge.Collection{ID: "col1"}
	seedDoc(repo, "col1", "docA", "a.md", "   ")

	uc := bizknowledge.NewUsecaseFromRepo(repo)
	uc.SetLinkRepos(repo, repo)
	llm := &stubSummaryLLM{resp: `{"entities":[]}`}
	e := newEntityExtractorWithModel(llm, uc)

	applied, err := e.ExtractAndLink(context.Background(), "col1", "docA")
	require.NoError(t, err)
	assert.False(t, applied)
	assert.Equal(t, 0, llm.calls, "空正文不调 LLM")
}
