package team

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// TestPrepareUserTurnOptions_InputRiskDegradedInject 钉死输入安全三档（Q6）：
// Deny（rm -rf）零 LLM 拒绝；HITL（fault_inject）降级注入警示后继续。
func TestPrepareUserTurnOptions_InputRiskDegradedInject(t *testing.T) {
	r := &Runner{lg: loggateway.NewNoop(), runTransitioner: nopRunTransitioner{}}
	// 零值 Settings → intent pass 未开启 → ShouldRun=false，无 LLM 依赖。
	ar := anchorResolution{agent: biz.Agent{ID: "ag-1", AgentKey: "leader", DisplayName: "Leader"}}
	newRun := func() biz.TeamRunRecord {
		return biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	}

	t.Run("deny input refuses before llm", func(t *testing.T) {
		run := newRun()
		_, status, err := r.prepareUserTurnOptions(
			context.Background(), ar, "rm -rf /tmp/data",
			biz.Session{ID: "sess-1"}, &run, biz.Team{}, "default", time.Now(), pendingTeamIntent{})
		if err == nil || status != biz.TeamMemberStepStatusError {
			t.Fatalf("deny must fail turn: status=%s err=%v", status, err)
		}
		if !strings.Contains(err.Error(), "不可逆") {
			t.Errorf("deny error = %v, want SafetyDenyUserMessage", err)
		}
	})

	t.Run("hitl input injects caution notice", func(t *testing.T) {
		run := newRun()
		opts, status, err := r.prepareUserTurnOptions(
			context.Background(), ar, "请对 sw1 执行 fault_inject",
			biz.Session{ID: "sess-1"}, &run, biz.Team{}, "default", time.Now(), pendingTeamIntent{})
		if err != nil || status != biz.TeamMemberStepStatusOK {
			t.Fatalf("prepareUserTurnOptions: status=%s err=%v", status, err)
		}
		if len(opts.intentRunOpts) != 1 {
			t.Fatalf("intentRunOpts len = %d, want 1（HITL 降级注入）", len(opts.intentRunOpts))
		}
		var ro trpcagent.RunOptions
		opts.intentRunOpts[0](&ro)
		if len(ro.InjectedContextMessages) != 1 ||
			!strings.Contains(ro.InjectedContextMessages[0].Content, "Input safety notice") {
			t.Errorf("injected messages = %+v, want 1 条含 Input safety notice 头", ro.InjectedContextMessages)
		}
	})

	t.Run("safe input no injection", func(t *testing.T) {
		run := newRun()
		opts, status, err := r.prepareUserTurnOptions(
			context.Background(), ar, "帮我查一下 sw1 的端口状态",
			biz.Session{ID: "sess-1"}, &run, biz.Team{}, "default", time.Now(), pendingTeamIntent{})
		if err != nil || status != biz.TeamMemberStepStatusOK {
			t.Fatalf("prepareUserTurnOptions: status=%s err=%v", status, err)
		}
		if len(opts.intentRunOpts) != 0 {
			t.Errorf("intentRunOpts len = %d, want 0（安全输入不注入）", len(opts.intentRunOpts))
		}
	})

	t.Run("direct reply skips intent", func(t *testing.T) {
		run := newRun()
		opts, status, err := r.prepareUserTurnOptions(
			context.Background(), ar, "你好，请介绍你自己。不要调用工具。",
			biz.Session{ID: "sess-1"}, &run, biz.Team{}, "default", time.Now(), pendingTeamIntent{skip: true})
		if err != nil || status != biz.TeamMemberStepStatusOK {
			t.Fatalf("prepareUserTurnOptions: status=%s err=%v", status, err)
		}
		if len(opts.intentRunOpts) != 0 {
			t.Errorf("direct-reply skip must not inject intent, got %d opts", len(opts.intentRunOpts))
		}
	})
}

func TestStartTeamIntentPass_SkipDirectReply(t *testing.T) {
	r := &Runner{lg: loggateway.NewNoop()}
	p := r.startTeamIntentPass(context.Background(), anchorResolution{agent: biz.Agent{ID: "ag-1"}}, "你好，请介绍你自己。不要调用工具。")
	if !p.skip || p.ch != nil {
		t.Fatalf("direct reply must skip intent, skip=%v ch=%v", p.skip, p.ch)
	}
}

type nopRunTransitioner struct{}

func (nopRunTransitioner) TransitionRunStatus(context.Context, string, string) (biz.TeamRunRecord, error) {
	return biz.TeamRunRecord{}, errors.New("test skip persist")
}
