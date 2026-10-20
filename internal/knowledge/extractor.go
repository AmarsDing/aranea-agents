// Package knowledge — 统一摄取管线：Extractor 模态路由抽象。
// 任何模态提取后归一为文本（Markdown 优先），下游 Organize/Chunk/Embed 无模态差异。
package knowledge

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// Extractor 将上传字节按模态提取为文本。
type Extractor interface {
	// Supports 判定是否可处理该来源（按扩展名/MIME 路由，ext 含点号小写）。
	Supports(ext, mimeType string) bool
	// Extract 提取文本；多模态实现（Phase 9 VisionExtractor）直接输出结构化 Markdown。
	Extract(ctx context.Context, raw []byte, source, mimeType string) (string, error)
}

// ExtractorRegistry 按注册顺序路由到首个 Supports 的实现。
type ExtractorRegistry struct {
	extractors []Extractor
}

// NewExtractorRegistry 构造注册表；nil 接收方安全（无提取器可用）。
func NewExtractorRegistry(extractors ...Extractor) *ExtractorRegistry {
	return &ExtractorRegistry{extractors: extractors}
}

// Supports 报告是否存在可处理该来源的提取器。
func (r *ExtractorRegistry) Supports(source, mimeType string) bool {
	if r == nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(source))
	for _, ex := range r.extractors {
		if ex != nil && ex.Supports(ext, mimeType) {
			return true
		}
	}
	return false
}

// Extract 路由到首个 Supports 的实现；无匹配时返回明确错误。
func (r *ExtractorRegistry) Extract(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("unsupported document type: %q (%s): no extractor registered", source, mimeType)
	}
	ext := strings.ToLower(filepath.Ext(source))
	for _, ex := range r.extractors {
		if ex != nil && ex.Supports(ext, mimeType) {
			return ex.Extract(ctx, raw, source, mimeType)
		}
	}
	return "", fmt.Errorf("unsupported document type: %q (%s)", source, mimeType)
}

// TextExtractor 覆盖文本直读（txt/md/json/csv/代码源码类/html/xml/yaml 等）与
// Office 类（pdf/doc/docx/xlsx/pptx，经 trpc document/reader）。
// 图片不属于本提取器（Phase 9 VisionExtractor 接管）。
type TextExtractor struct{}

// NewTextExtractor 构造文本提取器。
func NewTextExtractor() *TextExtractor { return &TextExtractor{} }

// Supports 判定扩展名是否在文本/Office 支持集合内。
func (TextExtractor) Supports(ext, _ string) bool {
	_, ok := supportedExtractExts[ext]
	return ok
}

// Extract 按扩展名分派提取；图片返回明确的多模态未上线错误。
func (TextExtractor) Extract(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	ext := strings.ToLower(filepath.Ext(source))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
		return "", fmt.Errorf("multimodal ingest not available yet: %q (%s) requires vision extractor", source, mimeType)
	case ".txt", ".md", ".markdown", ".text", ".log", ".json", ".csv", ".tsv", ".yaml", ".yml", ".xml",
		// U2-3 解析纵深：代码/源码类按 txt 同一纯文本直读路径入库（不动 MIME 结构）。
		".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".c", ".cc", ".cpp", ".h", ".hpp",
		".rs", ".cs", ".rb", ".php", ".swift", ".kt", ".sh", ".sql", ".proto", ".vue", ".css", ".scss":
		return string(raw), nil
	case ".html", ".htm":
		// U2-3：优先正文抽取，失败/过短回落全量可见文本（document_extract.go）。
		return extractHTMLMainText(string(raw)), nil
	case ".pdf", ".doc", ".docx", ".xlsx", ".pptx":
		return extractOfficeText(ctx, raw, source, mimeType)
	default:
		if mt := strings.ToLower(strings.TrimSpace(mimeType)); mt != "" {
			switch {
			case strings.HasPrefix(mt, "text/"):
				return string(raw), nil
			case mt == "application/xhtml+xml":
				return extractHTMLMainText(string(raw)), nil
			}
		}
		return "", fmt.Errorf("unsupported document type: %q (%s)", source, mimeType)
	}
}
