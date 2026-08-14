package agent

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// ─── PinnedModelSelector（P3-4 评测态 profile，ADR-E D2）─────────────────────
//
// 与 CascadeModelSelector 的保守 nil 语义刻意不同：评测钉住必须 fail-loud——
// 在错误模型上跑出「看似有效」的评测结果比 run 失败更糟（DSH 能力缝原则）。

// 钉住对 leader 与 member 一视同仁：任意 AgentName 都返回固定模型。
func TestPinnedModelSelector_PinsLeaderAndMember(t *testing.T) {
	sel := PinnedModelSelector("openai", "gpt-4o", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	for _, agentName := range []string{"leader-key", "member-key", ""} {
		m, err := sel(context.Background(), &trpcagent.Invocation{AgentName: agentName})
		if err != nil {
			t.Fatalf("agent %q: err = %v, want nil", agentName, err)
		}
		if m == nil {
			t.Fatalf("agent %q: model = nil, want pinned gpt-4o", agentName)
		}
		if got := m.Info().Name; got != "gpt-4o" {
			t.Fatalf("agent %q: model = %q, want gpt-4o", agentName, got)
		}
	}
}

// 钉住模型解析失败必须返回 error（fail-loud），禁止静默回退 base。
func TestPinnedModelSelector_FailLoudOnUnknownModel(t *testing.T) {
	sel := PinnedModelSelector("openai", "no-such-model", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	m, err := sel(context.Background(), &trpcagent.Invocation{AgentName: "member-key"})
	if err == nil {
		t.Fatal("err = nil, want fail-loud error for unknown pinned model")
	}
	if m != nil {
		t.Fatalf("model = %v, want nil on failure", m.Info().Name)
	}
}

// model 为空 = 钉住不生效（配置完整性由 team 侧装配判定）。
func TestPinnedModelSelector_EmptyModelDisabled(t *testing.T) {
	sel := PinnedModelSelector("openai", "", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	m, err := sel(context.Background(), &trpcagent.Invocation{AgentName: "member-key"})
	if err != nil || m != nil {
		t.Fatalf("empty model: got (%v, %v), want (nil, nil)", m, err)
	}
}

func TestPinnedModelSelector_NilInvocation(t *testing.T) {
	sel := PinnedModelSelector("openai", "gpt-4o", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	m, err := sel(context.Background(), nil)
	if err != nil || m != nil {
		t.Fatalf("nil invocation: got (%v, %v), want (nil, nil)", m, err)
	}
}
