package knowledge

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader"
)

// supportedExtractExts 是服务层/前端用于提示“可提取文本”的扩展名集合（小写、含点）。
// 图片不在此集合内（Phase 9 由 VisionExtractor 接管，见 extractor.go）。
var supportedExtractExts = map[string]struct{}{
	".txt": {}, ".md": {}, ".markdown": {}, ".text": {}, ".log": {},
	".json": {}, ".csv": {}, ".tsv": {}, ".yaml": {}, ".yml": {}, ".xml": {},
	".html": {}, ".htm": {},
	".pdf": {}, ".doc": {}, ".docx": {}, ".xlsx": {}, ".pptx": {},
}

// ExtractSupported 报告该来源扩展名是否可提取文本（用于前端提示）。
func ExtractSupported(source string) bool {
	_, ok := supportedExtractExts[strings.ToLower(filepath.Ext(source))]
	return ok
}

// ExtractDocumentText 提取文本内容（兼容保留的包级入口，内部委托 TextExtractor）。
// 新代码应使用 ExtractorRegistry 路由（见 extractor.go）。
func ExtractDocumentText(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	return TextExtractor{}.Extract(ctx, raw, source, mimeType)
}

// ExtractVisibleText 从 HTML 提取可见文本：剥离 script/style/noscript 与标签，
// 折叠空白。解析失败时退化为去除标签的纯文本（尽力而为，不误拒上传）。
func ExtractVisibleText(raw string) string {
	node, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return stripTagsFallback(raw)
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "template":
				return
			}
		}
		if n.Type == html.TextNode {
			if t := strings.TrimSpace(n.Data); t != "" {
				b.WriteString(t)
				b.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(b.String()), " ")
}

// stripTagsFallback 在 HTML 解析失败时退化处理：去除 <...> 标签与 script/style 块。
func stripTagsFallback(raw string) string {
	var b strings.Builder
	inTag := false
	for _, r := range raw {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// extractOfficeText 使用 trpc document/reader 解析 PDF/DOCX 等二进制文档。
func extractOfficeText(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	ext := strings.ToLower(filepath.Ext(source))
	r, ok := reader.GetReader(ext, reader.WithChunk(false))
	if !ok {
		return "", fmt.Errorf("no document reader for %q (%s)", source, mimeType)
	}
	name := filepath.Base(source)
	if name == "" || name == "." {
		name = "document"
	}
	docs, err := r.ReadFromReader(name, strings.NewReader(string(raw)))
	if err != nil {
		return "", fmt.Errorf("read document %q: %w", source, err)
	}
	var b strings.Builder
	for _, d := range docs {
		if d == nil {
			continue
		}
		b.WriteString(d.Content)
		b.WriteByte('\n')
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return "", fmt.Errorf("empty content extracted from %q", source)
	}
	_ = ctx // reader 接口暂无 ctx；保留参数便于后续切换
	return text, nil
}
