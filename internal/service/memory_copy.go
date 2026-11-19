package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// CopyMemoryFact 跨域复制一条 L3 事实（D3 拍板 acceptance #3：copyMemory
// 等价物——note 从 user 域复制到 workspace）。双侧权限校验：
//
//  1. 源 scope 只读——先取源行解析其 scope，再 authorizeMemoryScope(source,
//     write=false)；行解析失败/ACL 拒绝即返回，行内容不外泄、不进入复制链。
//  2. 目标 scope 写——authorizeMemoryScope(target, write=true) 归一化
//     target_scope_id（user 域强制为调用者本人，workspace 域强制本租户）。
//  3. user 域 PII 门兜底：源行 pii_flag=1 且 scope_type=user 直接拒绝
//     （隔离态行唯一的读者路径是 admin ListPIIFlaggedFacts）。
//
// 注意：本 rpc 的 proto 已标注 (aranea.auth.v1.requirement)=memory.write，
// 但 memory 域暂不在 protoc-gen-aranea-auth 的敏感域清单内，middleware
// 硬判未生效——目标写权限当前由上面的 authorizeMemoryScope(write=true)
// 承担。memory 域全注解普查（含角色映射补全）列为 W4 评审待决项。
func (s *MemoryService) CopyMemoryFact(ctx context.Context, req *v1.CopyMemoryFactRequest) (*v1.CopyMemoryFactResponse, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	sourceFactID := strings.TrimSpace(req.GetSourceFactId())
	if sourceFactID == "" {
		return nil, apierror.BadRequest(apierror.DomainMemory, "source_fact_id is required")
	}
	targetScopeType := strings.ToLower(strings.TrimSpace(req.GetTargetScopeType()))
	if targetScopeType == "" {
		return nil, apierror.BadRequest(apierror.DomainMemory, "target_scope_type is required")
	}

	// ---- 源 scope 只读 ACL：PeekFactScope 只回 scope 三元组（最小披露），
	// 判定通过后才进入完整取行复制链；拒绝即无任何行内容外泄 ----
	srcScopeType, srcScopeID, srcPII, err := s.admin.PeekFactScope(ctx, sourceFactID)
	if err != nil {
		return nil, err
	}
	if srcPII && srcScopeType == "user" {
		return nil, apierror.Forbidden(apierror.DomainMemory,
			"PII-flagged user facts are quarantined and cannot be copied across scopes")
	}
	if _, err := authorizeMemoryScope(ctx, srcScopeType, srcScopeID, false); err != nil {
		return nil, err
	}

	// ---- 目标 scope 写 ACL（与 UpsertMemoryFact 同序）----
	targetScopeID, err := authorizeMemoryScope(ctx, targetScopeType, req.GetTargetScopeId(), true)
	if err != nil {
		return nil, err
	}
	if err := s.assertAgentScopeIfAgent(ctx, targetScopeType, targetScopeID); err != nil {
		return nil, err
	}
	wsID, err := authorizeMemoryWorkspaceField(ctx, "")
	if err != nil {
		return nil, err
	}

	// agent 目标 scope：agent_id 列归因到该 agent（与 UpsertMemoryFact 的
	// AgentID 语义一致），跨租户已由 assertAgentScopeIfAgent 拦截。
	targetAgentID := ""
	if targetScopeType == "agent" {
		targetAgentID = targetScopeID
	}

	raw, err := s.admin.CopyFactRow(ctx, biz.FactCopy{
		SourceFactID:      sourceFactID,
		TargetScopeType:   targetScopeType,
		TargetScopeID:     targetScopeID,
		TargetAgentID:     targetAgentID,
		TargetWorkspaceID: wsID,
		Statement:         strings.TrimSpace(req.GetStatement()),
		DetailsMarkdown:   req.GetDetailsMarkdown(),
		FactKind:          strings.TrimSpace(req.GetFactKind()),
	})
	if err != nil {
		return nil, err
	}
	pb, err := pbMemoryFact(raw)
	if err != nil || pb == nil {
		return nil, apierror.Internal(apierror.DomainMemory, "failed to hydrate fact after copy")
	}
	return &v1.CopyMemoryFactResponse{Fact: pb}, nil
}
