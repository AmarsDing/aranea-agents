package service

import (
	"context"
	"encoding/json"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"google.golang.org/protobuf/types/known/structpb"
)

// nativeSendChatMessage is the native implementation of SendChatMessage.
func (o *ChatOrchestrator) nativeSendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "content is required")
	}
	// P2（2026-08-20）：turn 执行与 HTTP 请求 ctx 解耦。客户端断连/浏览器
	// 刷新不再中止 turn——服务端继续跑完并落库（P1 保证收尾写入存活），
	// 前端重连/刷新后拉到完整结果，对齐 WS user_message 与
	// submitChatMessageAsync 的既有语义。取消不受影响：用户取消经
	// RunRegistry（StopGeneration），不依赖此 ctx。用 WithoutCancel 而非
	// appctx.Ctx() 以保留 workspace/trace/user 等 values（EVAL-01 教训：
	// 裸 Background 会丢租户域）。
	ctx = context.WithoutCancel(ctx)
	input := turnInputFromProto(req)
	// P3（2026-08-20）：提交幂等——重试/双击/断连重发携带同一 request_id，
	// 重复提交返回空成功（对齐 queued 语义），不产生第二条用户消息。
	if !o.claimTurnIdem(input.SessionID, input.RequestID) {
		o.lg().Info("duplicate chat submission dropped (sync)",
			loggateway.StepID("chat.submit_duplicate"),
			loggateway.SessionID(input.SessionID),
			loggateway.Str("request_id", input.RequestID))
		return &chatv1.SendChatMessageResponse{}, nil
	}
	tr, err := o.Execute(ctx, input)
	if err != nil {
		if isTurnMessageQueued(err) {
			return &chatv1.SendChatMessageResponse{}, nil
		}
		// 执行失败撤销幂等占位：用户重试须能重新受理。
		o.releaseTurnIdem(input.SessionID, input.RequestID)
		return nil, err
	}
	if tr.Outcome != biz.TurnOutcomeCompleted {
		return &chatv1.SendChatMessageResponse{}, nil
	}
	userMsg, assistantMsg := tr.UserMsg, tr.AssistantMsg
	um := chatMessageToMap(userMsg)
	am := chatMessageToMap(assistantMsg)
	out := &chatv1.SendChatMessageResponse{}
	if st, err := structpb.NewStruct(um); err != nil {
		o.lg().Warn("encode user_message struct failed",
			loggateway.StepID("chat_native.encode_user_msg"),
			loggateway.Err(err))
		return nil, apierror.Internal(apierror.DomainChat, "encode user_message failed")
	} else {
		out.UserMessage = st
	}
	if st, err := structpb.NewStruct(am); err != nil {
		o.lg().Warn("encode agent_message struct failed",
			loggateway.StepID("chat_native.encode_agent_msg"),
			loggateway.Err(err))
		return nil, apierror.Internal(apierror.DomainChat, "encode agent_message failed")
	} else {
		out.AgentMessage = st
	}
	if tid := strings.TrimSpace(req.GetTeamId()); tid != "" {
		if o.td().Pipeline.EventBus != nil {
			o.td().Pipeline.EventBus.Publish(ctx, biz.NewSystemNoticeEvent(req.GetSessionId(), "team_stage_hint", "", map[string]any{
				"team_id": tid,
				"hint":    true,
				"status":  "completed",
				"stage":   "completed",
			}))
		}
	}
	return out, nil
}

// submitChatMessageAsync submits a user message asynchronously and returns an
// ACK immediately (B2 channel separation). Turn execution runs in a background
// goroutine using the process-lifecycle context; all message/state/streaming
// data is delivered via the WS data channel.
//
// Design notes:
//   - The HTTP request context is cancelled as soon as the ACK is returned, so
//     the background turn MUST derive from appctx.Ctx() (process-lifecycle).
//     Cancellation is handled via the RunRegistry (StopGeneration RPC).
//   - On turn failure, an error envelope is published to the event bus so
//     WS-connected clients see the failure inline. Queued messages are not
//     treated as errors — the pending queue tracks them and the WS data
//     channel delivers updates when the turn eventually runs.
//   - The response intentionally carries no message content; message_id and
//     turn_id are empty on accept and delivered via WS events
//     (`message.persisted`, `run_status=running`).
func (o *ChatOrchestrator) submitChatMessageAsync(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SubmitChatMessageResponse, error) {
	if strings.TrimSpace(req.GetSessionId()) == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	if strings.TrimSpace(req.GetContent()) == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "content is required")
	}
	input := turnInputFromProto(req)
	sessionID := input.SessionID
	lg := o.lg().With(loggateway.SessionID(sessionID), loggateway.StepID("chat.submit_async"))

	// P3（2026-08-20）：提交幂等——重试/断连重发携带同一 request_id，
	// 重复提交直接 ACK（status=duplicate），不产生第二条用户消息。
	if !o.claimTurnIdem(sessionID, input.RequestID) {
		lg.Info("duplicate chat submission dropped (async)",
			loggateway.Str("request_id", input.RequestID))
		return &chatv1.SubmitChatMessageResponse{
			Accepted: true,
			Status:   "duplicate",
		}, nil
	}

	// Derive from appctx.Ctx() so the turn outlives the HTTP request.
	// StopGeneration cancels via the RunRegistry, not via this context.
	// P0-02 fix: extract the authenticated userID from the HTTP context and
	// propagate it into the background context so the Runner session key,
	// memory tools, and quota checks use the real user scope.
	bgCtx := ctxuser.WithUserID(appctx.Ctx(), ctxuser.FromContext(ctx))
	safego.Go(bgCtx, "chat-submit-async", func() {
		if _, err := o.Execute(bgCtx, input); err != nil {
			if isTurnMessageQueued(err) {
				lg.Info("SubmitChatMessage: message queued (active run)")
				return
			}
			// 执行失败撤销幂等占位：用户重试须能重新受理。
			o.releaseTurnIdem(sessionID, input.RequestID)
			lg.Warn("SubmitChatMessage: turn execution failed", loggateway.Err(err))
			if bus := o.td().Pipeline.EventBus; bus != nil {
				bus.Publish(context.Background(), biz.NewSystemNoticeEvent(sessionID, "send_failed", err.Error(), map[string]any{
					"error_type": "send_failed",
				}))
			}
		}
	})

	return &chatv1.SubmitChatMessageResponse{
		Accepted: true,
		Status:   "accepted",
	}, nil
}

// nativeGetChatOptions returns chat options.
func (o *ChatOrchestrator) nativeGetChatOptions(ctx context.Context, req *chatv1.GetChatOptionsRequest) (*chatv1.GetChatOptionsResponse, error) {
	typed := strings.TrimSpace(req.GetType())
	switch typed {
	case "", "dialog_mode":
		return &chatv1.GetChatOptionsResponse{Items: nativeDialogModeChatOptions()}, nil
	case "provider":
		return o.nativeGetProviderOptions(ctx)
	case "model":
		return o.nativeGetModelOptions(ctx)
	default:
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
}

func (o *ChatOrchestrator) nativeGetProviderOptions(ctx context.Context) (*chatv1.GetChatOptionsResponse, error) {
	if o.td().ReadDeps.LLM == nil {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	rows, err := o.td().ReadDeps.LLM.List(ctx)
	if err != nil {
		o.lg().Warn("list providers failed", loggateway.Err(err))
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	seen := make(map[string]struct{})
	var items []*chatv1.ChatOption
	for _, row := range rows {
		p := strings.TrimSpace(row.Provider)
		if p == "" || row.Enabled == false {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		items = append(items, &chatv1.ChatOption{
			Type:      "provider",
			Key:       p,
			Label:     p,
			Enabled:   true,
			SortOrder: int32(len(items) + 1),
		})
	}
	return &chatv1.GetChatOptionsResponse{Items: items}, nil
}

func (o *ChatOrchestrator) nativeGetModelOptions(ctx context.Context) (*chatv1.GetChatOptionsResponse, error) {
	if o.td().ReadDeps.LLM == nil {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	rows, err := o.td().ReadDeps.LLM.List(ctx)
	if err != nil {
		o.lg().Warn("list models failed", loggateway.Err(err))
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	var items []*chatv1.ChatOption
	for i, row := range rows {
		if row.Enabled == false {
			continue
		}
		type modelMeta struct {
			Provider string `json:"provider,omitempty"`
			Model    string `json:"model,omitempty"`
		}
		mj := "{}"
		if row.Provider != "" || row.Model != "" {
			if b, err := json.Marshal(modelMeta{Provider: row.Provider, Model: row.Model}); err == nil {
				mj = string(b)
			}
		}
		label := row.Name
		if label == "" {
			label = row.Key
		}
		if label == "" {
			label = row.Model
		}
		items = append(items, &chatv1.ChatOption{
			Type:         "model",
			Key:          row.Key,
			Label:        label,
			Enabled:      true,
			SortOrder:    int32(i + 1),
			MetadataJson: mj,
		})
	}
	return &chatv1.GetChatOptionsResponse{Items: items}, nil
}

// turnInputFromProto converts a proto SendChatMessageRequest to a biz-level TurnInput.
// This adapter lives in the service layer (the proto boundary) so that internal
// packages never need to import api/*/v1.
func turnInputFromProto(req *chatv1.SendChatMessageRequest) biz.TurnInput {
	if req == nil {
		return biz.TurnInput{}
	}
	input := biz.TurnInput{
		SessionID: req.GetSessionId(),
		Content:   req.GetContent(),
		AgentKey:  req.GetAgentKey(),
		RequestID: req.GetRequestId(),
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint:  biz.EntryPointWeb,
			AllowQueue:  true,
			AllowStream: true,
		},
	}
	if req.TeamId != nil {
		input.TeamID = *req.TeamId
	}
	if opts := req.GetOptions(); opts != nil {
		input.Options = biz.TurnOptions{
			DialogMode:     opts.GetDialogMode(),
			Provider:       opts.GetProvider(),
			Model:          opts.GetModel(),
			KnowledgeBases: opts.GetKnowledgeBases(),
		}
		for _, att := range opts.GetAttachments() {
			if att != nil {
				input.Options.AttachmentIDs = append(input.Options.AttachmentIDs, att.GetId())
			}
		}
	}
	return input
}
