# Frontend Dependencies Fix

## Overview
修复前端 features→components 反向依赖：features/orchestration/teamGraphAdapter.ts 从 components/teams/teamUtils.ts 导入。

## ADDED Requirements

### Requirement: 将 buildGraphFromDefinition 下移到 features 层
- buildGraphFromDefinition SHALL 从 components 层下移到 features 层，消除反向依赖。
- 将 `buildGraphFromDefinition` 及其私有辅助函数从 `components/teams/teamUtils.ts` 移动到 `features/teams/graphUtils.ts`
- `components/teams/teamUtils.ts` 改为从 features 层 re-export
- `features/orchestration/teamGraphAdapter.ts` 的 import 路径改为从 `features/teams/graphUtils` 导入

#### Scenario: features 层不再反向依赖 components 层
- Given features/orchestration/teamGraphAdapter.ts 需要 buildGraphFromDefinition
- When 它 import 该函数
- Then 从 features/teams/graphUtils 导入（features→features，合规）
- And components/teams/teamUtils.ts 通过 re-export 保持向后兼容
