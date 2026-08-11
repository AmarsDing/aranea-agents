# Skill 管理界面 Dogfood 检查报告

> 日期：2026-08-11 ｜ 范围：Skill 管理主页面 + 全部弹框 + 运行记录/标签字典/经验报告子页
> 方法：浏览器实操（dev 账号）+ API 数据核对 + 后端代码/日志交叉验证
> 证据：`dogfood-output/screenshots/01~44`（44 张截图）

---

## 一、总体结论

| 维度 | 评分 | 结论 |
|------|------|------|
| UI 美观/协调/科技感 | ★★★★☆ | 深色科技风整体统一、信息层级清晰；标签字典弹框配色与主站不一致，主列表信息密度过高 |
| 功能完整性/正确性 | ★★☆☆☆ | 核心 CRUD/发布/启停/版本回滚/标签治理全部可用；**上传导入必然超时（P0）**、复制功能无 UI 入口、运行记录无详情视图 |
| 配置人性化/数据关联 | ★★★☆☆ | 标签字典引用重写（改名/删除事务同步）验证通过，是亮点；但运行记录/经验报告的 version、agent、skillName 字段回填缺失，英文错误消息/摘要未本地化 |

**最紧急**：上传 Skill 功能在当前数据量（37 个已有 Skill）下 100% 超时失败，功能实质不可用。

---

## 二、UI 检查（美观/协调/科技感/配色）

### 正面

1. **整体风格统一**：深色底 + 青色（cyan）强调色贯穿主页面、运行记录、经验报告、标签字典四页； eyebrow kicker（SKILL REGISTRY / SKILL OBSERVABILITY / SKILL INTELLIGENCE）+ 大标题 + 副标题的页头模式一致，科技感强（截图 05/37/44/39）。
2. **状态可视化规范**：已发布=绿色徽章、失败=红色徽章、orphan 标签=琥珀色警示行，色彩语义一致。
3. **磁盘缺失预警横幅**：主页顶部「1 个 Skill 磁盘目录缺失」+ 根目录路径 +「仅看磁盘缺失」过滤开关，信息明确、可操作（截图 27）。
4. **弹框操作防呆**：删除确认说明"软删除，列表中不再显示"；新建标签弹框给出格式提示「小写字母/数字开头，可用 _ -，支持维度前缀，如 file_type:dsx、domain:sales」（截图 27/40）。

### 问题

| # | 级别 | 问题 | 证据 |
|---|------|------|------|
| UI-1 | 中 | **标签字典弹框（新建/改名）为亮灰背景**，与 Skill 管理页的深色弹框（删除确认/上传）明显不一致，视觉上像"另一个系统"的组件 | 截图 40（亮灰）vs 27/31（深色） |
| UI-2 | 中 | **主列表 1440px 宽度下操作列被裁切**，需横向滚动才能看到全部操作按钮 | 截图 07 |
| UI-3 | 低 | 使用统计单元格信息密度过高：使用/成功/失败/耗时 4 组数字 + 红绿小字堆叠在一个单元格，难以快速扫读 | 截图 07 |
| UI-4 | 低 | 筛选栏 8 个控件（搜索/启用状态/状态/标签/来源/磁盘状态/排序/重置/刷新）挤在一行，标签筛选等下拉宽度不足 | 截图 07 |
| UI-5 | 低 | 运行记录每行 SKILL 名下方挂 `unknown` 徽章（version 空），成功/失败徽章右侧还有一个意义不明的空心圆图标，造成视觉噪音 | 截图 37/38 |

---

## 三、功能检查（需求符合度/正确性/完整性）

### 已验证通过的功能

| 功能 | 验证方式 | 结果 |
|------|---------|------|
| 创建 Skill（含校验） | UI 实操 | ✅ 空名校验、创建成功（截图 08~10） |
| 编辑元数据（名称/描述/标签） | UI 实操 | ✅ 保存生效（截图 16/19） |
| 编辑文件（SKILL.md） | UI 实操 | ✅（截图 20） |
| 发布（草稿→已发布） | UI 实操 | ✅（截图 21/25） |
| 启用/停用开关 | UI 实操 | ✅（截图 26） |
| 删除（软删除确认） | UI 实操 | ✅（截图 27） |
| 版本历史 + 回滚确认 | UI 实操 | ✅（截图 28~30） |
| 搜索/多条件筛选/排序 | UI 实操 | ✅（截图 11/18） |
| 运行记录筛选（成功/失败/执行中） | UI 实操 | ✅ 失败筛选出 145 条（截图 38） |
| 标签字典：创建/改名/删除 | API + UI | ✅ 后端校验非法名（400）|
| **标签引用重写（业务关联）** | API 实证 | ✅ 改名 `rewritten:1`、删除 `rewritten:1`，Skill 上标签同步更新 |
| orphan 标签收录 | 代码 + UI | ✅ 收录=同名预建（`onAdopt`） |

### 功能问题（按严重度排序）

| # | 级别 | 问题 | 根因与证据 |
|---|------|------|-----------|
| FN-1 | **P0** | **上传 Skill 必然超时失败**：zip 选择后点击「开始上传检查」，30s 后前端报超时，导入中断 | 三重叠加：① 前端 axios 对 `/v1/skills/import` 用默认 30s 超时（`axiosHandler.ts` 未将其加入 120s 长超时白名单）；② 后端 `Engine.Import` 在 HTTP 请求 ctx 内**同步**执行 inspect（`engine.go:225`），其中 `inspectSimilarity` 对每个已有 Skill 做 1 次 LLM 相似度调用，串行、上限 50 次（当前 37 个 Skill = 37 次串行 DeepSeek 调用）；③ 首个 LLM 调用 30s 未返回（疑思考模式未关闭）。日志实证：2026-08-11 04:01:15.938~940 全部 37 次检查在 2ms 内级联报 `context canceled`——前端 30s 取消请求 ctx 的典型特征（`aranea-pipeline.log` L110785+，截图 34/35） |
| FN-2 | 高 | **复制（Duplicate）功能无 UI 入口**：后端 `POST /v1/skills/{id}/duplicate`、前端 `api.duplicateSkill`、store `duplicate` action 全部就绪，但无任何 Vue 组件调用。Teams/Graphs/Agents 列表均有复制按钮，唯独 Skills 没有 | `stores/skills/index.ts:75`；全仓 `.vue` 无 `.duplicate(` 调用（skill 域）|
| FN-3 | 高 | **运行记录无详情视图**：API 返回 `permissions.canViewDetail:true`，且失败记录含 `errorCode`/`errorMessage`（如 `directory_slug_mismatch`），但表格行不可点击，错误信息只能 hover 失败徽章在 tooltip 里看——不可发现、不可复制 | 截图 38；`SkillRunsTable.vue` 无行点击/详情抽屉 |
| FN-4 | 中 | **运行记录数据关联缺失**：全部记录 `skillVersion:""`（UI 显示 unknown 徽章）、`agentId`/`agentDisplayName:""`（AGENT 列全 "-"）——"追踪使用频率和执行质量"的核心维度（哪个 Agent 调的）完全没数据 | API 实证（`/v1/skill-runs` 响应）|
| FN-5 | 中 | **失败运行记录 SKILL 列空白**：filesystem_reconcile 类失败的记录 `skillId`/`skillName` 均为空，主显示位空白只剩 unknown 徽章；有用信息（如 "rca-root-cause-analysis sync"）藏在 inputPreview hover 里 | 截图 38；API 实证 |
| FN-6 | 中 | **详情页健康度误导**：0 次调用的 Skill 显示 Health「异常」红色徽章 + 成功率 0% 红色。逻辑上 `overallLabel` 只看 `success_rate_7d`，0 调用时 rate=0 直接落入「异常」，缺「无调用数据」前置分支 | 截图 29；`SkillHealthCard.vue:131-137` |
| FN-7 | 中 | **经验报告 SKILL 名称列全 "-"**：后端 `skillName` 字段回填为空（`skillId` 有值），1024 条报告全部显示 "-" | 截图 44；API 实证 |
| FN-8 | 低 | 经验报告的流程摘要/优化建议为英文（"Skill skill_p2func01 completed successfully in 100ms." / "No optimization needed."），中文 UI 下突兀；标签非法名的 400 错误消息也是英文原文（"tag name must match [a-z0-9]..."）直接展示 | 截图 44；API 实证 |

---

## 四、配置人性化 & 数据业务关联

### 做得好的

1. **标签字典的业务关联是范本**：改名/删除在事务内重写所有 Skill 引用并返回重写条数（实测 `rewritten:1`），使用中但未收录的标签以 orphan 合成进列表供一键收录——配置治理闭环完整，学习成本低。
2. **筛选维度贴业务**：主列表支持按启用状态/发布状态/标签/来源/磁盘状态筛选，运行记录按 Skill/Agent/结果/日期筛选，与运维排障动线一致。
3. **危险操作有解释**：删除说明软删除语义、标签删除区分"使用中/未使用"两种确认文案（`deleteConfirmUsed`/`deleteConfirmUnused`）。
4. **空状态有引导**：标签页空状态有图标+标题+提示（`emptyTitle`/`emptyHint`）。

### 不足

| # | 问题 | 建议 |
|---|------|------|
| CF-1 | 上传弹框无进度/阶段提示：点击「开始上传检查」后只有转圈，用户不知道在做 LLM 相似度检查、还要等多久（实际必然超时） | 见 FN-1 修复；短期至少加文案"正在执行安全与相似度检查，可能需要 1~2 分钟" |
| CF-2 | 错误消息未本地化（英文后端消息原文展示） | 前端按 `reason` 码映射中文文案，后端 message 仅作 fallback |
| CF-3 | 运行记录/经验报告的关联字段（version/agent/skillName）不回填，配置页数据"断链" | 写入 invocation 时补齐 version 与 agent 上下文；经验报告写入时联表带 skillName |
| CF-4 | 健康度对 0 调用 Skill 显示「异常」，新装 Skill 一进来就"红" | `overallLabel` 前置判断 `invocation_count_7d == 0` → 「无数据」灰色 |

---

## 五、修复建议优先级

| 优先级 | 项 | 建议方案 |
|--------|-----|---------|
| P0 | FN-1 上传超时 | ① 前端将 `/v1/skills/import` 加入 120s 长超时白名单（1 行）；② 后端 `inspectSimilarity` 并发执行 LLM 调用（如 errgroup 限流 5 并发），并给单次调用包 15s 超时 ctx；③ 相似度检查的 chat model 强制 `thinking_disabled`；④ 中期：inspect 异步化（POST 立即返回 job_id，前端轮询 `GET /v1/skills/import/{id}`——该 API 已存在） |
| P1 | FN-2 复制入口 | SkillTable 操作列加复制按钮，调 `store.duplicate`（前后端已就绪，纯 UI 接线） |
| P1 | FN-3/FN-5 运行记录详情 | 行点击展开详情抽屉：errorCode/errorMessage/inputPreview/outputPreview 完整展示；失败记录主显示位 fallback 到 inputPreview |
| P1 | FN-4/CF-3 关联字段回填 | invocation 写入处补 skillVersion/agentId；经验报告联表补 skillName |
| P2 | FN-6/CF-4 健康度 | 0 调用显示「无数据」 |
| P2 | UI-1 弹框配色 | 标签字典弹框复用主站深色 dialog 卡片样式（`skill-tags-dialog-card` 去掉亮色背景） |
| P2 | FN-8/CF-2 本地化 | 错误码→中文映射；经验报告摘要生成用中文 prompt |
| P3 | UI-2/3/4/5 | 主列表列宽/密度治理，unknown 徽章空值不渲染 |

---

## 六、测试残留清理说明

- 测试 Skill `dogfood-test-skill`（保留，tag 已还原为 `ops`）
- 测试标签 `dogfood-tag` → 改名 `dogfood-tag2` → 已删除，引用已清理，无残留
- 上传测试 zip：`test/dogfood-skill-upload/`（可整目录删除）
