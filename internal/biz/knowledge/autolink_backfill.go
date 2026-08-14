package knowledge

import (
	"context"
	"strings"
	"unicode/utf8"

	"aranea-agents/pkg/loggateway"
)

// 历史文档离线成链（US-38）与确认后写回本页出链（US-39）。
// 全量 RebuildCollectionBlockIndex 仍 allowBackfill=false，不改源文本；
// 本流程是显式「编译双链」写路径，随后由块索引重建吃到新的 [[wikilink]]。

const autolinkPreviewMaxRunes = 2000

// AutolinkBackfillResult 集合级出链回填统计。
type AutolinkBackfillResult struct {
	Scanned      int
	Changed      int
	Replacements int
	Failed       int
}

// AutolinkPreview 单文档出链预览（不落盘）。
type AutolinkPreview struct {
	DocID        string
	Replacements int
	Preview      string
	Unchanged    bool
}

// AutolinkApplyResult 单文档确认成链结果。
type AutolinkApplyResult struct {
	DocID        string
	Replacements int
}

// PreviewOutgoingAutolinks 预览把本文未链接提及编成 [[wikilink]] 的结果。
func (u *Usecase) PreviewOutgoingAutolinks(ctx context.Context, docID string) (AutolinkPreview, error) {
	if err := u.requireRepo(); err != nil {
		return AutolinkPreview{}, err
	}
	doc, col, content, err := u.autolinkSource(ctx, docID)
	if err != nil {
		return AutolinkPreview{}, err
	}
	linked, n := u.AutolinkOutgoing(ctx, col.ID, doc.ID, "", content)
	return AutolinkPreview{
		DocID:        doc.ID,
		Replacements: n,
		Preview:      truncateRunes(linked, autolinkPreviewMaxRunes),
		Unchanged:    n == 0,
	}, nil
}

// ApplyOutgoingAutolinks 确认后把本文未链接提及写入（local=CAS 文件，team=PG）。
func (u *Usecase) ApplyOutgoingAutolinks(ctx context.Context, docID string) (AutolinkApplyResult, error) {
	if err := u.requireRepo(); err != nil {
		return AutolinkApplyResult{}, err
	}
	doc, col, _, err := u.autolinkSource(ctx, docID)
	if err != nil {
		return AutolinkApplyResult{}, err
	}
	n, err := u.applyOutgoingOnDoc(ctx, col, doc)
	if err != nil {
		return AutolinkApplyResult{DocID: doc.ID}, err
	}
	return AutolinkApplyResult{DocID: doc.ID, Replacements: n}, nil
}

// BackfillOutgoingAutolinks 扫描集合全部可成链文档并持久化出链。
// 单文档失败计数后继续；不改 RebuildCollectionBlockIndex 的「重建不改源」契约。
func (u *Usecase) BackfillOutgoingAutolinks(ctx context.Context, collectionID string, onProgress func(done, total, failed int)) (AutolinkBackfillResult, error) {
	if err := u.requireRepo(); err != nil {
		return AutolinkBackfillResult{}, err
	}
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return AutolinkBackfillResult{}, ErrCollectionIDRequired
	}
	col, err := u.collections.GetCollection(ctx, collectionID)
	if err != nil {
		return AutolinkBackfillResult{}, err
	}
	var res AutolinkBackfillResult
	for offset := 0; ; offset += rebuildPageSize {
		docs, total, err := u.documents.ListDocuments(ctx, collectionID, rebuildPageSize, offset)
		if err != nil {
			return res, err
		}
		if res.Scanned == 0 {
			res.Scanned = 0
		}
		_ = total
		for _, summary := range docs {
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			if !isAutolinkableDoc(summary) {
				continue
			}
			full, gerr := u.documents.GetDocument(ctx, summary.ID)
			if gerr != nil {
				res.Failed++
				if onProgress != nil {
					onProgress(res.Changed+res.Failed, total, res.Failed)
				}
				continue
			}
			n, aerr := u.applyOutgoingOnDoc(ctx, col, full)
			if aerr != nil {
				res.Failed++
				u.lg.Warn("历史成链回填单文档失败",
					loggateway.StepID("knowledge.autolink.backfill"),
					loggateway.Str("doc_id", full.ID),
					loggateway.Err(aerr),
				)
			} else {
				if n > 0 {
					res.Changed++
					res.Replacements += n
				}
			}
			res.Scanned++
			if onProgress != nil {
				onProgress(res.Changed+res.Failed, total, res.Failed)
			}
		}
		if len(docs) < rebuildPageSize {
			break
		}
	}
	u.lg.Info("历史成链回填完成",
		loggateway.StepID("knowledge.autolink.backfill"),
		loggateway.Str("collection_id", collectionID),
		loggateway.Int("scanned", res.Scanned),
		loggateway.Int("changed", res.Changed),
		loggateway.Int("replacements", res.Replacements),
		loggateway.Int("failed", res.Failed),
	)
	return res, nil
}

func (u *Usecase) autolinkSource(ctx context.Context, docID string) (Document, Collection, string, error) {
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return Document{}, Collection{}, "", ErrIDRequired
	}
	doc, err := u.documents.GetDocument(ctx, docID)
	if err != nil {
		return Document{}, Collection{}, "", err
	}
	col, err := u.collections.GetCollection(ctx, doc.CollectionID)
	if err != nil {
		return Document{}, Collection{}, "", err
	}
	content := doc.ContentText
	if isLocalVaultFile(col, doc) && u.filer != nil {
		if body, _, rerr := u.GetVaultDocumentRaw(ctx, doc.ID); rerr == nil && body != "" {
			content = body
		}
	}
	return doc, col, content, nil
}

func (u *Usecase) applyOutgoingOnDoc(ctx context.Context, col Collection, doc Document) (int, error) {
	if !isAutolinkableDoc(doc) {
		return 0, nil
	}
	content := doc.ContentText
	baseHash := ""
	if isLocalVaultFile(col, doc) && u.filer != nil {
		body, hash, err := u.GetVaultDocumentRaw(ctx, doc.ID)
		if err == nil {
			content = body
			baseHash = hash
		}
	}
	linked, n := u.AutolinkOutgoing(ctx, col.ID, doc.ID, "", content)
	if n == 0 || linked == content {
		return 0, nil
	}
	if baseHash != "" {
		_, _, err := u.UpdateVaultDocumentContent(ctx, doc.ID, linked, baseHash)
		return n, err
	}
	if err := u.documents.UpdateDocumentContent(ctx, doc.ID, linked, true); err != nil {
		return n, err
	}
	if err := u.RebuildBlockIndex(ctx, col.ID, doc.ID, linked); err != nil {
		u.lg.Warn("成链后块索引重建失败（正文已落，重建可自愈）",
			loggateway.StepID("knowledge.autolink.backfill"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err),
		)
	}
	return n, nil
}

func isLocalVaultFile(col Collection, doc Document) bool {
	return col.VaultBackend != VaultBackendTeam &&
		strings.TrimSpace(col.RootPath) != "" &&
		strings.TrimSpace(doc.RelPath) != ""
}

func isAutolinkableDoc(d Document) bool {
	rel := strings.ToLower(strings.TrimSpace(d.RelPath))
	src := strings.ToLower(strings.TrimSpace(d.Source))
	mime := strings.ToLower(strings.TrimSpace(d.MimeType))
	if strings.HasSuffix(rel, ".md") || strings.HasSuffix(rel, ".markdown") || strings.HasSuffix(rel, ".txt") {
		return true
	}
	if strings.HasSuffix(src, ".md") || strings.HasSuffix(src, ".markdown") || strings.HasSuffix(src, ".txt") {
		return true
	}
	if mime == "" || strings.HasPrefix(mime, "text/") {
		return true
	}
	return false
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}
