// Package htmltext HTML 正文抽取（U2-3 解析纵深）：剥离脚本/样式/导航/页脚
// /侧栏/表单/嵌框等噪声块，为候选容器按「文本量 × (1 − 链接密度)」打分，
// 取最优块渲染为 Markdown-ish 纯文本（h1-h3 井号前缀、li 短横前缀、块级
// 换行保序）。上层在产物过短时回落全量可见文本（document_extract.go
// extractHTMLMainText）。
package htmltext

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// dropTags 整块剔除的噪声元素（含 head：标题/元信息不构成正文）。
var dropTags = map[string]struct{}{
	"script": {}, "style": {}, "noscript": {}, "nav": {}, "header": {},
	"footer": {}, "aside": {}, "form": {}, "iframe": {}, "head": {},
}

// candidateTags 参与正文候选打分的容器元素。
var candidateTags = map[string]struct{}{
	"article": {}, "main": {}, "section": {}, "div": {}, "body": {},
}

// ExtractMainContent 从已解析的 HTML 树抽取正文文本；无有效候选时返回 ""
// （调用方据此回落全量可见文本）。
func ExtractMainContent(top *html.Node) string {
	if top == nil {
		return ""
	}
	var best *html.Node
	bestScore := 0.0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if _, drop := dropTags[n.Data]; drop {
				return
			}
			if _, ok := candidateTags[n.Data]; ok {
				if st := blockStatsOf(n); st.textLen > 0 {
					// 同分取后遍历到（更深）的容器：body 的得分恒等于其内唯一
					// 正文块的得分（body 分 = 文本量 − 链接量），严格大于会让
					// body 压过 article/div，把平级链接块带进渲染。
					if sc := st.value(); sc > 0 && sc >= bestScore {
						bestScore = sc
						best = n
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(top)
	if best == nil {
		return ""
	}
	return render(best)
}

// blockStats 候选块的文本统计。
type blockStats struct {
	textLen    int // 可见文本总字符（rune）数
	linkLen    int // 其中落在 <a> 内的字符数
	paragraphs int // <p>/<li> 块数（留作后续加权扩展）
}

// value 打分：文本量 × (1 − 链接密度)——链接密布块（导航/推荐位残骸）被
// 压到近零，正文块凭文本量胜出。
func (s blockStats) value() float64 {
	if s.textLen <= 0 {
		return 0
	}
	linkDensity := float64(s.linkLen) / float64(s.textLen)
	return float64(s.textLen) * (1 - linkDensity)
}

// blockStatsOf 统计候选块子树的文本量/链接占比（噪声子树不计）。
func blockStatsOf(n *html.Node) blockStats {
	var st blockStats
	var walk func(*html.Node, bool)
	walk = func(x *html.Node, inLink bool) {
		if x.Type == html.ElementNode {
			if _, drop := dropTags[x.Data]; drop {
				return
			}
			switch x.Data {
			case "a":
				inLink = true
			case "p", "li":
				st.paragraphs++
			}
		}
		if x.Type == html.TextNode {
			l := utf8.RuneCountInString(strings.TrimSpace(x.Data))
			st.textLen += l
			if inLink {
				st.linkLen += l
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inLink)
		}
	}
	walk(n, false)
	return st
}

// render 把选中的容器渲染为 Markdown-ish 纯文本：h1-h3 井号前缀、li 短横
// 前缀，块级元素各占一行，行间散落 inline 文本在块边界落行。
func render(root *html.Node) string {
	var lines []string
	var inline strings.Builder

	flush := func() {
		if t := strings.TrimSpace(inline.String()); t != "" {
			lines = append(lines, t)
		}
		inline.Reset()
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if _, drop := dropTags[n.Data]; drop {
				return
			}
			switch n.Data {
			case "h1", "h2", "h3":
				flush()
				if t := blockText(n); t != "" {
					lines = append(lines, strings.Repeat("#", int(n.Data[1]-'0'))+" "+t)
				}
				return
			case "p":
				flush()
				if t := blockText(n); t != "" {
					lines = append(lines, t)
				}
				return
			case "li":
				flush()
				if t := blockText(n); t != "" {
					lines = append(lines, "- "+t)
				}
				return
			case "br":
				flush()
				return
			}
		}
		if n.Type == html.TextNode {
			if t := strings.TrimSpace(n.Data); t != "" {
				inline.WriteString(t)
				inline.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	flush()
	return strings.Join(lines, "\n")
}

// blockText 提取节点内全部可见文本折叠为单行（块内 inline 元素不炸行）。
func blockText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode {
			if _, drop := dropTags[x.Data]; drop {
				return
			}
		}
		if x.Type == html.TextNode {
			if t := strings.TrimSpace(x.Data); t != "" {
				b.WriteString(t)
				b.WriteByte(' ')
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}
