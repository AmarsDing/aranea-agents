package biz

import "aranea-agents/internal/biz/flowlog"

// Re-export flow log types from sub-package for backward compatibility.
type (
	FlowLogRecord     = flowlog.Record
	FlowLogQuery      = flowlog.Query
	FlowLogListResult = flowlog.ListResult
	FlowLogRepo       = flowlog.Repo
	FlowLogUsecase    = flowlog.Usecase
)

// Re-export flow log constructor for backward compatibility.
var NewFlowLogUsecase = flowlog.NewUsecase
