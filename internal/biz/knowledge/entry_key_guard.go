package knowledge

import (
	"regexp"
	"strings"
)

// 词条键守卫（2026-08-18 域 D 污染事故根治）：LLM 抽取的 tags/实体名常把写回块
// provenance 字段名、kind 词表值、词条模板导语样板词误抽为话题键，落库即成
// 垃圾词条（entries/session-id.md 等）与伪实体边，进而污染 autolink target 与
// hub 度数（moc_emerge 提案风暴）。统一在此收口，三个消费方共用：
//   - normalizedEntryTags（写回词条定位，tags 过滤）
//   - llmExtractEntities（M2 实体抽取过滤）
//   - autolink target 构造（防御性，垃圾词条清理前兜底）

// reservedEntryKeys 保留键：写回结构化字段名 + kind 词表 + 词条模板样板词。
var reservedEntryKeys = map[string]struct{}{
	// provenance 字段名（FormatWriteBackAppendix / FormatPendingAppendix 结构化行）
	"fact_id": {}, "session_id": {}, "agent_id": {}, "user_id": {},
	"confidence": {}, "kind": {}, "tags": {}, "source": {}, "source_id": {},
	"entry": {}, "statement": {},
	// 写回 kind 词表（provenance 值泄漏高频，作为话题键无意义）
	"preference": {}, "profile": {}, "goal": {}, "constraint": {},
	"decision": {}, "relationship": {},
	// 词条页模板导语样板词（writeBackEntryHeader/导语泄漏）
	"会话沉淀": {}, "词条页": {}, "同主题新事实": {},
}

var uuidLikeEntryKeyRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// versionLikeEntryKeyRE 版本号形态：v1.2.3 / 1.2.3（纯数字版 isAutolinkNoiseKey 已覆盖，
// 这里补带 v 前缀类）。SW-Eval-01 / amc-2026.08 等含多字母词的不误伤。
var versionLikeEntryKeyRE = regexp.MustCompile(`^[a-zA-Z]?\d+(\.\d+)+$`)

// longHexEntryKeyRE 长 hex ID 形态：session_id / trace_id 类（去连字符后 ≥16 位 hex，
// 含 a-f 字母——纯数字长串已被 isAutolinkNoiseKey 覆盖）。
var longHexEntryKeyRE = regexp.MustCompile(`^[0-9a-f]{16,}$`)

// IsReservedEntryKey 判定保留键（连字符/空白归一为下划线后比对，大小写不敏感）。
func IsReservedEntryKey(k string) bool {
	norm := strings.ToLower(strings.TrimSpace(k))
	norm = strings.NewReplacer("-", "_", " ", "_").Replace(norm)
	_, ok := reservedEntryKeys[norm]
	return ok
}

// IsNoiseEntryKey 判定噪声键：空、纯数字/符号（无字母，IP/版本号类）、UUID/长 hex ID
// 形态、版本号形态、路径/wikilink 形态（含 / \ [[ ]]）。
func IsNoiseEntryKey(k string) bool {
	k = strings.TrimSpace(k)
	if k == "" {
		return true
	}
	lower := strings.ToLower(k)
	if uuidLikeEntryKeyRE.MatchString(lower) || versionLikeEntryKeyRE.MatchString(lower) {
		return true
	}
	if longHexEntryKeyRE.MatchString(strings.NewReplacer("-", "", "_", "").Replace(lower)) {
		return true
	}
	if strings.ContainsAny(k, "/\\") || strings.Contains(k, "[[") || strings.Contains(k, "]]") {
		return true
	}
	return isAutolinkNoiseKey(k)
}
