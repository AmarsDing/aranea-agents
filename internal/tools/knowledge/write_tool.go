package knowledge

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// knowledge_write（P1，2026-08-15 评审修订）：Agent 主动写知识库。
// 高置信（≥0.85）走 P0-1 同一词条 upsert 直写（词条页 + 日记 provenance）；
// 低置信（0.60~0.85）进 pending 队列等人确认（既有 HITL 审核链，不弹窗——
// 所有写入都弹窗模型就不会用了）；<0.60 直接拒绝。
// 身份安全：user/session 从 invocation 解析，LLM 不能指定写到谁名下。

const (
	knowledgeWriteDirectMinConfidence = 0.85 // 与 writeBackMinConfidence 同水位
	knowledgeWriteReviewMinConfidence = 0.6  // 与 writeBackReviewMinConfidence 同水位
	knowledgeWriteDefaultConfidence   = 0.95 // 显式写 = 高意图
	knowledgeWriteMinStatementRunes   = 8
	knowledgeWriteSourceKind          = "knowledge_write"
	knowledgeWriteMaxStatementRunes   = 500
	knowledgeWriteMaxTags             = 4
)

type knowledgeWriteInput struct {
	Statement  string   `json:"statement" jsonschema:"description=要写入知识库的事实陈述（一句话，原子、可独立理解）,required"`
	Tags       []string `json:"tags" jsonschema:"description=主题标签（首个为词条页主题；与已有词条 basename/title/aliases 命中时合并进该页）,required"`
	FactKind   string   `json:"fact_kind,omitempty" jsonschema:"description=事实类型，默认 decision,enum=decision,enum=constraint,enum=preference,enum=profile,enum=goal,enum=relationship"`
	Confidence float64  `json:"confidence,omitempty" jsonschema:"description=置信度 0~1：≥0.85 直接写入词条页；0.6~0.85 进待确认队列由人审核；<0.6 拒绝。默认 0.95"`
	FactID     string   `json:"fact_id,omitempty" jsonschema:"description=幂等键：同一 fact_id 再写入会替换词条页中的旧段落（用于更新已写过的事实）。留空时按陈述内容派生"`
}

type knowledgeWriteOutput struct {
	Status  string `json:"status"` // written | pending_review
	FactID  string `json:"fact_id"`
	Entry   string `json:"entry,omitempty"` // 落点词条名（直写时）
	Message string `json:"message,omitempty"`
}

// NewWriteTool 构建 knowledge_write 工具。uc 为 nil 时返回 nil（装配层跳过注册）。
func NewWriteTool(uc *biz.KnowledgeUsecase) trpctool.CallableTool {
	if uc == nil || uc.IsUnavailable() {
		return nil
	}
	execute := func(ctx context.Context, in knowledgeWriteInput) (knowledgeWriteOutput, error) {
		stmt := strings.TrimSpace(in.Statement)
		if stmt == "" {
			return knowledgeWriteOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_write: statement is required")
		}
		if r := utf8.RuneCountInString(stmt); r < knowledgeWriteMinStatementRunes || r > knowledgeWriteMaxStatementRunes {
			return knowledgeWriteOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_write: statement length must be 8~500 runes")
		}
		kind := strings.ToLower(strings.TrimSpace(in.FactKind))
		if kind == "" {
			kind = "decision"
		}
		switch kind {
		case "decision", "constraint", "preference", "profile", "goal", "relationship":
		default:
			return knowledgeWriteOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_write: fact_kind must be decision|constraint|preference|profile|goal|relationship")
		}
		tags := make([]string, 0, len(in.Tags))
		for _, t := range in.Tags {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		if len(tags) == 0 {
			return knowledgeWriteOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_write: tags is required（词条页按主题标签定位）")
		}
		if len(tags) > knowledgeWriteMaxTags {
			tags = tags[:knowledgeWriteMaxTags]
		}
		conf := in.Confidence
		if conf <= 0 {
			conf = knowledgeWriteDefaultConfidence
		}
		if conf > 1 {
			conf = 1
		}
		if conf < knowledgeWriteReviewMinConfidence {
			return knowledgeWriteOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_write: confidence < 0.6，事实可信度不足，请先核实再写")
		}

		factID := strings.TrimSpace(in.FactID)
		if factID == "" {
			factID = knowledgeWriteDerivedFactID(stmt)
		}
		userID, sessionID := knowledgeWriteInvocationIdentity(ctx)
		wbIn := biz.KnowledgeWriteBackInput{
			SessionID: sessionID,
			UserID:    userID,
			Workspace: workspace.IDFromContext(ctx),
			Facts: []biz.KnowledgeWriteBackFact{{
				FactID:     factID,
				Statement:  stmt,
				FactKind:   kind,
				Confidence: conf,
				SourceKind: knowledgeWriteSourceKind,
				Tags:       tags,
			}},
		}

		// 低置信 → pending 队列（既有 HITL 审核链：人在知识库页面确认后才落词条）。
		if conf < knowledgeWriteDirectMinConfidence {
			if _, err := uc.EnqueueWriteBackReview(ctx, wbIn); err != nil {
				return knowledgeWriteOutput{}, apierror.Internal(apierror.DomainKnowledge, "knowledge_write: "+err.Error())
			}
			return knowledgeWriteOutput{
				Status:  "pending_review",
				FactID:  factID,
				Message: "置信度低于直写阈值，已进待确认队列，需人在知识库页面审核后生效",
			}, nil
		}

		// 高置信 → P0-1 同一词条 upsert（词条页正文 + 日记 provenance）。
		res, err := uc.WriteBackSessionFacts(ctx, wbIn)
		if err != nil {
			return knowledgeWriteOutput{}, apierror.Internal(apierror.DomainKnowledge, "knowledge_write: "+err.Error())
		}
		return knowledgeWriteOutput{
			Status: "written",
			FactID: factID,
			Entry:  res.EntryOf(factID),
		}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("knowledge_write"),
		function.WithDescription("把一条高置信事实写入团队知识库词条页（按 tags 定位/新建词条，同一 fact_id 再写入会更新旧段落）。confidence≥0.85 直写；0.6~0.85 进待确认队列由人审核。用于会话中明确值得长期保留的结论、决定、约束；临时上下文不要写。"),
	)
}

// knowledgeWriteDerivedFactID 未显式给 fact_id 时按归一化陈述派生：同一陈述
// 重试/重放得到同一键，命中词条页 fact_id 替换语义（幂等，不重复追加）。
func knowledgeWriteDerivedFactID(stmt string) string {
	norm := strings.ToLower(strings.Join(strings.Fields(stmt), " "))
	sum := sha1.Sum([]byte(norm))
	return "kw-" + hex.EncodeToString(sum[:8])
}

// knowledgeWriteInvocationIdentity 从 trpc invocation 解析 user/session（同
// memoryremember.invocationIdentity 契约；身份永不取自工具入参）。
func knowledgeWriteInvocationIdentity(ctx context.Context) (userID, sessionID string) {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return "", ""
	}
	return strings.TrimSpace(inv.Session.UserID), strings.TrimSpace(inv.Session.ID)
}
