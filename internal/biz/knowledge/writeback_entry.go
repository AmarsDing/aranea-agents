package knowledge

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── P0 词条优先写回（2026-08-15 评审修订三刀之 1/3）──────────────────────────
//
// 过门事实不再只堆日记：按记忆侧已抽取的 tags 定位主题词条页（entries/<slug>.md），
// 命中已有页（tag ∈ basename/title/aliases）则 upsert 正文，未命中新建词条；
// 同一 fact_id 再写入时替换词条里的旧段（替换语义走正文，不动 chunk 派生模型）。
// 无 tags 的事实回退当日日记流水。词条页是 Markdown 真相源：人可直接编辑，
// 保存后走既有文档级更新路径（CAS + 重嵌入 + 块索引重建）。

// writeBackEntryDir 词条页目录（写回库内约定路径）。
const writeBackEntryDir = "entries"

// upsertFactsToEntries 把带 tags 的事实 upsert 到词条页；返回 factIdx→词条显示名
// 与实际改动的词条文档（service 层据此重放 chunk/FTS——2026-08-15 修复：词条页
// 此前不在 WriteBackResult 里，chunks 无人重建，写入成功但检索永远不可见）。
// 单条失败 Warn 后继续——日记流水是兜底，词条失败不阻断主流程。
func (u *Usecase) upsertFactsToEntries(ctx context.Context, col Collection, in WriteBackInput, facts []WriteBackFact) (map[int]string, []PromoteTouchedDoc) {
	landed := make(map[int]string, len(facts))
	var touched []PromoteTouchedDoc
	// 按词条文档分组，同页事实一次读改写。
	type group struct {
		rel   string
		title string
		tags  []string // 新建页 header 用（首个分组事实的 tags）
		idxs  []int
	}
	groups := make(map[string]*group)
	var order []string
	for i, f := range facts {
		rel, title := u.resolveEntryForFact(ctx, col, f)
		if rel == "" {
			continue // 无 tags：回退日记
		}
		g, ok := groups[rel]
		if !ok {
			g = &group{rel: rel, title: title, tags: normalizedEntryTags(f.Tags)}
			groups[rel] = g
			order = append(order, rel)
		}
		g.idxs = append(g.idxs, i)
	}
	for _, rel := range order {
		g := groups[rel]
		batch := make([]WriteBackFact, 0, len(g.idxs))
		for _, i := range g.idxs {
			batch = append(batch, facts[i])
		}
		doc, changed, err := u.upsertEntryDoc(ctx, col, g.rel, g.title, g.tags, in, batch)
		if err != nil {
			u.lg.Warn("词条页 upsert 失败（事实仍落日记流水）",
				loggateway.StepID("knowledge.writeback.entry"),
				loggateway.Str("rel_path", g.rel),
				loggateway.Err(err),
			)
			continue
		}
		if changed {
			touched = append(touched, doc)
		}
		for _, i := range g.idxs {
			landed[i] = g.title
		}
	}
	return landed, touched
}

// resolveEntryForFact 词条定位：首选 tag 起，按序命中已有词条（basename/title/aliases）
// 返回其 rel_path 与显示名；未命中返回新建路径 entries/<slug>.md。无 tags 返回空。
func (u *Usecase) resolveEntryForFact(ctx context.Context, col Collection, f WriteBackFact) (rel, title string) {
	tags := normalizedEntryTags(f.Tags)
	if len(tags) == 0 {
		return "", ""
	}
	if u.resolveIndex != nil {
		cands, err := u.resolveIndex.ListResolveCandidates(ctx, []string{col.ID})
		if err != nil {
			u.lg.Warn("词条候选列举失败，退化为新建路径",
				loggateway.StepID("knowledge.writeback.entry"),
				loggateway.Str("collection_id", col.ID),
				loggateway.Err(err),
			)
		} else {
			keys := make(map[string]ResolveDocCandidate) // normKey → 词条候选
			for _, c := range cands {
				if !strings.HasPrefix(c.RelPath, writeBackEntryDir+"/") {
					continue
				}
				for _, k := range entryCandidateKeys(c) {
					if _, dup := keys[k]; !dup {
						keys[k] = c
					}
				}
			}
			for _, t := range tags {
				if c, ok := keys[strings.ToLower(t)]; ok {
					display := strings.TrimSpace(c.Title)
					if display == "" {
						display = mentionNeedle(c.RelPath, "")
					}
					return c.RelPath, display
				}
			}
		}
	}
	slug := entrySlug(tags[0])
	if slug == "" {
		return "", ""
	}
	return writeBackEntryDir + "/" + slug + ".md", tags[0]
}

// normalizedEntryTags 清洗 tags：去空白、去重（大小写不敏感）、保持原序。
func normalizedEntryTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || utf8.RuneCountInString(t) < 2 {
			continue
		}
		k := strings.ToLower(t)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, t)
	}
	return out
}

// entryCandidateKeys 词条候选的全部匹配键：basename + title + aliases（小写）。
func entryCandidateKeys(c ResolveDocCandidate) []string {
	var keys []string
	if base := mentionNeedle(c.RelPath, ""); base != "" {
		keys = append(keys, strings.ToLower(base))
	}
	if t := strings.TrimSpace(c.Title); t != "" {
		keys = append(keys, strings.ToLower(t))
	}
	for _, a := range c.Aliases {
		if a = strings.TrimSpace(a); a != "" {
			keys = append(keys, strings.ToLower(a))
		}
	}
	return keys
}

// entrySlug tag → 文件名安全 slug：ASCII 小写、空白/下划线折叠为连字符，
// 保留 Unicode 字母与数字（中文词条名原样可用）。
func entrySlug(tag string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(tag) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case unicode.IsSpace(r) || r == '_' || r == '-':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// writeBackEntryHeader 词条页头：YAML frontmatter 物化 title/aliases（Resolver 与
// 词条匹配共用解析键），首选 tag 为 title，其余 tag 落 aliases——同义 tag 后续
// 写回可命中本页而不是另起新页。
func writeBackEntryHeader(tags []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + yamlQuote(tags[0]) + "\n")
	if len(tags) > 1 {
		b.WriteString("aliases:\n")
		for _, a := range tags[1:] {
			b.WriteString("  - " + yamlQuote(a) + "\n")
		}
	}
	b.WriteString("---\n\n")
	b.WriteString("# " + tags[0] + "\n\n")
	b.WriteString("> 会话沉淀自动维护的词条页：同主题新事实按 kind 追加；同一 fact_id 再写入时更新原段。人工可直接编辑。\n")
	return b.String()
}

// yamlQuote 单行 YAML 标量：纯安全字符原样输出，否则双引号包裹并转义。
func yamlQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, ":#{}[]&*!|>'\"%@`\r\n\t") && strings.TrimSpace(s) == s {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

// upsertEntryDoc 单篇词条页的读改写：fact_id 命中 → 替换旧段；陈述已在 → 跳过；
// 否则尾部追加。写走后重建块索引（失败 Warn，正文已落可自愈）。返回 touched 文档
// 与是否实际改动（全量去重命中时 changed=false，不触发 chunk 重放）。
// tags 仅用于新建页 header；title 作为自动成链的 self 提示（防 H1 自链）。
func (u *Usecase) upsertEntryDoc(ctx context.Context, col Collection, rel, title string, tags []string, in WriteBackInput, facts []WriteBackFact) (PromoteTouchedDoc, bool, error) {
	doc, err := u.documents.GetDocumentByRelPath(ctx, col.ID, rel)
	created := false
	if err != nil {
		if !apierror.IsCode(err, apierror.CodeNotFound) {
			return PromoteTouchedDoc{}, false, err
		}
		created = true
	}
	body := ""
	if created {
		if len(tags) == 0 {
			tags = []string{title}
		}
		body = writeBackEntryHeader(tags)
	} else {
		body = doc.ContentText
	}
	changed := false
	appended := 0
	for _, f := range facts {
		block := strings.TrimSpace(FormatWriteBackAppendix(in, []WriteBackFact{f}))
		if block == "" {
			continue
		}
		if id := strings.TrimSpace(f.FactID); id != "" {
			marker := "fact_id: `" + id + "`"
			if nb, ok := replaceH2BlockContaining(body, marker, block); ok {
				// 替换语义走正文：同一 fact_id 再写入改旧段；内容未变（幂等重放）
				// 不算改动——避免无效 UPDATE 与下游 chunk/embedding 重放。
				if nb != body {
					body = nb
					changed = true
				}
				continue
			}
		}
		if writeBackAlreadyPresent(body, "", f.Statement) {
			continue
		}
		body = strings.TrimRight(body, "\n") + "\n\n" + block + "\n"
		changed = true
		appended++
	}
	if !changed {
		return PromoteTouchedDoc{}, false, nil
	}
	body = u.MaybeAutolinkOutgoing(ctx, col.ID, doc.ID, title, body)
	if created {
		doc, err = u.CreateDocument(ctx, Document{
			CollectionID: col.ID,
			RelPath:      rel,
			Source:       rel,
			MimeType:     "text/markdown",
			ContentText:  body,
			Organized:    true,
			Status:       "pending",
		})
		if err != nil {
			return PromoteTouchedDoc{}, false, err
		}
	} else {
		if err := u.documents.UpdateDocumentContent(ctx, doc.ID, body, true); err != nil {
			return PromoteTouchedDoc{}, false, err
		}
	}
	if err := u.RebuildBlockIndex(ctx, col.ID, doc.ID, body); err != nil {
		u.lg.Warn("词条页块索引重建失败（正文已落，重建可自愈）",
			loggateway.StepID("knowledge.writeback.entry"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err),
		)
	}
	u.lg.Info("写回飞轮已 upsert 词条页",
		loggateway.StepID("knowledge.writeback.entry"),
		loggateway.Str("collection_id", col.ID),
		loggateway.Str("doc_id", doc.ID),
		loggateway.Str("rel_path", rel),
		loggateway.Int("appended", appended),
	)
	return PromoteTouchedDoc{DocID: doc.ID, Created: created}, true, nil
}

// replaceH2BlockContaining 替换包含 marker 的 H2 小节整段（含小节标题行）为
// newBlock；marker 不存在返回 false。小节边界："\n## " 或文档开头的 "## "。
func replaceH2BlockContaining(body, marker, newBlock string) (string, bool) {
	idx := strings.Index(body, marker)
	if idx < 0 {
		return body, false
	}
	start := strings.LastIndex(body[:idx], "\n## ")
	if start < 0 {
		if !strings.HasPrefix(body, "## ") {
			return body, false // marker 位于前言区，非事实块
		}
	} else {
		start++ // 越过 '\n' 指向 '#'
	}
	end := len(body)
	if nxt := strings.Index(body[start+3:], "\n## "); nxt >= 0 {
		end = start + 3 + nxt
	}
	prefix := strings.TrimRight(body[:start], "\n")
	suffix := strings.TrimLeft(body[end:], "\n")
	var b strings.Builder
	if prefix != "" {
		b.WriteString(prefix)
		b.WriteString("\n\n")
	}
	b.WriteString(strings.TrimSpace(newBlock))
	if suffix != "" {
		b.WriteString("\n\n")
		b.WriteString(suffix)
	}
	b.WriteString("\n")
	return b.String(), true
}
