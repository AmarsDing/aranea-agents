package monitor

import (
	"aranea-agents/internal/biz/monitor/alert"
)

// DEV-05: the alert metric registry and built-in metrics live in the alert
// subpackage; aliases keep the historical monitor.* API surface intact.

var ErrAlertMetricNoData = alert.ErrAlertMetricNoData

type (
	AlertMetric                 = alert.AlertMetric
	AlertMetricInfo             = alert.AlertMetricInfo
	AlertMetricCatalogProvider  = alert.AlertMetricCatalogProvider
	AlertBreachDetailer         = alert.AlertBreachDetailer
	AlertMetricRegistry         = alert.AlertMetricRegistry
	RunnerErrorRateMetric       = alert.RunnerErrorRateMetric
	SkillFilesystemMissingMetric = alert.SkillFilesystemMissingMetric
	DeadLetterCountReader       = alert.DeadLetterCountReader
	SequencerDeadLetterMetric   = alert.SequencerDeadLetterMetric
	// FilesystemHealthReader supplies live skill filesystem health for alerts.
	FilesystemHealthReader = alert.FilesystemHealthReader
)

var (
	NewAlertMetricRegistry          = alert.NewAlertMetricRegistry
	NewRunnerErrorRateMetric        = alert.NewRunnerErrorRateMetric
	NewSkillFilesystemMissingMetric = alert.NewSkillFilesystemMissingMetric
	NewSequencerDeadLetterMetric    = alert.NewSequencerDeadLetterMetric
)
