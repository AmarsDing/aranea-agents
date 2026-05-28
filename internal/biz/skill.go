package biz

import "aranea-agents/internal/biz/skill"

type (
	SkillTag                = skill.SkillTag
	SkillVersionSummary     = skill.SkillVersionSummary
	SkillVersionDetail      = skill.SkillVersionDetail
	SkillVersionListQuery   = skill.VersionListQuery
	SkillVersionListResult  = skill.VersionListResult
	SkillPermissions        = skill.SkillPermissions
	Skill                   = skill.Skill
	SkillListQuery          = skill.ListQuery
	SkillListResult         = skill.ListResult
	SkillInvocationPermissions = skill.SkillInvocationPermissions
	SkillInvocation         = skill.SkillInvocation
	SkillRunQuery           = skill.RunQuery
	SkillRunResult          = skill.RunResult
	SkillRepo               = skill.Repo
	SkillUpdateDraft        = skill.UpdateDraft
	SkillInvocationWrite    = skill.InvocationWrite
	SkillCreateInput        = skill.CreateInput
	SkillDiskSyncInput      = skill.DiskSyncInput
	SkillDiskSyncOutcome    = skill.DiskSyncOutcome
	SkillUsecase            = skill.Usecase
	SkillSimilaritySource   = skill.SimilaritySource
	SkillImportJob          = skill.ImportJob
	SkillImportCandidate    = skill.ImportCandidate
	SkillImportIssue        = skill.ImportIssue
	SkillSimilarityMetrics  = skill.SimilarityMetrics
	SkillConflictGroup      = skill.ConflictGroup
	SkillRefineRequest      = skill.RefineRequest
	SkillRefineResult       = skill.RefineResult
	SkillImportDecision     = skill.ImportDecision
	SkillImportApplyRequest = skill.ImportApplyRequest
	SkillImportApplyResult  = skill.ImportApplyResult
	SkillRuntimePolicy      = skill.RuntimePolicy
	SkillRuntimeCandidate   = skill.RuntimeCandidate
	SkillFilesystemHealthStats = skill.FilesystemHealthStats
	SkillEmbedder           = skill.SkillEmbedder
)

const (
	SkillInvocationSourceRuntime         = skill.InvocationSourceRuntime
	SkillInvocationSourceFilesystemScan  = skill.InvocationSourceFilesystemScan
	SkillInvocationSourceFilesystemWatch = skill.InvocationSourceFilesystemWatch
	SkillInvocationSourceFilesystemReconcile = skill.InvocationSourceFilesystemReconcile

	SkillSyncOriginFilesystem = skill.SyncOriginFilesystem
	SkillSyncOriginImport     = skill.SyncOriginImport
	SkillSyncOriginManual     = skill.SyncOriginManual
)

var (
	NewSkillUsecase      = skill.NewUsecase
	ParseSkillRuntimePolicy = skill.ParseRuntimePolicy
)
