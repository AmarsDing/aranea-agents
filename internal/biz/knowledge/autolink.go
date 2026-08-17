package knowledge

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	"aranea-agents/pkg/loggateway"
)

// ── 编译期出链成链（Obsidian unlinked mentions → [[wikilink]]）────────────────
//
// 论文/产品对照：Karpathy llm-wiki / Notemd 在写时把提及编译成互链；
// Obsidian 核心只展示未链接提及，社区插件（Link Unlinked Mentions）按词边界
// 把纯文本包成 [[title]]。本实现走写路径自动编译，查询期 Lazy GraphRAG
// 才能沿着 explicit 边扩展。不改 ingest 分块主链路，只改将要落盘/入库的正文。

const (
	autolinkMinRunes      = 2 // 与 mention.go 单字符噪声守卫一致
	autolinkASCIIMinRunes = 3 // 避免 Go/AI/to 等短英文误链
	autolinkTitleCap      = 500
	autolinkDocListLimit  = 2000
)

// AutolinkTarget 一个可成链目标（P0 别名成链）：Canonical 是 wikilink 落点
// （[[Canonical]]，取 rel_path basename），Keys 是参与提及匹配的全部显示名
// （basename + title + aliases）。同一 Key 出现在多个目标即歧义，跳过成链。
type AutolinkTarget struct {
	Canonical string
	Keys      []string
}

// AutolinkWikiMentions 把 content 中对 titles 的未链接提及包成 [[Title]]。
// selfTitle 不被链接（避免文档链向自己）。歧义标题（多个文档同一 needle）跳过。
// 返回新正文与替换次数；无变更时返回原文。
func AutolinkWikiMentions(content, selfTitle string, titles []string) (string, int) {
	targets := make([]AutolinkTarget, 0, len(titles))
	for _, t := range titles {
		if t = strings.TrimSpace(t); t != "" {
			targets = append(targets, AutolinkTarget{Canonical: t, Keys: []string{t}})
		}
	}
	var self []string
	if strings.TrimSpace(selfTitle) != "" {
		self = []string{selfTitle}
	}
	return AutolinkWikiMentionsMulti(content, self, targets)
}

// AutolinkWikiMentionsMulti 多键成链：content 中命中目标任一 Key 的未链接提及
// 包成 [[Canonical]]（别名提及链向正名）。selfKeys（自身全部显示名）不链。
func AutolinkWikiMentionsMulti(content string, selfKeys []string, targets []AutolinkTarget) (string, int) {
	if content == "" || len(targets) == 0 {
		return content, 0
	}
	type cand struct {
		needle string
		title  string
		ascii  bool
	}
	selfSet := make(map[string]struct{}, len(selfKeys))
	for _, s := range selfKeys {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			selfSet[s] = struct{}{}
		}
	}
	// key → 归属目标序号；跨目标同 key 即歧义（同目标内多键自然去重）。
	keyTarget := make(map[string]int)
	keyAmbiguous := make(map[string]bool)
	keyRaw := make(map[string]string)
	for ti, t := range targets {
		canonical := strings.TrimSpace(t.Canonical)
		if canonical == "" {
			continue
		}
		seenInTarget := make(map[string]struct{}, len(t.Keys))
		for _, k := range t.Keys {
			k = strings.TrimSpace(k)
			n := utf8.RuneCountInString(k)
			if n < autolinkMinRunes {
				continue
			}
			ascii := isASCIITitle(k)
			if ascii && n < autolinkASCIIMinRunes {
				continue
			}
			key := strings.ToLower(k)
			if _, isSelf := selfSet[key]; isSelf {
				continue
			}
			if _, dup := seenInTarget[key]; dup {
				continue
			}
			seenInTarget[key] = struct{}{}
			if prev, seen := keyTarget[key]; seen && prev != ti {
				keyAmbiguous[key] = true
				continue
			}
			keyTarget[key] = ti
			if _, ok := keyRaw[key]; !ok {
				keyRaw[key] = k
			}
		}
	}
	cands := make([]cand, 0, len(keyTarget))
	for key, ti := range keyTarget {
		if keyAmbiguous[key] {
			continue
		}
		t := targets[ti]
		cands = append(cands, cand{needle: key, title: strings.TrimSpace(t.Canonical), ascii: isASCIITitle(keyRaw[key])})
	}
	if len(cands) == 0 {
		return content, 0
	}
	// 长 key 优先：防短 key 先占位遮蔽长 key（「协议」不抢「通信协议」的命中区间）。
	sort.Slice(cands, func(i, j int) bool {
		ri := utf8.RuneCountInString(cands[i].needle)
		rj := utf8.RuneCountInString(cands[j].needle)
		if ri != rj {
			return ri > rj
		}
		return cands[i].needle < cands[j].needle
	})
	if len(cands) > autolinkTitleCap {
		cands = cands[:autolinkTitleCap]
	}

	mask := protectAutolinkMask(content)
	lower := strings.ToLower(content)
	type span struct {
		start, end int
		title      string
	}
	var spans []span
	for _, c := range cands {
		needleLen := len(c.needle)
		start := 0
		for {
			idx := strings.Index(lower[start:], c.needle)
			if idx < 0 {
				break
			}
			idx += start
			end := idx + needleLen
			if rangeFree(mask, idx, end) && asciiWordBoundary(content, idx, end, c.ascii) {
				spans = append(spans, span{start: idx, end: end, title: c.title})
				for i := idx; i < end; i++ {
					mask[i] = true
				}
				start = end
				continue
			}
			start = idx + 1
		}
	}
	if len(spans) == 0 {
		return content, 0
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })
	out := content
	for _, s := range spans {
		out = out[:s.start] + "[[" + s.title + "]]" + out[s.end:]
	}
	return out, len(spans)
}

// AutolinkOutgoing 编译期成链：把 content 中对同库文档的未链接提及包成 wikilink，
// 并返回替换次数。别名成链（P0）：resolveIndex 接线时 needle 扩为
// basename + title + aliases 多键；未接线降级为 basename 单键（旧行为）。
// excludeDocID 对应文档的全部显示名视为 self（不链向自己）；selfTitleHint 在文档
// 尚未入库时用 filename/source 充当 self。
func (u *Usecase) AutolinkOutgoing(ctx context.Context, collectionID, excludeDocID, selfTitleHint, content string) (string, int) {
	out, n := u.compileOutgoingMentions(ctx, collectionID, excludeDocID, selfTitleHint, content)
	if n > 0 {
		u.lg.Info("自动成链已编译未链接提及",
			loggateway.StepID("knowledge.autolink.applied"),
			loggateway.Str("collection_id", collectionID),
			loggateway.Int("replacements", n),
		)
	}
	return out, n
}

// compileOutgoingMentions 把未链接提及编译成 wikilink，不写源、不打 Info。
// 索引重建与写路径共用；全量重建不得刷 Info。
func (u *Usecase) compileOutgoingMentions(ctx context.Context, collectionID, excludeDocID, selfTitleHint, content string) (string, int) {
	if u == nil || u.documents == nil || strings.TrimSpace(content) == "" || strings.TrimSpace(collectionID) == "" {
		return content, 0
	}
	if u.resolveIndex != nil {
		cands, err := u.resolveIndex.ListResolveCandidates(ctx, []string{collectionID})
		if err != nil {
			u.lg.Warn("自动成链列举解析候选失败，降级 basename 单键",
				loggateway.StepID("knowledge.autolink.list_fail"),
				loggateway.Str("collection_id", collectionID),
				loggateway.Err(err),
			)
		} else {
			return autolinkFromCandidates(cands, excludeDocID, selfTitleHint, content)
		}
	}
	docs, _, err := u.documents.ListDocuments(ctx, collectionID, autolinkDocListLimit, 0)
	if err != nil {
		u.lg.Warn("自动成链列出文档失败，跳过",
			loggateway.StepID("knowledge.autolink.list_fail"),
			loggateway.Str("collection_id", collectionID),
			loggateway.Err(err),
		)
		return content, 0
	}
	selfTitle := mentionNeedle("", selfTitleHint)
	titles := make([]string, 0, len(docs))
	for _, d := range docs {
		n := mentionNeedle(d.RelPath, d.Source)
		if excludeDocID != "" && d.ID == excludeDocID {
			if n != "" {
				selfTitle = n
			}
			continue
		}
		if n != "" {
			titles = append(titles, n)
		}
	}
	return AutolinkWikiMentions(content, selfTitle, titles)
}

// autolinkFromCandidates 多键成链装配：候选文档的 basename 为 canonical，
// basename/title/aliases 全键参与匹配；self 文档全键豁免。
func autolinkFromCandidates(cands []ResolveDocCandidate, excludeDocID, selfTitleHint, content string) (string, int) {
	selfKeys := []string{mentionNeedle("", selfTitleHint)}
	targets := make([]AutolinkTarget, 0, len(cands))
	for _, c := range cands {
		canonical := mentionNeedle(c.RelPath, "")
		if canonical == "" {
			continue
		}
		keys := []string{canonical}
		if t := strings.TrimSpace(c.Title); t != "" {
			keys = append(keys, t)
		}
		for _, a := range c.Aliases {
			if a = strings.TrimSpace(a); a != "" {
				keys = append(keys, a)
			}
		}
		if excludeDocID != "" && c.DocID == excludeDocID {
			selfKeys = append(selfKeys, keys[1:]...)
			selfKeys[0] = canonical // 以物化 basename 为准
			continue
		}
		targets = append(targets, AutolinkTarget{Canonical: canonical, Keys: keys})
	}
	return AutolinkWikiMentionsMulti(content, selfKeys, targets)
}

// MaybeAutolinkOutgoing 写路径便捷封装：丢弃替换次数。
func (u *Usecase) MaybeAutolinkOutgoing(ctx context.Context, collectionID, excludeDocID, selfTitleHint, content string) string {
	out, _ := u.AutolinkOutgoing(ctx, collectionID, excludeDocID, selfTitleHint, content)
	return out
}

func isASCIITitle(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

func isASCIIWordChar(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
}

func asciiWordBoundary(content string, start, end int, ascii bool) bool {
	if !ascii {
		return true
	}
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(content[:start])
		if isASCIIWordChar(r) {
			return false
		}
	}
	if end < len(content) {
		r, _ := utf8.DecodeRuneInString(content[end:])
		if isASCIIWordChar(r) {
			return false
		}
	}
	return true
}

func rangeFree(mask []bool, start, end int) bool {
	if start < 0 || end > len(mask) || start >= end {
		return false
	}
	for i := start; i < end; i++ {
		if mask[i] {
			return false
		}
	}
	return true
}

func markRange(mask []bool, start, end int) {
	if start < 0 {
		start = 0
	}
	if end > len(mask) {
		end = len(mask)
	}
	for i := start; i < end; i++ {
		mask[i] = true
	}
}

func consumeLineEnd(s string, pos int) int {
	if pos >= len(s) {
		return len(s)
	}
	if s[pos] == '\r' {
		pos++
		if pos < len(s) && s[pos] == '\n' {
			pos++
		}
		return pos
	}
	if s[pos] == '\n' {
		return pos + 1
	}
	return pos
}

// protectAutolinkMask 标记禁止成链的区间：YAML frontmatter、围栏代码、行内代码、
// 已有 [[wikilink]]、Markdown 链接与裸 URL。
func protectAutolinkMask(content string) []bool {
	n := len(content)
	mask := make([]bool, n)
	if n == 0 {
		return mask
	}

	off := 0
	if strings.HasPrefix(content, "---\r\n") {
		off = 5
	} else if strings.HasPrefix(content, "---\n") {
		off = 4
	}
	if off > 0 {
		if idx := strings.Index(content[off:], "\n---"); idx >= 0 {
			closeAt := off + idx + 1
			nl := strings.IndexAny(content[closeAt:], "\r\n")
			end := n
			if nl >= 0 {
				end = consumeLineEnd(content, closeAt+nl)
			}
			markRange(mask, 0, end)
		}
	}

	i := 0
	atLineStart := true
	for i < n {
		if mask[i] {
			if content[i] == '\n' {
				atLineStart = true
			} else if content[i] != '\r' {
				atLineStart = false
			}
			i++
			continue
		}
		if atLineStart && i+2 < n && (content[i:i+3] == "```" || content[i:i+3] == "~~~") {
			fence := content[i : i+3]
			lineEnd := strings.IndexAny(content[i:], "\r\n")
			body := n
			if lineEnd >= 0 {
				body = consumeLineEnd(content, i+lineEnd)
			}
			ci := strings.Index(content[body:], "\n"+fence)
			if ci < 0 {
				markRange(mask, i, n)
				break
			}
			closePos := body + ci + 1
			nl := strings.IndexAny(content[closePos:], "\r\n")
			end := n
			if nl >= 0 {
				end = consumeLineEnd(content, closePos+nl)
			}
			markRange(mask, i, end)
			i = end
			atLineStart = true
			continue
		}
		if i+1 < n && content[i] == '[' && content[i+1] == '[' {
			j := strings.Index(content[i+2:], "]]")
			if j >= 0 {
				end := i + 2 + j + 2
				markRange(mask, i, end)
				i = end
				atLineStart = false
				continue
			}
		}
		if content[i] == '[' {
			if rb := strings.IndexByte(content[i+1:], ']'); rb >= 0 {
				after := i + 1 + rb + 1
				if after < n && content[after] == '(' {
					if rp := strings.IndexByte(content[after+1:], ')'); rp >= 0 {
						end := after + 1 + rp + 1
						markRange(mask, i, end)
						i = end
						atLineStart = false
						continue
					}
				}
			}
		}
		if content[i] == '`' {
			j := i + 1
			for j < n && content[j] != '`' && content[j] != '\n' && content[j] != '\r' {
				j++
			}
			if j < n && content[j] == '`' {
				markRange(mask, i, j+1)
				i = j + 1
				atLineStart = false
				continue
			}
		}
		if hasHTTPScheme(content, i) {
			j := i
			for j < n && !isURLEnd(content[j]) {
				j++
			}
			markRange(mask, i, j)
			i = j
			atLineStart = false
			continue
		}
		if content[i] == '\n' {
			atLineStart = true
		} else if content[i] != '\r' {
			atLineStart = false
		}
		i++
	}
	return mask
}

func hasHTTPScheme(s string, i int) bool {
	if i+8 <= len(s) && s[i:i+8] == "https://" {
		return true
	}
	return i+7 <= len(s) && s[i:i+7] == "http://"
}

func isURLEnd(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ')' || c == '<' || c == '>'
}
