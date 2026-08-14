package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// SP7 G1：把 L3 活动事实只读投影为团队库 agents/{agentID}.md（覆盖写，不追加）。
// 独立 struct，不往 Usecase 堆字段（AS-COG-01）。失败不得阻断 AutoMemory。

const (
	AgentMemoryRelPrefix    = "agents/"
	agentMemoryProjectLimit = 80
	agentMemoryMinRunes     = 4
)

// AgentFactLister 按 agent 列出活动 L3 事实（生产：service 适配 biz.L3FactReader）。
// Stability: evolving
type AgentFactLister interface {
	ListAgentFacts(ctx context.Context, agentID string, limit int) ([]WriteBackFact, error)
}

// AgentMemoryProjectPort 会话后投影入口（AutoMemoryWorker 依赖此窄接口）。
// Stability: evolving
type AgentMemoryProjectPort interface {
	ProjectAgentMemory(ctx context.Context, workspace, agentID string) error
}

// AgentMemoryProjectResult 投影落点（供 service 重放 chunk/FTS）。
type AgentMemoryProjectResult struct {
	CollectionID string
	DocID        string
	FactCount    int
}

var _ AgentMemoryProjectPort = (*AgentMemoryProjector)(nil)

// AgentMemoryProjector 记忆 → agent vault 投影器。
type AgentMemoryProjector struct {
	uc    *Usecase
	facts AgentFactLister
	lg    loggateway.Logger
}

// NewAgentMemoryProjector 构造投影器。uc 或 facts 为 nil 时 Project 为 no-op。
func NewAgentMemoryProjector(uc *Usecase, facts AgentFactLister, lg loggateway.Logger) *AgentMemoryProjector {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &AgentMemoryProjector{uc: uc, facts: facts, lg: lg}
}

// ProjectAgentMemory 实现 AgentMemoryProjectPort。
func (p *AgentMemoryProjector) ProjectAgentMemory(ctx context.Context, workspace, agentID string) error {
	_, err := p.Project(ctx, workspace, agentID)
	return err
}

// Project 覆盖写入 agents/{safeID}.md。无事实时仍写空投影（标明只读）。
func (p *AgentMemoryProjector) Project(ctx context.Context, workspace, agentID string) (AgentMemoryProjectResult, error) {
	if p == nil || p.uc == nil || p.facts == nil {
		return AgentMemoryProjectResult{}, nil
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentMemoryProjectResult{}, nil
	}
	rows, err := p.facts.ListAgentFacts(ctx, agentID, agentMemoryProjectLimit)
	if err != nil {
		return AgentMemoryProjectResult{}, err
	}
	col, err := p.uc.resolveWriteBackCollection(ctx, workspace)
	if err != nil {
		return AgentMemoryProjectResult{}, err
	}
	rel := AgentMemoryRelPath(agentID)
	body := FormatAgentMemoryProjection(agentID, rows, time.Now())
	body = p.uc.MaybeAutolinkOutgoing(ctx, col.ID, "", rel, body)

	doc, err := p.uc.documents.GetDocumentByRelPath(ctx, col.ID, rel)
	if err != nil {
		if !apierror.IsCode(err, apierror.CodeNotFound) {
			return AgentMemoryProjectResult{}, err
		}
		_, err = p.uc.CreateDocument(ctx, Document{
			CollectionID: col.ID,
			RelPath:      rel,
			Source:       rel,
			MimeType:     "text/markdown",
			ContentText:  body,
			Organized:    true,
			Status:       "pending",
		})
		if err != nil {
			return AgentMemoryProjectResult{}, err
		}
		doc, err = p.uc.documents.GetDocumentByRelPath(ctx, col.ID, rel)
		if err != nil {
			return AgentMemoryProjectResult{}, err
		}
	} else if err := p.uc.documents.UpdateDocumentContent(ctx, doc.ID, body, true); err != nil {
		return AgentMemoryProjectResult{}, err
	}
	if err := p.uc.RebuildBlockIndex(ctx, col.ID, doc.ID, body); err != nil {
		p.lg.Warn("agent 记忆投影块索引重建失败（正文已落）",
			loggateway.StepID("knowledge.memory.project"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err),
		)
	}
	p.lg.Info("agent 记忆已投影到知识库",
		loggateway.StepID("knowledge.memory.project"),
		loggateway.Str("collection_id", col.ID),
		loggateway.Str("doc_id", doc.ID),
		loggateway.Str("agent_id", agentID),
		loggateway.Int("facts", len(rows)),
	)
	return AgentMemoryProjectResult{CollectionID: col.ID, DocID: doc.ID, FactCount: len(rows)}, nil
}

// AgentMemoryRelPath 投影相对路径（纯函数）。
func AgentMemoryRelPath(agentID string) string {
	return AgentMemoryRelPrefix + sanitizeAgentPathID(agentID) + ".md"
}

func sanitizeAgentPathID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if out == "" {
		return "unknown"
	}
	return out
}

// FormatAgentMemoryProjection 渲染只读投影 Markdown（覆盖写，纯函数）。
func FormatAgentMemoryProjection(agentID string, facts []WriteBackFact, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nprojection: agent-memory\nagent_id: %s\nupdated: %s\n---\n\n",
		agentID, now.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "# Agent %s\n\n", agentID)
	b.WriteString("> 只读投影，来自自动记忆 L3 活动事实。请勿手工编辑；下次会话结束会覆盖。\n\n")
	if len(facts) == 0 {
		b.WriteString("_暂无活动事实。_\n")
		return b.String()
	}
	byKind := map[string][]WriteBackFact{}
	order := make([]string, 0)
	for _, f := range facts {
		stmt := strings.TrimSpace(f.Statement)
		if stmt == "" || utf8Len(stmt) < agentMemoryMinRunes {
			continue
		}
		kind := strings.TrimSpace(f.FactKind)
		if kind == "" {
			kind = "fact"
		}
		if _, ok := byKind[kind]; !ok {
			order = append(order, kind)
		}
		byKind[kind] = append(byKind[kind], f)
	}
	for _, kind := range order {
		fmt.Fprintf(&b, "## %s\n\n", kind)
		for _, f := range byKind[kind] {
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(f.Statement))
			if f.FactID != "" {
				fmt.Fprintf(&b, "  - fact_id: `%s`\n", f.FactID)
			}
			fmt.Fprintf(&b, "  - confidence: %.2f\n", f.Confidence)
			fmt.Fprintf(&b, "  - agent_id: `%s`\n", agentID)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
