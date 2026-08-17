# 01 Agent 管理 测试结果

**结论：10 PASS / 1 FAIL（AGT-03 暴露 P1 BUG）**

| ID | 用例 | 结果 | 说明 |
|----|------|------|------|
| AGT-01 | 列表分页 | PASS | total=308 |
| AGT-02 | 详情 | PASS | deepseek/deepseek-v4-flash |
| AGT-03 | 无 position 创建 | **FAIL** | 400 AGENT_KEY_CONFLICT |
| AGT-03B | 带 position 创建 | PASS | workaround 有效 |
| AGT-04 | 更新 | PASS | |
| AGT-05 | 收藏 | PASS | |
| AGT-06 | prompt 预览 | PASS | 16564 字符完整装配 |
| AGT-07 | effective tools | PASS | 18701 字符清单 |
| AGT-08 | creators | PASS | |
| AGT-09 | 删除 | PASS | |
| AGT-10 | 删后查询 | PASS | 404 |
| AGT-11 | limit/offset | PASS | items==limit |

## BUG-01（P1）：无 position 的 Agent 创建必然失败（存量 ('','') 行存在时）

**现象**：`POST /v1/agents` 不传 `position_key`/`agent_variant` → 400 `AGENT_KEY_CONFLICT_BAD_REQUEST`，报唯一索引 `agent_position_key_agent_variant` 冲突。

**原因分析**：唯一索引 `CREATE UNIQUE INDEX agent_position_key_agent_variant ON agents(position_key, agent_variant)` 无表达式、无部分条件。无岗位 Agent 落库默认值 `('','')`，全库只允许存在一行。当前库中 `p23_online_probe_203330` 已占 `('','')`，此后任何无岗位创建都冲突。这正是 2026-08-15 已记录的「PG 唯一约束改动须同步查全部 ON CONFLICT 推断点」同类陷阱的另一种表现——把业务上可重复的 ('','') 默认值纳入了唯一键。

**解决方案（三选一，推荐 A）**：
- A. 索引改部分唯一：`CREATE UNIQUE INDEX ... ON agents(position_key, agent_variant) WHERE position_key <> ''`（空岗位不参与唯一约束，迁移替换旧索引）。
- B. 无岗位 Agent 落库时 position_key 写 NULL（NULL 不参与唯一冲突），同时核对所有 ON CONFLICT 推断点。
- C. 创建接口在缺省 position 时自动生成占位 position_key（如 `free_<agent_key>`）。

**临时绕过**：创建时显式传 `position_key` + `agent_variant`（本测试 AGT-03B 已验证可行）。

## 经验备注
- 资源定位：URL `{id}` 是 hash 资源 id（如 `71096314087d86e2caa20488`），不是 agent_key；需先列表按 agentKey 解析。
- 分页参数是 `limit/offset`；`page/page_size` 被静默忽略（按默认 limit 返回）。
