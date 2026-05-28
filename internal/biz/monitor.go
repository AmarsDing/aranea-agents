package biz

import "aranea-agents/internal/biz/monitor"

type FilesystemHealthReader = monitor.FilesystemHealthReader

type (
	AuditLog                = monitor.AuditLog
	AuditQuery              = monitor.AuditQuery
	AuditListResult         = monitor.AuditListResult
	MonitorPlatformRow      = monitor.PlatformRow
	MonitorEventsQuery      = monitor.EventsQuery
	MonitorTracesQuery      = monitor.TracesQuery
	MonitorListResult       = monitor.ListResult
	MonitorEventWrite       = monitor.EventWrite
	MonitorTraceWrite       = monitor.TraceWrite
	MonitorTraceSpanWrite   = monitor.TraceSpanWrite
	MonitorAlertRule        = monitor.AlertRule
	MonitorAlertFiringState = monitor.AlertFiringState
	AlertNotifier           = monitor.AlertNotifier
	MonitorRepo             = monitor.Repo
	MonitorUsecase          = monitor.Usecase
	RunnerMetricsSummary    = monitor.RunnerMetricsSummary
	RunnerCompletionBridge  = monitor.RunnerCompletionBridge
	FlowFileAppender        = monitor.FlowFileAppender
	RunnerCompletionRow     = monitor.RunnerCompletionRow
	AlertMetricRegistry     = monitor.AlertMetricRegistry
	AlertMetric             = monitor.AlertMetric
	DiagBundleGenerator     = monitor.DiagBundleGenerator
)

var (
	NewMonitorUsecase               = monitor.NewUsecase
	NewTraceProjector               = monitor.NewTraceProjector
	NewFlowFileAppender             = monitor.NewFlowFileAppender
	NewAlertMetricRegistry          = monitor.NewAlertMetricRegistry
	NewRunnerErrorRateMetric        = monitor.NewRunnerErrorRateMetric
	NewSkillFilesystemMissingMetric = monitor.NewSkillFilesystemMissingMetric
	NewDiagBundleGenerator          = monitor.NewDiagBundleGenerator
	MergeRunnerCompletionUsagePatch = monitor.MergeRunnerCompletionUsagePatch
)
