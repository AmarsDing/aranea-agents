package team

import (
	"testing"

	"aranea-agents/internal/biz"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestBuildEscalationFunc_ToolCallApprove(t *testing.T) {
	fn := buildEscalationFunc(&CriticLoopConfig{ScoreThreshold: 0.8})
	ev := &trpcevent.Event{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{
					ToolCalls: []trpcmodel.ToolCall{{
						Function: trpcmodel.FunctionDefinitionParam{
							Name:      biz.OrchestrationControlToolName,
							Arguments: []byte(`{"action":"approve","score":0.9}`),
						},
					}},
				},
			}},
		},
	}
	if !fn(ev) {
		t.Fatal("expected escalation via tool call approve")
	}
}

func TestBuildEscalationFunc_ToolCallScoreAboveThreshold(t *testing.T) {
	fn := buildEscalationFunc(&CriticLoopConfig{ScoreThreshold: 0.7})
	ev := &trpcevent.Event{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{
					ToolCalls: []trpcmodel.ToolCall{{
						Function: trpcmodel.FunctionDefinitionParam{
							Name:      biz.OrchestrationControlToolName,
							Arguments: []byte(`{"action":"retry","score":0.8}`),
						},
					}},
				},
			}},
		},
	}
	if !fn(ev) {
		t.Fatal("expected escalation via tool call score above threshold")
	}
}

func TestDefaultEscalationFunc_ToolCallApprove(t *testing.T) {
	ev := &trpcevent.Event{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{
					ToolCalls: []trpcmodel.ToolCall{{
						Function: trpcmodel.FunctionDefinitionParam{
							Name:      biz.OrchestrationControlToolName,
							Arguments: []byte(`{"action":"approve"}`),
						},
					}},
				},
			}},
		},
	}
	if !defaultEscalationFunc(ev) {
		t.Fatal("expected escalation via tool call approve in default func")
	}
}

func TestDefaultEscalationFunc_FallbackString(t *testing.T) {
	ev := &trpcevent.Event{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{
					Content: "The work is approved.",
				},
			}},
		},
	}
	if !defaultEscalationFunc(ev) {
		t.Fatal("expected escalation via string fallback")
	}
}
