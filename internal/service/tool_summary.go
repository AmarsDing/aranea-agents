package service

import (
	v1 "aranea-agents/api/kratos/tool/v1"
	"aranea-agents/internal/biz"
)

func bizSummaryToProto(s biz.ToolSummary) *v1.ToolSummary {
	return &v1.ToolSummary{
		TotalTools:      int32(s.TotalTools),
		EnabledTools:    int32(s.EnabledTools),
		HighRiskEnabled: int32(s.HighRiskEnabled),
		Calls_24H:       int32(s.Calls24h),
		FailureRate_24H: s.FailureRate24h,
		Categories:      s.Categories,
		UsedTools_7D:    int32(s.UsedTools7d),
	}
}
