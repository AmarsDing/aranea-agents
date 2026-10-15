package biz

import (
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"context"
	"fmt"

	"strings"
	"unicode/utf8"
)

// UpstreamDeliverableSeed returns the cross-team deliverable seed for a
// downstream DAG team (2026-08-08 问题3 修复): the business topics of every
// COMPLETED upstream dependency's graph deliverable map, merged in DependsOn
// order (later deps win on topic collision, Warn-logged). Reserved keys
// (summary/cognition) and intra-team ack/* keys are never seeded — the
// downstream team's own summary must come from its own members, and ack
// signals are intra-team coordination, not content.
//
// The seed is installed into the downstream graph's initial deliverable
// state at turn start (Runner RuntimeState injection), so members can
// get_deliverable(topic=...) the upstream contract topics directly — the
// per-execution graph state is otherwise isolated, which was the 01:54 断链
// root cause (downstream get_deliverable always returned found=false).
//
// HasRealDeliverable / WriteDeliverablesToSession recompute and subtract
// this same seed (subtractUpstreamSeed): seed round-trip alone can never
// satisfy the real-deliverable gate, and upstream topics never leak into
// this team's own envelope.
//
// Conservative on infra failure: an unreadable upstream state skips that
// dependency (Warn) — the text injection prefix + read_upstream_deliverable
// tool remain as fallback. Domain: Delivery — cross-team state handoff.
func (d *SpiritDelivery) UpstreamDeliverableSeed(ctx context.Context, downstreamTeam Team) (map[string]any, error) {
	if len(downstreamTeam.DependsOn) == 0 || d.graphDelivReader == nil {
		return nil, nil
	}
	teams, err := d.teamUC.ListBySpiritSessionID(ctx, downstreamTeam.SpiritSessionID)
	if err != nil {
		return nil, err
	}
	teamByDagNode := make(map[string]Team, len(teams))
	for _, t := range teams {
		if t.DagNodeID != "" {
			teamByDagNode[t.DagNodeID] = t
		}
	}
	var seed map[string]any
	for _, depID := range downstreamTeam.DependsOn {
		upstream, ok := teamByDagNode[depID]
		if !ok || upstream.Status != TeamStatusCompleted {
			continue
		}
		anchor, ok := d.stateDeliverableChannel(upstream)
		if !ok {
			continue
		}
		sessID, serr := d.resolveTeamMainSessionID(ctx, upstream.ID)
		if serr != nil {
			d.lg.Warn("上游交付物种子：解析上游团队 session 失败，跳过该依赖",
				loggateway.StepID("spirit.upstream_seed"),
				loggateway.Str("upstream_team_id", upstream.ID),
				loggateway.Err(serr),
			)
			continue
		}
		if sessID == "" {
			continue
		}
		m, rerr := d.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), sessID)
		if rerr != nil {
			d.lg.Warn("上游交付物种子：读取上游 graph state 失败，跳过该依赖",
				loggateway.StepID("spirit.upstream_seed"),
				loggateway.Str("upstream_team_id", upstream.ID),
				loggateway.Err(rerr),
			)
			continue
		}
		for k, v := range m {
			if k == deliverableReservedKeySummary || k == deliverableReservedKeyCognition || strings.HasPrefix(k, deliverableAckKeyPrefix) {
				continue
			}
			if seed == nil {
				seed = make(map[string]any, len(m))
			}
			if _, dup := seed[k]; dup {
				d.lg.Warn("上游交付物 topic 冲突，按 DependsOn 顺序后者覆盖",
					loggateway.StepID("spirit.upstream_seed"),
					loggateway.Str("topic", k),
					loggateway.Str("upstream_team_id", upstream.ID),
				)
			}
			seed[k] = v
		}
	}
	return seed, nil
}

// InjectUpstreamDeliverables collects upstream team deliverables and formats
// them as a prefix for the downstream team's input message.
// Called when a DAG activates a downstream team.
// It first tries to read from the persisted deliverable output cache
// (written by WriteDeliverablesToSession), then falls back to
// extracting from the team output directly.
// Domain: Delivery — collect and format upstream deliverables for downstream team input.
func (d *SpiritDelivery) InjectUpstreamDeliverables(ctx context.Context, downstreamTeam Team) string {
	if len(downstreamTeam.DependsOn) == 0 {
		return ""
	}
	teams, err := d.teamUC.ListBySpiritSessionID(ctx, downstreamTeam.SpiritSessionID)
	if err != nil {
		return ""
	}

	teamByDagNode := make(map[string]Team, len(teams))
	for _, t := range teams {
		if t.DagNodeID != "" {
			teamByDagNode[t.DagNodeID] = t
		}
	}

	var deliverableParts []string
	for _, depID := range downstreamTeam.DependsOn {
		upstream, ok := teamByDagNode[depID]
		if !ok || upstream.Status != TeamStatusCompleted {
			continue
		}

		ref, refOK := d.readDeliverableRef(upstream)
		if !refOK || ref.Summary == "" {
			continue
		}

		if ref.TeamID == "" {
			ref.TeamID = upstream.ID
		}
		part := fmt.Sprintf("## 上游团队: %s\n%s%s", upstream.DisplayName, contractDeclarationLines(upstream.Deliverables), ref.Summary)
		part += renderArtifactSections(ref, downstreamTeam)
		if cog := renderCognitionLines(ref.Cognition); cog != "" {
			part += "\n" + cog
		}
		if ref.Truncated {
			part += fmt.Sprintf("\n[交付物全文共 %d 字符，以上为截断摘要。需要完整内容时调用 read_upstream_deliverable(team_id=\"%s\") 获取]",
				ref.SizeChars, ref.TeamID)
		}
		deliverableParts = append(deliverableParts, part)
	}

	if len(deliverableParts) == 0 {
		return ""
	}

	return fmt.Sprintf("--- 上游交付物 ---\n%s\n--- 请基于以上上游交付物执行任务 ---\n\n",
		strings.Join(deliverableParts, "\n\n"))
}

// DeliverableProtocolSuffix renders the mandatory delivery-protocol block
// appended to a DAG team's first-turn input (2026-07-25 Fix 2b). Without it a
// team has no way of knowing that "reply text is not a deliverable": the
// real-deliverable gate (Fix 1) flips completed-without-set_deliverable teams
// to failed, so the obligation must be declared up front. Non-DAG teams carry
// no deliverable obligation and get no suffix.
func (d *SpiritDelivery) DeliverableProtocolSuffix(t Team) string {
	if t.DagNodeID == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n--- 交付协议（强制） ---\n")
	sb.WriteString("本团队是 DAG 编排节点。任务完成前必须调用 set_deliverable 提交结构化交付物（只在回复文本中给出内容不算产出）。交付范式：摘要是指针，载荷是数据。\n")
	sb.WriteString("- \"summary\"（保留 key，必填）：交付物摘要，供下游团队与最终报告消费。固定写法：①一句话结论 ②3-5 条 \"- \" 要点 ③末行载荷清单（载荷：<topic>(<format>, <n>字)）。注意：摘要超过 500 字将被截断——正文严禁写进 summary，必须放入契约 topic 的 content 字段。\n")
	if contracts := contractSubmissionLines(t.Deliverables); contracts != "" {
		sb.WriteString("- 交付契约（逐项按指定 topic 提交，禁止自创 topic 名）：\n")
		sb.WriteString(contracts)
	}
	sb.WriteString("- 文档型交付物（报告/文章/复盘等长文）载荷固定结构：{\"title\": \"标题\", \"format\": \"markdown\", \"content\": \"完整正文\", \"meta\": {...}}——content 承载全文，不会被截断。\n")
	sb.WriteString("样例 1（短交付物，内容可全部写进摘要）：\n")
	sb.WriteString("  set_deliverable(data={\"summary\": \"完成服务器巡检，整体健康。\\n- CPU/内存正常\\n- 磁盘使用率 85%，建议扩容\\n载荷：无（结论即全文）\"})\n")
	sb.WriteString("样例 2（长文交付物，正文放契约 topic，摘要只写指针。注意顺序：先写摘要、后写 topic 载荷——不带 topic 的写入会整体覆盖本节点本地视图，topic 载荷写在最后可保住本地读回）：\n")
	sb.WriteString("  set_deliverable(data={\"summary\": \"《云计算十年》已完成并复核。\\n- 观点：云原生进入平台工程阶段\\n- 数据：引用 12 组行业数据\\n载荷：article(markdown, 8234字)\"})\n")
	sb.WriteString("  set_deliverable(topic=\"article\", data={\"title\": \"云计算十年\", \"format\": \"markdown\", \"content\": \"# 云计算十年\\n……（完整正文，8234 字）\"})\n")
	sb.WriteString("- Brief：summary 必填（一句话结论 + 要点）。大文件/二进制禁止写入 content 指望下游整段注入；把路径或制品列入同一 topic（rel_path / artifact_id），下游工作区 inbox/<上游团队ID>/ 仅含声明附件。\n")
	sb.WriteString("未调用 set_deliverable 的 completed 将被判定为 failed，下游团队不会收到输入；信息不足时在 summary 中如实写明阻塞或待澄清事项，禁止虚构交付物。\n")
	return sb.String()
}

// BuildTeamTurnInput composes a DAG team's first-turn input:
// upstream deliverable prefix + task description + delivery protocol suffix.
// The stored Team.TaskDescription stays pure — injection happens only on the
// turn input (both the orchestrator dispatch path and the lazy DAG activation
// path compose through this single function so the two cannot drift).
func (d *SpiritDelivery) BuildTeamTurnInput(ctx context.Context, t Team) string {
	d.materializeUpstreamInbox(ctx, t)
	taskDesc := t.TaskDescription
	prefix := d.InjectUpstreamDeliverables(ctx, t)
	protocol := d.DeliverableProtocolSuffix(t)

	trimmed := TrimPrefixBudget(PrefixBudget{Brief: prefix, Protocol: protocol})
	if trimmed.Brief != "" {
		taskDesc = trimmed.Brief + taskDesc
	}
	return taskDesc + trimmed.Protocol
}

// readDeliverableRef reads the persisted DeliverableRef for the team's own
// dag_node_id. Dual-mode: P2 envelopes parse with full metadata; legacy
// plain-string values yield a summary-only ref. ok=false when absent or
// unparseable.
// Domain: Delivery — read persisted deliverable envelope from team's cache.
func (d *SpiritDelivery) readDeliverableRef(t Team) (DeliverableRef, bool) {
	if t.DagNodeID == "" {
		return DeliverableRef{}, false
	}
	ref, ok := ParseDeliverableRefs(t.DeliverablesOutput)[t.DagNodeID]
	return ref, ok
}

// ReadUpstreamDeliverable returns the full deliverable text of a COMPLETED
// upstream team, truncated to maxChars (default DefaultUpstreamDeliverableMaxChars,
// hard cap MaxUpstreamDeliverableChars). Backs the read_upstream_deliverable
// tool: the injection prefix only carries a truncated summary, and downstream
// team members call the tool when they genuinely need the full text.
//
// 2026-07-25 Fix 7: the content source is the DELIVERABLE itself — graph
// state first, then the persisted envelope — never the reply text. After the
// Fix-1 gate "reply is not a deliverable", a reply-sourced full text would
// contradict the injection prefix (which renders the envelope summary).
//
// Phase B (runtime contract validation): when readerSessionID resolves to a
// reader team with a declared InputContract, that contract is checked against
// the upstream team's declared Deliverables BEFORE the (expensive) full-text
// extraction; a mismatch returns a structured *ContractMismatchError so the
// calling agent can auto-correct and retry.
// Domain: Delivery — full-text retrieval of an upstream team's deliverable.
func (d *SpiritDelivery) ReadUpstreamDeliverable(ctx context.Context, readerSessionID, teamID string, maxChars int) (UpstreamDeliverableContent, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "team_id is required")
	}
	if maxChars <= 0 || maxChars > MaxUpstreamDeliverableChars {
		maxChars = DefaultUpstreamDeliverableMaxChars
	}
	t, err := d.teamUC.Get(ctx, teamID)
	if err != nil {
		return UpstreamDeliverableContent{}, err
	}
	if t.Status != TeamStatusCompleted {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "upstream team %s has not completed yet (status=%s)", teamID, t.Status)
	}
	if err := d.validateUpstreamContract(ctx, readerSessionID, t); err != nil {
		return UpstreamDeliverableContent{}, err
	}
	full, err := d.resolveDeliverableFullContent(ctx, t)
	if err != nil {
		return UpstreamDeliverableContent{}, err
	}
	if full.Content == "" {
		return UpstreamDeliverableContent{}, apierror.NotFound("SPIRIT", "no deliverable content found for team %s", teamID)
	}

	if err := d.validateUpstreamContractSchema(ctx, readerSessionID, t, full.Content); err != nil {
		return UpstreamDeliverableContent{}, err
	}

	sizeChars := utf8.RuneCountInString(full.Content)
	content := full.Content
	truncated := sizeChars > maxChars
	if truncated {
		content = TruncateRunes(content, maxChars) + fmt.Sprintf("\n...[已截断，全文共 %d 字符]", sizeChars)
	}
	d.lg.Info("上游交付物全文读取",
		loggateway.StepID("spirit.read_upstream_deliverable"),
		loggateway.Str("team_id", teamID),
		loggateway.Int("size_chars", sizeChars),
		loggateway.Bool("truncated", truncated),
	)
	return UpstreamDeliverableContent{
		Content:   content,
		SizeChars: sizeChars,
		Truncated: truncated,
		TeamID:    teamID,
		SessionID: full.SessionID,
	}, nil
}

// ReadUpstreamDeliverableKey returns the full content of ONE payload key of
// an upstream team's deliverable (document-paradigm payloads render as
// title + content; other shapes as indented JSON). Same completion/contract
// gates as ReadUpstreamDeliverable. The keyed read exists for the long-
// article paradigm: downstream teams fetch exactly the contracted artifact
// instead of the summary+everything concatenation.
// Domain: Delivery — keyed full-text retrieval of an upstream deliverable payload.
func (d *SpiritDelivery) ReadUpstreamDeliverableKey(ctx context.Context, readerSessionID, teamID, key string, maxChars int) (UpstreamDeliverableContent, error) {
	teamID = strings.TrimSpace(teamID)
	key = strings.TrimSpace(key)
	if teamID == "" {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "team_id is required")
	}
	if key == "" {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "key is required")
	}
	if key == deliverableReservedKeySummary || key == deliverableReservedKeyCognition || strings.HasPrefix(key, deliverableAckKeyPrefix) {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "key %q 为保留 key：摘要已在注入前缀中，cognition/ack 不支持按 key 读取", key)
	}
	if maxChars <= 0 || maxChars > MaxUpstreamDeliverableChars {
		maxChars = DefaultUpstreamDeliverableMaxChars
	}
	t, err := d.teamUC.Get(ctx, teamID)
	if err != nil {
		return UpstreamDeliverableContent{}, err
	}
	if t.Status != TeamStatusCompleted {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "upstream team %s has not completed yet (status=%s)", teamID, t.Status)
	}
	if err := d.validateUpstreamContract(ctx, readerSessionID, t); err != nil {
		return UpstreamDeliverableContent{}, err
	}
	full, available, found, err := d.resolveDeliverableKeyContent(ctx, t, key)
	if err != nil {
		return UpstreamDeliverableContent{}, err
	}
	if full.Content == "" {
		switch {
		case found:
			return UpstreamDeliverableContent{}, apierror.NotFound("SPIRIT", "deliverable key %q 内容为空（team %s）：载荷存在但无正文；或不带 key 调用 read_upstream_deliverable 查看交付物整体", key, teamID)
		case len(available) > 0:
			return UpstreamDeliverableContent{}, apierror.NotFound("SPIRIT", "deliverable key %q not found for team %s（可用 key：%s；或不带 key 调用 read_upstream_deliverable 读取交付物全文）", key, teamID, strings.Join(available, ", "))
		default:
			return UpstreamDeliverableContent{}, apierror.NotFound("SPIRIT", "no deliverable content found for team %s（可不带 key 调用 read_upstream_deliverable 读取交付物全文）", teamID)
		}
	}
	if err := d.validateUpstreamContractSchema(ctx, readerSessionID, t, full.Content); err != nil {
		return UpstreamDeliverableContent{}, err
	}

	sizeChars := utf8.RuneCountInString(full.Content)
	content := full.Content
	truncated := sizeChars > maxChars
	if truncated {
		content = TruncateRunes(content, maxChars) + fmt.Sprintf("\n...[已截断，全文共 %d 字符]", sizeChars)
	}
	d.lg.Info("上游交付物按 key 读取",
		loggateway.StepID("spirit.read_upstream_deliverable_key"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("key", key),
		loggateway.Int("size_chars", sizeChars),
		loggateway.Bool("truncated", truncated),
	)
	return UpstreamDeliverableContent{
		Content:   content,
		SizeChars: sizeChars,
		Truncated: truncated,
		TeamID:    teamID,
		SessionID: full.SessionID,
	}, nil
}
