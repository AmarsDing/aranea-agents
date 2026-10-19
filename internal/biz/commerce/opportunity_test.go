package commerce

import "testing"

func TestOpportunityAudienceAndWebhook(t *testing.T) {
	var events []string
	uc := NewOpportunityUsecase(NewMemoryOpportunityRepo())
	uc.SetWebhook(func(eventType string, payload map[string]any) error {
		events = append(events, eventType)
		return nil
	})
	if _, err := uc.Create(Opportunity{Title: "x", Audience: "bad"}); err == nil {
		t.Fatal("bad audience")
	}
	o, err := uc.Create(Opportunity{Title: "Intel", Audience: AudienceBoth, Score: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0] != EventOpportunityCreated {
		t.Fatalf("events=%v", events)
	}
	if _, err := uc.Acknowledge(o.ID); err != nil {
		t.Fatal(err)
	}
}
