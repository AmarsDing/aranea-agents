package biz

import "strings"

// GraphTaskInputFromNode derives Kanban task fields from a graph node definition.
func GraphTaskInputFromNode(node NodeDef) (requiredRole, assignmentMode, assignmentStrategy, input string) {
	assignmentMode = strings.TrimSpace(node.AssignmentMode)
	if assignmentMode == "" {
		assignmentMode = "static"
	}
	input = strings.TrimSpace(node.Description)
	if input == "" {
		input = node.ID
	}
	return strings.TrimSpace(node.RequiredRole), assignmentMode, strings.TrimSpace(node.AssignmentStrategy), input
}
