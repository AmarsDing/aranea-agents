package knowledge

import (
	"context"
	"strings"
)

// extractorCompiler 把 ExtractorRegistry 适配为 VaultSyncApplier 的 BodyCompiler 端口（M0）。
// 职责：按 relPath 推 MIME → 路由到对应 Extractor → 返回正文并保留原件 MIME。
type extractorCompiler struct {
	reg *ExtractorRegistry
}

// NewBodyCompiler 用 ExtractorRegistry 构造 vault 同步的编译端口。
// reg 为 nil 时返回 nil（VaultSyncApplier 将走「无编译端口降级 error」路径）。
func NewBodyCompiler(reg *ExtractorRegistry) BodyCompiler {
	if reg == nil {
		return nil
	}
	return &extractorCompiler{reg: reg}
}

// Compile 实现 BodyCompiler：抽取原始字节为 Markdown。
func (c *extractorCompiler) Compile(ctx context.Context, relPath string, raw []byte) (string, string, error) {
	mime := mimeTypeFor(relPath)
	md, err := c.reg.Extract(ctx, raw, relPath, mime)
	if err != nil {
		return "", "", err
	}
	md = strings.TrimSpace(md)
	// The body is Markdown, but the document still points at the original file.
	// Preserve its MIME so previews and editors never treat binary originals as notes.
	return md, mime, nil
}
