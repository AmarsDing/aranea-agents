package biz

import "testing"

func TestEvaluateIngressPolicy(t *testing.T) {
	cases := []struct {
		name string
		in   IngressPolicyInput
		want IngressDecision
	}{
		{"duplicate skips", IngressPolicyInput{IsRecentDuplicate: true}, IngressSkipDuplicate},
		{"cancel command", IngressPolicyInput{IsCancelCommand: true}, IngressCancel},
		{"background command", IngressPolicyInput{IsBackgroundCommand: true}, IngressRouteBackground},
		{"route async", IngressPolicyInput{RouteAsync: true}, IngressRouteAsync},
		{"status with active run", IngressPolicyInput{IsStatusQuery: true, HasActiveRun: true}, IngressStatus},
		{"status without active run", IngressPolicyInput{IsStatusQuery: true, HasActiveRun: false}, IngressAdmit},
		{"context pressure with active run no queue", IngressPolicyInput{ContextPressure: true, HasActiveRun: true, EntryPoint: EntryPointChannel}, IngressQueue},
		{"context pressure with active run allow queue", IngressPolicyInput{ContextPressure: true, HasActiveRun: true, AllowQueue: true}, IngressQueue},
		{"no active run admits", IngressPolicyInput{}, IngressAdmit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateIngressPolicy(tc.in)
			if got.Decision != tc.want {
				t.Errorf("EvaluateIngressPolicy(%+v).Decision = %v, want %v", tc.in, got.Decision, tc.want)
			}
		})
	}
}

func TestAllowPendingQueueFromEntry(t *testing.T) {
	cases := []struct {
		entry      TurnEntryPoint
		allowQueue bool
		want       bool
	}{
		{"", false, true},
		{"", true, true},
		{EntryPointChannel, false, false},
		{EntryPointChannel, true, true},
	}
	for _, tc := range cases {
		got := AllowPendingQueueFromEntry(tc.entry, tc.allowQueue)
		if got != tc.want {
			t.Errorf("AllowPendingQueueFromEntry(%q, %v) = %v, want %v", tc.entry, tc.allowQueue, got, tc.want)
		}
	}
}

func TestIngressDecisionNeedsTurn(t *testing.T) {
	cases := []struct {
		decision IngressDecision
		want     bool
	}{
		{IngressAdmit, true},
		{IngressQueue, true},
		{IngressInterrupt, true},
		{IngressCancel, false},
		{IngressRouteAsync, false},
		{IngressRouteBackground, false},
		{IngressSkipDuplicate, false},
		{IngressStatus, false},
	}
	for _, tc := range cases {
		got := IngressDecisionNeedsTurn(tc.decision)
		if got != tc.want {
			t.Errorf("IngressDecisionNeedsTurn(%v) = %v, want %v", tc.decision, got, tc.want)
		}
	}
}

func TestIngressIntentLabel(t *testing.T) {
	cases := []struct {
		decision IngressDecision
		want     string
	}{
		{IngressAdmit, "admit"},
		{IngressQueue, "queue"},
		{IngressSkipDuplicate, "skip_duplicate"},
	}
	for _, tc := range cases {
		got := IngressIntentLabel(tc.decision)
		if got != tc.want {
			t.Errorf("IngressIntentLabel(%v) = %q, want %q", tc.decision, got, tc.want)
		}
	}
}
