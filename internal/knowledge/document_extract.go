package knowledge

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader"

	"aranea-agents/internal/knowledge/htmltext"
	"aranea-agents/internal/knowledge/parser"
)

// supportedExtractExts 是服务层/前端用于提示“可提取文本”的扩展名集合（小写、含点）。
// 图片不在此集合内（Phase 9 由 VisionExtractor 接管，见 extractor.go）。
var supportedExtractExts = map[string]struct{}{
	".txt": {}, ".md": {}, ".markdown": {}, ".text": {}, ".log": {},
	".json": {}, ".csv": {}, ".tsv": {}, ".yaml": {}, ".yml": {}, ".xml": {},
	".html": {}, ".htm": {},
	".pdf": {}, ".doc": {}, ".docx": {}, ".xlsx": {}, ".pptx": {},
	// 代码/源码类（U2-3 解析纵深）：与 txt 同一纯文本直读路径，不动 MIME 结构。
	".go": {}, ".py": {}, ".js": {}, ".jsx": {}, ".ts": {}, ".tsx": {},
	".java": {}, ".c": {}, ".cc": {}, ".cpp": {}, ".h": {}, ".hpp": {},
	".rs": {}, ".cs": {}, ".rb": {}, ".php": {}, ".swift": {}, ".kt": {},
	".sh": {}, ".sql": {}, ".proto": {}, ".vue": {}, ".css": {}, ".scss": {},
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

// htmlMainMinRunes 正文抽取产物低于该字符数视为抽取不充分（短文档/噪声
// 剥离后所剩无几），回落全量可见文本，保证行为不劣于改造前。
const htmlMainMinRunes = 64

// extractHTMLMainText U2-3 HTML 正文提取：优先 htmltext.ExtractMainContent
// 剥离导航/页脚等噪声块、按密度打分取最优正文容器；HTML 解析失败或正文
// 不足 htmlMainMinRunes 字符时回落既有 ExtractVisibleText 全量可见文本。
func extractHTMLMainText(raw string) string {
	if node, err := html.Parse(strings.NewReader(raw)); err == nil {
		if text := htmltext.ExtractMainContent(node); utf8.RuneCountInString(strings.TrimSpace(text)) >= htmlMainMinRunes {
			return text
		}
	}
	return ExtractVisibleText(raw)
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

// extractOfficeText 分派二进制文档抽取：xlsx/pptx 走自研 OOXML 抽取器
// （ooxml_extract.go；trpc reader 无这两种格式）；pdf 优先走自研版面感知
// 解析层（parser.PDFToMarkdown，G-05 起步），解析失败或零文本时回退 trpc
// reader 纯文本链路；docx 走 trpc reader；legacy 二进制 .doc 明确拒绝并给
// 出转换指引。
func extractOfficeText(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	ext := strings.ToLower(filepath.Ext(source))
	switch ext {
	case ".xlsx":
		return extractXLSXText(raw, source)
	case ".pptx":
		return extractPPTXText(raw, source)
	case ".pdf":
		return extractPDFText(ctx, raw, source, mimeType)
	case ".doc":
		return "", fmt.Errorf("legacy .doc (binary OLE2) is not supported for %q: 请转换为 .docx 后重试（LibreOffice: soffice --headless --convert-to docx）", source)
	}
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

// extractPDFText G-05 起步：版面感知解析层默认开启（新增结构信息、不丢旧
// 文本）；解析错误或纯图片 PDF 零文本时静默回退旧 trpc 纯文本链路（行为不
// 劣于改造前）。<!-- page:N --> 锚点注释见 parser 包注释。
func extractPDFText(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	if text, err := parser.PDFToMarkdown(raw); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}
	r, ok := reader.GetReader(".pdf", reader.WithChunk(false))
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
	_ = ctx
	return text, nil
}
