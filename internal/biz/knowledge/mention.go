package knowledge

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// ── P2-7：unlinked mentions（未链接提及，Obsidian 语义） ─────────────────────
//
// 定义：目标文档显示名（basename 去扩展名）在其他文档正文中以纯文本出现
// （[[wikilink]] 内的出现不计），即「提到了但没链接」。候选由端口 SQL ILIKE
// 预筛，本层做精确剔除与计数；端口未接线时降级为空（不阻断主流程）。

// mentionCandidateLimit SQL 候选上限；mentionOutputLimit 输出上限。
const (
	mentionCandidateLimit = 200
	mentionOutputLimit    = 50
)

// wikiLinkSpanRe 匹配 [[target]] / [[target#heading]] / [[target|alias]] 整段。
var wikiLinkSpanRe = regexp.MustCompile(`\[\[[^\]]*\]\]`)

// UnlinkedMention 一条未链接提及（按来源文档聚合）。
type UnlinkedMention struct {
	SrcDocID   string
	SrcDocName string // 源文档显示名（rel_path 优先）
	Count      int    // 纯文本出现次数
	Snippet    string // 首次出现上下文片段
}

// DocContentHit 提及扫描候选（文档全文投影）。
type DocContentHit struct {
	DocID   string
	DocName string
	Content string
}

// DocContentSearcher unlinked mentions 候选扫描端口（P2-7）。
// 实现方按 needle 做大小写不敏感包含预筛（SQL ILIKE），返回 content_text 全文。
// Stability:evolving
type DocContentSearcher interface {
	// SearchDocContentMentions 返回 collection 内正文含 needle 的文档投影
	// （排除 excludeDocID；按 doc id 字典序，至多 limit 条）。
	SearchDocContentMentions(ctx context.Context, collectionID, needle, excludeDocID string, limit int) ([]DocContentHit, error)
}

// SetMentionSearcher 接线提及扫描端口（P2-7；可选能力，未接线降级为空）。
func (u *Usecase) SetMentionSearcher(searcher DocContentSearcher) {
	u.mentionSearch = searcher
}

// ListUnlinkedMentions 未链接提及：目标文档显示名在库内其他文档的纯文本出现。
// 别名成链（P0）：needle 扩为 basename + title + aliases 多键（resolveIndex
// 接线时）；未接线降级为 basename 单键。输出按 Count 降序、SrcDocID 字典序；
// 至多 mentionOutputLimit 条。
func (u *Usecase) ListUnlinkedMentions(ctx context.Context, docID string) ([]UnlinkedMention, error) {
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return nil, ErrIDRequired
	}
	target, err := u.documents.GetDocument(ctx, docID)
	if err != nil {
		return nil, err
	}
	needles := u.mentionNeedles(ctx, target)
	if len(needles) == 0 || u.mentionSearch == nil {
		return nil, nil
	}
	// 逐 needle 预筛并合并：同一源文档命中多个显示名时计数累加、片段取首个。
	// （「协议」与「通信协议」重叠命中会重复计数——Count 是热度提示，非精确值。）
	merged := make(map[string]*UnlinkedMention)
	for _, needle := range needles {
		hits, err := u.mentionSearch.SearchDocContentMentions(ctx, target.CollectionID, needle, docID, mentionCandidateLimit)
		if err != nil {
			return nil, err
		}
		lowerNeedle := strings.ToLower(needle)
		for _, h := range hits {
			plain := wikiLinkSpanRe.ReplaceAllString(h.Content, " ")
			lower := strings.ToLower(plain)
			count := strings.Count(lower, lowerNeedle)
			if count == 0 {
				continue // ILIKE 命中可能全部位于 [[...]] 内
			}
			m := merged[h.DocID]
			if m == nil {
				m = &UnlinkedMention{SrcDocID: h.DocID, SrcDocName: h.DocName}
				merged[h.DocID] = m
			}
			m.Count += count
			if m.Snippet == "" {
				byteIdx := strings.Index(lower, lowerNeedle)
				if byteIdx >= 0 && byteIdx <= len(plain) {
					// ToLower 对本域字符集（CJK/ASCII）等长，byteIdx 可直接索引原文；
					// 罕见 Unicode 扩展小写导致错位时退化为空片段（Count 仍准确）。
					m.Snippet = mentionSnippet(plain, byteIdx, utf8.RuneCountInString(needle))
				}
			}
		}
	}
	out := make([]UnlinkedMention, 0, len(merged))
	for _, m := range merged {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].SrcDocID < out[j].SrcDocID
	})
	if len(out) > mentionOutputLimit {
		out = out[:mentionOutputLimit]
	}
	return out, nil
}

// mentionNeedles 目标文档的全部提及匹配键：basename + title + aliases
// （去重、去空白、≥2 字符；单字符名匹配噪声过大直接丢弃）。
// resolveIndex 未接线或候选查询失败时降级为 basename 单键。
func (u *Usecase) mentionNeedles(ctx context.Context, target Document) []string {
	var out []string
	seen := make(map[string]struct{})
	add := func(s string) {
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) < 2 {
			return
		}
		k := strings.ToLower(s)
		if _, dup := seen[k]; dup {
			return
		}
		seen[k] = struct{}{}
		out = append(out, s)
	}
	add(mentionNeedle(target.RelPath, target.Source))
	if u.resolveIndex == nil {
		return out
	}
	cands, err := u.resolveIndex.ListResolveCandidates(ctx, []string{target.CollectionID})
	if err != nil {
		return out
	}
	for _, c := range cands {
		if c.DocID != target.ID {
			continue
		}
		add(c.Title)
		for _, a := range c.Aliases {
			add(a)
		}
	}
	return out
}

// mentionNeedle 目标文档显示名：rel_path/source 取 basename，去扩展名。
func mentionNeedle(relPath, source string) string {
	name := relPath
	if name == "" {
		name = source
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		// dotfile（如 ".md" 无主干名）一并剔除 → 空 needle 走降级。
		name = name[:i]
	}
	return strings.TrimSpace(name)
}

// mentionSnippet 首次出现上下文：byteIdx 为 strings.Index 结果（小写化后的
// 字节偏移；大小写不敏感匹配等长，可直接索引原文），前后各扩 48 rune。
func mentionSnippet(plain string, byteIdx, needleRunes int) string {
	runes := []rune(plain)
	center := utf8.RuneCountInString(plain[:byteIdx])
	start := max(center-48, 0)
	end := min(center+needleRunes+48, len(runes))
	prefix, tail := "", ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(runes) {
		tail = "…"
	}
	return prefix + strings.TrimSpace(string(runes[start:end])) + tail
}
