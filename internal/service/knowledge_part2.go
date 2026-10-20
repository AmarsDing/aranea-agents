package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"context"
	"encoding/json"
	"strings"
)

// extractText 经 ExtractorRegistry 按模态路由提取文本；
// registry 为 nil 时退化 TextExtractor（单测/兼容路径）。
func (s *KnowledgeService) extractText(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	if s.extractors != nil {
		return s.extractors.Extract(ctx, raw, source, mimeType)
	}
	return knowledge.ExtractDocumentText(ctx, raw, source, mimeType)
}

// resolveIngestCollection 解析入库目标 Collection（US-14 上传免预选）：
// collectionID 非空时按原逻辑校验存在性；为空时懒创建「默认知识库」。
// Embedder 未配置时仍创建词法收件箱（team Vault），不阻断粘贴/拖入。
func (s *KnowledgeService) resolveIngestCollection(ctx context.Context, collectionID string) (biz.KnowledgeCollection, error) {
	if strings.TrimSpace(collectionID) != "" {
		return s.uc.GetCollection(ctx, collectionID)
	}
	model, dim := "", 0
	if s.embedderAdmin != nil {
		_, _, m, d, _, _ := s.embedderAdmin.Config()
		model, dim = m, d
	}

	ws := ""
	if !workspace.IsSystem(ctx) {
		ws = workspace.IDFromContext(ctx)
	}
	return s.uc.EnsureDefaultCollection(ctx, model, dim, ws)
}

// isImageIngest 报告本次入库是否为图片模态（委托 knowledge.IsImageSource）。
func isImageIngest(source, mimeType string) bool {
	return knowledge.IsImageSource(source, mimeType)
}

// mergeIngestMetadata 将用户 metadata 与系统模态标记（modality/extractor）
// 合并为一个 JSON 对象；用户字段与系统键冲突时以系统键为准（血缘可信）。
func mergeIngestMetadata(userJSON, modality, extractor string) (string, error) {
	merged := map[string]any{}
	if s := strings.TrimSpace(userJSON); s != "" {
		if !json.Valid([]byte(s)) {
			return "", apierror.BadRequest(apierror.DomainKnowledge, "metadata_json must be valid JSON")
		}
		if err := json.Unmarshal([]byte(s), &merged); err != nil {
			return "", apierror.BadRequest(apierror.DomainKnowledge, "metadata_json must be a JSON object")
		}
	}
	merged["modality"] = modality
	merged["extractor"] = extractor
	out, err := json.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
