# ModelRegistry Refactor

## Why

当前模型注册表（ModelRegistry）存在架构问题：模型配置分散在多个位置，缺少统一的版本门控机制，种子数据与运行时配置耦合。随着 LLM 厂商和模型数量增长，现有结构难以维护和扩展。需要重构为更清晰的分层架构，并引入种子版本门控（Seed Version Gating）机制，确保种子数据升级时不会覆盖用户自定义配置。

## Goals

- 重构 ModelRegistry 为清晰的分层架构（Provider → Model → Config）
- 引入 Seed Version Gating 机制：种子数据按版本号管理，仅当种子版本高于已存在记录时才更新
- 解耦种子数据与运行时配置，支持用户自定义覆盖
- 保持现有 API 兼容性，渐进式迁移

## Non-goals

- 不改变 LLM Provider 的运行时调用接口
- 不涉及前端模型选择 UI 的重构
- 不引入模型灰度发布或 A/B 测试机制
