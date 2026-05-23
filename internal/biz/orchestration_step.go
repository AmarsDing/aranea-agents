package biz

// OrchestrationStep is a persisted activity snapshot for timeline reconstruction.
type OrchestrationStep struct {
	ID                   string
	TeamRunID            string
	GraphExecutionID     string
	NodeID               string
	ActivitySnapshotJSON string
	Status               string
	StartedAt            string
	FinishedAt           string
	CreatedAt            string
}
