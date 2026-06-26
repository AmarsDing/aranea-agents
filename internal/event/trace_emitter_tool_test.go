package event

import (
	"testing"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

func TestObserveFrameworkEventDoesNotCloseToolOnResult(t *testing.T) {
	em := NewTraceEmitter(nil, TraceContext{TraceID: "tr_x", RunID: "r1"}, nil)
	callEV := &trpcevent.Event{Response: &model.Response{
		Choices: []model.Choice{{
			Message: model.Message{
				ToolCalls: []model.ToolCall{{ID: "call_1", Function: model.FunctionDefinitionParam{Name: "search"}}},
			},
		}},
	}}
	em.ObserveFrameworkEvent(callEV)
	resultEV := &trpcevent.Event{Response: &model.Response{
		Choices: []model.Choice{{
			Message: model.Message{ToolID: "call_1", Role: model.RoleTool},
		}},
	}}
	em.ObserveFrameworkEvent(resultEV)
	em.CompleteToolCall("call_1", "search", 50, "success")
	em.FinishRoot("ok")
}
