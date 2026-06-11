package orchestrator

import "fmt"

// VerifyOutputFormat checks that all agent results are non-empty.
func VerifyOutputFormat(results map[string]any) (verified bool, issues []string) {
	for agentKey, result := range results {
		var resultStr string
		switch v := result.(type) {
		case string:
			resultStr = v
		case nil:
			resultStr = ""
		default:
			resultStr = fmt.Sprint(v)
		}
		if resultStr == "" {
			issues = append(issues, fmt.Sprintf("agent %s returned empty result", agentKey))
		}
	}
	return len(issues) == 0, issues
}

// VerifyTaskCompletion checks that all assigned agents have reported results.
func VerifyTaskCompletion(results map[string]any, expectedAgents []string) (verified bool, issues []string) {
	for _, agentKey := range expectedAgents {
		if _, exists := results[agentKey]; !exists {
			issues = append(issues, fmt.Sprintf("agent %s has not reported results", agentKey))
		}
	}
	return len(issues) == 0, issues
}
