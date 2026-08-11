# 知识库 UI 升级方案：Liquid Glass 真折射 + Galaxy 星系视图 + 图谱布局切换

> 日期：2026-08-12 ｜ 类型：实施方案（plan）｜ 状态：已评审（用户裁决方案 A）
> 关联调研：`2026-08-07-research-knowledge-graph-oss.md`（G5 图谱选型）、`2026-08-11-research-ui-liquid-glass-visual.md`（液态玻璃美学）、`2026-08-11-research-ui-fluid-3d-particle-plan.md`（流体/粒子）
> 外部参考：Liquid Glass React 系实现（技术调研）、Obsidian 插件 `Longwind1984/galaxy-view`（MIT，星系视图设计借鉴）、AntV G6 / ArcGIS Knowledge（图谱布局谱系）
> 模块文档：`docs/development/37-knowledge.design.md`（实施完成后同步 V12.9 章节）

---

## 1. 背景与目标

知识库模块已具备：G5 自研 three.js GPU 图谱渲染层（Worker 力学、画质 governor、Bloom/星云/粒子/标签/拾取分层）、SP2 深空液态玻璃工作台（GlassPanel 三层液态效果，ADR-8 纯 CSS+SVG 自研零依赖）、5 个自研特效组件。

本次升级目标（用户 2026-08-12 裁决）：

1. **应用范围**：升级知识库现有 UI（KnowledgeGraph3D 图谱 + Workbench 玻璃），不做独立演示页、不动全局主题
2. **Liquid Glass**：从"光纹贴图拟折射"升级为 **feDisplacementMap 真位移折射**
3. **Galaxy view 借鉴**：螺旋星系盘物理、电影感镜头（飞入/巡游/创世）、聚焦模式 + 节点卡片、过滤图例 + 透镜 —— 全部四项
4. **布局切换**：3D 力导向 ↔ 星系盘切换（动画过渡）

### 1.1 调研结论摘要

**Liquid Glass React（技术本质，框架无关）**：

```
backdrop-filter: blur(12-20px) url(#lens) saturate(1.6) brightness(1.05);
#lens = feTurbulence(有机噪声) → feDisplacementMap(按噪声扭曲背景像素)
```

- 真折射与普通玻璃拟态的分水岭：边缘处**背景内容被弯曲放大**（调研报告 §A1 一致）
- 降级链：SVG 滤镜（Chromium/Firefox）→ 纯 blur（Safari）→ 半透明底；`@supports` 纯 CSS 检测，零 JS
- 成本：一条 CSS 声明 + 小型 SVG 滤镜定义；折射昂贵，**仅用于小面积浮层**（调研报告 §A1 落地建议一致）

**Galaxy view（Obsidian 插件，可借鉴特性与实现要点）**：

| 特性 | 实现要点 | 我们现状 |
|------|---------|---------|
| 螺旋星系盘物理 | `coreGravity` 向核引力 + Y 轴压平 + 切向旋臂力 | ❌ 需新增 |
| 电影感镜头 | 点击偏轴飞入+即环绕、闲置巡游、创世绽放开场 | 🟡 有基础飞行，缺巡游/创世 |
| 聚焦模式 | 非邻域全局调暗 + 信息卡（反链/标签/摘要） | 🟡 有高亮无调暗/卡片 |
| 过滤图例 | 图例即过滤器（点击隐藏/悬停只看） | ❌ 需新增 |
| 曲线连线 | 绕核轨道弧（曲率可配，0=直线） | ❌ 直线 |
| Worker 力学 | 布局零主线程卡顿 | ✅ 已有 |
| 画质分档 | 移动端自动降档 | ✅ 已有（governor） |

**图谱布局切换工程共识**：动画过渡而非销毁重建；同一图模型的多种投影切换不重新取数。

### 1.2 关键约束（既有 ADR 合规）

- **ADR-8 / ADR-4**：液态玻璃纯 CSS+SVG 自研，零新依赖；galaxy-view 仅借鉴设计不搬代码（Vue 3 + 自研 GPU 引擎更优）
- **NFR-G5-4**：视觉换肤限定 `.kg-hud` / `.kb-workbench` 作用域，不改全局明暗双主题 token
- **AS-FSM-01**：CameraDirector 显式状态机（>3 状态）
- **数据契约**：`ListCollectionGraph`（B8）不破坏；节点无 tags 字段（详见 §6.3 决策）

---

## 2. 里程碑总览（方案 A：G5 引擎增量增强）

```
M1 真折射玻璃（独立，可先行）
M2 星系盘物理 + 布局切换（基础）
M3 电影感镜头（依赖 M2 布局稳定）
M4 聚焦模式 + 节点卡（依赖 M1 玻璃卡）
M5 过滤图例 + 透镜（依赖 M4 dim 机制）
```

每里程碑：TDD（先失败测试）→ `pnpm lint && pnpm test && pnpm build` → 运行时浏览器实测（R3）→ 再进下一里程碑。

---

## 3. M1 — Liquid Glass 真折射升级

### 3.1 现状差距

[GlassPanel.vue](file:///f:/aranea-agents/web/src/components/knowledge/effects/GlassPanel.vue) 的 `#kb-liquid-refract`（feTurbulence+feDisplacementMap scale=10）仅作用于 `__sheen` 装饰光纹层（`filter: url()` 应用于渐变 div），**背景内容不弯曲**——是"拟折射"。

### 3.2 设计

1. **新增 `LiquidGlassDefs.vue`**（workbench 级挂载一次）：
   - `#kb-liquid-lens`：`feTurbulence type="fractalNoise" baseFrequency="0.008 0.012" numOctaves="2" seed="7"` + `feDisplacementMap scale="24" xChannelSelector="R" yChannelSelector="G"`
   - 参数依据：调研报告 §A1（squircle 表面函数理念）+ Liquid Glass React 系通用参数区间（scale 20-40）
2. **GlassPanel 新增 `refract?: boolean` prop** → `.kb-glass-panel--refract` 修饰类：

```sass
.kb-glass-panel--refract
  @supports (backdrop-filter: url(#x))
    backdrop-filter: blur(var(--kb-blur)) url(#kb-liquid-lens) saturate(1.6) brightness(1.05)
  // 不支持时自动回落现有三层效果（sheen 滤镜方案），纯 CSS 零 JS 检测
```

3. **边缘色差近似**（调研报告 §A3）：`::before` conic-gradient 边缘色散环（mask 仅留 2px 边缘，cyan→magenta 微错位渐变），hover 时强度 +50%。真实 RGB 通道分离在单次 backdrop-filter 不可行，此为业界通用近似
4. **应用纪律（红线）**：`refract` 仅用于浮层/卡片级——QuickSwitcher、CommandPalette、SearchPanel、FocusCard（M4）、GraphLegend（M5）。编辑器/侧栏/长列表**禁用**（backdrop-filter url() 每帧 GPU 重绘成本）

### 3.3 验收

- Chromium：浮层边缘背景内容可见弯曲；Safari/不支持浏览器：视觉与现状一致
- 单测：GlassPanel `refract` prop 类名切换；i18n 无新增文案
- 性能：开启 3 个真折射浮层时工作台交互不掉帧（DevTools FPS 目测）

---

## 4. M2 — 星系盘物理 + 布局切换

### 4.1 新增力（[forces.ts](file:///f:/aranea-agents/web/src/features/knowledge/graph3d/forces.ts)，Worker 内运行）

| 力 | 公式要点 | 效果 |
|----|---------|------|
| `coreGravity(strength)` | 指向核心的引力，替代/叠加现有弱 center pull | 致密亮核 |
| `discFlatten(strength)` | Y 轴弹簧力 `F = -y * strength` 拉向 y=0 平面 | 点云压成盘 |
| `spiralSwirl(strength, arms)` | 切向力（垂直于径向 `(x,z)`）+ 节点按 id hash 分 `arms` 条旋臂，对数螺旋相位偏移 | 旋臂形态 |

参数入 protocol.ts（协议版本化字段 `layoutMode` + `galaxyParams`），physics.worker.ts 透传，Worker 创建失败时主线程 RAF 同一 forces.ts 引擎（沿用 NFR-G5-5 降级路径）。

### 4.2 布局切换：物理 morph 而非坐标插值（关键决策）

Worker 收到 `layoutMode: 'force' | 'galaxy'` 切换后：以当前 positions 为起点，**alpha 再加热至 0.6 重新降温收敛**——节点从旧平衡自然飞向新平衡，即为连续过渡动画（galaxy-view 拖滑杆实时重排同机制）。不引入主线程插值系统，复用现有 Worker 管线，鲁棒性更高。

HUD 工具条新增布局切换开关（`.kg-hud__switch` 风格，i18n 文案 `knowledgePage.graphLayoutForce` / `graphLayoutGalaxy`）。

### 4.3 曲线连线（EdgeLayer 升级）

- 现状：LineSegments 直线
- 升级：每边二次贝塞尔（8 段 LineStrip），控制点 = 中点 + 绕核轨道弧方向偏移 × `curvature` uniform；`curvature=0` 时退化为直线（MID/LOW 画质档默认 0）
- HUD 不暴露曲率滑杆（YAGNI），星系盘模式默认 0.6、力导向默认 0

### 4.4 验收

- 单测：forces.spec.ts 新力数值断言（coreGravity 向心、discFlatten 收敛 y→0、spiralSwirl 切向正交）；protocol 版本字段；physicsWorker.spec 同步
- 运行时：力导向↔星系盘切换连续无跳变；星系盘呈致密核+旋臂盘；governor 不掉档（HIGH 档 ≥55fps @ 现有数据规模）

---

## 5. M3 — 电影感镜头（CameraDirector）

### 5.1 新增 [cameraDirector.ts](file:///f:/aranea-agents/web/src/features/knowledge/graph3d/cameraDirector.ts)（显式状态机，AS-FSM-01）

```
idle ──focusSignal(点击节点)──→ flying ──到达──→ orbiting ──ESC/背景点击──→ idle
idle ──30s 无交互──→ cruising ──任意交互/再次点击──→ idle
flying/orbiting/cruising ──generation 变化──→ idle（数据优先）
```

- **flying（增强飞入）**：三段式——偏轴接近（目标点 + 邻域质心方向偏移向量）→ 减速滑入（easeInOutQuad 复用）→ 到达即环绕
- **orbiting**：以目标为轴心缓慢环绕（扫过邻居密集侧）
- **cruising（Wander 巡游）**：按 degree 降序 + 随机扰动选目标，slow drift；HUD 显示「巡游中」状态，任一输入即退出
- 飞行期间禁用 OrbitControls 输入，结束后归还控制权

### 5.2 创世绽放（reveal 动画）

generation 变化（新数据布局收敛）时：`revealT 0→1` uniform（~1.2s）驱动 NodeLayer 节点 scale/透明度**按距核距离 stagger** 绽放（近核先亮）+ bloom 强度从峰值衰减 + 相机 dolly-out。LOW 画质档跳过动画直接呈现。

### 5.3 验收

- 单测：cameraDirector 状态机转换表（含非法转换拒绝）、巡游目标选择（degree 优先）、交互中断
- 运行时：开场绽放流畅；点击飞入落点偏轴且到达即环绕；30s 闲置入巡游，鼠标一动即退

---

## 6. M4 — 聚焦模式 + 节点信息卡

### 6.1 全局 dim 机制

NodeLayer/EdgeLayer/LabelLayer 加 `dimFactor` uniform（默认 1.0）。选中节点时：

- BFS 邻域集合（1-2 跳，HUD 可选）保持 1.0，其连线满饱和
- 集合外节点/边 → 0.15 透明度（shader 混合，零 CPU 重排）
- 取消选中（背景点击/ESC，沿用现有 `select-node ''` 链路）→ 全体恢复

### 6.2 FocusCard.vue（真折射玻璃卡，复用 M1）

- 内容：文档标题、doc_type（G4 图例色点）、反链 top5（复用 PanelBacklinks 数据链路）、摘要 snippet
- 交互：可拖动（pointer 拖拽）、可收起为标题条、「在编辑器打开」按钮走现有 `open-in-explorer` 链路
- 位置：画布右侧浮层，不遮挡 HUD 工具条

### 6.3 M5 前置数据事实（决策记录）

`CollectionGraphNode` = `{ doc_id, name, rel_path, doc_type, degree }`，**无 tags 字段**；分组配色 = doc_type 稳定哈希（palette.ts 复用 G4 `graphDocTypeColor`）。→ M5 透镜走纯前端方案（见 §7），真标签透镜列为可选后端扩展（proto 变更，不在本轮）。

### 6.4 验收

- 单测：dim BFS 集合计算（1/2 跳边界）、uniform 接线
- 运行时：选中节点全局调暗、邻域全亮；卡片内容正确、可拖可收；ESC 恢复

---

## 7. M5 — 过滤图例 + 透镜（纯前端）

### 7.1 GraphLegend.vue（HUD 左上，真折射玻璃）

- **doc_type 分组列表**：色点（复用 G4 调色板语义，与操作台图例一致）+ 名称 + 节点计数
- 交互（galaxy-view 范式）：**点击 = 隐藏/恢复该组**；**悬停 = 「只看」**（仅显示该组，移开还原）；头部「全部显示」一键重置
- 附加维度：顶层目录（rel_path 第一段）chip 过滤

### 7.2 透镜聚焦

点击图例项/目录 chip → 复用 M4 dim 机制：非匹配节点降至 0.15，匹配节点全亮。与普通节点选择互斥（选择节点临时覆盖透镜，取消后透镜恢复——galaxy-view 同语义）。

### 7.3 过滤管线

model.ts 增加 `hiddenGroups: Set<string>` 过滤 → 过滤后 nodes/edges 重 feed 引擎（轻量 reheat alpha=0.3，不重建 Worker/纹理布局）。过滤状态持久化 localStorage（`kb-graph-filters`，沿用 `kb-panels-collapsed` 先例）。

### 7.4 验收

- 单测：hiddenGroups 过滤管线（边随端点级联）、图例计数、透镜与节点选择互斥
- 运行时：图例点击隐藏组且计数正确；悬停只看；透镜 dim 正确；刷新后过滤状态恢复

---

## 8. 测试与验证策略

| 层 | 内容 |
|----|------|
| 单测（vitest） | 每里程碑配套：forces 新力 / protocol 版本 / cameraDirector 状态机 / dim BFS / 过滤管线 / GlassPanel refract 类。沿用 `__tests__/` 现有 12+ spec 风格 |
| 门禁 | 每里程碑 `pnpm lint`（含 check-i18n，新文案同步 i18n 文件）+ `pnpm test` + `pnpm build` |
| 运行时验证（R3 红线） | 每里程碑起 dev 实测 + 浏览器截图确认视觉效果与交互；确认 FPS governor 不掉档；WebGL 失败占位不回归 |
| 文档同步（DOC-SYNC） | 完成后：37-knowledge.design.md 增补 V12.9 章节（G5 图谱增强 + 玻璃真折射）；37-knowledge.development.md 任务清单状态；本文档保留为方案档案 |

## 9. 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| `backdrop-filter: url()` 仅 Chromium/Firefox 完整支持 | Safari 无真折射 | @supports 自动降级现有三层效果；Tauri 桌面端 Chromium 不受影响 |
| 星系盘力参数致节点重叠/标签失真 | 视觉回归 | discFlatten 与斥力平衡调参（单测数值护栏）；标签阈值机制不变；governor 降档兜底 |
| Worker 协议变更破坏既有消费 | 图谱白屏 | protocol.ts 版本化字段 + physicsWorker.spec 同步；Worker 失败主线程降级路径不变 |
| 创世动画大图卡顿 | LOW 档设备掉帧 | revealT 仅驱动 uniform（零 CPU 布局）；LOW 档跳过动画 |
| dim/透镜状态与既有选中链路冲突 | 交互混乱 | 状态优先级显式定义（节点选择 > 透镜 > 默认）；cameraDirector 状态机单源管理 |

## 10. 改动文件清单（预估）

**新增**：
- `web/src/components/knowledge/effects/LiquidGlassDefs.vue`（M1）
- `web/src/features/knowledge/graph3d/cameraDirector.ts` + `__tests__/cameraDirector.spec.ts`（M3）
- `web/src/components/knowledge/graph3d/FocusCard.vue`（M4）
- `web/src/components/knowledge/graph3d/GraphLegend.vue`（M5）

**修改**：
- `web/src/components/knowledge/effects/GlassPanel.vue`（M1：refract prop）
- `web/src/css/deep-space.sass`（M1：refract 修饰类 + 色差环，作用域内）
- `web/src/features/knowledge/graph3d/forces.ts` / `protocol.ts` / `physics.worker.ts` / `engine.ts`（M2）
- `web/src/components/knowledge/graph3d/render/EdgeLayer.ts`（M2 曲线 / M4 dim）
- `web/src/components/knowledge/graph3d/render/NodeLayer.ts` / `LabelLayer.ts`（M3 reveal / M4 dim）
- `web/src/components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue`（M2-M5 接线）
- `web/src/components/knowledge/KnowledgeGraph3D.vue`（HUD 开关 + 图例/卡片挂载）
- `web/src/features/knowledge/graph3d/model.ts`（M5 过滤管线）
- i18n 文案文件（M2/M3/M5 新增开关文案）

**不做**：不引入任何新 npm 依赖；不改 `ListCollectionGraph` 契约；不动全局主题 token；不改后端代码。
