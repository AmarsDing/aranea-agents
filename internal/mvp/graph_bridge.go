package mvp

import (
	"fmt"
	"strings"

	"aranea-agents/internal/biz/review"
)

// EnqueueFromGraphPause creates (or returns existing) ReviewTask when a Graph
// review/human node pauses for HITL. Follows existing waiting_human + GraphTask
// patterns — does not invent a parallel FSM.
func EnqueueFromGraphPause(executionID, nodeID, graphTaskID, contentItemID, draftTitle, draftBody string, platformTargets []string) (review.Task, error) {
	rt := Ensure()
	if rt == nil || rt.Review == nil {
		return review.Task{}, fmt.Errorf("mvp: review usecase not configured")
	}
	executionID = strings.TrimSpace(executionID)
	nodeID = strings.TrimSpace(nodeID)
	if executionID == "" || nodeID == "" {
		return review.Task{}, fmt.Errorf("mvp: graph_execution_id and graph_node_id required")
	}
	if existing, ok, err := rt.Review.GetByGraphExecution(executionID, nodeID); err != nil {
		return review.Task{}, err
	} else if ok {
		// Backfill GraphTaskID if the task board row arrived later.
		if graphTaskID != "" && existing.GraphTaskID == "" {
			existing.GraphTaskID = graphTaskID
			return rt.Review.UpdateLinks(existing)
		}
		return existing, nil
	}
	if strings.TrimSpace(contentItemID) == "" {
		contentItemID = "graph:" + executionID + ":" + nodeID
	}
	if draftTitle == "" && draftBody == "" {
		draftTitle = "Graph review: " + nodeID
		draftBody = "Awaiting human approval for graph execution " + executionID
	}
	task, err := rt.Review.Enqueue(review.Task{
		ContentItemID:    contentItemID,
		GraphExecutionID: executionID,
		GraphNodeID:      nodeID,
		GraphTaskID:      graphTaskID,
		DraftTitle:       draftTitle,
		DraftBody:        draftBody,
		PlatformTargets:  platformTargets,
	})
	if err != nil {
		return review.Task{}, err
	}
	if contentItemID != "" && !strings.HasPrefix(contentItemID, "graph:") && rt.Content != nil {
		_, _ = rt.Content.MarkInReview(contentItemID)
	}
	return task, nil
}

// AfterReviewDecision resumes Graph / completes GraphTask after approve/reject.
// Returns nil when the review task has no graph linkage (HTTP-only reviews).
func AfterReviewDecision(task review.Task) error {
	rt := Ensure()
	if rt == nil {
		return nil
	}
	decision := task.Status
	resumeValue := map[string]any{
		"review_task_id":  task.ID,
		"review_decision": decision,
		"review_note":     task.DecisionNote,
		"reviewer_id":     task.ReviewerID,
		"content_item_id": task.ContentItemID,
		"graph_node_id":   task.GraphNodeID,
	}
	var firstErr error
	if task.GraphTaskID != "" && rt.GraphTaskComplete != nil {
		output := fmt.Sprintf(`{"decision":%q,"note":%q}`, decision, task.DecisionNote)
		if err := rt.GraphTaskComplete(task.GraphTaskID, task.ReviewerID, output, decision); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if task.GraphExecutionID != "" && rt.GraphResume != nil {
		// Prefer GraphTaskComplete path when both are set; still resume as
		// safety net if task complete did not run (no GraphTaskID).
		if task.GraphTaskID == "" {
			if err := rt.GraphResume(task.GraphExecutionID, resumeValue); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
