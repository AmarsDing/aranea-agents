# 23 Tools Review

> **评分**：**86 / 100** | **风险等级**：P2  
> **文档**：[23-tools-development.md](../需求/23-tools-development.md)  
> **代码锚点**：`internal/tools/` · `internal/tools/trpc/` · `pkg/trpc-agent-go/tool/file/` · `internal/agent/tool_assembly.go`  
> **审查时间**：2026-05-22（Phase 4 + Phase 5 复核）

**专项 Review**：[Phase 4 片段编辑](./2026-05-22-Tools-Phase4-Fragment-Edit-Review.md) · [Phase 5 工作区统一](./2026-05-22-Tools-Phase5-Workspace-Unification-Review.md)

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 18 | 20 | Phase 4 `diff_edit`/`patch_file` + SessionFileState ✅；Phase 5 workspace 统一 ✅ |
| 架构一致性 | 23 | 25 | 算法在 `pkg/trpc-agent-go`；桥接在 `internal/tools`+`agent`；biz 无 trpc import ✅ |
| 后端实现质量 | 18 | 20 | 原子编辑、mtime 锁、hostexec 绑定、confirm 别名；`replace_content` 仍 raw string 读盘 |
| 前端实现质量 | 13 | 15 | catalog API 驱动；Agent 设置 defaultNativeToolKeys 已含新工具 |
| 测试与验证 | 7 | 10 | patch/diff/cache/confirm/testexec 有单测；缺 E2E 与 Windows file 套件全绿 |
| 文档一致性 | 7 | 10 | development/changelog/review 已同步；P3 UI diff 预览仍 backlog |

---

## 已验收功能（Phase 4 + 5 增量）

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

**状态**：四层职责清晰；Phase 4 算法正确下沉至框架包。

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
| FRAG-P2-04 | `replace_content` 未走 `textfile` 解码 | 独立小改，与 Phase 4 无关 |
| FRAG-P2-05 | seed `ON CONFLICT DO NOTHING` 不更新已有行 schema | 运维 sync 或 migration |
| WS-P2-01 | shell `workdir` 允许绝对路径 | Prompt 引导；可选 workspace 子路径校验 |
| TOOL-P2-01 | `/tools/audits` 侧栏文档 | 更新 `frontend-pages.md` |

### P3

| ID | 问题 |
|----|------|
| FRAG-P3-01 | Monitor/Chat 消费 `structured_patch` 做 diff 预览 |
| FRAG-P3-02 | 大文件行区间 patch（>1MB） |
| FRAG-P3-03 | `editFileSnapshot.Raw` 未使用 |

---

## 建议优化路径

1. Web E2E：`read_file` → `diff_edit` → shell 读同路径（Phase 4+5 联合验收）。
2. `replace_content` 对齐 `textfile.DecodeBytes`（FRAG-P2-04）。
3. Activity/Monitor 卡片展示 hunk 摘要（FRAG-P3-01）。
4. 补 `BuildToolsets` golden 测试（filesystem + hostexec 同 root）。

---

## 变更记录

| 日期 | 说明 |
|------|------|
| 2026-05-21 | 初版 80 分；Override/TestTool/审计主项 |
| 2026-05-22 | Phase 4+5 复核 → **86 分**；专项 review + follow-up changelog |
