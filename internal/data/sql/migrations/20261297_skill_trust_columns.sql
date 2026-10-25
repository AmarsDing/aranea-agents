-- Version 20261297: skill trust_level / source 两列（P0-7 批 D1，20-skill §8.4②③⑤）
-- 信任分级落账：trust_level T1-T4（builtin/audited/standard/untrusted 四档，
-- biz.SkillTrustLevel 字面值）、source builtin/llm_evolution/zip_import/
-- user_created（biz.SkillSource 字面值）。两列均可空且零默认值——NULL=未回填：
-- 存量行天然 NULL，由 ddlSkillTrustBackfill（本迁移 Func 段）三档 value-guarded
-- UPDATE 幂等回填（source 凭 metadata_json.sync_origin 溯源；trust T2=已绑定
-- 岗位走 agent_runtime_settings.skill_runtime_json.allowed_slugs 与 skill_key
-- SQL 转写 NormalizeSlug 比对、T3=存量兼容默认档 §8.5①、T1 本轮恒空集）。
-- 列同时由 ent schema（platform_skill.go）声明——ent AutoMigrate 与本迁移均
-- IF NOT EXISTS，谁先到谁创建，重跑/双写均安全。三处写入路径（importer 注册
-- CreateSkillWithVersion / 磁盘同步 UpsertSkillFromDisk 建行 / UI 复制
-- DuplicateSkill 携带）在 Go 侧经 biz.ResolveSkillTrustForPersist 落值
-- （manifest 声明只能更严、绝不覆盖 provenance，§8.4③⑤ fail-closed）。
-- 编号顺延当时 migrations 最大值（20261296 tool_result_spills）+1，无撞号。
-- 幂等，重跑安全。
ALTER TABLE skill ADD COLUMN IF NOT EXISTS trust_level TEXT;
ALTER TABLE skill ADD COLUMN IF NOT EXISTS source TEXT;
