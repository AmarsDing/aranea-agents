# 13 知识库与技能

## 功能

- **知识库**：团队文档的统一管理、向量检索与对话引用；
- **技能（Skill）**：可复用的专业能力包，CRUD + 版本管理 + 渐进式加载 + 文件系统双向同步。

## 13.1 知识库

### 原理

- 文档按库组织（个人库 / 团队库），支持 Markdown 笔记与文件上传；
- 文档分块 → Embedding → 向量索引（pgvector），对话时按需检索注入（聊天消息中的「已引用 N 条知识」）；
- 支持库级共享与团队收件箱。

### 界面配置

左侧导航 **知识库**：

![知识库](../assets/screenshots/aranea-knowledge.png)

- 左侧库树：浏览/新建库，库内文档列表与最近更新；
- 中间编辑区：Markdown 笔记编辑；顶部支持全库搜索、快速切换（Ctrl+O）、命令面板；
- 右侧关联面板：打开笔记后展示其关联（反链/出链）。

## 13.2 技能（Skill）

### 原理

Skill 是带 `SKILL.md` 的能力包：名称 + 描述 + 正文（指令/流程/约束）+ 可选 docs 参考资料 + 标签 + 继承（extends）。

**四种加载模式**：

| 模式 | 说明 |
|------|------|
| `once` | 下一次请求注入后卸载 |
| `turn` | 当前 invocation 内有效（默认） |
| `session` | 跨 invocation 直到会话过期 |
| `progressive` | 3 阶段渐进加载（推荐） |

**Progressive 三阶段**：

| 阶段 | 触发 | 注入内容 | Token 消耗 |
|------|------|----------|-----------|
| L0 清单 | 自动（每轮） | 所有 Skill 的 name + description 摘要 | 极低 |
| L1 Body | LLM 调用 `skill_load` | 指定 SKILL.md 正文 + 可选 docs | 按需 |
| L2 Refs | LLM 调用 `skill_select_docs` | 辅助文档和参考资料 | 按需 |

关键机制：Turn 级状态清理（每轮重新按需加载）；加载内容注入 tool result 而非系统提示（利于 prompt caching）；同时加载数量上限，超出按最近使用淘汰；意图路由（嵌入向量评分权重 0.3）自动匹配最相关 Skill。

**文件系统双向同步**：fsnotify 实时监听 + 定时对账——磁盘改 Skill 自动同步 DB，反之亦然；已发布 Skill 磁盘内容变更自动回退 draft + 禁用。

**版本管理**：major.minor.patch；版本回滚基于历史版本创建新版本（patch +1）。

### 设计要点

- 技能的**新生/成长/消亡/重生**完整生命周期见 [07 自动进化系统](07-evolution.md)；
- 技能管家工具：`evolve_skill / optimize_skill / recommend_skills / analyze_skill_usage / analyze_skill_health / analyze_tool_weights / analyze_orchestration / optimize_orchestration`。

### 界面配置

- **技能页**：Skill CRUD、启用/禁用、版本历史与回滚、健康度标记；
- **技能标签页**：标签体系管理；
- **进化建议 / 经验报告页**：见 [07 自动进化系统](07-evolution.md#界面配置)。

## 深入阅读

- [37 知识库设计](../../docs/development/37-knowledge.design.md)
- [65 模块交叉引用 · skill / knowledge 章节](../../docs/development/65-module-cross-reference-full.md)
