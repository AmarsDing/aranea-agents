package knowledge

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

const entityWikiMaxPages = 8

// EnsureEntityWikiPages 团队库实体抽取成功后，为每个实体确保 entries/<slug>.md
// 短页存在，并幂等追加来源文档 wikilink。失败 Warn 不回滚检索/实体轨。
// 本地 vault 不写文件（避免改用户磁盘）。
func (u *Usecase) EnsureEntityWikiPages(ctx context.Context, collectionID, sourceDocID string, entities []DocEntity) error {
	if u == nil || u.collections == nil || u.documents == nil {
		return nil
	}
	collectionID = strings.TrimSpace(collectionID)
	sourceDocID = strings.TrimSpace(sourceDocID)
	if collectionID == "" || sourceDocID == "" || len(entities) == 0 {
		return nil
	}
	col, err := u.collections.GetCollection(ctx, collectionID)
	if err != nil {
		return err
	}
	if col.VaultBackend != VaultBackendTeam {
		return nil
	}
	src, err := u.documents.GetDocument(ctx, sourceDocID)
	if err != nil {
		return err
	}
	sourceRel := strings.TrimSpace(src.RelPath)
	if sourceRel == "" {
		sourceRel = strings.TrimSpace(src.Source)
	}
	written := 0
	for _, en := range entities {
		if written >= entityWikiMaxPages {
			break
		}
		name := strings.TrimSpace(en.Name)
		slug := entrySlug(name)
		if slug == "" {
			continue
		}
		rel := writeBackEntryDir + "/" + slug + ".md"
		if strings.TrimSpace(src.RelPath) == rel {
			continue
		}
		ok, err := u.ensureOneEntityWiki(ctx, col, rel, name, en.EntityType, sourceRel)
		if err != nil {
			u.lg.Warn("实体词条页写入失败（检索不受影响）",
				loggateway.StepID("knowledge.entity.wiki"),
				loggateway.Str("rel_path", rel),
				loggateway.Err(err),
			)
			continue
		}
		if ok {
			written++
		}
	}
	return nil
}

func (u *Usecase) ensureOneEntityWiki(ctx context.Context, col Collection, rel, title, entityType, sourceRel string) (bool, error) {
	doc, err := u.documents.GetDocumentByRelPath(ctx, col.ID, rel)
	created := false
	body := ""
	if err != nil {
		if !apierror.IsCode(err, apierror.CodeNotFound) {
			return false, err
		}
		created = true
	} else {
		body = doc.ContentText
	}
	next, changed := ensureEntityWikiBody(body, title, entityType, sourceRel)
	if !changed {
		return false, nil
	}
	if created {
		doc, err = u.CreateDocument(ctx, Document{
			CollectionID: col.ID,
			RelPath:      rel,
			Source:       rel,
			MimeType:     "text/markdown",
			ContentText:  next,
			Organized:    true,
			Status:       "pending",
		})
		if err != nil {
			return false, err
		}
	} else if err := u.documents.UpdateDocumentContent(ctx, doc.ID, next, true); err != nil {
		return false, err
	}
	if err := u.RebuildBlockIndex(ctx, col.ID, doc.ID, next); err != nil {
		u.lg.Warn("实体词条页块索引重建失败（正文已落）",
			loggateway.StepID("knowledge.entity.wiki"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err),
		)
	}
	return true, nil
}

func entityWikiWikilink(sourceRel string) string {
	sourceRel = strings.TrimSpace(strings.ReplaceAll(sourceRel, "\\", "/"))
	sourceRel = strings.TrimSuffix(sourceRel, ".md")
	sourceRel = strings.TrimSuffix(sourceRel, ".markdown")
	return "[[" + sourceRel + "]]"
}

// ensureEntityWikiBody 幂等追加来源提及。纯函数，供单测。
func ensureEntityWikiBody(body, title, entityType, sourceRel string) (string, bool) {
	title = strings.TrimSpace(title)
	if title == "" {
		return body, false
	}
	link := entityWikiWikilink(sourceRel)
	if strings.TrimSpace(body) == "" {
		var b strings.Builder
		b.WriteString(writeBackEntryHeader([]string{title}))
		if t := strings.TrimSpace(entityType); t != "" {
			b.WriteString("\n类型：`")
			b.WriteString(t)
			b.WriteString("`\n")
		}
		b.WriteString("\n## 来源\n\n- ")
		b.WriteString(link)
		b.WriteString("\n")
		return b.String(), true
	}
	if strings.Contains(body, link) {
		return body, false
	}
	if strings.Contains(body, "## 来源") {
		return strings.TrimRight(body, "\n") + "\n- " + link + "\n", true
	}
	return strings.TrimRight(body, "\n") + "\n\n## 来源\n\n- " + link + "\n", true
}
