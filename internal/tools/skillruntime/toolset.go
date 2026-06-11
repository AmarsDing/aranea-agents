package skillruntime

// BuildTRPCSkillTools was removed — it always used FSRepositoryAdapter, making
// DB-only skills invisible at runtime. The correct skill tool assembly now
// lives in internal/agent/trpc_build.go (buildSkillDeps), which selects
// DBRepositoryAdapter when available and falls back to FSRepositoryAdapter.
// Slug resolution (ResolveSkillSlugs / ResolveSkillSlugsDetailed) and
// visibility filtering (AgentVisibilityFilter) remain in this package.
