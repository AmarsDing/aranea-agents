# L0 上下文压缩优化 — 需求

> **设计**：[`L0-compression.design.md`](./L0-compression.design.md) · **开发计划**：[`L0-compression-development.md`](./L0-compression-development.md) · **L0 基线**：[`L0.md`](./L0.md)
> **调研来源**：Cursor IDE / Trae IDE / Claude Code 上下文管理机制对比 + 2024-2026 上下文压缩论文（Focus / Memento / A-Mem / Mem0 / ROMEM / LLMLingua-3）
>
> **⚠️ 变更通知（2026-07-20）**：L1 MicroCompact 已全链路移除（cascade/policy/biz/proto/DB/前端）。移除原因：`loadCompressBody` 仅保留 user/assistant 消息，工具消息过滤逻辑恒不触发，功能从未生效。下文 F1.7 / N1.1 / N1.5 / F3.6 及 `micro_compact_*` 配置仅作历史记录保留，压缩级联现为两级：L2 Memory Compact → L3 LLM。详见 `docs/superpowers/plans/2026-07-20-session-compression-hardening.md`。

---

## 0. 指导思想

L0 压缩的核心矛盾：**上下文窗口是硬约束，但信息消耗是线性增长的**。当前项目已有 L0 压缩基础（摘要 + CAS 事务），但与业界最佳实践（Claude Code 三层代价递进）和前沿论文（Focus 自主压缩、A-Mem 记忆演化）相比，存在三个结构性缺陷：

1. **入口无管控**：工具结果全量进入上下文，是 token 膨胀的主要来源
2. **压缩单层化**：只有 LLM 摘要一种方式，无代价递进
3. **记忆无演化**：压缩后的记忆只创建和衰减，不会合并、修正、关联更新

本需求文档分三个阶段，每阶段独立可交付，渐进升级。

---

## 1. 阶段一：工程补强（P0）

> 对标 Claude Code 最佳实践，补齐入口管控和代价递进压缩。

### 1.1 心智模型

```
工具结果产生 → [入口管控] → 进入上下文 → [三层存量清理] → 压缩后上下文
                 ↑                           ↑
            阻止大内容进入              代价递进：零API → 零额外API → 1次API
```

### 1.2 功能需求

#### 1.2.1 入口管控：工具结果持久化

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| F1.1 | 工具结果大小检查 | 必须 | 单个结果 > 50K 字符时触发持久化 |
| F1.2 | 消息级预算检查 | 必须 | 一轮并行调用合计 > 200K 字符时，按大小排序持久化最大的 |
| F1.3 | 持久化到 SQLite | 必须 | 超限部分写入 `tool_result_blobs` 表，上下文仅保留 2KB 预览 + result_id |
| F1.4 | 替换决策冻结 | 必须 | 替换决策一旦做出，后续轮次用相同预览字符串重新注入，保证前缀一致性 |
| F1.5 | ReadToolResult 工具 | 必须 | Agent 可通过工具按需读取持久化的完整内容 |
| F1.6 | 可压缩工具白名单 | 必须 | 定义哪些工具的输出属于"一次性消费"型（文件读取、搜索、命令执行等） |

#### 1.2.2 三层代价递进压缩

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| F1.7 | ~~L1 MicroCompact~~ ❌ 已移除（2026-07-20） | — | ~~每次请求前静默清理旧工具结果（零 API 调用），替换为占位符~~ 功能从未生效（body 无 tool 消息），已全链路移除 |
| F1.8 | L2 Memory Compact | 必须 | 复用 L1 工作记忆 + 自动记忆提取结果作为摘要源（零额外 API 调用） |
| F1.9 | L3 AutoCompact（升级） | 必须 | 现有 LLM 摘要升级为 9 章节结构 + 用户消息原文保留 |
| F1.10 | 压缩层级自动升级 | 必须 | L1 估算后仍超阈值 → L2；L2 仍超 → L3 |

#### 1.2.3 摘要结构升级

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| F1.11 | 9 章节结构化摘要 | 必须 | 替代现有 6 章节 schema，增加错误修复、用户消息原文、当前工作状态 |
| F1.12 | 用户消息原文逐条保留 | 必须 | 摘要中第 6 章逐条列出用户每条消息原文，不可省略 |
| F1.13 | 当前工作状态记录 | 必须 | 摘要中第 9 章记录最后操作的文件、未完成的编辑、下一步计划 |

#### 1.2.4 LLM 响应缓存

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| F1.18 | LLM 压缩响应缓存 | 必须 | 对同一会话相同消息序列的 LLM 摘要调用缓存结果，避免 CAS 冲突重试、并发触发、L1/L2 升级到 L3 时重复调用 LLM |
| F1.19 | 缓存键确定性 | 必须 | 缓存键基于 `(sessionID, messageHash, promptVersion, compressVersion)` 生成 sha256 指纹，保证相同输入命中同一缓存 |
| F1.20 | 缓存 TTL 绑定压缩版本 | 必须 | 缓存 TTL 与 `compressVersion` 绑定，版本推进时旧缓存自动失效 |
| F1.21 | 缓存命中可观测 | 必须 | 缓存命中/未命中记录日志，压缩完成通知中标注是否命中缓存 |

#### 1.2.5 手动压缩与可观测性

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| F1.14 | CompactSession API | 必须 | 用户可主动触发会话压缩 |
| F1.15 | 自定义保留指令 | 推荐 | 压缩时可指定"重点保留"规则（如"保留所有数据库 schema 相关讨论"） |
| F1.16 | 压缩进度通知 | 必须 | 前端 toast 提示压缩进行中 / 已完成 |
| F1.17 | 压缩详情可查 | 推荐 | 压缩后可查看压缩了哪些轮次、保留了什么 |

### 1.3 非功能需求

| # | 需求 | 目标值 |
|---|------|--------|
| N1.1 | ~~L1 MicroCompact 延迟 P99~~ ❌ 已移除 | ~~< 5 ms（纯规则，无 API 调用）~~ |
| N1.2 | L2 Memory Compact 延迟 P99 | < 50 ms（读记忆 + 拼装，无额外 API 调用） |
| N1.3 | 工具结果持久化写入延迟 P99 | < 20 ms |
| N1.4 | 入口管控减少工具结果 token | ≥ 30%（对比无管控基线） |
| N1.5 | ~~L1 MicroCompact 减少旧工具结果 token~~ ❌ 已移除 | ~~≥ 20%（对比无 MicroCompact 基线）~~ |
| N1.6 | 摘要后信息丢失率 | 用户消息零丢失；关键决策丢失率 < 5% |
| N1.7 | 替换决策冻结保证 | 同一工具结果在所有后续轮次中预览字符串字节级一致 |
| N1.8 | LLM 响应缓存命中率 | ≥ 30%（同一会话短时间内的重复压缩请求） |
| N1.9 | LLM 响应缓存命中延迟 P99 | < 1 ms（内存读取，无 LLM 调用） |
| N1.10 | LLM 响应缓存容量上限 | 默认 256 条，LRU 淘汰 |

### 1.4 配置需求

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `tool_result_max_size_chars` | 50000 | 单个工具结果字符上限 |
| `tool_result_max_per_message_chars` | 200000 | 单条消息内工具结果合计上限 |
| `tool_result_preview_size_chars` | 2000 | 持久化后保留的预览字符数 |
| ~~`micro_compact_enabled`~~ ❌ 已移除 | ~~true~~ | ~~L1 MicroCompact 开关~~ |
| ~~`micro_compact_min_age_turns`~~ ❌ 已移除 | ~~2~~ | ~~工具结果至少经历 N 轮后才可被 MicroCompact 清理~~ |
| `memory_compact_enabled` | true | L2 Memory Compact 开关 |
| `memory_compact_min_tokens` | 10000 | L2 压缩后至少保留的近期消息 token 数 |
| `memory_compact_max_tokens` | 40000 | L2 保留消息的硬上限 |
| `compress_llm_cache_enabled` | true | LLM 压缩响应缓存开关 |
| `compress_llm_cache_max_entries` | 256 | LLM 压缩响应缓存最大条目数 |
| `compress_llm_cache_ttl_sec` | 600 | LLM 压缩响应缓存 TTL（秒） |

### 1.5 验收标准

- 工具结果超过 50K 字符时自动持久化，上下文仅保留预览 + result_id
- Agent 可通过 ReadToolResult 工具读取持久化内容
- ~~L1 MicroCompact 在每次请求前自动清理旧工具结果，零 API 调用~~ ❌ 已移除（2026-07-20）
- L2 Memory Compact 复用已有记忆，零额外 API 调用
- L3 AutoCompact 摘要包含 9 章节，用户消息原文逐条保留
- 手动压缩 API 可正常触发，前端有压缩进度提示
- 替换决策冻结：同一工具结果在所有后续轮次中预览字符串完全一致
- LLM 压缩响应缓存命中时零 LLM 调用，未命中时正常调用并写入缓存
- 缓存键确定性：相同输入（sessionID + 消息序列 + promptVersion）命中同一缓存
- 压缩版本推进时旧缓存自动失效

### 1.6 非目标

- 不改变现有 CAS + 事务原子性保证（红线第 14 条）
- 不改变现有防抖/去重/inFlight 机制
- 不改变现有 L0CompressProvider/L0CompressModel 专用压缩模型配置
- 不引入 Prompt Cache 感知（本项目使用多模型，Cache 机制不统一，后续单独规划）
- 不引入分布式缓存（LLM 压缩响应缓存为进程内 LRU，不依赖 Redis 等外部存储）

---

## 2. 阶段二：记忆演化（P2）

> 对标 A-Mem / Mem0 / ROMEM，让记忆从"创建-衰减"升级为"形成-巩固-演化-检索/再巩固"完整生命周期。

### 2.1 心智模型

```
当前：记忆创建 → 衰减 → 删除
目标：记忆形成 → 巩固 → 演化(合并/修正/关联) → 检索/再巩固
         ↑                                    ↓
         └──────── 反馈闭环 ──────────────────┘
```

### 2.2 功能需求

#### 2.2.1 记忆操作语义化

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| F2.1 | 双阶段提取-更新 | 必须 | LLM 先判断操作类型（ADD/UPDATE/DELETE/MERGE/NOOP），再执行结构化写入 |
| F2.2 | UPDATE 操作 | 必须 | 更新已有事实（如"用户偏好从中文改为英文"），而非新增+旧条衰减 |
| F2.3 | MERGE 操作 | 必须 | 多条相关事实合并为一条更精炼的事实 |
| F2.4 | DELETE 操作 | 必须 | 事实已过时/矛盾时显式删除，而非仅衰减 |
| F2.5 | 操作审计日志 | 必须 | 每次记忆变更记录操作类型、变更前值、变更后值、触发来源 |

#### 2.2.2 时间维度

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| F2.6 | 事实作用域分类 | 必须 | 区分 static（不衰减）/ dynamic（按速率衰减）/ episodic（时间窗口后归档） |
| F2.7 | 动态事实衰减 | 必须 | dynamic 事实按 DecayRate 衰减，超期自动降权 |
| F2.8 | 静态事实保护 | 必须 | static 事实不衰减，只在显式 UPDATE/DELETE 时变更 |
| F2.9 | 有效期标注 | 必须 | 每条事实记录 ValidFrom / ValidUntil，支持过期自动归档 |

#### 2.2.3 动态链接

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| F2.10 | 新记忆写入时自动关联 | 必须 | 用 embedding 检索最相关的已有记忆，LLM 判断关系类型 |
| F2.11 | 关系类型 | 必须 | contradicts（矛盾→触发 UPDATE/DELETE）、elaborates（细化→LINK）、depends_on（依赖→LINK） |
| F2.12 | 双向链接 | 必须 | 关联建立后，两端记忆均可通过链接找到对方 |
| F2.13 | 矛盾自动检测 | 推荐 | contradicts 关系自动触发 UPDATE/DELETE 提示，需用户确认 |

### 2.3 非功能需求

| # | 需求 | 目标值 |
|---|------|--------|
| N2.1 | 记忆操作延迟 P99 | < 200 ms（不含 LLM 调用） |
| N2.2 | 动态链接检索延迟 P99 | < 100 ms |
| N2.3 | 记忆去重巩固精度 | ≥ 97%（Human-Inspired Memory 论文基准） |
| N2.4 | 存储缩减 | ≥ 30%（去重巩固后） |

### 2.4 验收标准

- 记忆提取产生 ADD/UPDATE/DELETE/MERGE/NOOP 五种操作，不再只有 ADD
- UPDATE 操作正确更新已有事实而非新增+旧条衰减
- static/dynamic/episodic 三种作用域事实有不同的衰减策略
- 新记忆写入时自动建立与已有记忆的链接
- contradicts 关系可触发 UPDATE/DELETE 提示

### 2.5 非目标

- 不改变 L3 向量存储的底层实现
- 不引入连续相位旋转（ROMEM 的复向量空间，属于远期研究）
- 不改变前端 Memory Center 的基础布局，仅扩展操作类型展示

---

## 3. 阶段三：Agent 自主压缩（P3）

> 对标 Focus 论文 / Memento / Aider，让 Agent 从"被动接受压缩"进化为"主动管理上下文"。

### 3.1 心智模型

```
当前：系统策略驱动 → 阈值触发 → 外部压缩
目标：Agent 自主决策 → 主动压缩 → 按需恢复
      ↑                               ↓
      └──── 压缩工具 + 恢复工具 ───────┘
```

### 3.2 功能需求

#### 3.2.1 Agent 自主压缩工具

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| F3.1 | CompactContext 工具 | 必须 | Agent 在完成子任务后主动调用，将当前轮次之前的对话压缩为摘要 |
| F3.2 | preserve_keys 参数 | 必须 | Agent 可指定必须保留的关键信息类别 |
| F3.3 | RecallDetail 工具 | 必须 | Agent 需要回忆被压缩的细节时调用，从持久化记录中检索 |
| F3.4 | 压缩后完整记录保留 | 必须 | 压缩后的完整对话记录不删除，仅从 Runner 快照中移除 |

#### 3.2.2 代码结构感知压缩

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| F3.5 | 代码骨架提取 | 推荐 | 基于 tree-sitter 提取函数签名 + 类定义 + 类型声明 + import |
| F3.6 | ~~MicroCompact 代码感知~~ ❌ 已移除 | — | ~~L1 MicroCompact 对代码类工具结果自动替换为骨架~~ 随 L1 一并移除 |
| F3.7 | 骨架→全文按需恢复 | 推荐 | Agent 可通过 ReadFile 工具读取完整实现 |

### 3.3 非功能需求

| # | 需求 | 目标值 |
|---|------|--------|
| N3.1 | Agent 自主压缩 token 节省 | ≥ 20%（Focus 论文基准：22.7%） |
| N3.2 | 自主压缩后准确率 | 不低于无压缩基线（Focus 论文：60% = 60%） |
| N3.3 | 代码骨架压缩比 | ≥ 80%（5000 tokens → 1000 tokens） |
| N3.4 | RecallDetail 检索延迟 P99 | < 500 ms |

### 3.4 验收标准

- Agent 可在对话中主动调用 CompactContext 工具压缩上下文
- Agent 可通过 RecallDetail 工具按需恢复被压缩的细节
- 自主压缩后 Agent 仍能正确继续当前任务
- 代码骨架提取保留函数签名/类定义/接口定义，丢弃函数体/注释

### 3.5 非目标

- 不微调模型使其具备自主压缩能力（Memento 路线，属于远期研究）
- 不引入 KV Cache 层面的压缩（需要模型厂商支持）
- 不改变现有工具注册机制（新工具走标准 ToolRegistration 流程）

---

## 4. 与现有系统的关系

### 4.1 保留的现有能力

| 能力 | 说明 |
|------|------|
| CAS + 事务原子性 | 红线第 14 条，不改变 |
| 异步为主、同步为辅 | AfterNativeTurn 异步 + BeforeDurableTurn 同步 |
| 专用压缩模型 | L0CompressProvider/L0CompressModel |
| 防抖与去重 | inFlight / compressDebounceActive / SessionSummaryExists |
| 四种截断策略 | summary/drop_oldest/hybrid/drop_tool_results |
| 记忆联动 | 压缩后清除旧记忆实体 |

### 4.2 变更影响面

| 变更 | 影响模块 | 影响程度 |
|------|---------|---------|
| 工具结果持久化 | `internal/tools`（结果返回路径）、`internal/data`（新表）、`internal/biz`（新端口） | 中 |
| LLM 压缩响应缓存 | `internal/compress`（缓存装饰器）、`internal/session/compressor.go`（缓存键构建 + 命中判断） | 低 |
| ~~MicroCompact~~ ❌ 已移除 | ~~`internal/session/compressor.go`（判断链前置）~~ | — |
| Memory Compact | `internal/session/compressor.go`（L2 分支）、`internal/memory`（读取记忆） | 中 |
| 摘要结构升级 | `internal/compress/prompt.go`（系统提示词） | 低 |
| 手动压缩 API | `api/`（proto）、`internal/service`、`internal/server` | 中 |
| 记忆操作语义化 | `internal/compress/memory_extract.go`、`internal/memory` | 高 |
| 时间维度 | `internal/biz`（MemoryFact 模型）、`internal/data`（Ent schema） | 高 |
| 动态链接 | `internal/memory`（链接存储 + 检索） | 高 |
| Agent 自主压缩 | `internal/tools`（新工具）、`internal/agent`（工具注入） | 中 |
| 代码骨架提取 | `internal/tools`（新工具，依赖 tree-sitter） | 高 |

---

## 5. 论文与业界参考索引

| # | 来源 | 类型 | 核心借鉴点 |
|---|------|------|-----------|
| 1 | Claude Code 上下文管理 | 业界实践 | 三层代价递进压缩、工具结果持久化、9 章节摘要、替换决策冻结 |
| 2 | Cursor IDE 上下文管理 | 业界实践 | 文件精简三级策略、`/summarize` 手动压缩 |
| 3 | Trae IDE SKILL 系统 | 业界实践 | 前置压缩——结构化知识替代重复消耗 token |
| 4 | Focus (arXiv:2601.07190, 2026) | 论文 | Agent 自主压缩（start_focus/complete_focus），22.7% token 缩减 |
| 5 | Memento (Microsoft, 2026.04) | 论文 | 模型内分段压缩（memento），KV Cache 峰值降 2-3x |
| 6 | A-Mem (NeurIPS 2025) | 论文 | Zettelkasten 动态链接记忆，85-93% token 缩减 |
| 7 | Mem0 (arXiv:2504.19413, 2025) | 论文 | 双阶段提取-更新（ADD/UPDATE/DELETE/NOOP） |
| 8 | ROMEM (arXiv:2604.11544, 2026) | 论文 | 连续相位旋转实现时间感知，时序推理 2-3x MRR 提升 |
| 9 | Human-Inspired Memory (Microsoft, arXiv:2605.08538, 2026) | 论文 | 六大认知机制：睡眠期巩固、干扰性遗忘、印迹成熟 |
| 10 | LLMLingua-3 (2025) | 论文 | 生成式压缩，2-8x 压缩比 |
| 11 | Aider repo-map | 开源实践 | tree-sitter 代码骨架提取，大幅减少代码上下文 token |
