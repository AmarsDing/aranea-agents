package knowledge

import (
	"crypto/sha1" //nolint:gosec // 漂移对账键非加密用途，与 content_hash 先例一致用 sha1
	"encoding/hex"
	"sort"
	"strings"

	"aranea-agents/pkg/strutil"
)

// G-33 实体/谓词级向量召回（ADR-KN-20261020-G）：最小嵌入单元是「卡片文」——
// 词条文一档（实体名片，每实体一卡）、谓词文一档（active semantic 边陈述，
// 每边一卡），嵌入进 knowledge_graph_embeddings 独立表作为 RRF 第三腿来源。
// 本文件是卡片文的唯一构造点：投影工人写侧与检索读侧的（重嵌/审计）复用保证
// 文本与 source_hash 同源。

const (
	// GraphCardTextMaxRunes 卡片文 rune 上限（沿用 relationContextMaxRunes=240
	// 截断纪律；超出时按 strutil.TruncateRunesEllipsis 截断并追加省略号）。
	GraphCardTextMaxRunes = 240
	// GraphCardKindEntity 词条文（实体名片）。
	GraphCardKindEntity = "entity"
	// GraphCardKindEdge 谓词文（active semantic 边陈述）。
	GraphCardKindEdge = "edge"
)

// EntityCardText 构造词条文：`展示名`（+entity_type 非空时全角括号追加）
// + 别名行（alias_norm 升序去重，刻意不并入展示名本体——名与别名异名同锚，
// 嵌入侧需要的就是词形覆盖）。
func EntityCardText(name, entityType string, aliasNorms []string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	b.WriteString(name)
	if t := strings.TrimSpace(entityType); t != "" {
		b.WriteString("（")
		b.WriteString(t)
		b.WriteString("）")
	}
	aliases := normalizeCardAliases(aliasNorms)
	if len(aliases) > 0 {
		b.WriteString("\n别名：")
		b.WriteString(strings.Join(aliases, ", "))
	}
	return b.String()
}

func normalizeCardAliases(aliasNorms []string) []string {
	seen := make(map[string]struct{}, len(aliasNorms))
	out := make([]string, 0, len(aliasNorms))
	for _, a := range aliasNorms {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if _, dup := seen[a]; dup {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// EdgeCardText 构造谓词文：`主语展示名 <relation> 宾语展示名 — <evidence>`，
// 全长按 GraphCardTextMaxRunes 截断（主谓宾在前，优先保 SPO、截证据尾）。
// evidence 逐字过手：边 context 是抽取时快照，旧实体名不漂白、不解析
// （名称重写纪律只服 entity 轨 wikilink）。
func EdgeCardText(subjectName, relation, objectName, evidence string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(subjectName))
	b.WriteString(" ")
	b.WriteString(strings.TrimSpace(relation))
	b.WriteString(" ")
	b.WriteString(strings.TrimSpace(objectName))
	if ev := strings.TrimSpace(evidence); ev != "" {
		b.WriteString(" — ")
		b.WriteString(ev)
	}
	return strutil.TruncateRunesEllipsis(b.String(), GraphCardTextMaxRunes)
}

// GraphCardSourceHash 漂移对账键：卡片文 sha1（小写 hex）。投影工人每轮比对
// 存储的 source_hash，不一致即重嵌（更名/别名过户/归并 keeper 自愈同式）。
func GraphCardSourceHash(text string) string {
	sum := sha1.Sum([]byte(text)) //nolint:gosec // 见 import 注释
	return hex.EncodeToString(sum[:])
}
