package biz

import "aranea-agents/internal/biz/monitor"

type FilesystemHealthReader = monitor.FilesystemHealthReader

type (
	AuditLog                    = monitor.AuditLog
	AdminAuditEntry             = monitor.AdminAuditEntry
	AuditQuery                  = monitor.AuditQuery
	AuditListResult             = monitor.AuditListResult
	MonitorPlatformRow          = monitor.PlatformRow
	MonitorEventsQuery          = monitor.EventsQuery
	MonitorTracesQuery          = monitor.TracesQuery
	MonitorListResult           = monitor.ListResult
	MonitorEventWrite           = monitor.EventWrite
	MonitorTraceWrite           = monitor.TraceWrite
	MonitorTraceSpanWrite       = monitor.TraceSpanWrite
	MonitorTraceSpan            = monitor.TraceSpan
	MonitorTraceSpanReader      = monitor.TraceSpanReader
	MonitorAlertRule            = monitor.AlertRule
	MonitorAlertFiringState     = monitor.AlertFiringState
	AlertNotifier               = monitor.AlertNotifier
	MonitorAuditRepo            = monitor.AuditRepo
	MonitorEventRepo            = monitor.EventRepo
	MonitorTraceRepo            = monitor.TraceRepo
	MonitorTraceCompletion      = monitor.TraceCompletion
	MonitorUsageAggregate       = monitor.UsageAggregate
	MonitorTraceUsageRepo       = monitor.TraceUsageRepo
	MonitorAlertRepo            = monitor.AlertRepo
	MonitorRunnerCompletionRepo = monitor.RunnerCompletionRepo
	MonitorUsecase              = monitor.Usecase
	RunnerMetricsSummary        = monitor.RunnerMetricsSummary
	RunnerCompletionBridge      = monitor.RunnerCompletionBridge
	RunnerCompletionLinkParams  = monitor.RunnerCompletionLinkParams
	FlowFileAppender            = monitor.FlowFileAppender
	RunnerCompletionRow         = monitor.RunnerCompletionRow
	AlertMetricRegistry         = monitor.AlertMetricRegistry
	AlertMetric                 = monitor.AlertMetric
	DiagBundleGenerator         = monitor.DiagBundleGenerator
	UsecaseOption               = monitor.UsecaseOption
	SelfHealUsecase             = monitor.SelfHealUsecase
	SelfHealObserver            = monitor.SelfHealObserver
	HealActionHandler           = monitor.HealActionHandler
	HealRecordRepo              = monitor.HealRecordRepo
	HealRecord                  = monitor.HealRecord
	HealRecordQuery             = monitor.HealRecordQuery
	HealRecordListResult        = monitor.HealRecordListResult
	HealStats                   = monitor.HealStats
	FixAction                   = monitor.FixAction
	DiagnoseAndHealResult       = monitor.DiagnoseAndHealResult
	RootCauseConditionResult    = monitor.RootCauseConditionResult
	AutoHealedResult            = monitor.AutoHealedResult
	HealAttemptsResult          = monitor.HealAttemptsResult
	SelfCheckResult             = monitor.SelfCheckResult
)

var (
	NewMonitorUsecase               = monitor.NewUsecase
	DefaultAlertRules               = monitor.DefaultAlertRules
	WithFilesystemHealthReader      = monitor.WithFilesystemHealthReader
	WithTraceSpanReader             = monitor.WithTraceSpanReader
	WithRingBuffer                  = monitor.WithRingBuffer
	WithEvalWorker                  = monitor.WithEvalWorker
	WithRegistry                    = monitor.WithRegistry
	WithLogger                      = monitor.WithLogger
	NewTraceProjector               = monitor.NewTraceProjector
	NewFlowFileAppender             = monitor.NewFlowFileAppender
	NewAlertMetricRegistry          = monitor.NewAlertMetricRegistry
	NewRunnerErrorRateMetric        = monitor.NewRunnerErrorRateMetric
	NewSkillFilesystemMissingMetric = monitor.NewSkillFilesystemMissingMetric
	NewDiagBundleGenerator          = monitor.NewDiagBundleGenerator
	MergeRunnerCompletionUsagePatch = monitor.MergeRunnerCompletionUsagePatch
	NewSelfHealUsecase              = monitor.NewSelfHealUsecase
	NewSelfHealObserver             = monitor.NewSelfHealObserver
)

var (
	AuditAction   = monitor.AuditAction
	AuditSeverity = monitor.AuditSeverity
)

const (
	AuditVerbCreate      = monitor.AuditVerbCreate
	AuditVerbUpdate      = monitor.AuditVerbUpdate
	AuditVerbDelete      = monitor.AuditVerbDelete
	AuditVerbToggle      = monitor.AuditVerbToggle
	AuditVerbArchive     = monitor.AuditVerbArchive
	AuditVerbUnarchive   = monitor.AuditVerbUnarchive
	AuditVerbPin         = monitor.AuditVerbPin
	AuditVerbUnpin       = monitor.AuditVerbUnpin
	AuditVerbSync        = monitor.AuditVerbSync
	AuditVerbCredentials = monitor.AuditVerbCredentials

	AuditSeverityInfo    = monitor.AuditSeverityInfo
	AuditSeverityWarning = monitor.AuditSeverityWarning
)
