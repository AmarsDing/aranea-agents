package biz

import "aranea-agents/internal/biz/monitor"

type (
	AuditLog               = monitor.AuditLog
	AuditQuery             = monitor.AuditQuery
	AuditListResult        = monitor.AuditListResult
	MonitorPlatformRow     = monitor.PlatformRow
	MonitorEventsQuery     = monitor.EventsQuery
	MonitorTracesQuery     = monitor.TracesQuery
	MonitorListResult      = monitor.ListResult
	MonitorEventWrite      = monitor.EventWrite
	MonitorAlertRule       = monitor.AlertRule
	AlertNotifier          = monitor.AlertNotifier
	MonitorRepo            = monitor.Repo
	MonitorUsecase         = monitor.Usecase
	RunnerMetricsSummary   = monitor.RunnerMetricsSummary
	RunnerCompletionBridge = monitor.RunnerCompletionBridge
)

var (
	NewMonitorUsecase               = monitor.NewUsecase
	MergeRunnerCompletionUsagePatch = monitor.MergeRunnerCompletionUsagePatch
)
