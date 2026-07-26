// Package deliverable_test contains runtime reproduction tests that verify
// the set_deliverable StateDelta chain end-to-end at the flow layer:
// tool registration -> FunctionCallResponseProcessor -> attachStateDelta ->
// event StateDelta -> session state persistence.
package deliverable_test

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	toolsgov "aranea-agents/internal/tools"
	deliverabletools "aranea-agents/internal/tools/deliverable"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

// sequenceModel returns a tool_call response on the first request and a final
// text response on the second request.
type sequenceModel struct {
	calls int
}

func (m *sequenceModel) GenerateContent(
	_ context.Context,
	_ *model.Request,
) (<-chan *model.Response, error) {
	ch := make(chan *model.Response, 1)
	m.calls++
	if m.calls == 1 {
		args, _ := json.Marshal(map[string]any{
			"data": map[string]any{"result": "ok-42"},
		})
		ch <- &model.Response{
			Object: model.ObjectTypeChatCompletion,
			Done:   true,
			Choices: []model.Choice{{
				Message: model.Message{
					Role: model.RoleAssistant,
					ToolCalls: []model.ToolCall{{
						Type: "function",
						ID:   "call-1",
						Function: model.FunctionDefinitionParam{
							Name:      "set_deliverable",
							Arguments: args,
						},
					}},
				},
			}},
		}
	} else {
		ch <- &model.Response{
			Object: model.ObjectTypeChatCompletion,
			Done:   true,
			Choices: []model.Choice{{
				Message: model.Message{Role: model.RoleAssistant, Content: "done"},
			}},
		}
	}
	close(ch)
	return ch, nil
}

func (m *sequenceModel) Info() model.Info { return model.Info{Name: "seq-model"} }

// TestSetDeliverable_StateDeltaReachesSession reproduces the 22:35 failure:
// the set_deliverable tool response must carry a StateDelta and the session
// state must contain the deliverable key after the run.
func TestSetDeliverable_StateDeltaReachesSession(t *testing.T) {
	sessSvc := inmemory.NewSessionService()
	agt := llmagent.New(
		"m1",
		llmagent.WithModel(&sequenceModel{}),
		llmagent.WithTools(deliverabletools.Tools()),
	)
	r := runner.NewRunner("app", agt, runner.WithSessionService(sessSvc))
	defer r.Close()

	ctx := context.Background()
	evCh, err := r.Run(ctx, "u1", "s1", model.NewUserMessage("go"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var toolEv *event.Event
	for ev := range evCh {
		if ev == nil || ev.Response == nil || len(ev.Response.Choices) == 0 {
			continue
		}
		msg := ev.Response.Choices[0].Message
		if msg.Role == model.RoleTool && msg.ToolName == "set_deliverable" {
			toolEv = ev
		}
	}
	if toolEv == nil {
		t.Fatal("no set_deliverable tool response event observed")
	}
	raw, ok := toolEv.StateDelta[biz.DeliverableStateKey]
	if !ok || len(raw) == 0 {
		t.Fatalf("tool response event missing StateDelta[%q]; keys=%v",
			biz.DeliverableStateKey, deltaKeysOf(toolEv.StateDelta))
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal delta: %v", err)
	}
	if m["result"] != "ok-42" {
		t.Fatalf("deliverable delta=%v, want result=ok-42", m)
	}

	sess, err := sessSvc.GetSession(ctx, session.Key{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	stateRaw, found := sess.GetState(biz.DeliverableStateKey)
	if !found || len(stateRaw) == 0 {
		t.Fatalf("session state missing %q", biz.DeliverableStateKey)
	}
	var stored map[string]any
	if err := json.Unmarshal(stateRaw, &stored); err != nil {
		t.Fatalf("unmarshal session state: %v", err)
	}
	if stored["result"] != "ok-42" {
		t.Fatalf("session deliverable=%v, want result=ok-42", stored)
	}
}

// TestSetDeliverable_DecoratedStateDeltaReachesSession reproduces the 22:35
// production failure mode: tools assembled by buildToolsetsForAgent are
// wrapped via tools.ApplyDecorators (timeout/budget/cache governance). The
// decorated set_deliverable must still surface its StateDelta through the
// framework's Original() unwrapping — this guards the ToolDecorator.Original
// contract end-to-end at the runner layer.
func TestSetDeliverable_DecoratedStateDeltaReachesSession(t *testing.T) {
	sessSvc := inmemory.NewSessionService()
	assembled := &toolsgov.AssembledToolsets{Tools: deliverabletools.Tools()}
	toolsgov.ApplyDecorators(assembled, toolsgov.ToolDecoratorConfig{
		ResultBudget: toolsgov.DefaultResultBudget,
		EnableCache:  true,
	})
	agt := llmagent.New(
		"m1",
		llmagent.WithModel(&sequenceModel{}),
		llmagent.WithTools(assembled.Tools),
	)
	r := runner.NewRunner("app", agt, runner.WithSessionService(sessSvc))
	defer r.Close()

	ctx := context.Background()
	evCh, err := r.Run(ctx, "u1", "s1", model.NewUserMessage("go"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var toolEv *event.Event
	for ev := range evCh {
		if ev == nil || ev.Response == nil || len(ev.Response.Choices) == 0 {
			continue
		}
		msg := ev.Response.Choices[0].Message
		if msg.Role == model.RoleTool && msg.ToolName == "set_deliverable" {
			toolEv = ev
		}
	}
	if toolEv == nil {
		t.Fatal("no set_deliverable tool response event observed")
	}
	raw, ok := toolEv.StateDelta[biz.DeliverableStateKey]
	if !ok || len(raw) == 0 {
		t.Fatalf("decorated tool event missing StateDelta[%q]; keys=%v",
			biz.DeliverableStateKey, deltaKeysOf(toolEv.StateDelta))
	}

	sess, err := sessSvc.GetSession(ctx, session.Key{
		AppName: "app", UserID: "u1", SessionID: "s1",
	})
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	stateRaw, found := sess.GetState(biz.DeliverableStateKey)
	if !found || len(stateRaw) == 0 {
		t.Fatalf("session state missing %q", biz.DeliverableStateKey)
	}
	var stored map[string]any
	if err := json.Unmarshal(stateRaw, &stored); err != nil {
		t.Fatalf("unmarshal session state: %v", err)
	}
	if stored["result"] != "ok-42" {
		t.Fatalf("session deliverable=%v, want result=ok-42", stored)
	}
}

// InvocationFromContext is unused but referenced to keep the agent import
// aligned with future graph-layer reproduction steps.
var _ = agent.InvocationFromContext

func deltaKeysOf(d map[string][]byte) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	return keys
}
