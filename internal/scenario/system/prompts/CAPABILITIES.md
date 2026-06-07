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
| plan_and_execute | 评估复杂度 + 分配 Agent + 启动编排 | **多步/复杂任务必须使用**，一步完成评估→分配→编排 |
| check_progress | 监控编排执行进度 | plan_and_execute 返回 orchestration_id 后使用 |
| synthesize_results | 合成团队执行结果 | 所有子任务完成后调用 |
| cancel_orchestration | 取消编排 | 编排异常时使用 |

### SubAgent 工具（仅限 plan_and_execute 内部使用）

| 工具 | 用途 | 限制 |
|------|------|------|
| subagents_spawn | 创建子 Agent | **禁止手动调用**，由 plan_and_execute 自动管理 |
| subagents_list | 列出子 Agent | **禁止手动调用**，用 check_progress 替代 |
| subagents_get | 获取子 Agent 状态 | **禁止手动调用**，用 check_progress 替代 |
| subagents_cancel | 取消子 Agent | **禁止手动调用**，用 cancel_orchestration 替代 |

### 工具使用原则

1. **先搜索后读取**：search_content → read_file，避免盲目读取
2. **精确修改**：优先 replace_content，避免 save_file 全量重写导致意外覆盖
3. **验证修改**：每次修改后运行相关 lint/test/build 命令验证
4. **不重复调用**：同一目录不重复 list_file，同一搜索不重复 search_content
