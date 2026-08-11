package main

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/loggateway"
)

// stubQueuedTurnGateway 模拟生产链路的真实返回：消息被准入层成功入队时，
// ChatService.ExecuteTurn 返回 (TurnOutcomeQueued, ErrTurnMessageQueued)。
type stubQueuedTurnGateway struct {
	biz.TurnExecutorGateway
}

func (stubQueuedTurnGateway) ExecuteTurn(_ context.Context, _ biz.TurnInput) (biz.TurnResult, error) {
	return biz.TurnResult{Outcome: biz.TurnOutcomeQueued, PendingID: "pend-1"}, service.ErrTurnMessageQueued
}

// TestWSTurnExecutorAdapter_QueuedIsNotError 复现 P2：用户在 Agent 运行期间
// 追加消息（steer/入队）时，后端准入层成功受理入队，但 WS 适配器丢弃了
// TurnResult.Outcome，把 ErrTurnMessageQueued 哨兵当作普通错误透传给
// handleUserMessage，后者发布 send_failed 错误卡片——用户看到"入队成功"
// 通知与"发送失败"错误并存的矛盾 UI。
//
// 期望行为：Outcome == TurnOutcomeQueued 时适配器返回 nil（入队是成功受理，
// 前端已通过 message_queued 通知获得反馈）。
func TestWSTurnExecutorAdapter_QueuedIsNotError(t *testing.T) {
	var exec server.WSTurnExecutor = provideWSTurnExecutor(stubQueuedTurnGateway{}, loggateway.NewNoop())
	err := exec.ExecuteTurn(context.Background(), server.WSTurnInput{
		SessionID: "sess-1",
		Content:   "follow up",
	})
	if err != nil {
		t.Fatalf("P2: queued outcome must not surface as error (frontend renders false send_failed card), got %v", err)
	}
}

// captureTurnGateway 捕获透传的 biz.TurnInput 供断言。
type captureTurnGateway struct {
	biz.TurnExecutorGateway
	got biz.TurnInput
}

func (g *captureTurnGateway) ExecuteTurn(_ context.Context, in biz.TurnInput) (biz.TurnResult, error) {
	g.got = in
	return biz.TurnResult{}, nil
}

// TestWSTurnExecutorAdapter_VoiceMetaPassthrough 复现 2026-08-11 真机回归：
// 语音轮次 WSTurnInput.Voice 在适配器被丢弃 → biz.TurnInput.Voice=nil →
// prepareRunContext 不打 voice fast-path 标记 → 主 LLM 思考未禁用（真机实测
// 语音 TTFT 含 1.7s+ reasoning 流），且 V2-T6 语音溯源元数据未持久化。
//
// 期望行为：Voice 必须透传到 biz.TurnInput。
func TestWSTurnExecutorAdapter_VoiceMetaPassthrough(t *testing.T) {
	gw := &captureTurnGateway{}
	var exec server.WSTurnExecutor = provideWSTurnExecutor(gw, loggateway.NewNoop())
	err := exec.ExecuteTurn(context.Background(), server.WSTurnInput{
		SessionID: "sess-1",
		Content:   "你好",
		Voice:     &biz.VoiceTurnMeta{ASRProvider: "volcengine_sauc", DurationMs: 2740},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw.got.Voice == nil {
		t.Fatal("Voice must be passed through to biz.TurnInput (voice fast-path thinking-disable depends on it)")
	}
	if gw.got.Voice.ASRProvider != "volcengine_sauc" || gw.got.Voice.DurationMs != 2740 {
		t.Fatalf("Voice meta mangled: %+v", gw.got.Voice)
	}
}
