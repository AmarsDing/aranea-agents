package team

import (
	"context"

	"aranea-agents/internal/biz"
)

type taskUsecaseCreator struct {
	uc *biz.TaskUsecase
}

// NewTaskUsecaseGraphTaskCreator adapts TaskUsecase for team Graph task nodes.
func NewTaskUsecaseGraphTaskCreator(uc *biz.TaskUsecase) TeamGraphTaskCreator {
	if uc == nil {
		return nil
	}
	return taskUsecaseCreator{uc: uc}
}

func (c taskUsecaseCreator) CreateGraphTask(ctx context.Context, graphExecutionID, _ string, node biz.NodeDef) error {
	var meta biz.NodeTaskMeta
	role, mode, strategy, input := biz.GraphTaskInputFromNode(node, meta)
	_, err := c.uc.CreateTask(ctx, node.ID, graphExecutionID, role, mode, strategy, input, "{}")
	return err
}
