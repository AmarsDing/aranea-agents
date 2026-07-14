package data

const (
	// Pack 种子版本号
	// NOTE: 这些版本号存储在 schema_migrations 表中，绝不能与 DDL 迁移版本号
	// (见 schema_migrations.go 中的常量) 冲突。原值 20260901/20260902 与
	// drop_event_store_subsystem/drop_messages_subsystem DDL 迁移冲突，导致
	// 种子被误认为已应用而跳过。已改为 20261101/20261102 避免冲突。
	SeedPackBuiltinV1 = 20261101
	SeedPackBuiltinV2 = 20261102 // 新增 teams 定义
)
