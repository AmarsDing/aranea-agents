package knowledge

import (
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
	"fmt"
	"strings"
	"time"
)

// RetryDegradedEmbeddings 按指数退避重试熔断中文档的向量补齐（SP2 #9）。
// 熔断窗口内的文档跳过（不打 API）；成功复位熔断并回填向量，失败递增计数加深退避。
// 单文档失败不阻塞后续文档。无语义层/无 embedder 时 no-op。
func (a *VaultSyncApplier) RetryDegradedEmbeddings(ctx context.Context, vault bizknowledge.Collection) error {
	if a.embedder == nil || strings.TrimSpace(vault.EmbeddingModel) == "" {
		return nil
	}
	docs, err := a.uc.ListEmbedDegradedDocuments(ctx, vault.ID, embedRetryBatchSize)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, d := range docs {
		if !bizknowledge.EmbedCircuitAllow(d.EmbedFailCount, d.EmbedLastTried, now) {
			continue
		}
		if err := a.retryDocEmbeddings(ctx, d); err != nil {

			a.lg.Warn("vault embed retry failed; backoff deepened",
				loggateway.Str("vault_id", vault.ID),
				loggateway.Str("doc_id", d.ID),
				loggateway.Int("fail_count", d.EmbedFailCount+1),
				loggateway.Err(err),
			)
		}
	}
	return nil
}

// retryDocEmbeddings 重嵌单文档：从镜像正文重分块 → embed → 回填向量 → 复位熔断；
// 失败递增计数并刷新退避起点。
func (a *VaultSyncApplier) retryDocEmbeddings(ctx context.Context, doc bizknowledge.Document) error {
	splits, err := SplitWithStrategy(ChunkByMarkdown, doc.ContentText, 0, 0)
	if err != nil || len(splits) == 0 {
		return err
	}
	texts := make([]string, len(splits))
	for i, sp := range splits {
		texts[i] = sp.Content
	}
	vecs, err := a.embedder.Embed(ctx, texts)
	if err == nil && len(vecs) != len(splits) {
		err = apierror.Internal(apierror.DomainKnowledge,
			"vault embed retry: embedding count mismatch: expected %d, got %d", len(splits), len(vecs))
	}
	if err != nil {
		if uerr := a.uc.UpdateDocumentEmbedCircuit(ctx, doc.ID, doc.EmbedFailCount+1, time.Now()); uerr != nil {
			a.lg.Warn("vault sync: deepen embed circuit failed",
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(uerr),
			)
		}
		return err
	}

	updates := make([]bizknowledge.ChunkEmbedding, len(splits))
	for i, sp := range splits {
		updates[i] = bizknowledge.ChunkEmbedding{
			ChunkID:   fmt.Sprintf("%s-ch-%d", doc.ID, sp.ChunkIndex),
			Embedding: vecs[i],
		}
	}
	if err := a.uc.UpdateChunkEmbeddings(ctx, doc.ID, updates); err != nil {
		return err
	}
	return a.uc.UpdateDocumentEmbedCircuit(ctx, doc.ID, 0, time.Time{})
}
