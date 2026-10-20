/*
U2-3 解析纵深回归：代码文件纯文本直读入库 / HTML 正文抽取与短页回落 /
.doc 拒绝给出友好转换指引。
*/
package knowledge

import (
	"context"
	"strings"
	"testing"
)

// 代码/源码扩展名按 txt 同一直读路径原样入库（不动 MIME 结构），
// 且 ExtractSupported 判定同步放开。
func TestTextExtractorExtractCodeFiles(t *testing.T) {
	ex := NewTextExtractor()
	ctx := context.Background()
	cases := []struct {
		source, content string
	}{
		{"main.go", "package main\n\nfunc main() { println(\"hi\") }\n"},
		{"tool.py", "def main():\n    print('hi')\n"},
		{"schema.sql", "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);\n"},
	}
	for _, tc := range cases {
		text, err := ex.Extract(ctx, []byte(tc.content), tc.source, "application/octet-stream")
		if err != nil {
			t.Fatalf("%s extract err: %v", tc.source, err)
		}
		if text != tc.content {
			t.Fatalf("%s extract = %q, want raw passthrough %q", tc.source, text, tc.content)
		}
		if !ExtractSupported(tc.source) {
			t.Errorf("ExtractSupported(%q) = false, want true", tc.source)
		}
	}
}

// 带导航/页脚/侧栏噪声的文章页：HTML 分支走正文抽取，噪声整块剔除。
func TestTextExtractorHTMLMainContent(t *testing.T) {
	ex := NewTextExtractor()
	raw := `<html><head><style>.n{}</style><script>var t=1;</script></head><body>` +
		`<header><nav><a href="/">站点首页</a><a href="/login">立即登录</a></nav></header>` +
		`<aside><a href="/x">推荐阅读甲乙丙丁</a></aside>` +
		`<article><h1>知识库解析纵深设计</h1>` +
		`<p>解析纵深把网页正文从模板噪声中分离，避免导航链接稀释向量召回质量，这是全文要点之一。</p>` +
		`<p>候选容器按文本量与链接密度打分，取最优块输出，全局兜底为全量可见文本回落。</p>` +
		`<ul><li>噪声标签整块剔除</li><li>正文容器密度打分</li></ul></article>` +
		`<footer>备案号 京ICP备XXXXXX号</footer></body></html>`
	text, err := ex.Extract(context.Background(), []byte(raw), "article.html", "text/html")
	if err != nil {
		t.Fatalf("extract err: %v", err)
	}
	for _, want := range []string{"知识库解析纵深设计", "解析纵深把网页正文从模板噪声中分离", "噪声标签整块剔除"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	for _, bad := range []string{"站点首页", "立即登录", "推荐阅读", "备案号", "var t=1"} {
		if strings.Contains(text, bad) {
			t.Errorf("noise %q leaked into main content:\n%s", bad, text)
		}
	}
}

// 短页（正文不足 64 字符）：回落既有 ExtractVisibleText 全量可见文本，
// 导航等词按旧行为保留（去噪不退化为丢内容）。
func TestTextExtractorHTMLShortPageFallsBack(t *testing.T) {
	ex := NewTextExtractor()
	raw := `<html><body><nav><a href="/">关于我们导航词</a></nav><main><p>很短</p></main></body></html>`
	text, err := ex.Extract(context.Background(), []byte(raw), "short.html", "text/html")
	if err != nil {
		t.Fatalf("extract err: %v", err)
	}
	if !strings.Contains(text, "关于我们导航词") {
		t.Fatalf("short page should fall back to full visible text (nav kept), got: %q", text)
	}
	if !strings.Contains(text, "很短") {
		t.Fatalf("short page body missing: %q", text)
	}
}

// legacy .doc 拒绝文案带可操作的转换指引。
func TestExtractLegacyDocFriendlyHint(t *testing.T) {
	ex := NewTextExtractor()
	_, err := ex.Extract(context.Background(), []byte("\xd0\xcf\x11\xe0 binary"), "old.doc", "application/msword")
	if err == nil {
		t.Fatal("legacy .doc must be rejected explicitly")
	}
	for _, want := range []string{"请转换为 .docx 后重试", "soffice --headless --convert-to docx"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}
