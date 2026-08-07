package biz

import "aranea-agents/internal/biz/skill"

type (
	SkillTag                   = skill.SkillTag
	SkillVersionSummary        = skill.SkillVersionSummary
	SkillVersionDetail         = skill.SkillVersionDetail
	SkillVersionListQuery      = skill.VersionListQuery
	SkillVersionListResult     = skill.VersionListResult
	SkillPermissions           = skill.SkillPermissions
	Skill                      = skill.Skill
	SkillListQuery             = skill.ListQuery
	SkillListResult            = skill.ListResult
	SkillInvocationPermissions = skill.SkillInvocationPermissions
	SkillInvocation            = skill.SkillInvocation
	SkillRunQuery              = skill.RunQuery
	SkillRunResult             = skill.RunResult
	SkillRepo                  = skill.Repo
	SkillReader                = skill.SkillReader
	SkillWriter                = skill.SkillWriter
	SkillQueryReader           = skill.SkillQueryReader
	SkillLookupReader          = skill.SkillLookupReader
	SkillRuntimeReader         = skill.SkillRuntimeReader
	SkillMutationWriter        = skill.SkillMutationWriter
	SkillSyncWriter            = skill.SkillSyncWriter
	SkillVersionWriter         = skill.SkillVersionWriter
	SkillImportWriter          = skill.SkillImportWriter
	SkillImportVersionInput    = skill.ImportVersionInput
	SkillCreateVersionInput    = skill.CreateVersionInput
	SkillUpdateDraft           = skill.UpdateDraft
	SkillInvocationWrite       = skill.InvocationWrite
	SkillCreateInput           = skill.CreateInput
	SkillDiskSyncInput         = skill.DiskSyncInput
	SkillDiskSyncOutcome       = skill.DiskSyncOutcome
	SkillUsecase               = skill.Usecase
	SkillSimilaritySource      = skill.SimilaritySource
	SkillImportJob             = skill.ImportJob
	SkillImportCandidate       = skill.ImportCandidate
	SkillImportIssue           = skill.ImportIssue
	SkillSimilarityMetrics     = skill.SimilarityMetrics
	SkillConflictGroup         = skill.ConflictGroup
	SkillRefineRequest         = skill.RefineRequest
	SkillRefineResult          = skill.RefineResult
	SkillImportDecision        = skill.ImportDecision
	SkillImportApplyRequest    = skill.ImportApplyRequest
	SkillImportApplyResult     = skill.ImportApplyResult
	SkillRuntimePolicy         = skill.RuntimePolicy
	SkillRuntimeCandidate      = skill.RuntimeCandidate
	SkillFilesystemHealthStats = skill.FilesystemHealthStats
	SkillEmbedder              = skill.SkillEmbedder
	DedupCacheInvalidator      = skill.DedupCacheInvalidator
	SkillGuidanceEntry         = skill.SkillGuidanceEntry
	SkillEnabledRef            = skill.EnabledRef
	SkillFilesystem            = skill.SkillFilesystem
	SkillFileEntry             = skill.SkillFileEntry
	SkillFileContent           = skill.SkillFileContent
	SkillFilePathResolver      = skill.SkillFilePathResolver
	SkillFileReader            = skill.SkillFileReader
	SkillFileWriter            = skill.SkillFileWriter
	SkillTagInfo               = skill.TagInfo
	SkillTagRepo               = skill.TagRepo
	SkillTagReader             = skill.SkillTagReader
	SkillTagWriter             = skill.SkillTagWriter
)

const (
	SkillInvocationSourceRuntime             = skill.InvocationSourceRuntime
	SkillInvocationSourceFilesystemScan      = skill.InvocationSourceFilesystemScan
	SkillInvocationSourceFilesystemWatch     = skill.InvocationSourceFilesystemWatch
	SkillInvocationSourceFilesystemReconcile = skill.InvocationSourceFilesystemReconcile

	SkillSyncOriginFilesystem = skill.SyncOriginFilesystem
	SkillSyncOriginImport     = skill.SyncOriginImport
	SkillSyncOriginManual     = skill.SyncOriginManual
)

var (
	NewSkillUsecase         = skill.NewUsecase
	ParseSkillRuntimePolicy = skill.ParseRuntimePolicy
	NormalizeSkillSlug      = skill.NormalizeSlug
)
