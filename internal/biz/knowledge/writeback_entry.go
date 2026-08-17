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
			keys := make(map[string]map[string]ResolveDocCandidate) // normKey → docID → 词条候选
			for _, c := range cands {
				if !strings.HasPrefix(c.RelPath, writeBackEntryDir+"/") {
					continue
				}
				for _, k := range entryCandidateKeys(c) {
					if keys[k] == nil {
						keys[k] = make(map[string]ResolveDocCandidate)
					}
					keys[k][c.DocID] = c
				}
			}
			for _, t := range tags {
				matches := keys[strings.ToLower(t)]
				if len(matches) > 1 {
					u.lg.Warn("词条别名存在歧义，跳过自动归并并回退日记",
						loggateway.StepID("knowledge.writeback.entry_ambiguous"),
						loggateway.Str("collection_id", col.ID),
						loggateway.Str("tag", t),
						loggateway.Int("candidate_count", len(matches)),
					)
					return "", ""
				}
				for _, c := range matches {
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

// normalizedEntryTags 清洗 tags：去空白、去重（大小写不敏感）、保持原序；
// 过滤保留键与噪声键（entry_key_guard）。
func normalizedEntryTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || utf8.RuneCountInString(t) < 2 {
			continue
		}
		if IsReservedEntryKey(t) || IsNoiseEntryKey(t) {
			continue // 保留键/噪声键不成话题（entry_key_guard：堵垃圾词条源头）
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
// M3 演化时序（增量叠加，不改主流程）：
//   - M3.1：fact_id 整段替换生效时，旧段快照入 knowledge_fact_version（supersedes 版本链）；
//   - M3.2：追加前经 arbiter 对同页既有段仲裁——supersedes 改走版本链替换目标段，
//     contradicts 落高风险治理提案（旧段不覆盖，新事实仍追加留痕待人工二审）；
//   - 版本/提案均在正文持久化成功后 best-effort 留痕（失败 Warn 不回滚）。
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
	var versions []FactVersion
	var proposals []map[string]any
	type appendCand struct {
		idx   int
		block string
	}
	var appends []appendCand
	for i, f := range facts {
		block := strings.TrimSpace(FormatWriteBackAppendix(in, []WriteBackFact{f}))
		if block == "" {
			continue
		}
		// 有真实 fact_id 才走整段替换（同 ID 再写入更新旧段）；无 ID 仅靠陈述去重，
		// 避免多条无 ID 事实共用占位键互相顶替（2026-08-15 修复）。
		if id := strings.TrimSpace(f.FactID); id != "" && id != "stmt" {
			marker := "fact_id: `" + id + "`"
			if nb, ok := replaceH2BlockContaining(body, marker, block); ok {
				// 替换语义走正文：同一 fact_id 再写入改旧段；内容未变（幂等重放）
				// 不算改动——避免无效 UPDATE 与下游 chunk/embedding 重放。
				if nb != body {
					oldBlock, _ := extractH2BlockContaining(body, marker)
					body = nb
					changed = true
					// M3.1：supersedes 版本链留痕（旧段快照，演化轨迹不丢）。
					versions = append(versions, FactVersion{
						CollectionID: col.ID, DocID: doc.ID,
						FactID: id, OldBody: oldBlock, NewBody: block,
					})
				}
				continue
			}
		}
		if writeBackAlreadyPresent(body, "", f.Statement) {
			continue
		}
		appends = append(appends, appendCand{idx: i, block: block})
	}
	// M3.2：写入时冲突检测（仅对存量页 + 有待追加事实 + 页内有带 ID 事实段时仲裁；
	// 仲裁器未接线/调用失败一律降级原追加行为，不阻断写回）。
	if u.arbiter != nil && !created && len(appends) > 0 {
		existing := extractFactBlocks(body)
		if len(existing) > 0 {
			news := make([]WriteBackFact, 0, len(appends))
			for _, a := range appends {
				news = append(news, facts[a.idx])
			}
			verdicts, aerr := u.arbiter.ArbitrateWriteBack(ctx, title, existing, news)
			if aerr != nil {
				u.lg.Warn("写回冲突仲裁失败（降级为直接追加）",
					loggateway.StepID("knowledge.writeback.arbitrate"),
					loggateway.Str("rel_path", rel),
					loggateway.Err(aerr),
				)
			} else {
				byIdx := make(map[int]WriteBackArbitration, len(verdicts))
				for _, v := range verdicts {
					if v.FactIndex >= 0 && v.FactIndex < len(appends) {
						byIdx[v.FactIndex] = v
					}
				}
				kept := appends[:0]
				for j, a := range appends {
					v, ok := byIdx[j]
					if ok && v.Verdict == "supersedes" && v.Confidence >= arbitrateSupersedeMinConfidence && v.TargetFactID != "" {
						marker := "fact_id: `" + v.TargetFactID + "`"
						lineageBlock := preserveSupersededFactID(a.block, facts[a.idx].FactID, v.TargetFactID)
						if nb, ok2 := replaceH2BlockContaining(body, marker, lineageBlock); ok2 && nb != body {
							oldBlock, _ := extractH2BlockContaining(body, marker)
							body = nb
							changed = true
							versions = append(versions, FactVersion{
								CollectionID: col.ID, DocID: doc.ID,
								FactID: v.TargetFactID, OldBody: oldBlock, NewBody: lineageBlock,
							})
							continue // supersede 生效：顶替旧段，不再追加
						}
					}
					if ok && v.Verdict == "contradicts" && v.Confidence >= arbitrateContradictMinConfidence {
						// 旧段不覆盖：新事实仍追加留痕，矛盾交人工二审（高风险提案）。
						u.recordConflictProposalLater(&proposals, doc.ID, rel, facts[a.idx], v)
					}
					kept = append(kept, a)
				}
				appends = kept
			}
		}
	}
	for _, a := range appends {
		body = strings.TrimRight(body, "\n") + "\n\n" + a.block + "\n"
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
	// M3 留痕：正文已落库，版本链/提案 best-effort 落库（失败仅 Warn）。
	for _, v := range versions {
		u.recordFactVersion(ctx, v.CollectionID, doc.ID, v.FactID, v.OldBody, v.NewBody)
	}
	for _, p := range proposals {
		u.recordConflictProposal(ctx, col.ID, doc.ID, p)
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

// 仲裁置信度门槛：supersedes 会顶替旧段（写操作），门槛从严；
// contradicts 仅留提案（不改正文语义），门槛放宽。
const (
	arbitrateSupersedeMinConfidence  = 0.8
	arbitrateContradictMinConfidence = 0.7
)

// preserveSupersededFactID keeps the original fact_id as the stable lineage
// identity. The incoming extraction id remains provenance only; changing it
// must not break later updates of the same assertion.
func preserveSupersededFactID(block, incomingFactID, targetFactID string) string {
	incomingFactID = strings.TrimSpace(incomingFactID)
	targetFactID = strings.TrimSpace(targetFactID)
	if incomingFactID == "" || targetFactID == "" || incomingFactID == targetFactID {
		return block
	}
	incomingMarker := "fact_id: `" + incomingFactID + "`"
	targetMarker := "fact_id: `" + targetFactID + "`\n- source_id: `" + incomingFactID + "`"
	return strings.Replace(block, incomingMarker, targetMarker, 1)
}

// recordConflictProposalLater 队列一条矛盾提案载荷（正文持久化成功后统一落库）。
func (u *Usecase) recordConflictProposalLater(proposals *[]map[string]any, docID, relPath string, f WriteBackFact, v WriteBackArbitration) {
	newFactKey := strings.TrimSpace(f.FactID)
	if newFactKey == "" {
		newFactKey = HashContent(strings.TrimSpace(f.Statement))
	}
	dedupKey := "conflict:fact:" + docID + ":" + strings.TrimSpace(v.TargetFactID) + ":" + newFactKey
	*proposals = append(*proposals, map[string]any{
		"dedup_key":      dedupKey,
		"doc_id":         docID,
		"rel_path":       relPath,
		"new_statement":  f.Statement,
		"new_fact_id":    f.FactID,
		"target_fact_id": v.TargetFactID,
		"confidence":     v.Confidence,
		"reason":         v.Reason,
		"arbiter":        "llm",
	})
}

// replaceH2BlockContaining 替换包含 marker 的 H2 小节整段（含小节标题行）为
// newBlock；marker 不存在返回 false。小节边界："\n## " 或文档开头的 "## "。
func replaceH2BlockContaining(body, marker, newBlock string) (string, bool) {
	start, end, ok := h2BlockBounds(body, marker)
	if !ok {
		return body, false
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

// extractH2BlockContaining 取出包含 marker 的 H2 小节整段原文（M3.1 旧段快照用）。
func extractH2BlockContaining(body, marker string) (string, bool) {
	start, end, ok := h2BlockBounds(body, marker)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(body[start:end]), true
}

// removeH2BlockContaining 删除包含 marker 的 H2 小节；marker 不存在返回 false。
func removeH2BlockContaining(body, marker string) (string, bool) {
	start, end, ok := h2BlockBounds(body, marker)
	if !ok {
		return body, false
	}
	prefix := strings.TrimRight(body[:start], "\n")
	suffix := strings.TrimLeft(body[end:], "\n")
	if prefix == "" {
		if suffix == "" {
			return "", true
		}
		return suffix + "\n", true
	}
	if suffix == "" {
		return prefix + "\n", true
	}
	return prefix + "\n\n" + suffix + "\n", true
}

// h2BlockBounds 定位包含 marker 的 H2 小节区间 [start, end)；marker 不存在或位于
// 前言区（非事实块）返回 ok=false。
func h2BlockBounds(body, marker string) (start, end int, ok bool) {
	idx := strings.Index(body, marker)
	if idx < 0 {
		return 0, 0, false
	}
	start = strings.LastIndex(body[:idx], "\n## ")
	if start < 0 {
		if !strings.HasPrefix(body, "## ") {
			return 0, 0, false // marker 位于前言区，非事实块
		}
	} else {
		start++ // 越过 '\n' 指向 '#'
	}
	end = len(body)
	if nxt := strings.Index(body[start+3:], "\n## "); nxt >= 0 {
		end = start + 3 + nxt
	}
	return start, end, true
}

// factIDMarkerPrefix 事实段内 fact_id 标记前缀（FormatWriteBackAppendix 产出形态）。
const factIDMarkerPrefix = "fact_id: `"

// extractFactBlocks 枚举词条页内带 fact_id 标记的 H2 事实段（M3.2 仲裁候选）。
// 无 ID 段不作候选（不可作 supersede 目标——顶替需定位键）。
func extractFactBlocks(body string) []WriteBackFactBlock {
	var out []WriteBackFactBlock
	// 切段：文档开头 "## " 或 "\n## " 为界。
	var spans []int
	if strings.HasPrefix(body, "## ") {
		spans = append(spans, 0)
	}
	off := 0
	for {
		i := strings.Index(body[off:], "\n## ")
		if i < 0 {
			break
		}
		spans = append(spans, off+i+1)
		off += i + 1
	}
	for i, s := range spans {
		end := len(body)
		if i+1 < len(spans) {
			end = spans[i+1]
		}
		block := strings.TrimSpace(body[s:end])
		mi := strings.Index(block, factIDMarkerPrefix)
		if mi < 0 {
			continue
		}
		rest := block[mi+len(factIDMarkerPrefix):]
		factID := rest
		if ti := strings.Index(rest, "`"); ti >= 0 {
			factID = rest[:ti]
		}
		factID = strings.TrimSpace(factID)
		if factID == "" || factID == "stmt" {
			continue // 占位键非真实 ID（与 upsert 主流程同纪律）
		}
		heading := block
		if nl := strings.Index(block, "\n"); nl >= 0 {
			heading = block[:nl]
		}
		out = append(out, WriteBackFactBlock{
			Heading: strings.TrimSpace(strings.TrimPrefix(heading, "##")),
			Body:    block,
			FactID:  factID,
		})
	}
	return out
}
