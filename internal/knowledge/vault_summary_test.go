package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// stubSummaryLLM 可携带 onCall 回调（模拟 LLM 调用期间的外部编辑）。
type stubSummaryLLM struct {
	resp   string
	err    error
	calls  int
	onCall func()
}

func (s *stubSummaryLLM) Call(_ context.Context, _ biz.LLMCallRequest) (string, int, error) {
	s.calls++
	if s.onCall != nil {
		s.onCall()
	}
	if s.err != nil {
		return "", 0, s.err
	}
	return s.resp, 0, nil
}

func newSummaryGeneratorWithModel(llm biz.LLMCaller, filer *bizknowledge.VaultFiler) *VaultSummaryGenerator {
	g := NewVaultSummaryGenerator(llm,
		stubSysGetter{s: biz.SystemSetting{
			DefaultRefineLLM: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o-mini"},
		}},
		nil, filer, loggateway.NewNoop())
	g.retryAfter = 0 // 测试默认禁节流
	return g
}

const summaryTestBody = "# 季度财报\n\n2026 Q2 营收增长 20%，主要来自订阅业务。\n"

func writeRawVaultFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
}

func TestVaultSummaryGenerateAndApply(t *testing.T) {
	root := t.TempDir()
	filer := bizknowledge.NewVaultFiler(nil)
	require.NoError(t, filer.WriteDoc(root, "a.md", &bizknowledge.VaultDoc{
		Frontmatter: bizknowledge.DocFrontmatter{ID: "doc1"},
		Extra:       map[string]any{"custom": "保留"},
		Body:        summaryTestBody,
	}))

	llm := &stubSummaryLLM{resp: `{"summary":"2026 Q2 财报分析：营收增长 20%。","tags":["财报","季度"],"type":"report"}`}
	g := newSummaryGeneratorWithModel(llm, filer)

	applied, err := g.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err)
	assert.True(t, applied)
	assert.Equal(t, 1, llm.calls)

	doc, err := filer.ReadDoc(root, "a.md")
	require.NoError(t, err)
	assert.Equal(t, "2026 Q2 财报分析：营收增长 20%。", doc.Frontmatter.Summary)
	assert.Equal(t, []string{"财报", "季度"}, doc.Frontmatter.Tags)
	assert.Equal(t, "report", doc.Frontmatter.Type)
	assert.Equal(t, bizknowledge.HashContent(doc.Body), doc.Frontmatter.SummaryHash)
	// 用户字段与既有受管字段不受损
	assert.Equal(t, "保留", doc.Extra["custom"])
	assert.Equal(t, "doc1", doc.Frontmatter.ID)
	// 写回后不再 stale（收敛，不重复生成）
	assert.False(t, bizknowledge.SummaryStale(doc.Body, doc.Frontmatter.SummaryHash))
}

func TestVaultSummarySkipsWhenFresh(t *testing.T) {
	root := t.TempDir()
	filer := bizknowledge.NewVaultFiler(nil)
	require.NoError(t, filer.WriteDoc(root, "a.md", &bizknowledge.VaultDoc{
		Frontmatter: bizknowledge.DocFrontmatter{
			Summary:     "已有摘要",
			SummaryHash: bizknowledge.HashContent(summaryTestBody),
		},
		Body: summaryTestBody,
	}))

	llm := &stubSummaryLLM{resp: `{"summary":"x"}`}
	g := newSummaryGeneratorWithModel(llm, filer)

	applied, err := g.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err)
	assert.False(t, applied, "fresh 文档不应重生成")
	assert.Equal(t, 0, llm.calls, "fresh 文档不应调 LLM")
}

func TestVaultSummaryNoLLMDegrades(t *testing.T) {
	root := t.TempDir()
	filer := bizknowledge.NewVaultFiler(nil)
	require.NoError(t, filer.WriteDoc(root, "a.md", &bizknowledge.VaultDoc{Body: summaryTestBody}))

	llm := &stubSummaryLLM{resp: `{"summary":"x"}`}
	// sys/catalog 均 nil → ResolveLLM 失败 → 降级 nil error 不调 LLM
	g := NewVaultSummaryGenerator(llm, nil, nil, filer, loggateway.NewNoop())
	g.retryAfter = 0

	applied, err := g.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err, "无 LLM 必须降级而非报错（NFR-11）")
	assert.False(t, applied)
	assert.Equal(t, 0, llm.calls)

	doc, err := filer.ReadDoc(root, "a.md")
	require.NoError(t, err)
	assert.Empty(t, doc.Frontmatter.Summary)
}

func TestVaultSummaryNilGeneratorAndNilLLM(t *testing.T) {
	root := t.TempDir()
	filer := bizknowledge.NewVaultFiler(nil)
	require.NoError(t, filer.WriteDoc(root, "a.md", &bizknowledge.VaultDoc{Body: summaryTestBody}))

	var nilGen *VaultSummaryGenerator
	applied, err := nilGen.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err)
	assert.False(t, applied)

	g := NewVaultSummaryGenerator(nil, nil, nil, filer, loggateway.NewNoop())
	applied, err = g.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err)
	assert.False(t, applied)
}

func TestVaultSummaryInvalidJSONDegrades(t *testing.T) {
	root := t.TempDir()
	filer := bizknowledge.NewVaultFiler(nil)
	require.NoError(t, filer.WriteDoc(root, "a.md", &bizknowledge.VaultDoc{Body: summaryTestBody}))

	llm := &stubSummaryLLM{resp: "这不是 JSON 输出"}
	g := newSummaryGeneratorWithModel(llm, filer)

	applied, err := g.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err, "LLM 输出不可解析必须降级而非报错")
	assert.False(t, applied)

	doc, err := filer.ReadDoc(root, "a.md")
	require.NoError(t, err)
	assert.Empty(t, doc.Frontmatter.Summary)
}

func TestVaultSummaryEmptyBodySkips(t *testing.T) {
	root := t.TempDir()
	filer := bizknowledge.NewVaultFiler(nil)
	require.NoError(t, filer.WriteDoc(root, "a.md", &bizknowledge.VaultDoc{Body: "  \n  "}))

	llm := &stubSummaryLLM{resp: `{"summary":"x"}`}
	g := newSummaryGeneratorWithModel(llm, filer)

	applied, err := g.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err)
	assert.False(t, applied, "空正文不生成摘要")
	assert.Equal(t, 0, llm.calls)
}

func TestVaultSummaryMergesOntoConcurrentEdit(t *testing.T) {
	root := t.TempDir()
	filer := bizknowledge.NewVaultFiler(nil)
	require.NoError(t, filer.WriteDoc(root, "a.md", &bizknowledge.VaultDoc{
		Frontmatter: bizknowledge.DocFrontmatter{ID: "doc1"},
		Body:        summaryTestBody,
	}))

	userEdited := "---\nid: doc1\n---\n\n# 季度财报\n\n用户改写：Q2 营收实际增长 25%。\n"
	llm := &stubSummaryLLM{
		resp: `{"summary":"基于旧正文的摘要。","tags":["财报"],"type":"report"}`,
		// LLM 调用期间用户外部编辑正文
		onCall: func() { writeRawVaultFile(t, root, "a.md", userEdited) },
	}
	g := newSummaryGeneratorWithModel(llm, filer)

	applied, err := g.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err)
	assert.True(t, applied)

	doc, err := filer.ReadDoc(root, "a.md")
	require.NoError(t, err)
	// 用户正文不被回滚（合并语义：只覆盖受管摘要字段）
	assert.Contains(t, doc.Body, "用户改写")
	assert.Equal(t, "基于旧正文的摘要。", doc.Frontmatter.Summary)
	// SummaryHash 记录的是被摘要的旧正文 → 与当前正文不匹配 → stale 自愈（下轮重生成）
	assert.Equal(t, bizknowledge.HashContent(summaryTestBody), doc.Frontmatter.SummaryHash)
	assert.True(t, bizknowledge.SummaryStale(doc.Body, doc.Frontmatter.SummaryHash))
}

func TestVaultSummaryThrottleAfterFailure(t *testing.T) {
	root := t.TempDir()
	filer := bizknowledge.NewVaultFiler(nil)
	require.NoError(t, filer.WriteDoc(root, "a.md", &bizknowledge.VaultDoc{Body: summaryTestBody}))

	llm := &stubSummaryLLM{err: errors.New("boom")}
	g := newSummaryGeneratorWithModel(llm, filer)
	g.retryAfter = time.Hour

	applied, err := g.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err)
	assert.False(t, applied)
	assert.Equal(t, 1, llm.calls)

	// 同内容在节流窗口内不再调 LLM（防无 LLM/失败时高频重试）
	applied, err = g.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err)
	assert.False(t, applied)
	assert.Equal(t, 1, llm.calls, "节流窗口内不应重复调用")

	// 正文变更后视为新尝试（key 含 body hash）
	require.NoError(t, filer.WriteDoc(root, "a.md", &bizknowledge.VaultDoc{Body: summaryTestBody + "\n新增段落。"}))
	_, err = g.GenerateAndApply(context.Background(), root, "a.md")
	require.NoError(t, err)
	assert.Equal(t, 2, llm.calls, "内容变化后应立即重试")
}
