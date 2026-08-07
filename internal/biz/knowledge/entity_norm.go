package knowledge

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// entityFolder Unicode case-fold（ß→ss、希腊大写 sigma 等对等价写法折叠），
// 保证 "Straße"/"STRASSE"/"strasse" 聚合为同一实体。
var entityFolder = cases.Fold()

// NormalizeEntityName 实体名归一化（G5-F B9）：NFKC（canonical 组合 +
// 全/半角等兼容写法折叠，覆盖验收 "AI"/"ai"/"ＡＩ" 聚合）→ case-fold →
// 内部连续空白折叠为单个 ASCII 空格并去首尾（NFKC 已把 NBSP 折叠为普通空格）。
//
// 设计文档原文为 NFC；验收要求全角等价（ＡＩ≡AI），NFKC 为 NFC 超集变换
// （兼容分解 + 规范组合），同时满足两者，见 37-knowledge.design.md V12.8-3。
//
// 返回空串表示该名无治理价值（调用方跳过）。
func NormalizeEntityName(name string) string {
	s := norm.NFKC.String(strings.TrimSpace(name))
	s = entityFolder.String(s)
	return strings.Join(strings.Fields(s), " ")
}
