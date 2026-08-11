package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/channel/preview"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const flowStepChannelGateCard = "channel.gate_card"

// gateCardConfirmTimeoutCode 镜像 agent/v2.ConfirmTimeoutErrorCode（service 不反向
// 依赖 agent/v2 常量，保持字符串约定同步即可）。
const gateCardConfirmTimeoutCode = "confirm_timeout"

// 澄清卡片交互 guard：题数/选项数超限或含多选题时降级为纯文本卡（自由回复作答）。
const (
	gateCardMaxQuestions = 10
	gateCardMaxOptions   = 6
)

// gateCardChatGateway 是管理器对 ChatService 的最小依赖面（biz.ChannelTurnGateway
// 满足）。仅含确认/澄清两个卡片入口方法。
type gateCardChatGateway interface {
	ConfirmToolGateForCard(ctx context.Context, sessionID, stepID, replyToken string) (accepted bool, reply string)
	SubmitClarificationForCard(ctx context.Context, sessionID, stepID string, answers []biz.ClarificationAnswer) (reply string, err error)
}

// ChannelGateCards 管理渠道会话的交互门卡片（工具确认 / 澄清）。
//
// 职责：
//  1. 订阅 v2 EventBus：confirm tool_blocked / clarify awaiting_input 时向渠道会话
//     发送交互卡片；step 到达终态（completed/cancelled/timeout）时 PATCH 为结果卡。
//  2. 拥有卡片点击入口（HandleConfirmClick / SelectClarifyOption）：确认经
//     ChannelTurnGateway 复用 RPC 同核状态机；澄清按题累积选项、全部作答后自动提交。
//  3. 双向同步：Web 端确认/澄清 → step 事件 → 本管理器 PATCH 飞书卡片；
//     飞书端操作 → gateway → step 事件 → Web WS 更新。
//
// 跟踪态为进程内内存（stepID → messageID）。进程重启后已发出卡片无法 PATCH，
// 但回调仍可经 gateway 正常处理（DB 状态机兜底），属可接受降级。
type ChannelGateCards struct {
	eventBus biz.EventBus
	sessions *biz.SessionUsecase
	channels *biz.ChannelUsecase
	chat     gateCardChatGateway
	steps    biz.StepV2Reader
	http     *http.Client
	lg       loggateway.Logger

	mu      sync.Mutex
	tracked map[string]*gateCardRef
}

// gateCardRef 是一张已发出（或点击中重建）的交互门卡片的跟踪状态。
type gateCardRef struct {
	stepID        string
	sessionID     string // 卡片归属的渠道绑定会话（card value 中的 session_id）
	kind          biz.StepKind
	toolName      string
	channelID     string
	recipient     string
	receiveIDType string
	messageID     string // 空 = 瞬态 ref（重启后点击重建，无卡片可 PATCH）
	questions     []preview.ClarifyGateQuestion
	selections    [][]string // 与 questions 对齐；单选模式每题 0..1 项
}

func NewChannelGateCards(
	eventBus biz.EventBus,
	sessions *biz.SessionUsecase,
	channels *biz.ChannelUsecase,
	chat gateCardChatGateway,
	steps biz.StepV2Reader,
	lg loggateway.Logger,
) *ChannelGateCards {
	if sessions == nil || channels == nil || chat == nil || steps == nil {
		return nil
	}
	return &ChannelGateCards{
		eventBus: eventBus,
		sessions: sessions,
		channels: channels,
		chat:     chat,
		steps:    steps,
		http:     lark.DefaultHTTPClient(),
		lg:       lg.With(loggateway.Domain("channel_gate_cards")),
		tracked:  make(map[string]*gateCardRef),
	}
}

// Start 订阅 v2 事件总线。事件处理在订阅 goroutine 内串行执行（gate 事件低频，
// 卡片网络 I/O ~200ms 可接受；串行保证同一步骤的发卡/PATCH 不乱序）。
func (m *ChannelGateCards) Start(ctx context.Context) {
	if m == nil || m.eventBus == nil {
		return
	}
	ch, unsub := m.eventBus.Subscribe(biz.EventSubscribeOptions{})
	safego.Go(ctx, "channel.gate_cards", func() {
		defer unsub()
		m.lg.Info("channel gate cards consumer started", loggateway.StepID(flowStepChannelGateCard))
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				m.handleEvent(ctx, e)
			}
		}
	})
}

func (m *ChannelGateCards) handleEvent(ctx context.Context, e biz.Event) {
	switch ev := e.(type) {
	case *biz.StepCreatedEvent:
		m.maybeOpenGate(ctx, ev.Step)
	case *biz.StepUpdatedEvent:
		m.maybeOpenGate(ctx, ev.Step)
		m.maybeCloseGate(ctx, ev.Step, "")
	case *biz.StepCompletedEvent:
		m.maybeCloseGate(ctx, ev.Step, "")
	case *biz.SystemNoticeEvent:
		m.handleNotice(ctx, ev)
	}
}

// === 发卡 ===

// maybeOpenGate 在确认门（confirm/tool_blocked）或澄清门（clarify/awaiting_input）
// 打开时发送卡片。confirm step 由 BeginStep（pending）→ EmitConfirmRequest
// （tool_blocked, StepUpdated）两段发布，故 Created/Updated 都要过本判据；
// 已跟踪步骤幂等跳过。
func (m *ChannelGateCards) maybeOpenGate(ctx context.Context, step biz.Step) {
	switch {
	case step.Kind == biz.StepKindConfirm && step.Status == biz.StepStatusToolBlocked:
	case step.Kind == biz.StepKindClarify && step.Status == biz.StepStatusAwaitingInput:
	default:
		return
	}
	if m.isTracked(step.ID) {
		return
	}
	m.openGate(ctx, step)
}

func (m *ChannelGateCards) openGate(ctx context.Context, step biz.Step) {
	meta, cardSessionID, ok := m.resolveChannelMeta(ctx, step)
	if !ok {
		return
	}
	recipient, ridType := gateCardReceiveTarget(meta)
	if recipient == "" {
		return
	}

	var cardJSON string
	var ref *gateCardRef
	switch step.Kind {
	case biz.StepKindConfirm:
		card, err := preview.BuildFeishuConfirmGateCardJSON(preview.ConfirmGateCardParams{
			StepID:      step.ID,
			SessionID:   cardSessionID,
			ToolName:    step.ToolName,
			ArgsSummary: strings.TrimSpace(string(step.ToolArgs)),
		})
		if err != nil {
			m.lg.Warn("build confirm gate card failed", loggateway.StepID(flowStepChannelGateCard), loggateway.Err(err))
			return
		}
		cardJSON = card
		ref = &gateCardRef{kind: step.Kind, toolName: strings.TrimSpace(step.ToolName)}
	case biz.StepKindClarify:
		var envelope biz.ClarificationEnvelope
		if err := json.Unmarshal([]byte(step.Content), &envelope); err != nil {
			m.lg.Warn("parse clarify envelope failed", loggateway.StepID(flowStepChannelGateCard), loggateway.Err(err))
			return
		}
		questions := gateCardClarifyQuestions(envelope.Questions)
		interactive := gateCardClarifyInteractive(envelope.Questions)
		card, err := preview.BuildFeishuClarifyGateCardJSON(preview.ClarifyGateCardParams{
			StepID:      step.ID,
			SessionID:   cardSessionID,
			Questions:   questions,
			Selections:  make([][]string, len(questions)),
			Interactive: interactive,
		})
		if err != nil {
			m.lg.Warn("build clarify gate card failed", loggateway.StepID(flowStepChannelGateCard), loggateway.Err(err))
			return
		}
		cardJSON = card
		ref = &gateCardRef{kind: step.Kind, questions: questions, selections: make([][]string, len(questions))}
	default:
		return
	}
	ref.stepID = step.ID
	ref.sessionID = cardSessionID
	ref.channelID = meta.ChannelID
	ref.recipient = recipient
	ref.receiveIDType = ridType

	sender, err := m.feishuSender(ctx, meta.ChannelID)
	if err != nil {
		m.lg.Warn("gate card sender build failed",
			loggateway.StepID(flowStepChannelGateCard),
			loggateway.Str("channel_id", meta.ChannelID),
			loggateway.Err(err))
		return
	}
	msgID, err := sender.UpsertToolCard(ctx, recipient, "", cardJSON)
	if err != nil {
		m.lg.Warn("gate card send failed",
			loggateway.StepID(flowStepChannelGateCard),
			loggateway.Str("channel_id", meta.ChannelID),
			loggateway.Str("session_id", cardSessionID),
			loggateway.Str("step_id", step.ID),
			loggateway.Str("kind", string(step.Kind)),
			loggateway.Err(err))
		return
	}
	ref.messageID = msgID
	m.track(ref)
	m.lg.Info("gate card sent",
		loggateway.StepID(flowStepChannelGateCard),
		loggateway.Str("session_id", cardSessionID),
		loggateway.Str("step_id", step.ID),
		loggateway.Str("kind", string(step.Kind)),
		loggateway.Str("message_id", msgID),
	)
}

// === 终态 PATCH ===

// maybeCloseGate 在跟踪中的 step 离开挂起态时 PATCH 结果卡并移除跟踪。
// noticeDecision 仅 notice 路径使用（"approved"/"rejected"），事件路径为空。
func (m *ChannelGateCards) maybeCloseGate(ctx context.Context, step biz.Step, noticeDecision string) {
	ref := m.untrack(step.ID)
	if ref == nil {
		return
	}
	if ref.messageID == "" {
		return
	}
	cardJSON, ok := m.resultCard(ctx, ref, &step, noticeDecision)
	if !ok {
		return
	}
	m.patchCard(ctx, ref, cardJSON)
}

// handleNotice 覆盖 confirmToolGate 直写 DB + notice 的路径（运行已死/恢复续跑时
// projector 不会发 StepCompleted），以及澄清提交的兜底。
func (m *ChannelGateCards) handleNotice(ctx context.Context, ev *biz.SystemNoticeEvent) {
	if ev == nil {
		return
	}
	stepID := strings.TrimSpace(gateCardMetaString(ev.Meta, "step_id"))
	if stepID == "" {
		return
	}
	switch ev.NoticeType {
	case "tool_confirm_approved", "tool_confirm_rejected":
		decision := "approved"
		if ev.NoticeType == "tool_confirm_rejected" {
			decision = "rejected"
		}
		ref := m.untrack(stepID)
		if ref == nil || ref.messageID == "" {
			return
		}
		cardJSON, ok := m.resultCard(ctx, ref, nil, decision)
		if !ok {
			return
		}
		m.patchCard(ctx, ref, cardJSON)
	case "clarification_submitted":
		// StepUpdated(completed) 与 notice 同源发布，先到者关卡；此处兜底。
		if !m.isTracked(stepID) {
			return
		}
		step, err := m.steps.GetStep(ctx, stepID)
		if err != nil {
			return
		}
		m.maybeCloseGate(ctx, step, "")
	}
}

// resultCard 构建终态卡。step 可空（notice 路径无 step 载体，用 ref 快照 + decision）。
func (m *ChannelGateCards) resultCard(ctx context.Context, ref *gateCardRef, step *biz.Step, noticeDecision string) (string, bool) {
	switch ref.kind {
	case biz.StepKindConfirm:
		p := preview.GateResultCardParams{Title: "已处理", Template: "grey"}
		tool := ref.toolName
		if tool == "" {
			tool = "未知工具"
		}
		switch {
		case noticeDecision == "approved" || (step != nil && step.Status == biz.StepStatusCompleted):
			p = preview.GateResultCardParams{
				Template: "green",
				Title:    "✓ 已批准 · " + tool,
				Lines:    []string{fmt.Sprintf("工具 **%s** 已获批准，正在继续执行。", tool)},
			}
		case noticeDecision == "rejected":
			p = preview.GateResultCardParams{
				Template: "red",
				Title:    "✕ 已拒绝 · " + tool,
				Lines:    []string{fmt.Sprintf("工具 **%s** 已被拒绝执行。", tool)},
			}
		case step != nil && step.Status == biz.StepStatusCancelled && step.ToolErrorCode == gateCardConfirmTimeoutCode:
			p = preview.GateResultCardParams{
				Template: "grey",
				Title:    "⏱ 确认已超时 · " + tool,
				Lines:    []string{fmt.Sprintf("工具 **%s** 的确认已超时，未执行。", tool)},
			}
		case step != nil && step.Status == biz.StepStatusCancelled:
			p = preview.GateResultCardParams{
				Template: "red",
				Title:    "✕ 已拒绝 · " + tool,
				Lines:    []string{fmt.Sprintf("工具 **%s** 已被拒绝执行。", tool)},
			}
		default:
			return "", false
		}
		card, err := preview.BuildFeishuGateResultCardJSON(p)
		if err != nil {
			m.lg.Warn("build confirm result card failed", loggateway.StepID(flowStepChannelGateCard), loggateway.Err(err))
			return "", false
		}
		return card, true
	case biz.StepKindClarify:
		if step == nil || step.Status != biz.StepStatusCompleted {
			if step != nil && (step.Status == biz.StepStatusCancelled || step.Status == biz.StepStatusFailed) {
				card, err := preview.BuildFeishuGateResultCardJSON(preview.GateResultCardParams{
					Template: "grey",
					Title:    "澄清已取消",
					Lines:    []string{"该澄清已取消或已失效。"},
				})
				if err != nil {
					return "", false
				}
				return card, true
			}
			return "", false
		}
		var envelope biz.ClarificationEnvelope
		if err := json.Unmarshal([]byte(step.Content), &envelope); err != nil {
			return "", false
		}
		lines := make([]string, 0, len(envelope.Questions))
		for i, q := range envelope.Questions {
			ans := "（按推荐）"
			if i < len(envelope.Answers) && len(envelope.Answers[i].Selected) > 0 {
				ans = strings.Join(envelope.Answers[i].Selected, "、")
			} else if len(q.Recommended) > 0 {
				ans = "按推荐：" + strings.Join(q.Recommended, "、")
			}
			lines = append(lines, fmt.Sprintf("**Q%d** %s\n→ %s", i+1, gateCardTruncate(q.Question, 80), ans))
		}
		card, err := preview.BuildFeishuGateResultCardJSON(preview.GateResultCardParams{
			Template: "green",
			Title:    "✓ 已提交澄清回答",
			Lines:    lines,
		})
		if err != nil {
			m.lg.Warn("build clarify result card failed", loggateway.StepID(flowStepChannelGateCard), loggateway.Err(err))
			return "", false
		}
		return card, true
	}
	return "", false
}

func (m *ChannelGateCards) patchCard(ctx context.Context, ref *gateCardRef, cardJSON string) {
	sender, err := m.feishuSender(ctx, ref.channelID)
	if err != nil {
		m.lg.Warn("gate card patch sender build failed", loggateway.StepID(flowStepChannelGateCard), loggateway.Err(err))
		return
	}
	if _, err := sender.UpsertToolCard(ctx, ref.recipient, ref.messageID, cardJSON); err != nil {
		m.lg.Warn("gate card patch failed",
			loggateway.StepID(flowStepChannelGateCard),
			loggateway.Str("step_id", ref.stepID),
			loggateway.Str("message_id", ref.messageID),
			loggateway.Err(err))
		return
	}
	m.lg.Info("gate card patched to result",
		loggateway.StepID(flowStepChannelGateCard),
		loggateway.Str("step_id", ref.stepID),
		loggateway.Str("message_id", ref.messageID),
	)
}

// === 卡片点击入口（ChannelIngress 回调） ===

// HandleConfirmClick 处理确认卡片按钮点击。cardSessionID 已由渠道 peer 绑定解析，
// 此处校验 step 归属（含团队子会话的 SpiritSessionID 回退）后复用 RPC 同核状态机。
// 成功后的结果卡 PATCH 由 step 事件/notice 驱动；失败（已处理/已超时）时原地置灰。
func (m *ChannelGateCards) HandleConfirmClick(ctx context.Context, cardSessionID, stepID, replyKey string) string {
	if m == nil {
		return channelCardActionServiceUnavailable
	}
	token := gateCardConfirmReplyToken(replyKey)
	if token == "" {
		return "未知的确认操作"
	}
	step, err := m.steps.GetStep(ctx, strings.TrimSpace(stepID))
	if err != nil {
		return "确认不存在或已删除"
	}
	if !gateCardStepBelongs(step, cardSessionID) {
		return "确认不属于当前会话"
	}
	accepted, reply := m.chat.ConfirmToolGateForCard(ctx, step.SessionID, step.ID, token)
	if !accepted {
		m.closeStale(ctx, step.ID, reply)
	}
	return reply
}

// SelectClarifyOption 处理澄清卡片选项点击：记录该题选择、PATCH 卡片回显，
// 全部作答后自动经 gateway 提交（终态 PATCH 由 StepUpdated 事件驱动）。
func (m *ChannelGateCards) SelectClarifyOption(ctx context.Context, cardSessionID, stepID string, questionIndex int, option string) string {
	if m == nil {
		return channelCardActionServiceUnavailable
	}
	step, err := m.steps.GetStep(ctx, strings.TrimSpace(stepID))
	if err != nil {
		return "澄清不存在或已删除"
	}
	if !gateCardStepBelongs(step, cardSessionID) {
		return "澄清不属于当前会话"
	}
	if step.Kind != biz.StepKindClarify || step.Status != biz.StepStatusAwaitingInput {
		m.closeStale(ctx, step.ID, "")
		return "该澄清已提交或已失效"
	}
	var envelope biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(step.Content), &envelope); err != nil {
		return "澄清内容解析失败"
	}
	if questionIndex < 0 || questionIndex >= len(envelope.Questions) {
		return "未知的问题"
	}
	q := envelope.Questions[questionIndex]
	if q.Mode != "" && q.Mode != biz.ClarificationModeSingle {
		return "该问题支持多选，请直接回复文字作答"
	}
	option = strings.TrimSpace(option)
	if !gateCardContainsOption(q.Options, option) {
		return "未知的选项"
	}

	m.mu.Lock()
	ref := m.tracked[step.ID]
	if ref == nil {
		// 重启后点击：无 messageID 可 PATCH，但仍可累积选择并提交。
		ref = &gateCardRef{
			stepID:     step.ID,
			sessionID:  cardSessionID,
			kind:       step.Kind,
			questions:  gateCardClarifyQuestions(envelope.Questions),
			selections: make([][]string, len(envelope.Questions)),
		}
		m.tracked[step.ID] = ref
	}
	if len(ref.selections) != len(envelope.Questions) {
		ref.selections = make([][]string, len(envelope.Questions))
	}
	if len(ref.questions) != len(envelope.Questions) {
		ref.questions = gateCardClarifyQuestions(envelope.Questions)
	}
	ref.selections[questionIndex] = []string{option}
	selections := make([][]string, len(ref.selections))
	copy(selections, ref.selections)
	questions := ref.questions
	m.mu.Unlock()

	// 中间态 PATCH 回显选择（仅持卡 ref）。
	if ref.messageID != "" {
		if cardJSON, err := preview.BuildFeishuClarifyGateCardJSON(preview.ClarifyGateCardParams{
			StepID:      step.ID,
			SessionID:   cardSessionID,
			Questions:   questions,
			Selections:  selections,
			Interactive: true,
		}); err == nil {
			m.patchCard(ctx, ref, cardJSON)
		}
	}

	answered := 0
	for _, sel := range selections {
		if len(sel) > 0 {
			answered++
		}
	}
	total := len(envelope.Questions)
	if answered < total {
		return fmt.Sprintf("已记录第 %d 题选择（%d/%d）", questionIndex+1, answered, total)
	}

	answers := make([]biz.ClarificationAnswer, total)
	for i, sel := range selections {
		answers[i].Selected = sel
	}
	reply, err := m.chat.SubmitClarificationForCard(ctx, step.SessionID, step.ID, answers)
	if err != nil {
		m.closeStale(ctx, step.ID, "")
		m.lg.Warn("clarify card submit failed",
			loggateway.StepID(flowStepChannelGateCard),
			loggateway.Str("step_id", step.ID),
			loggateway.Err(err))
		return "该澄清已提交或已失效"
	}
	return reply
}

// closeStale 将无法继续操作的卡片 PATCH 为灰色失效卡并移除跟踪
// （重复点击/运行已取消等无事件终态的兜底）。
func (m *ChannelGateCards) closeStale(ctx context.Context, stepID, reason string) {
	ref := m.untrack(stepID)
	if ref == nil || ref.messageID == "" {
		return
	}
	line := "该操作已在其他端处理或已失效。"
	if strings.TrimSpace(reason) != "" {
		line = reason
	}
	cardJSON, err := preview.BuildFeishuGateResultCardJSON(preview.GateResultCardParams{
		Template: "grey",
		Title:    "已失效",
		Lines:    []string{line},
	})
	if err != nil {
		return
	}
	m.patchCard(ctx, ref, cardJSON)
}

// === 内部工具 ===

func (m *ChannelGateCards) isTracked(stepID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.tracked[stepID]
	return ok
}

func (m *ChannelGateCards) track(ref *gateCardRef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tracked[ref.stepID] = ref
}

func (m *ChannelGateCards) untrack(stepID string) *gateCardRef {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref := m.tracked[stepID]
	if ref != nil {
		delete(m.tracked, stepID)
	}
	return ref
}

// resolveChannelMeta 解析 step 所属会话的渠道绑定。团队模式下 confirm step 挂在
// 成员子会话（无渠道绑定），回退到 SpiritSessionID 根会话解析。
// 返回的 cardSessionID 是实际解析出渠道绑定的会话 ID（写入卡片 value）。
func (m *ChannelGateCards) resolveChannelMeta(ctx context.Context, step biz.Step) (biz.ChannelSessionMeta, string, bool) {
	for _, sid := range []string{strings.TrimSpace(step.SessionID), strings.TrimSpace(step.SpiritSessionID)} {
		if sid == "" {
			continue
		}
		sess, err := m.sessions.Get(ctx, sid)
		if err != nil {
			continue
		}
		meta, ok := biz.ParseChannelSessionMeta(sess.MetadataJSON)
		if !ok || strings.TrimSpace(meta.ChannelID) == "" {
			continue
		}
		platform := strings.ToLower(strings.TrimSpace(meta.Platform))
		if platform != "feishu" && platform != "lark" {
			continue
		}
		return meta, sid, true
	}
	return biz.ChannelSessionMeta{}, "", false
}

// feishuSender 按渠道行构建卡片发送器（凭证实时读取，与工具卡预览同路径）。
func (m *ChannelGateCards) feishuSender(ctx context.Context, channelID string) (*lark.CardSender, error) {
	chRow, err := m.channels.Get(ctx, channelID)
	if err != nil {
		return nil, err
	}
	region, appID, err := feishuAppAndRegion(chRow.ConfigJSON)
	if err != nil {
		return nil, err
	}
	creds, err := m.channels.ListCredentialsRaw(ctx, channelID)
	if err != nil {
		return nil, err
	}
	sec, err := resolveCredentialPlain(ctx, m.channels, creds, "app_secret", m.lg)
	if err != nil {
		return nil, err
	}
	return &lark.CardSender{
		Region:    region,
		AppID:     appID,
		AppSecret: sec,
		HTTP:      m.http,
	}, nil
}

// gateCardReceiveTarget 从会话渠道元数据推导接收者。oc_ 前缀为群聊 chat_id，
// 其余按 open_id 处理（与 lark.ResolveReceiveTarget 语义对齐）。
func gateCardReceiveTarget(meta biz.ChannelSessionMeta) (recipient, receiveIDType string) {
	recipient = strings.TrimSpace(meta.PeerID)
	if recipient == "" {
		recipient = strings.TrimSpace(meta.PeerKey)
	}
	if strings.HasPrefix(recipient, "oc_") {
		return recipient, lark.ReceiveIDTypeChatID
	}
	return recipient, lark.ReceiveIDTypeOpenID
}

// gateCardStepBelongs 校验 step 归属卡片会话：直接归属，或团队子会话经
// SpiritSessionID 归属根会话。
func gateCardStepBelongs(step biz.Step, cardSessionID string) bool {
	cardSessionID = strings.TrimSpace(cardSessionID)
	if cardSessionID == "" {
		return false
	}
	if strings.TrimSpace(step.SessionID) == cardSessionID {
		return true
	}
	return strings.TrimSpace(step.SpiritSessionID) != "" && strings.TrimSpace(step.SpiritSessionID) == cardSessionID
}

// gateCardConfirmReplyToken 把卡片短回复键映射为 serviceawaitreply 结构化 token。
func gateCardConfirmReplyToken(replyKey string) string {
	switch strings.ToLower(strings.TrimSpace(replyKey)) {
	case "approve":
		return serviceawaitreply.ReplyApprove
	case "deny":
		return serviceawaitreply.ReplyDeny
	case "approve_session":
		return serviceawaitreply.ReplyApproveSession
	case "approve_always":
		return serviceawaitreply.ReplyApproveAlways
	default:
		return ""
	}
}

func gateCardClarifyQuestions(qs []biz.ClarificationQuestion) []preview.ClarifyGateQuestion {
	out := make([]preview.ClarifyGateQuestion, len(qs))
	for i, q := range qs {
		out[i] = preview.ClarifyGateQuestion{
			Question:    q.Question,
			Options:     q.Options,
			Recommended: q.Recommended,
		}
	}
	return out
}

// gateCardClarifyInteractive 判定是否渲染可点击选项：全部单选、题数与选项数
// 在飞书 action 行限制内。否则降级为纯文本说明卡（自由回复作答）。
func gateCardClarifyInteractive(qs []biz.ClarificationQuestion) bool {
	if len(qs) == 0 || len(qs) > gateCardMaxQuestions {
		return false
	}
	for _, q := range qs {
		if q.Mode != "" && q.Mode != biz.ClarificationModeSingle {
			return false
		}
		if len(q.Options) == 0 || len(q.Options) > gateCardMaxOptions {
			return false
		}
	}
	return true
}

func gateCardContainsOption(options []string, opt string) bool {
	for _, o := range options {
		if o == opt {
			return true
		}
	}
	return false
}

func gateCardMetaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	if s, ok := meta[key].(string); ok {
		return s
	}
	return ""
}

func gateCardTruncate(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}
