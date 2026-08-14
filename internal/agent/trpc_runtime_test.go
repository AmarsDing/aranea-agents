package agent

import (
	"context"
	"testing"

	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

var _ trpcagent.Agent = (*staticTRPCAgent)(nil)

type staticTRPCAgent struct {
	name  string
	reply string
}

func (a staticTRPCAgent) Run(ctx context.Context, inv *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	ch := make(chan *trpcevent.Event, 1)
	safego.Go(ctx, "static-agent-reply", func() {
		defer close(ch)
		select {
		case <-ctx.Done():
			ch <- trpcevent.NewErrorEvent(inv.InvocationID, a.name, trpcmodel.ErrorTypeCancelled, ctx.Err().Error())
		default:
			ch <- trpcevent.NewResponseEvent(inv.InvocationID, a.name, &trpcmodel.Response{
				Object: trpcmodel.ObjectTypeChatCompletion,
				Done:   true,
				Choices: []trpcmodel.Choice{{
					Index:   0,
					Message: trpcmodel.NewAssistantMessage(a.reply),
				}},
			})
		}
	})
	return ch, nil
}

func (a staticTRPCAgent) Tools() []trpctool.Tool { return nil }

func (a staticTRPCAgent) Info() trpcagent.Info {
	return trpcagent.Info{Name: a.name, Description: "static test agent"}
}

func (a staticTRPCAgent) SubAgents() []trpcagent.Agent { return nil }

func (a staticTRPCAgent) FindSubAgent(name string) trpcagent.Agent { return nil }

func TestNewTRPCRunnerAndRunUserTurn(t *testing.T) {
	r, err := NewTRPCRunner(staticTRPCAgent{name: "assistant", reply: "hello"}, TRPCRunnerDeps{
		SessionService: sessiontrpc.NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatalf("NewTRPCRunner() error = %v", err)
	}
	defer r.Close()

	events, err := RunTRPCUserTurnMsg(context.Background(), r, "user-1", "session-1", trpcmodel.NewUserMessage("hi"))
	if err != nil {
		t.Fatalf("RunTRPCUserTurnMsg() error = %v", err)
	}

	var sawReply, sawCompletion bool
	for ev := range events {
		if ev == nil || ev.Response == nil {
			continue
		}
		if ev.IsRunnerCompletion() {
			sawCompletion = true
			continue
		}
		if len(ev.Response.Choices) > 0 && ev.Response.Choices[0].Message.Content == "hello" {
			sawReply = true
		}
	}
	if !sawReply {
		t.Fatal("expected assistant reply event")
	}
	if !sawCompletion {
		t.Fatal("expected runner completion event")
	}
}

func TestRunTRPCUserTurnMsgValidatesRequiredIDs(t *testing.T) {
	r, err := NewTRPCRunner(staticTRPCAgent{name: "assistant", reply: "hello"}, TRPCRunnerDeps{})
	if err != nil {
		t.Fatalf("NewTRPCRunner() error = %v", err)
	}
	defer r.Close()

	if _, err := RunTRPCUserTurnMsg(context.Background(), r, "", "session-1", trpcmodel.NewUserMessage("hi")); err == nil {
		t.Fatal("expected missing user id error")
	}
	if _, err := RunTRPCUserTurnMsg(context.Background(), r, "user-1", "", trpcmodel.NewUserMessage("hi")); err == nil {
		t.Fatal("expected missing session id error")
	}
}

func TestNewTRPCRunnerReturnsManagedRunner(t *testing.T) {
	r, err := NewTRPCRunner(staticTRPCAgent{name: "assistant", reply: "hello"}, TRPCRunnerDeps{
		SessionService: sessiontrpc.NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatalf("NewTRPCRunner() error = %v", err)
	}
	defer r.Close()

	if _, ok := interface{}(r).(trpcrunner.Runner); !ok {
		t.Fatal("ManagedRunner should also implement Runner")
	}
}

