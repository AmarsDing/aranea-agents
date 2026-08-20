package data

// 自治理知识图谱 M3 演化时序层持久化：supersedes 版本链旧段快照 + 治理提案。
// 留痕派生数据：best-effort 写入（biz 层失败仅 Warn），可审计可回滚，不影响写回主流程。

import (
	"context"
	"encoding/json"
	"strings"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

var (
	_ bizknowledge.FactVersionRepo        = (*knowledgeRepo)(nil)
	_ bizknowledge.GovernanceProposalRepo = (*knowledgeRepo)(nil)
)

// InsertFactVersion 留痕一条 supersedes 版本（旧段快照 + 新段）。
func (r *knowledgeRepo) InsertFactVersion(ctx context.Context, v bizknowledge.FactVersion) error {
	if strings.TrimSpace(v.DocID) == "" || strings.TrimSpace(v.OldBody) == "" {
		return nil
	}
	var factID *string
	if id := strings.TrimSpace(v.FactID); id != "" {
		factID = &id
	}
	_, err := r.data.Postgres().ExecContext(ctx,
		`INSERT INTO knowledge_fact_version (collection_id, doc_id, fact_id, old_body, new_body)
		 VALUES ($1,$2,$3,$4,$5)`,
		v.CollectionID, v.DocID, factID, v.OldBody, v.NewBody)
	if err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	return nil
}

// InsertProposal 留痕一条治理提案（payload 序列化为 JSONB；空载荷落 {}）。
// Status 空默认 pending；applied/rejected 时 resolved_at 一并落 NOW()。
func (r *knowledgeRepo) InsertProposal(ctx context.Context, p bizknowledge.GovernanceProposal) error {
	if strings.TrimSpace(p.CollectionID) == "" || strings.TrimSpace(p.Kind) == "" {
		return nil
	}
	payload, err := json.Marshal(p.Payload)
	if err != nil {
		payload = []byte("{}")
	}
	risk := strings.TrimSpace(p.Risk)
	if risk == "" {
		risk = bizknowledge.ProposalRiskHigh
	}
	status := strings.TrimSpace(p.Status)
	if status == "" {
		status = bizknowledge.ProposalStatusPending
	}
	_, err = r.data.Postgres().ExecContext(ctx,
		`INSERT INTO knowledge_governance_proposal (collection_id, kind, payload, risk, status, resolved_at)
		 VALUES ($1,$2,$3::jsonb,$4,$5, CASE WHEN $5 IN ('applied','rejected') THEN NOW() ELSE NULL END)`,
		p.CollectionID, p.Kind, string(payload), risk, status)
	if err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	return nil
}
