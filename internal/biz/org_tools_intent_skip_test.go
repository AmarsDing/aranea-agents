package biz

import "testing"

func TestApplyIntentSkipPolicy(t *testing.T) {
	t.Run("spirit", func(t *testing.T) {
		s := AgentRuntimeSettings{IntentSkipEnabled: true}
		ApplyIntentSkipPolicy(&s, Agent{AgentKey: SpiritAgentKey})
		if s.IntentSkipEnabled {
			t.Fatal("spirit must not skip intent pass")
		}
	})
	t.Run("dept lead", func(t *testing.T) {
		s := AgentRuntimeSettings{IntentSkipEnabled: true}
		ApplyIntentSkipPolicy(&s, Agent{AgentKey: "__dept_lead_mkt__", AgentVariant: "dept_lead"})
		if s.IntentSkipEnabled {
			t.Fatal("dept_lead must not skip intent pass")
		}
	})
	t.Run("company lead", func(t *testing.T) {
		s := AgentRuntimeSettings{IntentSkipEnabled: true}
		ApplyIntentSkipPolicy(&s, Agent{AgentKey: CompanyLeadAgentKeyPrefix + "acme__", AgentVariant: AgentVariantCompanyLead})
		if s.IntentSkipEnabled {
			t.Fatal("company_lead must not skip intent pass")
		}
	})
	t.Run("specialist unchanged", func(t *testing.T) {
		s := AgentRuntimeSettings{IntentSkipEnabled: true}
		ApplyIntentSkipPolicy(&s, Agent{AgentKey: "ops_copywriter"})
		if !s.IntentSkipEnabled {
			t.Fatal("ordinary agent keeps skip enabled")
		}
	})
}
