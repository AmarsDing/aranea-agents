package orchestrator

import "fmt"

// VerifyOutputFormat checks that all agent results are non-empty.
func VerifyOutputFormat(results map[string]any) (verified bool, issues []string) {
	for agentKey, result := range results {
		resultStr, ok := result.(string)
		if !ok || resultStr == "" {
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
