package knowledge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// M0：IsIngestibleExt 判定可摄取扩展名（扫描白名单）。
// 两类：文本直读（零成本）+ 需抽取（office/图片，applier 编译）。
func TestIsIngestibleExt(t *testing.T) {
	ingestible := []string{
		// 文本直读
		"a.md", "b.markdown", "c.txt", "d.text", "e.log",
		"f.json", "g.csv", "h.tsv", "i.yaml", "j.yml", "k.xml", "l.html", "m.htm",
		// 需抽取：Office
		"n.pdf", "o.doc", "p.docx", "q.xlsx", "r.pptx",
		// 需抽取：图片（视觉 LLM）
		"s.png", "t.jpg", "u.jpeg", "v.webp",
	}
	for _, name := range ingestible {
		assert.True(t, IsIngestibleExt(name), "%s 应可摄取", name)
	}

	notIngestible := []string{
		".hidden", "archive.zip", "binary.exe", "lib.dll", "data.db",
		"script.go", "style.css", "noext", "image.gif", "video.mp4",
	}
	for _, name := range notIngestible {
		assert.False(t, IsIngestibleExt(name), "%s 不应可摄取", name)
	}

	// 大小写不敏感
	assert.True(t, IsIngestibleExt("REPORT.PDF"), "大写扩展名应可摄取")
	assert.True(t, IsIngestibleExt("Note.MD"), "大小写混合应可摄取")
}

// M0：NeedsExtraction 区分「文本直读」与「需抽取编译」。
func TestNeedsExtraction(t *testing.T) {
	direct := []string{"a.md", "b.txt", "c.json", "d.html"}
	for _, name := range direct {
		assert.False(t, NeedsExtraction(name), "%s 应文本直读", name)
	}
	needExtract := []string{"a.pdf", "b.docx", "c.png", "d.jpg"}
	for _, name := range needExtract {
		assert.True(t, NeedsExtraction(name), "%s 应需抽取", name)
	}
}
