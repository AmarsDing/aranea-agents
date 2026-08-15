package graph

import (
	"context"
	"strings"
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// captureAgent 记录 Run 收到的 invocation 消息，用于验证注入结果。
type captureAgent struct {
	got      trpcmodel.Message
	gotValid bool
}

func (c *captureAgent) Run(_ context.Context, inv *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	if inv != nil {
		c.got = inv.Message
		c.gotValid = true
	}
	ch := make(chan *trpcevent.Event)
	close(ch)
	return ch, nil
}

func (c *captureAgent) Tools() []trpctool.Tool         { return nil }
func (c *captureAgent) Info() trpcagent.Info           { return trpcagent.Info{Name: "capture"} }
func (c *captureAgent) SubAgents() []trpcagent.Agent   { return nil }
func (c *captureAgent) FindSubAgent(string) trpcagent.Agent { return nil }

func TestWrapAgentNodeInstruction_emptyInstructionReturnsInner(t *testing.T) {
	inner := &captureAgent{}
	if got := wrapAgentNodeInstruction(inner, "  "); got != trpcagent.Agent(inner) {
		t.Fatalf("empty instruction should return inner unchanged")
	}
	if got := wrapAgentNodeInstruction(nil, "x"); got != nil {
		t.Fatalf("nil inner should return nil")
	}
}

func TestPrependNodeInstruction_contentString(t *testing.T) {
	msg := trpcmodel.Message{Role: trpcmodel.RoleUser, Content: "原始输入"}
	out := prependNodeInstruction(msg, "先核实再清障")
	if !strings.HasPrefix(out.Content, "【节点指令】\n先核实再清障\n\n【任务输入】\n原始输入") {
		t.Fatalf("content=%q", out.Content)
	}
	// 原消息不被修改
	if msg.Content != "原始输入" {
		t.Fatalf("original message mutated: %q", msg.Content)
	}
}

func TestPrependNodeInstruction_contentParts(t *testing.T) {
	orig := "原始part"
	msg := trpcmodel.Message{
		Role:         trpcmodel.RoleUser,
		ContentParts: []trpcmodel.ContentPart{{Type: trpcmodel.ContentTypeText, Text: &orig}},
	}
	out := prependNodeInstruction(msg, "指令A")
	if len(out.ContentParts) != 2 {
		t.Fatalf("parts=%d", len(out.ContentParts))
	}
	head := out.ContentParts[0]
	if head.Type != trpcmodel.ContentTypeText || head.Text == nil ||
		!strings.Contains(*head.Text, "指令A") {
		t.Fatalf("head=%+v", head)
	}
	if out.ContentParts[1].Text == nil || *out.ContentParts[1].Text != "原始part" {
		t.Fatalf("tail part lost: %+v", out.ContentParts[1])
	}
	// 原切片不被修改
	if len(msg.ContentParts) != 1 {
		t.Fatalf("original parts mutated: %d", len(msg.ContentParts))
	}
}

func TestPrependNodeInstruction_emptyMessage(t *testing.T) {
	out := prependNodeInstruction(trpcmodel.Message{Role: trpcmodel.RoleUser}, "指令B")
	if !strings.Contains(out.Content, "指令B") {
		t.Fatalf("content=%q", out.Content)
	}
}

func TestInstructionInjectAgent_RunInjectsAndDelegates(t *testing.T) {
	inner := &captureAgent{}
	wrapped := wrapAgentNodeInstruction(inner, "用 gns3_fault_clear 恢复端口")
	inv := &trpcagent.Invocation{
		Message: trpcmodel.Message{Role: trpcmodel.RoleUser, Content: "R1 端口 down"},
	}
	ch, err := wrapped.Run(context.Background(), inv)
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	if !inner.gotValid {
		t.Fatal("inner Run not called")
	}
	if !strings.Contains(inner.got.Content, "用 gns3_fault_clear 恢复端口") ||
		!strings.Contains(inner.got.Content, "R1 端口 down") {
		t.Fatalf("injected content=%q", inner.got.Content)
	}
	// 设计为原地改写（框架为每次节点运行全新 Clone invocation，对象由本次
	// 运行独占，见 agent_instruction.go Run 注释），调用方应观察到注入结果。
	if !strings.Contains(inv.Message.Content, "节点指令") {
		t.Fatalf("in-place injection expected, got: %q", inv.Message.Content)
	}
	// 接口委托
	if wrapped.Info().Name != "capture" {
		t.Fatalf("Info not delegated: %+v", wrapped.Info())
	}
	if wrapped.Tools() != nil || wrapped.SubAgents() != nil || wrapped.FindSubAgent("x") != nil {
		t.Fatal("delegation mismatch")
	}
}

func TestInstructionInjectAgent_RunNilInvocation(t *testing.T) {
	inner := &captureAgent{}
	wrapped := wrapAgentNodeInstruction(inner, "指令")
	if _, err := wrapped.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if inner.gotValid {
		t.Fatal("nil invocation should pass through without injection")
	}
}
