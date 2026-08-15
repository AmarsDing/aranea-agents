package knowledge

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 低置信写回人工过门（US-44）：L3 已过 0.6，但未达自动沉淀 0.85 的白名单事实
// 进入 inbox/writeback-pending.md；用户确认后再追加当日日记（确认即门，不再二次过滤）。

const (
	writeBackReviewMinConfidence = 0.6
	WriteBackPendingRelPath      = "inbox/writeback-pending.md"
)

// PendingWriteBackItem 待确认写回条目。
type PendingWriteBackItem struct {
	Fact      WriteBackFact
	SessionID string
	AgentID   string
	UserID    string
}

// SplitWriteBackFacts 把候选拆成自动过门与待确认两档。
func SplitWriteBackFacts(facts []WriteBackFact) (pass, review []WriteBackFact) {
	pass = FilterWriteBackFacts(facts)
	passKey := make(map[string]struct{}, len(pass))
	for _, f := range pass {
		passKey[pendingFactKey(f)] = struct{}{}
	}
	review = make([]WriteBackFact, 0)
	for _, f := range facts {
		stmt := strings.TrimSpace(f.Statement)
		if stmt == "" || utf8Len(stmt) < writeBackMinRunes {
			continue
		}
		kind := strings.TrimSpace(f.FactKind)
		if _, ok := writeBackKinds[kind]; !ok {
			continue
		}
		if f.Confidence < writeBackReviewMinConfidence || f.Confidence >= writeBackMinConfidence {
			continue
		}
		f.Statement = stmt
		f.FactKind = kind
		if _, dup := passKey[pendingFactKey(f)]; dup {
			continue
		}
		review = append(review, f)
		if len(review) >= writeBackMaxFacts {
			break
		}
	}
	return pass, review
}

func pendingFactKey(f WriteBackFact) string {
	if id := strings.TrimSpace(f.FactID); id != "" {
		return "id:" + id
	}
	return "stmt:" + strings.ToLower(strings.TrimSpace(f.Statement))
}

func utf8Len(s string) int {
	return len([]rune(s))
}

// FormatPendingAppendix 渲染待确认 Markdown 块（纯函数）。
func FormatPendingAppendix(in WriteBackInput, facts []WriteBackFact) string {
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, f := range facts {
		id := strings.TrimSpace(f.FactID)
		if id == "" {
			id = "stmt"
		}
		fmt.Fprintf(&b, "## pending:`%s`\n\n%s\n\n", id, f.Statement)
		if f.FactID != "" {
			fmt.Fprintf(&b, "- fact_id: `%s`\n", f.FactID)
		}
		if in.SessionID != "" {
			fmt.Fprintf(&b, "- session_id: `%s`\n", in.SessionID)
		}
		if in.AgentID != "" {
			fmt.Fprintf(&b, "- agent_id: `%s`\n", in.AgentID)
		}
		if in.UserID != "" {
			fmt.Fprintf(&b, "- user_id: `%s`\n", in.UserID)
		}
		fmt.Fprintf(&b, "- confidence: %.2f\n", f.Confidence)
		fmt.Fprintf(&b, "- kind: %s\n", f.FactKind)
		if len(f.Tags) > 0 {
			fmt.Fprintf(&b, "- tags: %s\n", strings.Join(f.Tags, ", "))
		}
		src := strings.TrimSpace(f.SourceKind)
		if src == "" {
			src = "auto_memory"
		}
		fmt.Fprintf(&b, "- source: %s\n\n", src)
	}
	return b.String()
}

func writeBackPendingHeader() string {
	return "# 待确认写回\n\n" +
		"置信度 0.60–0.84 的白名单事实。确认后按 tags 入词条页（无 tags 进当日沉淀日记）；未确认不会自动入库。\n\n"
}

// ParsePendingWriteBackItems 从 pending 日记解析条目（纯函数）。
func ParsePendingWriteBackItems(body string) []PendingWriteBackItem {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	parts := strings.Split(body, "## pending:")
	if len(parts) < 2 {
		return nil
	}
	out := make([]PendingWriteBackItem, 0, len(parts)-1)
	for _, part := range parts[1:] {
		item := parsePendingPart(part)
		if strings.TrimSpace(item.Fact.Statement) == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func parsePendingPart(part string) PendingWriteBackItem {
	var item PendingWriteBackItem
	rest := part
	headIsPlaceholder := false
	if i := strings.Index(part, "\n"); i >= 0 {
		head := strings.TrimSpace(strings.Trim(part[:i], "`"))
		item.Fact.FactID = strings.Trim(head, "`")
		headIsPlaceholder = item.Fact.FactID == "stmt" // FormatPendingAppendix 无 fact_id 时的占位符
		rest = part[i+1:]
	}
	body := strings.TrimSpace(rest)
	var stmtLines []string
	sawFactIDField := false
	for _, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- ") {
			key, val := splitPendingField(trim[2:])
			switch key {
			case "fact_id":
				item.Fact.FactID = val
				sawFactIDField = true
			case "session_id":
				item.SessionID = val
			case "agent_id":
				item.AgentID = val
			case "user_id":
				item.UserID = val
			case "confidence":
				if c, err := strconv.ParseFloat(val, 64); err == nil {
					item.Fact.Confidence = c
				}
			case "kind":
				item.Fact.FactKind = val
			case "tags":
				for _, t := range strings.Split(val, ",") {
					if t = strings.TrimSpace(t); t != "" {
						item.Fact.Tags = append(item.Fact.Tags, t)
					}
				}
			case "source":
				item.Fact.SourceKind = val
			}
			continue
		}
		if trim == "" && len(stmtLines) > 0 {
			// 陈述与字段之间的空行：陈述结束
			if item.Fact.FactID != "" || item.Fact.FactKind != "" {
				continue
			}
		}
		if !strings.HasPrefix(trim, "- ") && (len(stmtLines) == 0 || item.Fact.FactKind == "") {
			if trim != "" && !strings.HasPrefix(trim, "#") {
				stmtLines = append(stmtLines, trim)
			}
		}
	}
	item.Fact.Statement = strings.TrimSpace(strings.Join(stmtLines, "\n"))
	// head 占位符 "stmt" 且无真实 fact_id 字段行 → 视为无 fact_id，
	// 避免后续 writeBackAlreadyPresent/replaceH2BlockContaining 用假键误顶替。
	if headIsPlaceholder && !sawFactIDField {
		item.Fact.FactID = ""
	}
	return item
}

func splitPendingField(s string) (key, val string) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return strings.TrimSpace(s), ""
	}
	key = strings.TrimSpace(s[:i])
	val = strings.Trim(strings.TrimSpace(s[i+1:]), "`")
	return key, val
}

func removePendingItems(body string, ids map[string]struct{}) string {
	if len(ids) == 0 {
		return body
	}
	parts := strings.Split(body, "## pending:")
	if len(parts) < 2 {
		return body
	}
	var b strings.Builder
	b.WriteString(parts[0])
	for _, part := range parts[1:] {
		item := parsePendingPart(part)
		key := strings.TrimSpace(item.Fact.FactID)
		if key == "" {
			key = pendingFactKey(item.Fact)
		}
		if _, drop := ids[key]; drop {
			continue
		}
		if _, drop := ids[pendingFactKey(item.Fact)]; drop {
			continue
		}
		b.WriteString("## pending:")
		b.WriteString(part)
	}
	return b.String()
}

// EnqueueWriteBackReview 把待确认事实追加到 pending 日记（失败不报错给调用方时由 worker Warn）。
func (u *Usecase) EnqueueWriteBackReview(ctx context.Context, in WriteBackInput) (WriteBackResult, error) {
	if err := u.requireRepo(); err != nil {
		return WriteBackResult{}, err
	}
	_, review := SplitWriteBackFacts(in.Facts)
	if len(review) == 0 {
		return WriteBackResult{}, nil
	}
	col, err := u.resolveWriteBackCollection(ctx, in.Workspace)
	if err != nil {
		return WriteBackResult{}, err
	}
	return u.upsertMarkdownDoc(ctx, col, WriteBackPendingRelPath, writeBackPendingHeader(), FormatPendingAppendix(in, review))
}

// ListPendingWriteBack 列出集合 pending 日记中的待确认条目。
func (u *Usecase) ListPendingWriteBack(ctx context.Context, collectionID string) ([]PendingWriteBackItem, error) {
	if err := u.requireRepo(); err != nil {
		return nil, err
	}
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return nil, ErrCollectionIDRequired
	}
	doc, err := u.documents.GetDocumentByRelPath(ctx, collectionID, WriteBackPendingRelPath)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return []PendingWriteBackItem{}, nil
		}
		return nil, err
	}
	return ParsePendingWriteBackItems(doc.ContentText), nil
}

// ApplyPendingWriteBack 把 pending 条目确认进当日日记。factIDs 空 = 全部确认。
func (u *Usecase) ApplyPendingWriteBack(ctx context.Context, collectionID string, factIDs []string) (WriteBackResult, error) {
	if err := u.requireRepo(); err != nil {
		return WriteBackResult{}, err
	}
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return WriteBackResult{}, ErrCollectionIDRequired
	}
	col, err := u.collections.GetCollection(ctx, collectionID)
	if err != nil {
		return WriteBackResult{}, err
	}
	pendingDoc, err := u.documents.GetDocumentByRelPath(ctx, collectionID, WriteBackPendingRelPath)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return WriteBackResult{CollectionID: collectionID}, nil
		}
		return WriteBackResult{}, err
	}
	items := ParsePendingWriteBackItems(pendingDoc.ContentText)
	want := make(map[string]struct{}, len(factIDs))
	for _, id := range factIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	selected := make([]PendingWriteBackItem, 0, len(items))
	appliedIDs := make(map[string]struct{}, len(items))
	for _, it := range items {
		id := strings.TrimSpace(it.Fact.FactID)
		if len(want) > 0 {
			if _, ok := want[id]; !ok {
				continue
			}
		}
		selected = append(selected, it)
		if id != "" {
			appliedIDs[id] = struct{}{}
		} else {
			appliedIDs[pendingFactKey(it.Fact)] = struct{}{}
		}
	}
	var last WriteBackResult
	appended := 0
	for _, it := range selected {
		res, aerr := u.writeBackFacts(ctx, col, WriteBackInput{
			Workspace: col.Workspace,
			SessionID: it.SessionID,
			AgentID:   it.AgentID,
			UserID:    it.UserID,
			Facts:     []WriteBackFact{it.Fact},
		}, []WriteBackFact{it.Fact})
		if aerr != nil {
			return last, aerr
		}
		last = res
		appended += res.Appended
	}
	last.Appended = appended
	last.CollectionID = col.ID
	if len(appliedIDs) > 0 {
		newBody := removePendingItems(pendingDoc.ContentText, appliedIDs)
		if err := u.documents.UpdateDocumentContent(ctx, pendingDoc.ID, newBody, true); err != nil {
			u.lg.Warn("待确认写回已归档但 pending 清理失败",
				loggateway.StepID("knowledge.writeback.pending"),
				loggateway.Str("doc_id", pendingDoc.ID),
				loggateway.Err(err),
			)
		} else {
			_ = u.RebuildBlockIndex(ctx, col.ID, pendingDoc.ID, newBody)
		}
	}
	last.CollectionID = col.ID
	return last, nil
}
