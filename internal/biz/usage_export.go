package biz

import (
	"fmt"
	"strings"
)

func formatUsageEventsCSV(events []TokenUsageEvent) string {
	var b strings.Builder
	b.WriteString("occurred_at,usage_kind,agent_id,provider_code,model_api_id,session_id,team_id,input_tokens,output_tokens,total_tokens,total_cost_micro_usd,latency_ms,status,error_message\n")
	for _, e := range events {
		b.WriteString(fmt.Sprintf("%q,%q,%q,%q,%q,%q,%q,%d,%d,%d,%d,%d,%q,%q\n",
			e.OccurredAt, e.UsageKind, e.AgentID, e.ProviderCode, e.ModelAPIID, e.SessionID, e.TeamID,
			e.InputTokens, e.OutputTokens, e.TotalTokens, e.TotalCostMicroUSD, e.LatencyMS, e.Status, csvEscape(e.ErrorMessage),
		))
	}
	return b.String()
}

func csvEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `"`, `""`), "\n", " ")
}
