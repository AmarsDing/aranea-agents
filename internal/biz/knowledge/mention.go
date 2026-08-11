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

// ListUnlinkedMentions 未链接提及：目标文档名在库内其他文档的纯文本出现。
// 输出按 Count 降序、SrcDocID 字典序；至多 mentionOutputLimit 条。
func (u *Usecase) ListUnlinkedMentions(ctx context.Context, docID string) ([]UnlinkedMention, error) {
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return nil, ErrIDRequired
	}
	target, err := u.documents.GetDocument(ctx, docID)
	if err != nil {
		return nil, err
	}
	needle := mentionNeedle(target.RelPath, target.Source)
	// 单字符名匹配噪声过大（尤其单字中文笔记），直接降级为空。
	if utf8.RuneCountInString(needle) < 2 || u.mentionSearch == nil {
		return nil, nil
	}
	hits, err := u.mentionSearch.SearchDocContentMentions(ctx, target.CollectionID, needle, docID, mentionCandidateLimit)
	if err != nil {
		return nil, err
	}
	lowerNeedle := strings.ToLower(needle)
	out := make([]UnlinkedMention, 0, len(hits))
	for _, h := range hits {
		plain := wikiLinkSpanRe.ReplaceAllString(h.Content, " ")
		lower := strings.ToLower(plain)
		count := strings.Count(lower, lowerNeedle)
		if count == 0 {
			continue // ILIKE 命中可能全部位于 [[...]] 内
		}
		byteIdx := strings.Index(lower, lowerNeedle)
		snippet := ""
		if byteIdx >= 0 && byteIdx <= len(plain) {
			// ToLower 对本域字符集（CJK/ASCII）等长，byteIdx 可直接索引原文；
			// 罕见 Unicode 扩展小写导致错位时退化为空片段（Count 仍准确）。
			snippet = mentionSnippet(plain, byteIdx, utf8.RuneCountInString(needle))
		}
		out = append(out, UnlinkedMention{
			SrcDocID:   h.DocID,
			SrcDocName: h.DocName,
			Count:      count,
			Snippet:    snippet,
		})
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
