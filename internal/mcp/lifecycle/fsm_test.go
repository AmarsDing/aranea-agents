package lifecycle

import "testing"

func TestTransition_AllLegal(t *testing.T) {
	cases := []struct {
		from  State
		event Event
		want  State
	}{
		{StateUnknown, EventProbeOK, StateOK},
		{StateUnknown, EventProbeFail, StateError},
		{StateUnknown, EventProbeAuthRequired, StateAuthRequired},
		{StateOK, EventProbeFail, StateError},
		{StateOK, EventStale, StateDegraded},
		{StateError, EventProbeOK, StateOK},
		{StateAuthRequired, EventProbeOK, StateOK},
		{StateDegraded, EventProbeOK, StateOK},
		{StateOK, EventReset, StateUnknown},
	}
	for _, tc := range cases {
		got, err := Transition(tc.from, tc.event)
		if err != nil {
			t.Fatalf("%s --%s-->: %v", tc.from, tc.event, err)
		}
		if got != tc.want {
			t.Fatalf("%s --%s--> = %s, want %s", tc.from, tc.event, got, tc.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	if Normalize("Healthy") != StateOK {
		t.Fatal("expected Healthy → ok")
	}
	if Normalize("") != StateUnknown {
		t.Fatal("expected empty → unknown")
	}
}

func TestEventFromProbeStatus(t *testing.T) {
	if EventFromProbeStatus("ok") != EventProbeOK {
		t.Fatal("ok")
	}
	if EventFromProbeStatus("auth_required") != EventProbeAuthRequired {
		t.Fatal("auth_required")
	}
	if EventFromProbeStatus("boom") != EventProbeFail {
		t.Fatal("fail")
	}
}
