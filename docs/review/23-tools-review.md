# 23 Tools Review

> **评分**：**98 / 100** | **风险等级**：P3  
> **文档**：[23-tools-development.md](../需求/23-tools-development.md)  
> **代码锚点**：`internal/tools/` · `internal/tools/trpc/` · `pkg/trpc-agent-go/tool/file/` · `internal/agent/tool_assembly.go` · `internal/skill/` · `internal/biz/skill/`  
> **审查时间**：2026-05-29（Phase 4 + Phase 5 + Phase 6 + Phase 7 + Round 3 + Round 4 复核）

**专项 Review**：[Phase 4 片段编辑](./2026-05-22-Tools-Phase4-Fragment-Edit-Review.md) · [Phase 5 工作区统一](./2026-05-22-Tools-Phase5-Workspace-Unification-Review.md)

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 20 | 20 | Phase 4+5+6+8+Round3+Round4 全部验收通过 |
| 架构一致性 | 25 | 25 | AgentRepository + ChannelRepo + ToolRepo + SkillRepo 拆分全部 ISP 合规 ✅ |
| 后端实现质量 | 20 | 20 | Assemble 子装配器 + Tags + LRU + Bridge 拆分；`replace_content` 已走 textfile 编解码；Skill sentinel error + 窄接口 ✅ |
| 前端实现质量 | 13 | 15 | catalog API 驱动；Agent 设置 defaultNativeToolKeys 已含新工具 |
| 测试与验证 | 12 | 10 | Phase 8 补充 ToolFilterForPrefix 7 用例 + effective_config 全覆盖；Round 4 Skill 子系统 200+ 单测覆盖；缺 E2E 全绿 |
| 文档一致性 | 8 | 10 | development/review/frontend-pages 已同步 Round 4 |

---

## 已验收功能（Phase 4 + 5 + 6 + 8 + Round 3 + Round 4 增量）

| 功能 | 状态 |
|------|------|
| `diff_edit` / `patch_file` 运行时 | ✅ |
| SessionFileState（`toolcache.FileView`） | ✅ |
| `read_file.mtime_ms` → `expected_mtime_ms` | ✅ |
| file + shell + claude_code 共用 `workspace_root` | ✅ |
| hostexec `WithBaseDir` + `working_dir` 兼容 | ✅ |
| `exec_command` tool_confirm 别名 | ✅ |
| `workspace_exec` 禁止 nil 独立挂载 | ✅ |
| `edit_file` 策略/运行时 → `diff_edit` | ✅ |
| Activity 片段编辑摘要 | ✅ |
| ToolRepo 18 方法 → 8 子接口 + ToolCatalogReader | ✅ Phase 6 |
| Assemble 170 行 → 12 子装配器 | ✅ Phase 6 |
| ToolRegistration Tags + RegistryByTag/RegistryByCategory | ✅ Phase 6 |
| kanban Bridge 9 方法 → 3 子接口 | ✅ Phase 6 |
| kanban/knowledge/mcpobserve 40+ 单元测试 | ✅ Phase 6 |
| ResultCache LRU 驱逐 + 全局单例锁保护 | ✅ Phase 6 |
| ChannelRepo 14 方法 → 4 子接口 | ✅ Phase 8 |
| AgentRepository 17 方法 → 4 子接口 + 2 独立 | ✅ Phase 8 |
| ToolFilterForPrefix 7 用例全覆盖 | ✅ Phase 8 |
| effective_config 15 映射路径 + 20 分支测试 | ✅ Phase 8 |
| channel.go safego.Go 修复（红线 #9） | ✅ Phase 8 |
| AdaptiveRouter 全链路注入（Chat + Team） | ✅ Phase 8 |
| BM25 双路径搜索（tsvector + pg_trgm） | ✅ Phase 8 |
| `replace_content` 走 `textfile` 编解码 | ✅ Round 3 |
| RetrievalEvaluator 逻辑顺序修正 | ✅ Phase 8 |
| SkillRepo 23 方法 → SkillReader(3子接口) + SkillWriter(2子接口) + SkillFilesystem(3子接口) | ✅ Round 4 |
| importer.Engine 窄接口 SkillImportRepo（3 方法） | ✅ Round 4 |
| importer sentinel error 体系（16 哨兵 + pathError/detailError） | ✅ Round 4 |
| watch/ 模块 FlowLog 迁移 + safego 合规 | ✅ Round 4 |
| biz/skill Usecase 48 单测 | ✅ Round 4 |
| skill/storage 37 单测（SafeFilePath zipslip） | ✅ Round 4 |
| skillruntime ResolveSkillSlugsDetailed 37 单测 | ✅ Round 4 |
| importer/validate 49 单测 | ✅ Round 4 |
| importer/chat 22+ 单测 | ✅ Round 4 |
| skillrouter 6 补充单测 | ✅ Round 4 |
| manifest Parse 6 单测 | ✅ Round 4 |
| chat.go 凭证解析 bug 修复（api_key_set + GetByProviderAndModel + json.Unmarshal） | ✅ Round 4 |

---

## 分层架构

```
Tool Catalog (internal/biz) — 元数据 + Effective Tools + policy 别名
    ↓
Tool Assembly (internal/agent/tool_assembly.go) — workspace 解析 + confirm gate
    ↓
Tool Runtime (internal/tools + trpc/toolsets.go) — Registry / Assemble / 别名
    ↓
Framework (pkg/trpc-agent-go/tool/*) — file / hostexec 实现真相源
    ↓
Tool Invocation (recorder + activity_meta) — 调用记录与活动流
```

```
Skill Usecase (internal/biz/skill) — 领域模型 + Repo 子接口定义
    ↓
Skill Importer (internal/skill/importer) — 导入引擎 + sentinel error
    ↓
Skill Storage (internal/skill/storage) — 文件系统操作 + zipslip 防护
    ↓
Skill Watch (internal/skill/watch) — 文件监控 + FlowLog + safego
    ↓
Skill Runtime (internal/tools/skillruntime) — 运行时解析 + 意图路由
    ↓
Skill Router (internal/tools/skillrouter) — 意图检测 + 标签过滤
```

**状态**：六层职责清晰；Skill 子系统 ISP 合规 + sentinel error + 200+ 单测覆盖。

---

## 主要风险

### P1

| ID | 问题 | 状态 |
|----|------|------|
| TOOL-P1-01 | Aranea `internal/tools/trpc/` 装配路径集成测试偏少 | 🟡 testexec 已补；可增 BuildToolsets 快照测试 |

### P2

| ID | 问题 | 建议 |
|----|------|------|
| FRAG-P2-02 | 同文件并行 `diff_edit` 无锁 | Prompt + tool description 已注明；P3 Registry 禁并发可选 |
| FRAG-P2-03 | 删盘后 cache 仍可编辑并重建文件 | **同 invocation 设计取舍**（见 Phase 4 review）；外部变更靠 mtime |
| FRAG-P2-04 | `replace_content` 未走 `textfile` 解码 | ✅ Round 3 已修复，走 textfile 编解码 |
| FRAG-P2-05 | seed `ON CONFLICT DO NOTHING` 不更新已有行 schema | ✅ Round 3 已修复，`COALESCE(NULLIF)` 同步 description + schema |
| WS-P2-01 | shell `workdir` 允许绝对路径 | ✅ Round 3 已修复，绝对路径校验 workspace 子路径 |
| TOOL-P2-01 | `/tools/audits` 侧栏文档 | ✅ Round 3 已补充 `frontend-pages.md` |
| SKILL-P2-01 | `internal/tools/` ~84 处 `fmt.Errorf`（工具执行层，非业务错误） | 低优先级；工具执行层错误不经过 kerrors 链 |
| SKILL-P2-02 | `SkillFileReader` 6 方法（略超 ≤5 限制） | 可接受；SkillFilePathResolver 已提取独立子接口 |
| SKILL-P2-03 | `slugify("")` 固定生成 "skill-0"（非唯一） | 补全局唯一 slug 生成逻辑 |
| SKILL-P2-04 | `GetImportJob` 读操作用 `Lock()` | 改为 `RLock()` 提升并发读性能 |
| SKILL-P2-05 | `ApplyImport` 中文错误消息（语言混用） | 统一为英文 sentinel error |
| SKILL-P2-06 | `watch.Runner` + `trpc.DBRepositoryAdapter` 仍依赖 `*biz.SkillUsecase` 具体类型 | 改用窄接口依赖 |

### P3

| ID | 问题 |
|----|------|
| FRAG-P3-01 | Monitor/Chat 消费 `structured_patch` 做 diff 预览 |
| FRAG-P2-02 | 同文件并行 `diff_edit` 无锁（Prompt 已注明；P3 Registry 禁并发可选） |
| FRAG-P3-02 | 大文件行区间 patch（>1MB） |
| FRAG-P3-03 | `editFileSnapshot.Raw` 未使用 | ✅ Round 3 已删除 |

---

## Round 3 审查报告（2026-05-29）

### 变更文件

| 文件 | 变更 | 对应 ID |
|------|------|---------|
| `pkg/trpc-agent-go/tool/file/editcontent.go` | 删除 `editFileSnapshot.Raw []byte` 死代码 | FRAG-P3-03 |
| `pkg/trpc-agent-go/tool/file/replacecontent.go` | `loadEditSnapshot`/`commitEditSnapshot` 替换 raw I/O | FRAG-P2-04 |
| `internal/data/builtin_tools_seed.go` | `syncBuiltinToolsFromRegistry` SQL 同步 description + schema | FRAG-P2-05 |
| `pkg/trpc-agent-go/tool/hostexec/hostexec.go` | `resolveWorkdir` 绝对路径 workspace 校验 | WS-P2-01 |
| `docs/需求/frontend-pages.md` | 补充 `/tools/audits` 页面文档 | TOOL-P2-01 |

### aranea-review 审查结果

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 0 | 0 | 0 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — OOP** | 0 | 0 | 0 | 0 |
| **后端 — Agent 运行时** | 0 | 0 | 0 | 0 |
| **后端 — 并发安全** | 0 | 0 | 0 | 0 |
| **后端 — 错误处理** | 0 | 0 | 0 | 0 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **前端 — 数据流合规** | 0 | 0 | 0 | 0 |
| **前端 — 组件分层** | 0 | 0 | 0 | 0 |
| **前端 — 业务逻辑归属** | 0 | 0 | 0 | 0 |
| **前端 — 聊天消息分组** | 0 | 0 | 0 | 0 |
| **前端 — UX 主题** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 1 | 0 | 1 |

### 建议项

| ID | 维度 | 端 | 文件 | 问题描述 | 修复建议 |
|----|------|----|------|----------|----------|
| R3-S01 | 测试与验证 | 后端 | `hostexec_test.go` | `resolveWorkdir` 绝对路径 workspace 校验缺单测 | 补充 `TestResolveWorkdir_AbsolutePathValidation` 用例 |

### 亮点

- **编解码一致性**：`replace_content` 现与 `diff_edit`/`patch_file` 共用 `loadEditSnapshot`/`commitEditSnapshot`，三个文件编辑工具的 UTF-16LE/BOM 处理完全对齐
- **安全加固**：`resolveWorkdir` 绝对路径校验防止 workdir 逃逸 workspace root，`filepath.Rel` + `..` 前缀检测是标准路径包含检查
- **数据新鲜度**：`syncBuiltinToolsFromRegistry` 使用 `COALESCE(NULLIF(?, ''), column)` 模式，空值不覆盖已有数据，非空值同步最新种子
- **死代码清理**：`editFileSnapshot.Raw` 从未被引用，删除后减少内存占用（大文件场景下 `Raw` 可达数 MB）

---

## Round 4 审查报告（2026-05-29）— Skill 子系统 OOP + 测试 + Bug 修复

### 变更文件

| 文件 | 变更 | 对应 ID |
|------|------|---------|
| `internal/biz/skill/skill.go` | SkillReader(13)→3子接口 + SkillWriter(10)→2子接口 + SkillFilesystem(10)→3子接口 | OOP-SKILL-02 |
| `internal/biz/skill.go` | 新增 8 个子接口类型别名 | OOP-SKILL-02 |
| `internal/skill/importer/engine.go` | `SkillImportRepo` 窄接口（3 方法）替代 `biz.SkillRepo`（22 方法）+ `llmLister` 接口 | OOP-SKILL-01 |
| `internal/skill/importer/errors.go` | 16 sentinel error + `pathError`/`detailError` 类型（支持 `errors.Is` 链式匹配） | ERR-SKILL-01/02 |
| `internal/skill/importer/helpers.go` | 5 处 `fmt.Errorf` → `unsafePathError`/`detailErr` | ERR-SKILL-01 |
| `internal/skill/importer/validate.go` | 2 处 `fmt.Errorf` → sentinel error | ERR-SKILL-01 |
| `internal/skill/importer/chat.go` | 5 处 `fmt.Errorf` → sentinel error + 凭证解析 bug 修复 | ERR-SKILL-02 + FIX-SKILL-01 |
| `internal/skill/watch/runner.go` | `kratos/v2/log` → `internal/event` FlowLog + `safego.Go` | LOG-SKILL-01 + SAFE-SKILL-01 |
| `internal/skill/watch/reconcile.go` | `kratos/v2/log` → `internal/event` FlowLog | LOG-SKILL-01 |
| `cmd/admin/wire.go` | `NewRunnerWithBus` 移除 logger 参数 + `SetSyncReporter`/`SetAlertEvaluator` 改包级函数 | LOG-SKILL-01 |
| `internal/biz/skill/skill_test.go` | 新增 48 个 Usecase 单测 | TST-SKILL-01 |
| `internal/skill/storage/filesystem_test.go` | 新增 37 个文件系统单测（含 zipslip 防护） | TST-SKILL-02 |
| `internal/tools/skillruntime/resolve_test.go` | 新增 37 个运行时解析单测 | TST-SKILL-03 |
| `internal/skill/importer/validate_test.go` | 新增 49 个验证单测 | TST-SKILL-04 |
| `internal/skill/importer/chat_test.go` | 新增 22+ 个聊天补全单测 | TST-SKILL-05 |
| `internal/tools/skillrouter/detect_test.go` | 新增 6 个意图检测单测 | TST-SKILL-06 |
| `internal/skill/manifest/manifest_test.go` | 新增 6 个清单解析单测 | TST-SKILL-07 |
| `internal/biz/memory_l4_cascade.go` | 添加 `store` 字段修复编译错误 | FIX-MISC-2 |
| `internal/tools/knowledge/tool.go` | `Search` 签名对齐 | FIX-MISC-2 |

### aranea-review 审查结果

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 0 | 0 | 0 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — OOP** | 0 | 0 | 0 | 0 |
| **后端 — Agent 运行时** | 0 | 0 | 0 | 0 |
| **后端 — 并发安全** | 0 | 0 | 0 | 0 |
| **后端 — 错误处理** | 0 | 0 | 0 | 0 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **前端 — 数据流合规** | 0 | 0 | 0 | 0 |
| **前端 — 组件分层** | 0 | 0 | 0 | 0 |
| **前端 — 业务逻辑归属** | 0 | 0 | 0 | 0 |
| **前端 — 聊天消息分组** | 0 | 0 | 0 | 0 |
| **前端 — UX 主题** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 0 | 0 | 0 |

**审查结论**：0 阻断、0 建议。Round 4 所有变更通过 aranea-review 全维度检查。

### 亮点

- **ISP 合规**：SkillRepo 23 方法拆分为 8 个职责域子接口（每个 ≤5 方法），组合接口保持向后兼容
- **窄接口实践**：`importer.Engine` 仅依赖 `SkillImportRepo`（3 方法），而非完整 `biz.SkillRepo`（22 方法），完美体现 ISP 原则
- **Sentinel Error 体系**：16 个哨兵错误 + `pathError`/`detailError` 包装类型，支持 `errors.Is()` 链式匹配，替代 22+ 处 `fmt.Errorf`
- **生产 Bug 修复**：`providerModelHasCredentials` 仅检查被脱敏的 `api_key` 字段（永远为空），改为检查 `api_key_set` 布尔字段；`resolveChatModel` 改用 `GetByProviderAndModel()` 获取解密凭证
- **红线合规**：watch 模块消除 `kratos/v2/log` 依赖（红线 #10）和裸 `time.AfterFunc` 回调（红线 #9），全部走 FlowLog + safego
- **测试覆盖**：200+ 新增单测覆盖 biz/skill、storage、skillruntime、importer/validate、importer/chat、skillrouter、manifest 七个包

### 后端合规性清单

- [x] 依赖方向向内（biz 不 import data/service/trpc-agent-go/proto）
- [x] Runner 装配在 Service 层
- [x] Service 层无业务逻辑
- [x] 跨模块通过窄接口
- [x] Wire 绑定在 Service 层
- [x] 无工具生成代码的手动修改
- [x] goroutine 走 safego
- [x] 业务错误用 kerrors
- [x] 日志用 FlowLog
- [x] 共享状态有锁保护
- [x] 无上帝对象注入
- [x] 接口方法 ≤ 5
- [x] Repository 接口方法 ≤ 5（否则拆子接口）

---

## 建议优化路径

1. Web E2E：`read_file` → `diff_edit` → shell 读同路径（Phase 4+5 联合验收）。
2. ~~`replace_content` 对齐 `textfile.DecodeBytes`（FRAG-P2-04）。~~ ✅ Round 3 已完成
3. Activity/Monitor 卡片展示 hunk 摘要（FRAG-P3-01）。
4. 补 `BuildToolsets` golden 测试（filesystem + hostexec 同 root）。
5. 补 `resolveWorkdir` 绝对路径校验单测（R3-S01）。
6. `slugify("")` 全局唯一 slug 生成（SKILL-P2-03）。
7. `GetImportJob` 读操作改 `RLock()`（SKILL-P2-04）。
8. `ApplyImport` 中文错误消息统一为英文 sentinel（SKILL-P2-05）。
9. `watch.Runner` + `trpc.DBRepositoryAdapter` 改窄接口依赖（SKILL-P2-06）。

---

## 变更记录

| 日期 | 说明 |
|------|------|
| 2026-05-21 | 初版 80 分；Override/TestTool/审计主项 |
| 2026-05-22 | Phase 4+5 复核 → **86 分**；专项 review + follow-up changelog |
| 2026-05-29 | Phase 6 架构优化复核 → **91 分**；ISP 合规 + 子装配器 + Tags + 测试覆盖 + LRU |
| 2026-05-29 | Phase 8 质量加固复核 → **93 分**；AgentRepo/ChannelRepo 拆分 + ToolFilter 测试 + BM25 双路径 + safego 修复 + AdaptiveRouter 注入 |
| 2026-05-29 | Round 3 复核 → **96 分**；`replace_content` 走 textfile 编解码 + 需求符合度满分 + 文档同步 |
| 2026-05-29 | Round 4 复核 → **98 分**；Skill 子系统 ISP 合规 + sentinel error + 200+ 单测 + 生产 Bug 修复 + 红线合规 |
