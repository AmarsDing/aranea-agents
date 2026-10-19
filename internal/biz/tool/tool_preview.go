package tool

import (
	"aranea-agents/internal/tools/preview"
)

const toolPreviewMaxLen = 2000

// RedactToolPreview masks sensitive patterns then truncates.
//
// A4：脱敏实现已单源化到 internal/tools/preview.RedactAndTruncate
// （运行记录链与 biz 落档链此前各维护一份 regex，已分叉且 preview 侧会误伤
// Unix 时间戳）。本函数仅保留 biz 语义入口（maxLen 默认 2000），全部模式
// 以 preview 包为准；新增/调整脱敏模式只改 preview。
func RedactToolPreview(raw string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = toolPreviewMaxLen
	}
	return preview.RedactAndTruncate(raw, maxLen)
}
