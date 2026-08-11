# ADR-11: 内置共享插件开放登录管理员变更——workspace fail-closed 语义在 plugin 域的受控开口

## 状态：已接受（2026-08-11）

## 背景

Dogfood 测试（test/dogfood-plugins/report.md ISSUE-002/003）暴露插件管理页全部写操作 404：启停、配置、排序、作用域修改均失败。根因：`PluginService.checkPluginAccess` 对所有写操作套用 `workspace.AssertWorkspaceMutate`，该函数对共享资源（`workspace_id=""`）**fail-closed**——只有 system 调用者可写。而全部 9 个内置插件种子行的 `workspace_id=""`，导致管理页对内置插件的写功能全灭。

与此同时，需求（docs/development/22-plugin §0.3）明确：内置插件是**平台级配置**，管理员需要启停/排序/改配置/调作用域。fail-closed 语义与产品定位直接冲突。

关键事实核查（2026-08-11）：

1. 平台当前**无服务端角色门控体系**：`admins.access` 字段存在（初始管理员 `access=admin`），但无任何业务代码基于 access 做操作级判定；所有登录用户即管理员。
2. `AssertWorkspaceMutate` 的 fail-closed 是多租户防御纵深设计（knowledge 等域仍依赖它防跨租户写）。
3. 内置插件的启停/配置经 `reloadRuntime` 热加载，全局生效。

## 决策

**`checkPluginAccess` 按资源归属分流**：租户私有插件（`workspace_id != ""`）保持 `AssertWorkspaceMutate` 严格写隔离；共享/内置插件（`workspace_id == ""`）回退 `AssertWorkspaceOrShared`——任何登录管理员可变更。

1. 私有插件跨租户写仍 404（NotFound 防探测语义不变，回归测试覆盖）。
2. 共享插件写路径在进程日志留 `plugin.idor` Warn 审计线索（跨租户私有访问尝试时）。
3. **不引入角色门控**：当前无角色体系，为假想角色建门是 YAGNI。

## 后果

**正面**：
- 插件管理页写功能恢复，符合 22-plugin §0.3 产品定位。
- 多租户隔离对真正私有的插件资源不弱化。

**负面 / 风险**：
- 多 workspace 部署下，租户 A 的管理员可修改全局内置插件配置，影响租户 B。当前部署形态（单管理台、全管理员）下无实际暴露面。
- **触发条件（必须回补门控）**：未来引入非管理员角色或 workspace 级只读成员时，必须在 plugin 写路径前补平台级角色门，否则共享插件配置将成为低权限用户的提权面。回补点：`checkPluginAccess` mutate 分支。

## 替代方案

1. **种子行改为每个 workspace 复制一份私有副本**：否决——9 个内置插件 × N workspace 的副本矩阵会让「平台统一定义演进」（schema 中文 title、默认值修复）无法对存量生效（正是 ISSUE-009 要解决的问题）；且管理员要维护 N 份配置。
2. **保持 fail-closed，仅 system 调用者可写，管理页走内部特权通道**：否决——管理页请求与普通 API 同源，引入特权通道会增加一条绕过鉴权的路径，安全收益为负。
3. **立即引入角色门控（access=="admin" 才允许写共享插件）**：否决——当前所有登录用户都是 admin，门控恒真、无实际语义；待角色体系真实存在时按「触发条件」回补。
