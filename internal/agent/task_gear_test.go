package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestClassifyTaskGear(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   GearInput
		want TaskGear
	}{
		{"simple is light", GearInput{GateSimpleOrClarify: true}, GearLight},
		{"fact query is light", GearInput{FactQuery: true}, GearLight},
		{"fact query plus org chain is heavy", GearInput{FactQuery: true, UserWantsOrgChain: true}, GearHeavy},
		{"user org chain is heavy", GearInput{UserWantsOrgChain: true}, GearHeavy},
		{"long task is heavy", GearInput{LongTask: true}, GearHeavy},
		{"cross dept after plan is heavy", GearInput{CrossDeptDepends: true}, GearHeavy},
		{"two companies plus signal is heavy", GearInput{CompanyNodeCount: 2, IntercompanySignal: true}, GearHeavy},
		{"two companies no signal is medium", GearInput{CompanyNodeCount: 2}, GearMedium},
		{"default team task is medium", GearInput{}, GearMedium},
		{"heavy does not downgrade", GearInput{Current: GearHeavy, GateSimpleOrClarify: true}, GearHeavy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyTaskGear(tc.in); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestUpgradeGearAfterPlanNeverDowngrades(t *testing.T) {
	t.Parallel()
	if got := UpgradeGearAfterPlan(GearHeavy, false); got != GearHeavy {
		t.Fatalf("downgraded to %s", got)
	}
	if got := UpgradeGearAfterPlan(GearMedium, true); got != GearHeavy {
		t.Fatalf("did not upgrade: %s", got)
	}
}

func TestPlanHasCrossDeptDepends(t *testing.T) {
	t.Parallel()
	if PlanHasCrossDeptDepends([]biz.SubTask{
		{DomainPath: "软件/后端"},
		{DomainPath: "设计/视觉"},
	}) {
		t.Fatal("no DependsOn must not upgrade")
	}
	if !PlanHasCrossDeptDepends([]biz.SubTask{
		{DomainPath: "软件/后端"},
		{DomainPath: "设计/视觉", DependsOn: []string{"st1"}},
	}) {
		t.Fatal("cross-dept + DependsOn is heavy")
	}
}

func TestHasOrgChainIntent(t *testing.T) {
	t.Parallel()
	if !HasOrgChainIntent("这次请按组织链走编制汇报") {
		t.Fatal("expected intent")
	}
	if HasOrgChainIntent("改一句文案") {
		t.Fatal("light copy must not trip org chain")
	}
}
