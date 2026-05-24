package runtime

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestLaneScheduler_limitsCron(t *testing.T) {
	sched := NewLaneScheduler(LaneLimits{Cron: 1})
	release := sched.Acquire(context.Background(), TurnLaneCron)
	if sched.InFlight(TurnLaneCron) != 1 {
		t.Fatalf("inflight=%d", sched.InFlight(TurnLaneCron))
	}
	blockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	release2 := sched.Acquire(blockCtx, TurnLaneCron)
	release()
	if sched.InFlight(TurnLaneCron) != 0 {
		t.Fatal("slot should be released")
	}
	release3 := sched.Acquire(context.Background(), TurnLaneCron)
	if sched.InFlight(TurnLaneCron) != 1 {
		t.Fatal("third acquire should succeed")
	}
	release2()
	release3()
}

func TestTurnLaneFromEntry(t *testing.T) {
	if TurnLaneFromEntry(biz.EntryPointCron, "agent") != TurnLaneCron {
		t.Fatal("cron lane")
	}
	if TurnLaneFromEntry(biz.EntryPointDurable, "agent") != TurnLaneDurable {
		t.Fatal("durable lane")
	}
	if TurnLaneFromEntry(biz.EntryPointChannel, "team") != TurnLaneTeam {
		t.Fatal("team owner")
	}
	if TurnLaneFromEntry(biz.EntryPointWS, "agent") != TurnLaneMain {
		t.Fatal("main lane")
	}
}

func TestLaneScheduler_mainUnlimited(t *testing.T) {
	sched := NewLaneScheduler(DefaultLaneLimits())
	for i := 0; i < 8; i++ {
		sched.Acquire(context.Background(), TurnLaneMain)
	}
	if sched.InFlight(TurnLaneMain) != 0 {
		t.Fatalf("main lane should be unlimited, inflight=%d", sched.InFlight(TurnLaneMain))
	}
}
