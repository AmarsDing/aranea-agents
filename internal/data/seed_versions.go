package data

const (
	// Pack 种子版本号
	// NOTE: 这些版本号存储在 schema_migrations 表中，绝不能与 DDL 迁移版本号
	// (见 schema_migrations.go 中的常量) 冲突。原值 20260901/20260902 与
	// drop_event_store_subsystem/drop_messages_subsystem DDL 迁移冲突，导致
	// 种子被误认为已应用而跳过。已改为 20261101/20261102 避免冲突。
	SeedPackBuiltinV1 = 20261101
	SeedPackBuiltinV2 = 20261102 // 新增 teams 定义

	// CleanupNonSystemV1: 清除非系统 agent/team/organization 数据的一次性迁移。
	// 导入 agency-pack 前执行，保留 4 个系统 agent（spirit/memory/skills/system_admin）。
	SeedCleanupNonSystemV1 = 20261103

	// SeedPackAgencyV1: 导入 agency-pack（The Agency 230+ AI agent 模板库）。
	// 3 公司 / 26 部门 / 239 岗位 agent，位于 CleanupNonSystemV1 之后执行。
	// 版本从 20261104 bump 到 20261105：taxonomy.yaml 和 prompt 文件已翻译为中文，需重新导入。
	SeedPackAgencyV1 = 20261105
)
