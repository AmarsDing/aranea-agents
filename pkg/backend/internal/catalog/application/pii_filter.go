// PIIFilter：轻量正则 PII 检测与脱敏。供 Catalog 进化与 Memory L3 等共享。
//（长期归属见迁移表 P4 memory/application；本包实现供 P3 自进化与 L3 复用。）
package application

import (
	"regexp"
	"strings"
)

// PIIFilter 检测并脱敏自由文本中的个人可识别信息。零值即可使用。
type PIIFilter struct{}

// NewPIIFilter 返回带内置正则集合的默认 PII 过滤器。
func NewPIIFilter() *PIIFilter { return &PIIFilter{} }

// piiPatterns 按顺序与输入匹配。每则模式的匹配替换为固定掩码标记，下游
// 可据此识别脱敏而无需重解析。
var piiPatterns = []struct {
	name    string
	mask    string
	pattern *regexp.Regexp
}{
	{
		name:    "email",
		mask:    "[REDACTED_EMAIL]",
		pattern: regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`),
	},
	{
		name: "phone",
		mask: "[REDACTED_PHONE]",
		// 宽松国际电话：至少 7 位数字，可选分隔符/前置 +。前导边界避免
		// 吞掉无关标识符尾部的数字。
		pattern: regexp.MustCompile(`(?:\+?\d[\d\s\-]{6,}\d)`),
	},
	{
		name:    "credit_card",
		mask:    "[REDACTED_CARD]",
		pattern: regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`),
	},
	{
		name: "id_number",
		mask: "[REDACTED_ID]",
		// 长数字/字母数字串，形似国民身份证/SSN 等，且非先前几类（最后执行，
		// 故其它掩码已先处理）。
		pattern: regexp.MustCompile(`\b[A-Z0-9]{8,}\b`),
	},
}

// RedactPII 扫描输入，返回 (是否命中, 脱敏后文本)。未检测到 PII 时返回原文且 hit 为 false。
// 检测为尽力而为：目标是把明显泄露挡在共享域外，不保证零泄露。
func (f *PIIFilter) RedactPII(text string) (bool, string) {
	if strings.TrimSpace(text) == "" {
		return false, text
	}
	out := text
	hit := false
	for _, p := range piiPatterns {
		if p.pattern.MatchString(out) {
			out = p.pattern.ReplaceAllString(out, p.mask)
			hit = true
		}
	}
	return hit, out
}

// HasPII 为仅需布尔标记的轻量版（如 ACL 门槛）。
func (f *PIIFilter) HasPII(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, p := range piiPatterns {
		if p.pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// Hits 返回与输入匹配的所有 PII 模式名。供调用方（如 AgentEvolutionService.UpdateIdentity）展示
// *哪类* PII 被检出，而非仅布尔。无匹配时返回 nil。
func (f *PIIFilter) Hits(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var hits []string
	for _, p := range piiPatterns {
		if p.pattern.MatchString(text) {
			hits = append(hits, p.name)
		}
	}
	return hits
}
