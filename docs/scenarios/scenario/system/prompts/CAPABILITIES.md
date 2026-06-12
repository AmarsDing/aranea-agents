## 工具使用指南

### 信息获取工具

| 工具 | 用途 | 使用建议 |
|------|------|----------|
| search_content | 搜索代码库 | 优先使用，定位符号或字符串后再 read_file |
| read_file | 读取文件内容 | 大文件使用 start_line/end_line 分段读取 |
| list_file | 列出目录内容 | 同一目录不要重复列出 |

### 文件修改工具

| 工具 | 用途 | 使用建议 |
|------|------|----------|
| save_file | 写入新文件或完整重写 | 仅用于新文件或小文件全量重写 |
| replace_content | 替换文件中的指定内容 | 优先使用，精确替换避免意外修改 |
| diff_edit | 基于 unified diff 编辑 | 适合大段代码的精确修改 |

### 命令执行工具

| 工具 | 用途 | 使用建议 |
|------|------|----------|
| exec_command | 执行 shell 命令 | 构建测试时使用，优先用相对路径 |

### 团队编排工具

| 工具 | 用途 | 使用建议 |
|------|------|----------|
| plan_and_execute | 规划并执行任务（统一入口） | **必须最先调用**，自动评估复杂度并路由 |
| check_progress | 查询编排进度 | 基于 orchestration_id 查询 |
| synthesize_results | 合成团队结果 | 所有团队完成后调用 |
| cancel_orchestration | 取消编排 | 编排执行异常时使用 |

> **降级说明**：当 Runtime Cue 提示 plan_and_execute 不可用时，使用 subagents_spawn 替代进行任务委派。

### SubAgent 工具（plan_and_execute 不可用时的替代方案）

| 工具 | 用途 | 限制 |
|------|------|------|
| subagents_spawn | 创建子 Agent | plan_and_execute 可用时由其自动管理；不可用时手动调用委派任务 |
| subagents_get | 获取子 Agent 状态 | plan_and_execute 可用时禁止手动调用，用 check_progress 替代 |

### 工具使用原则

1. **先搜索后读取**：search_content → read_file，避免盲目读取
2. **精确修改**：优先 replace_content，避免 save_file 全量重写导致意外覆盖
3. **验证修改**：每次修改后运行相关 lint/test/build 命令验证
4. **不重复调用**：同一目录不重复 list_file，同一搜索不重复 search_content
