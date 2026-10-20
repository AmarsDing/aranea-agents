/*
U2-3 解析纵深：HTML 正文抽取回归——噪声块剔除、密度打分选块、Markdown-ish 渲染。
*/
package htmltext

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parse(t *testing.T, raw string) *html.Node {
	t.Helper()
	n, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return n
}

// 带导航/页脚噪声的文章页：只留正文，h1/li 带 Markdown 前缀，噪声全剔除。
func TestExtractMainContent_ArticlePage(t *testing.T) {
	raw := `<html><head><title>站点标题</title><style>.a{}</style><script>var s=1;</script></head>
<body>
<header><nav><a href="/">首页</a><a href="/login">登录</a></nav></header>
<aside><a href="/t1">相关阅读一</a><a href="/t2">相关阅读二</a></aside>
<article>
<h1>向量检索实战指南</h1>
<p>向量检索把文档编码为稠密向量，依赖近似最近邻索引完成大规模召回。</p>
<p>工程落地要关注索引构建成本、召回率与延迟的三角权衡。</p>
<ul><li>HNSW 图索引</li><li>IVF 倒排分区</li></ul>
</article>
<footer>版权所有 2026 某团队</footer>
</body></html>`
	got := ExtractMainContent(parse(t, raw))
	for _, want := range []string{"# 向量检索实战指南", "向量检索把文档编码为稠密向量", "三角权衡", "- HNSW 图索引", "- IVF 倒排分区"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"站点标题", "首页", "登录", "相关阅读", "版权所有", "var s=1", ".a{}"} {
		if strings.Contains(got, bad) {
			t.Errorf("noise %q leaked into main content:\n%s", bad, got)
		}
	}
	// 标题必须早于正文（换行保序）。
	if strings.Index(got, "# 向量检索实战指南") > strings.Index(got, "向量检索把文档编码为稠密向量") {
		t.Errorf("heading/body out of order:\n%s", got)
	}
}

// 同页多候选块：链接密布块（导航残骸型）必须输给正文块。
func TestExtractMainContent_LinkHeavyBlockLoses(t *testing.T) {
	raw := `<html><body>
<div><a href="/a">AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA</a><a href="/b">BBBBBBBBBBBBBBBBBBBBBBBBBBBBBB</a></div>
<section><p>正文段落具备真实信息量，讲述一次生产事故的根因定位过程与后续整改。</p><p>复盘覆盖时间线、影响面与预防项。</p></section>
</body></html>`
	got := ExtractMainContent(parse(t, raw))
	if !strings.Contains(got, "正文段落") {
		t.Fatalf("prose block lost:\n%s", got)
	}
	if strings.Contains(got, "AAAAAA") || strings.Contains(got, "BBBBBB") {
		t.Fatalf("link-heavy block won:\n%s", got)
	}
}

// 无有效正文候选（nil / 纯噪声页）返回空串，交给上层回落。
func TestExtractMainContent_NoCandidate(t *testing.T) {
	if got := ExtractMainContent(nil); got != "" {
		t.Fatalf("nil node => %q, want \"\"", got)
	}
	if got := ExtractMainContent(parse(t, `<html><body><nav><a href="/">仅导航</a></nav></body></html>`)); got != "" {
		t.Fatalf("noise-only page => %q, want \"\"", got)
	}
}
