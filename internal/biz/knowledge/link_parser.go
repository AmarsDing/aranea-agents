package knowledge

import "strings"

// ParseWikiLinks 提取 Obsidian 风格 [[...]] 引用（P2-4 explicit 轨）。
// 支持 [[target]]、[[target|alias]]（取目标）、[[target#anchor]]（剥离锚点）；
// 结果保序去重；空/未闭合引用跳过。返回的 ref 保留原始路径与大小写，
// 归一化（大小写/.md 后缀）在 ResolveLinkRefs 完成。
func ParseWikiLinks(body string) []string {
	var out []string
	seen := map[string]bool{}
	rest := body
	for {
		start := strings.Index(rest, "[[")
		if start < 0 {
			return out
		}
		rest = rest[start+2:]
		end := strings.Index(rest, "]]")
		if end < 0 {
			return out
		}
		raw := rest[:end]
		rest = rest[end+2:]
		// 未闭合嵌套（[[a [[b]]）：raw 内含 [[ 视为前者未闭合，取最内层内容为候选。
		if nested := strings.LastIndex(raw, "[["); nested >= 0 {
			raw = raw[nested+2:]
		}
		ref := raw
		if i := strings.Index(ref, "|"); i >= 0 {
			ref = ref[:i] // 别名取目标
		}
		if i := strings.Index(ref, "#"); i >= 0 {
			ref = ref[:i] // 锚点剥离
		}
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
}

// ResolveLinkRefs 把 [[ref]] 解析为目标文档 ID（ref → docID）。
// 规则（Obsidian 简化版）：
//  1. 去 .md 后按 rel_path 精确匹配（大小写不敏感）；
//  2. 否则按 basename 匹配（大小写不敏感）；多候选取字典序首个（确定性）；
//  3. 无匹配丢弃（悬空链不建，文档可能尚未同步）。
func ResolveLinkRefs(refs []string, candidates []Document) map[string]string {
	if len(refs) == 0 || len(candidates) == 0 {
		return nil
	}
	byPath := make(map[string]string, len(candidates))          // 归一化 rel_path → docID
	byBase := make(map[string][]baseCandidate, len(candidates)) // basename → 候选
	for _, d := range candidates {
		if d.RelPath == "" {
			continue
		}
		norm := normalizeLinkPath(d.RelPath)
		byPath[norm] = d.ID
		base := norm
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		byBase[base] = append(byBase[base], baseCandidate{id: d.ID, path: norm})
	}
	out := make(map[string]string, len(refs))
	for _, ref := range refs {
		norm := normalizeLinkPath(ref)
		if id, ok := byPath[norm]; ok {
			out[ref] = id
			continue
		}
		cands := byBase[norm] // basename 匹配要求 ref 本身无路径分隔
		if strings.Contains(norm, "/") || len(cands) == 0 {
			continue
		}
		best := cands[0]
		for _, c := range cands[1:] {
			if c.path < best.path {
				best = c
			}
		}
		out[ref] = best.id
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type baseCandidate struct {
	id   string
	path string
}

// normalizeLinkPath 归一化链接路径：正斜杠、去 .md 后缀、小写、去首尾空白。
func normalizeLinkPath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, `/`))
	p = strings.TrimPrefix(p, "/")
	if strings.HasSuffix(strings.ToLower(p), ".md") {
		p = p[:len(p)-len(".md")]
	}
	return strings.ToLower(p)
}
