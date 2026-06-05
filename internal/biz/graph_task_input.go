package biz

import "strings"

func GraphTaskInputFromNode(node NodeDef, meta NodeTaskMeta) (requiredRole, assignmentMode, assignmentStrategy, input string) {
	assignmentMode = strings.TrimSpace(meta.AssignmentMode)
	if assignmentMode == "" {
		assignmentMode = AssignmentModeStatic
	}
	input = strings.TrimSpace(node.Description)
	if input == "" {
		input = node.ID
	}
	return strings.TrimSpace(meta.RequiredRole), assignmentMode, strings.TrimSpace(meta.AssignmentStrategy), input
}
