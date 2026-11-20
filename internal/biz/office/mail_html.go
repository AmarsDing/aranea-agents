package office

// 邮件 HTML 清洗（09-邮件.md §3.1「正文按 HTML 安全渲染：去脚本、外链图片默认不加载」）。
//
// 三层防护，这里是第一层：
//  1. 入库前按白名单清洗（本文件）：丢脚本 / 表单 / 内嵌框架，事件属性一律去掉，
//     链接只留 http(s) / mailto 并强制新窗口打开，外链图片改写成 data-mail-src
//     （前端点「显示图片」才换回 src）；
//  2. 前端放进 sandbox iframe（不给 allow-scripts）；
//  3. iframe 里加 CSP：默认什么都不许连，图片放行要用户点。

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var mailAllowedTags = setOf(
	"a", "abbr", "address", "b", "big", "blockquote", "br", "caption", "center", "cite", "code",
	"col", "colgroup", "dd", "del", "div", "dl", "dt", "em", "font", "h1", "h2", "h3", "h4", "h5",
	"h6", "hr", "i", "img", "ins", "kbd", "li", "mark", "ol", "p", "pre", "q", "s", "small",
	"span", "strike", "strong", "style", "sub", "sup", "table", "tbody", "td", "tfoot", "th",
	"thead", "tr", "tt", "u", "ul", "wbr",
)

// 这些标签连同里面的内容一起丢掉。
var mailDropWithContent = setOf(
	"script", "head", "title", "iframe", "frame", "frameset", "object", "embed", "applet",
	"noscript", "template", "svg", "math", "form", "select", "textarea", "button", "audio",
	"video", "canvas", "base", "link", "meta",
)

var mailAllowedAttrs = setOf(
	"align", "valign", "width", "height", "colspan", "rowspan", "border", "cellpadding",
	"cellspacing", "bgcolor", "color", "face", "size", "title", "alt", "dir", "lang", "style",
	"start", "span", "class",
)

// CSS 里可能外连或执行的写法：@import、expression()、javascript:、behavior。
// url(...) 留着——iframe 的 CSP 会挡住，用户点「显示图片」后才放行。
var mailUnsafeCSS = regexp.MustCompile(`(?i)@import|expression\s*\(|javascript:|vbscript:|behavior\s*:|-moz-binding`)

func setOf(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}

func mailSafeURL(v string) (string, bool) {
	v = strings.TrimSpace(v)
	low := strings.ToLower(v)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") || strings.HasPrefix(low, "mailto:") {
		return v, true
	}
	return "", false
}

// sanitizeMailHTML 白名单清洗；输入为空返回空。
func sanitizeMailHTML(in string) string {
	if strings.TrimSpace(in) == "" {
		return ""
	}
	z := html.NewTokenizer(strings.NewReader(in))
	var b strings.Builder
	skipDepth := 0 // >0 表示在要整段丢弃的标签里
	inStyle := false
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(b.String())
		case html.TextToken:
			if skipDepth > 0 {
				continue
			}
			if inStyle {
				b.WriteString(mailUnsafeCSS.ReplaceAllString(string(z.Raw()), ""))
				continue
			}
			b.WriteString(html.EscapeString(string(z.Text())))
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			if mailDropWithContent[tag] {
				if tt == html.StartTagToken && !isVoidTag(tag) {
					skipDepth++
				}
				continue
			}
			if skipDepth > 0 || !mailAllowedTags[tag] {
				continue
			}
			if tag == "style" {
				inStyle = tt == html.StartTagToken
				b.WriteString("<style>")
				if !inStyle {
					b.WriteString("</style>")
				}
				continue
			}
			b.WriteString("<" + tag)
			var attrs [][2]string
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				attrs = append(attrs, [2]string{strings.ToLower(string(k)), string(v)})
			}
			writeMailAttrs(&b, tag, attrs)
			if tt == html.SelfClosingTagToken {
				b.WriteString(" />")
			} else {
				b.WriteString(">")
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if mailDropWithContent[tag] {
				if skipDepth > 0 {
					skipDepth--
				}
				continue
			}
			if skipDepth > 0 || !mailAllowedTags[tag] || isVoidTag(tag) {
				continue
			}
			if tag == "style" {
				inStyle = false
			}
			b.WriteString("</" + tag + ">")
		}
	}
}

func isVoidTag(tag string) bool {
	switch tag {
	case "br", "hr", "img", "col", "wbr", "base", "link", "meta":
		return true
	}
	return false
}

func writeMailAttrs(b *strings.Builder, tag string, attrs [][2]string) {
	put := func(k, v string) {
		b.WriteString(" " + k + `="` + html.EscapeString(v) + `"`)
	}
	for _, kv := range attrs {
		k, v := kv[0], kv[1]
		switch {
		case tag == "a" && k == "href":
			if u, ok := mailSafeURL(v); ok {
				put("href", u)
				put("target", "_blank")
				put("rel", "noopener noreferrer")
			}
		case tag == "img" && k == "src":
			low := strings.ToLower(strings.TrimSpace(v))
			switch {
			case strings.HasPrefix(low, "data:image/"):
				put("src", v)
			case strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://"):
				// 外链图片默认不加载（防追踪，M3）；前端点「显示图片」时换回 src。
				put("data-mail-src", strings.TrimSpace(v))
			}
		case k == "style":
			put("style", mailUnsafeCSS.ReplaceAllString(v, ""))
		case mailAllowedAttrs[k]:
			put(k, v)
		}
	}
}

// mailHTMLToText 只有 HTML 时抽出可读文字（列表摘要、搜索、纯文本回退用）。
func mailHTMLToText(in string) string {
	z := html.NewTokenizer(strings.NewReader(in))
	var b strings.Builder
	skip := 0
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			lines := strings.Split(b.String(), "\n")
			out := make([]string, 0, len(lines))
			blank := false
			for _, l := range lines {
				l = strings.Join(strings.Fields(l), " ")
				if l == "" {
					if !blank && len(out) > 0 {
						out = append(out, "")
					}
					blank = true
					continue
				}
				blank = false
				out = append(out, l)
			}
			return strings.TrimSpace(strings.Join(out, "\n"))
		case html.TextToken:
			if skip == 0 {
				b.WriteString(string(z.Text()))
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if tag == "script" || tag == "style" || tag == "head" || tag == "title" {
				if tt == html.StartTagToken {
					skip++
				}
				continue
			}
			switch tag {
			case "br", "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "table", "hr":
				b.WriteString("\n")
			case "td", "th":
				b.WriteString(" ")
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if (tag == "script" || tag == "style" || tag == "head" || tag == "title") && skip > 0 {
				skip--
				continue
			}
			switch tag {
			case "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "table":
				b.WriteString("\n")
			}
		}
	}
}
