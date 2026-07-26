package team

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	deliverabletools "aranea-agents/internal/tools/deliverable"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

// delivSeqModel issues one set_deliverable tool call, then answers with text.
type delivSeqModel struct{ calls int }

func (m *delivSeqModel) GenerateContent(
	_ context.Context,
	_ *model.Request,
) (<-chan *model.Response, error) {
	ch := make(chan *model.Response, 1)
	m.calls++
	if m.calls == 1 {
		args, _ := json.Marshal(map[string]any{
			"data": map[string]any{"from": "m1"},
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
				Message: model.Message{Role: model.RoleAssistant, Content: "m1 done"},
			}},
		}
	}
	close(ch)
	return ch, nil
}

func (m *delivSeqModel) Info() model.Info { return model.Info{Name: "deliv-seq"} }

type delivFinalModel struct{}

func (m *delivFinalModel) GenerateContent(
	_ context.Context,
	_ *model.Request,
) (<-chan *model.Response, error) {
	ch := make(chan *model.Response, 1)
	ch <- &model.Response{
		Object: model.ObjectTypeChatCompletion,
		Done:   true,
		Choices: []model.Choice{{
			Message: model.Message{Role: model.RoleAssistant, Content: "m2 done"},
		}},
	}
	close(ch)
	return ch, nil
}

func (m *delivFinalModel) Info() model.Info { return model.Info{Name: "deliv-final"} }

type delivResolver struct{ agents map[string]trpcagent.Agent }

func (r delivResolver) ResolveAgent(
	_ context.Context,
	ref string,
) (trpcagent.Agent, error) {
	if a, ok := r.agents[ref]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("agent %q not found", ref)
}

// TestGraphRuntime_DeliverableStateDeltaReachesGraphState reproduces the
// 22:35 incident at the graph layer: a member agent calling set_deliverable
// inside a deliverable-enabled team graph must (1) emit a tool response event
// that still carries the StateDelta after graph forwarding, and (2) land the
// deliverable map in the final session state via the output mapper and the
// graph completion snapshot.
func TestGraphRuntime_DeliverableStateDeltaReachesGraphState(t *testing.T) {
	lg := loggateway.NewNoop()
	def := Definition{
		Mode: "sequential",
		Members: []MemberDef{
			{AgentID: "a", SortOrder: 1, Name: "A"},
			{AgentID: "b", SortOrder: 2, Name: "B"},
		},
	}
	cfg, err := CompileToGraphRuntimeConfig(def, nil, lg)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cfg = finalizeRuntimeGraphConfig(
		cfg, def,
		`{"mode":"sequential","enable_state_deliverable":true}`,
		nil, nil,
	)

	m1 := llmagent.New(
		"member-1",
		llmagent.WithModel(&delivSeqModel{}),
		llmagent.WithTools(deliverabletools.Tools()),
	)
	m2 := llmagent.New(
		"member-2",
		llmagent.WithModel(&delivFinalModel{}),
	)
	resolver := delivResolver{agents: map[string]trpcagent.Agent{
		"A": m1,
		"B": m2,
	}}

	ctx := context.Background()
	g, agents, err := graphtrpc.BuildStateGraphWithAgents(
		ctx, cfg,
		&graphtrpc.GraphNodeResolverSet{Agents: resolver},
		lg,
	)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	ga, err := graphtrpc.NewGraphAgent("team-deliv", g, false, agents...)
	if err != nil {
		t.Fatalf("graph agent: %v", err)
	}

	sessSvc := inmemory.NewSessionService()
	r := runner.NewRunner("app", ga, runner.WithSessionService(sessSvc))
	defer r.Close()

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
		t.Fatalf("forwarded tool event lost StateDelta[%q]", biz.DeliverableStateKey)
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
		t.Fatalf("unmarshal session deliverable: %v", err)
	}
	if stored["from"] != "m1" {
		t.Fatalf("graph deliverable=%v, want from=m1", stored)
	}
}
