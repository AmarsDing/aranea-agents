package graph

import (
	"context"
	"strings"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// instructionInjectAgent 把图节点 instruction 注入 agent 节点的用户消息头部。
//
// 背景（2026-08-15 Step3-1 复盘根因）：agent 节点经 AddAgentNode 接线，框架
// 不接收节点 instruction（仅 LLM 节点 AddLLMNode 使用），图定义中编写的节点
// 指令被静默丢弃，agent 只带目录人设运行。
//
// 缓存纪律：注入只动用户消息（prompt 尾部），不触碰系统提示与工具声明
// （prompt 前缀），保持 LLM 前缀缓存与 agent 构建缓存命中。
type instructionInjectAgent struct {
	inner       trpcagent.Agent
	instruction string
}

// wrapAgentNodeInstruction 在 agent 节点外包一层 instruction 注入。
// instruction 为空或 inner 为 nil 时原样返回。
func wrapAgentNodeInstruction(inner trpcagent.Agent, instruction string) trpcagent.Agent {
	trimmed := strings.TrimSpace(instruction)
	if inner == nil || trimmed == "" {
		return inner
	}
	return &instructionInjectAgent{inner: inner, instruction: trimmed}
}

func (a *instructionInjectAgent) Tools() []trpctool.Tool { return a.inner.Tools() }

func (a *instructionInjectAgent) Info() trpcagent.Info { return a.inner.Info() }

func (a *instructionInjectAgent) SubAgents() []trpcagent.Agent { return a.inner.SubAgents() }

func (a *instructionInjectAgent) FindSubAgent(name string) trpcagent.Agent {
	return a.inner.FindSubAgent(name)
}

func (a *instructionInjectAgent) Run(ctx context.Context, inv *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	if inv != nil {
		// 原地改写 Message 是安全的：框架为每次 agent 节点执行经
		// parentInvocation.Clone(...) 全新构建 invocation（vendored
		// state_graph.go buildAgentInvocationWithStateScopeAndInputKey），
		// 该对象由本次节点运行独占；且改写发生在 inner.Run 之前，尚无
		// 任何消费方读取 Message。框架已冻结（FW-R1~R3），该行为被钉死。
		// 不整体拷贝/再 Clone：Invocation 内含 sync.Mutex（vet 禁止拷贝），
		// 而 Clone 会换新 InvocationID、改变 parent 链，影响事件关联。
		inv.Message = prependNodeInstruction(inv.Message, a.instruction)
	}
	return a.inner.Run(ctx, inv)
}

// prependNodeInstruction 返回指令在前、原始输入在后的新消息；原消息不被修改。
func prependNodeInstruction(msg trpcmodel.Message, instruction string) trpcmodel.Message {
	header := "【节点指令】\n" + instruction + "\n\n【任务输入】\n"
	out := msg
	switch {
	case msg.Content != "":
		out.Content = header + msg.Content
	case len(msg.ContentParts) > 0:
		parts := make([]trpcmodel.ContentPart, 0, len(msg.ContentParts)+1)
		parts = append(parts, trpcmodel.ContentPart{Type: trpcmodel.ContentTypeText, Text: &header})
		parts = append(parts, msg.ContentParts...)
		out.ContentParts = parts
	default:
		out.Content = header
	}
	return out
}
