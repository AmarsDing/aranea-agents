package knowledge

import (
	"path/filepath"
	"strings"
)

// M0 摄取编译层：vault 文件夹同步的可摄取扩展名白名单。
// 设计要点（knowledge-self-governing-graph.implementation.md M0）：
//   - 本文件在 biz 层，只做扩展名判定，不依赖 infra 抽取器（保持分层：biz 不 import internal/knowledge）。
//   - 两类：文本直读（零成本）与需抽取（office/图片，由 applier 编译为 Markdown）。

// textDirectExts 文本直读集合：applier 直接读取字节作为正文（Markdown 处理）。
var textDirectExts = map[string]struct{}{
	".md": {}, ".markdown": {}, ".txt": {}, ".text": {}, ".log": {},
	".json": {}, ".csv": {}, ".tsv": {}, ".yaml": {}, ".yml": {},
	".xml": {}, ".html": {}, ".htm": {},
}

// extractExts 需抽取集合：office 文档与图片，applier 经抽取器编译为 Markdown。
var extractExts = map[string]struct{}{
	".pdf": {}, ".doc": {}, ".docx": {}, ".xlsx": {}, ".pptx": {},
	".png": {}, ".jpg": {}, ".jpeg": {}, ".webp": {},
}

// extOf 取文件名的规范化扩展名（含点号、小写）。无扩展名返回空串。
func extOf(name string) string {
	return strings.ToLower(filepath.Ext(name))
}

// IsIngestibleExt 报告文件名是否可被 vault 同步摄取（扫描白名单）。
func IsIngestibleExt(name string) bool {
	ext := extOf(name)
	if ext == "" {
		return false
	}
	if _, ok := textDirectExts[ext]; ok {
		return true
	}
	_, ok := extractExts[ext]
	return ok
}

// NeedsExtraction 报告该文件是否需经抽取器编译（true）还是文本直读（false）。
// 仅对 IsIngestibleExt 为 true 的名字有意义；不可摄取名字按直读处理（调用方先过白名单）。
func NeedsExtraction(name string) bool {
	_, ok := extractExts[extOf(name)]
	return ok
}
