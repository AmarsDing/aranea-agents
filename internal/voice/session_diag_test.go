package voice

import (
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// 临时诊断：TestSessionExitWordAckThenDormant 第 90 行（退出词应答 TTS 写入）不达。
// 复现同流程，dump TTS 写入 / 下行 JSON 序列 / 终态，定位丢失点。诊断后删除。
func TestDiagExitWordFlow(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeCompanion})
	t.Logf("after start: state=%q", fx.down.lastState())
	fx.sess.Wake("kws")
	t.Logf("after wake: state=%q writes=%q", fx.down.lastState(), strings.Join(fx.ttsProv.allWrites(), "|"))
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "休息吧"}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fx.down.lastState() == "dormant" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("after final: state=%q writes=%q", fx.down.lastState(), strings.Join(fx.ttsProv.allWrites(), "|"))
	t.Logf("downlink types: %q", strings.Join(fx.down.typesOf(), ","))
}
