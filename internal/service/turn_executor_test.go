package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestTurnExecutor_Execute_rejectsEmptyInput(t *testing.T) {
	o := &ChatOrchestrator{}
	got, err := o.Execute(context.Background(), biz.TurnInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got.Outcome != biz.TurnOutcomeFailed {
		t.Fatalf("outcome=%q want failed", got.Outcome)
	}
}

func TestTurnExecutor_ExecuteTurnGateway(t *testing.T) {
	svc := &ChatService{orch: &ChatOrchestrator{}, lg: loggateway.NewNoop()}
	got, err := svc.ExecuteTurn(context.Background(), biz.TurnInput{SessionID: "s1"})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if got.Outcome != biz.TurnOutcomeFailed {
		t.Fatalf("outcome=%q", got.Outcome)
	}
}

// TestTurnIntentFromInput_PreservesVoice 复现 2026-08-11 真机回归（第二层）：
// 生产链路 ChatService.ExecuteTurn 走 turnPipeline，turnIntentFromInput 把
// TurnInput 折成 TurnIntent 时丢弃 Voice（TurnIntent 无该字段），pipeline
// 出口 intent.TurnInput() 重建后 Voice=nil —— orchestrator 侧
// shouldRunProactiveRecall / voice fast-path 标记全部失效（真机实测语音轮次
// 召回 528ms 照跑、思考未关）。
//
// 期望行为：TurnInput → TurnIntent → TurnInput 全链路保留 Voice。
func TestTurnIntentFromInput_PreservesVoice(t *testing.T) {
	in := biz.TurnInput{
		SessionID: "sess-1",
		Content:   "你好",
		Voice:     &biz.VoiceTurnMeta{ASRProvider: "volcengine_sauc", DurationMs: 2740},
	}
	got := turnIntentFromInput(in).TurnInput()
	if got.Voice == nil {
		t.Fatal("Voice dropped in TurnInput→TurnIntent→TurnInput round trip (voice fast-path & recall-skip depend on it)")
	}
	if got.Voice.ASRProvider != "volcengine_sauc" || got.Voice.DurationMs != 2740 {
		t.Fatalf("Voice meta mangled: %+v", got.Voice)
	}
}
