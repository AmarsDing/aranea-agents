package tools

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// ---------------------------------------------------------------------------
// delegate_to_spirit — 语音助手异步委派工具（M74 V9，设计 74 §15.4-B）
//
// 语音助手（__voice_butler__）唯一工具：复杂任务委派精灵后台执行，立即返回
// 不阻塞语音 turn；精灵 task 终态经事件总线 → voice eventLoop 关联播报。
// ---------------------------------------------------------------------------

// SpiritAgentLookupPort 按 agent_key 查 agent（委派目标解析）。
// *biz.AgentUsecase / biz.AgentRepository 直接满足。
// Stability:evolving
type SpiritAgentLookupPort interface {
	GetAgentByAgentKey(ctx context.Context, agentKey string) (biz.Agent, error)
}

// SpiritMainSessionPort 查/建用户精灵主会话（R8）。
// *biz.SessionUsecase 直接满足（方法名 Search/Create 一致）。
// Stability:evolving
type SpiritMainSessionPort interface {
	Search(ctx context.Context, q biz.SessionSearchQuery) (biz.SessionListResult, error)
	Create(ctx context.Context, in biz.Session) (biz.Session, error)
}

// VoiceDelegationRegistryPort 是委派登记窄端口（*voice.DelegationRegistry
// 经原始参数签名直接满足，service 注入零 adapter）。
// Stability:evolving
type VoiceDelegationRegistryPort interface {
	Register(voiceSessionID, spiritSessionID, content string) int64
	MarkSubmitFailed(regID int64, message string)
}

// DelegationTurnSubmitter 提交精灵后台 turn（R12 窄端口，service 侧适配
// ChatService.ExecuteTurn + ErrTurnMessageQueued 分类——tools 不反依赖 service）。
// 语义：accepted=true（err==nil 完成 / 排队受理）→ 终态经事件总线到达；
// accepted=false && err!=nil → 同步失败（准入拒绝/DB 错），永无 TaskCreated。
// Stability:evolving
type DelegationTurnSubmitter interface {
	SubmitDelegatedTurn(ctx context.Context, input biz.TurnInput) (accepted bool, err error)
}

// DelegateToSpiritDeps 是 delegate_to_spirit 工具的依赖集（orchestrator
// 条件注入装配，cli_admin_tools.go voiceButlerTools）。
type DelegateToSpiritDeps struct {
	Agents    SpiritAgentLookupPort
	Sessions  SpiritMainSessionPort
	Submitter DelegationTurnSubmitter
	Registry  VoiceDelegationRegistryPort
	LG        loggateway.Logger
}

// DelegateToSpiritInput is the input for the delegate_to_spirit tool.
type DelegateToSpiritInput struct {
	Task string `json:"task" jsonschema:"description=交给精灵助手后台执行的任务内容。用用户的原话或稍加整理，保持信息完整，不要缩写省略。"`
}

// DelegateToSpiritOutput is the output for the delegate_to_spirit tool.
type DelegateToSpiritOutput struct {
	Content string `json:"content"`
}

// NewDelegateToSpiritTool creates the delegate_to_spirit tool.
// deps 任一为 nil 时返回 nil（装配侧跳过注入，语音助手退化为纯快答）。
func NewDelegateToSpiritTool(deps DelegateToSpiritDeps) *trpcfunction.FunctionTool[DelegateToSpiritInput, DelegateToSpiritOutput] {
	if deps.Agents == nil || deps.Sessions == nil || deps.Submitter == nil || deps.Registry == nil {
		return nil
	}
	lg := deps.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	lg = lg.With(loggateway.Domain("voice_delegation"))
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input DelegateToSpiritInput) (DelegateToSpiritOutput, error) {
			task := strings.TrimSpace(input.Task)
			if task == "" {
				return DelegateToSpiritOutput{}, apierror.BadRequest("VOICE", "task is required")
			}
			voiceSessionID := voiceSessionIDFromCtx(ctx)
			if voiceSessionID == "" {
				return DelegateToSpiritOutput{}, apierror.BadRequest("VOICE", "voice session id not found in context")
			}
			userID := ctxuser.FromContext(ctx)

			spirit, err := deps.Agents.GetAgentByAgentKey(ctx, biz.SpiritAgentKey)
			if err != nil {
				return DelegateToSpiritOutput{}, apierror.Internal("VOICE", "resolve spirit agent: "+err.Error())
			}
			spiritSessionID, err := findOrCreateSpiritMainSession(ctx, deps.Sessions, spirit.ID, userID)
			if err != nil {
				return DelegateToSpiritOutput{}, apierror.Internal("VOICE", "resolve spirit session: "+err.Error())
			}

			// 先注册后提交：消除 TaskCreated 先于登记到达的漏绑窗口（R10）。
			regID := deps.Registry.Register(voiceSessionID, spiritSessionID, task)

			// 异步提交：ExecuteTurn 阻塞至 turn 完成，必须 detached goroutine
			// （voice 预热同款：appctx 独立存活 + userID 传播，R12）。
			safego.Go(appctx.Ctx(), "voice.delegate_submit", func() {
				submitCtx := ctxuser.WithUserID(appctx.Ctx(), userID)
				accepted, submitErr := deps.Submitter.SubmitDelegatedTurn(submitCtx, biz.TurnInput{
					SessionID: spiritSessionID,
					Content:   task,
				})
				switch {
				case submitErr == nil || accepted:
					// 已受理/已完成：终态（TaskCompleted/TaskFailed）经事件总线
					// 到达 voice eventLoop 播报，此处无需动作。
					lg.Info("delegation turn submitted",
						loggateway.StepID("voice.delegate.submitted"),
						loggateway.SessionID(spiritSessionID))
				default:
					// 同步失败：永无 TaskCreated → 通知 voice Session 口播失败，
					// 防 delegation 泄漏空等（R12）。
					lg.Warn("delegation turn submit failed",
						loggateway.StepID("voice.delegate.submit_failed"),
						loggateway.SessionID(spiritSessionID), loggateway.Err(submitErr))
					deps.Registry.MarkSubmitFailed(regID, "交给精灵助手的任务提交失败了，请在聊天窗口里直接告诉我重试")
				}
			})

			lg.Info("delegation registered",
				loggateway.StepID("voice.delegate.registered"),
				loggateway.SessionID(voiceSessionID),
				loggateway.Str("spirit_session_id", spiritSessionID))
			return DelegateToSpiritOutput{Content: "任务已交给精灵助手后台执行，完成后我会把结果告诉你。"}, nil
		},
		trpcfunction.WithName("delegate_to_spirit"),
		trpcfunction.WithDescription("把需要动手执行的复杂任务（查资料、写文件、操作工具、多步骤工作、组队协作、管理系统资源）异步交给精灵助手后台执行。调用后立即返回，不要等待、不要追问结果；任务完成后系统会自动通知你播报。闲聊、寒暄、一句话能答完的简单问答禁止使用本工具。"),
	)
}

// findOrCreateSpiritMainSession 查用户最近活跃的精灵根会话（与 chat 页默认
// 会话一致，R8）；无则创建。
func findOrCreateSpiritMainSession(ctx context.Context, sessions SpiritMainSessionPort, spiritAgentID, userID string) (string, error) {
	res, err := sessions.Search(ctx, biz.SessionSearchQuery{
		OwnerType: "agent",
		AgentID:   spiritAgentID,
		UserID:    userID,
		RootOnly:  true,
		Limit:     20,
	})
	if err != nil {
		return "", err
	}
	best := ""
	bestAt := ""
	for _, s := range res.Items {
		at := s.LastMessageAt
		if at == "" {
			at = s.UpdatedAt
		}
		if at == "" {
			at = s.CreatedAt
		}
		if best == "" || at > bestAt {
			best, bestAt = s.ID, at
		}
	}
	if best != "" {
		return best, nil
	}
	created, err := sessions.Create(ctx, biz.Session{
		OwnerType: "agent",
		AgentID:   spiritAgentID,
		UserID:    userID,
		Title:     "精灵助手",
	})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// voiceSessionIDFromCtx 取当前 turn 的 invocation session id（语音助手 turn
// 运行的会话 = 语音 WS 会话，voice_ws.go 以 session_id 键控）。
func voiceSessionIDFromCtx(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	if inv.Session != nil {
		return inv.Session.ID
	}
	return ""
}
