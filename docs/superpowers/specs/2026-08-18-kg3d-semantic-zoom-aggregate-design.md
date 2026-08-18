# Knowledge Graph 3D 语义缩放 + 聚合视图 + 边捆绑 设计文档

> Date: 2026-08-18
> Scope: Phase 1 (AggregateLayer + LOD) + Phase 2 (Edge Bundling + Smart Camera Reset)

## 1. 目标

解决当前 3D 知识图谱在**全局视角**下的三个核心 UX 问题：

1. **过密白雾**：>500 节点时，点/边/halo 叠加成"白雾"，标签不可读
2. **标签叠字**：远距离时 top-N 标签仍过多，贪婪碰撞算法在大规模下失效
3. **返回全局空白**：从邻域/过滤恢复时，物理引擎重新收敛导致 3-5 秒空白

## 2. 技术选型

| 方案 | 决策 | 理由 |
|------|------|------|
| 聚合方式 | **超点（Super-node）** 而非 2D 矩阵 | 保留 3D 空间关系，符合现有力导向布局 |
| 聚合粒度 | **按 doc_type 分组**（图谱已有分组字段） | 复用现有颜色/图例体系，无需社区发现算法 |
| LOD 层级 | **3 级**（远/中/近） | 平衡实现复杂度与体验收益 |
| 边渲染 | **贝塞尔曲线束（Edge Bundling）** | 同向边合并，降低视觉混乱；星系盘布局下效果最佳 |
| 相机重置 | **基于当前位置质心直接计算**，不等物理收敛 | 消除"返回全局空白" |

## 3. 数据流

```
当前:  节点数据 → NodeLayer(Points) + EdgeLayer(Lines) + LabelLayer(Sprites)
新增:  节点数据 → AggregateLayer(Points, 按组聚合) → 根据相机距离切换
       边数据   → EdgeLayer(贝塞尔曲线束, segments=8)
       相机     → 距离监听 → qualityTiers.ts 计算 LOD 级别 → 切换渲染层
```

## 4. 详细设计

### 4.1 AggregateLayer（新文件）

**职责**：按 `doc_type` 分组，将同组节点聚合为一个"超点"球体。

**输入**：
- `model: GraphModel`（现有，含 `docTypes: string[]`、`degree: Float32Array`）

**聚合规则**：
- 同组节点数 >= 3 才聚合；<3 保持原样（这些节点仍由 NodeLayer 渲染）
- 超点位置 = 组内节点质心
- 超点半径 = `sqrt(成员数) * 系数`（系数可调，默认 4）
- 超点颜色 = 组色（复用 `buildGroupPalette`）

**渲染**：
- 与 NodeLayer 相同技术：`THREE.Points` + 位置纹理
- 顶点着色器：从位置纹理取质心，按相机距离衰减透明度（远距离不透明，近距离渐隐）
- 片元着色器：柔光点（复用现有 core+halo 风格，但 halo 权重提高，超点更朦胧）

**标签**：
- 超点显示组名（如 "entries (272)"）
- 标签用现有 `LabelLayer` 的 `setFocusLabel` 机制，但作为常驻标签（非 hover/selected）

**交互**：
- 点击超点 → 触发 `focus-group` 事件（父组件过滤右侧面板到该组，并飞行相机到组中心）
- 相机距离 < 阈值时，超点自动溶解（透明度→0），NodeLayer 接管渲染

### 4.2 LOD 语义缩放（修改 qualityTiers.ts + Canvas）

**距离阈值**（相机到目标点的距离）：

| 级别 | 距离 | 渲染内容 |
|------|------|---------|
| FAR | > 300 | 只显示 AggregateLayer（超点 + 组标签） |
| MID | 150-300 | 显示 AggregateLayer + Top-20 hub 节点（度数） + 超点标签 |
| NEAR | < 150 | 显示全部节点（现有逻辑，隐藏 AggregateLayer） |

**实现**：
- 在 `KnowledgeGraph3DCanvas.vue` 的渲染循环中监听 `camera.position.distanceTo(controls.target)`
- 每 100ms 采样一次，避免频繁切换
- 距离变化时调用 `aggregateLayer.setVisible(lodLevel)` 和 `nodeLayer.setVisible(lodLevel)`
- 标签层同步：`labelVis.maxDistance` 按 LOD 级别动态调整

**过渡动画**：
- 超点与节点切换时用 `revealT` 机制做 0.6s 淡入淡出
- 避免瞬间跳变

### 4.3 EdgeLayer 边捆绑（修改 EdgeLayer.ts）

**当前**：直线段（segments=1），星系盘时用 8 段贝塞尔曲线（curvature=0.18）。

**改进**：
- 力导向布局：segments=1（保持性能）
- 星系盘布局：segments=8 + curvature=0.18（已有）
- **新增**：同向边捆绑——按目标节点分组，同一源节点的出边用**二次贝塞尔曲线**捆成一束
  - 控制点偏移量 = 源节点到目标节点中点，沿垂直方向偏移 10%
  - 效果：从 hub 节点发出的边呈"花束状"，而非杂乱无章的直线

**性能**：
- 捆绑计算在模型构建时一次完成（O(E)），不增加每帧开销
- 贝塞尔曲线在顶点着色器中由 `uCurvature` 控制（已有机制）

### 4.4 智能相机重置（修改 KnowledgeGraph3DCanvas.vue）

**问题**：`resetGlobalView` 触发 `loadGraph` → `generation++` → `zoomToFit` → 但此时节点位置是旧的（力导向重新收敛），导致相机飞到"当前质心"，而物理收敛后质心偏移 → 看起来"空白几秒"。

**解决**：
- 返回全局时，**不重置物理引擎**（`skipReheat = true`）
- 直接基于当前节点位置计算质心 + p90 距离
- 相机飞行 600ms 到目标位置
- 物理引擎在后台继续微调，但相机已就位，用户看到"从邻域平滑缩放回全局"

**触发条件**：
- 仅当 `neighborhoodHops` 从 >0 变为 0 且 `nodes/edges` 引用未变时
- 如果数据真的重新加载（如切换 vault），仍走原逻辑（等物理收敛）

## 5. 文件改动清单

| 文件 | 改动 | 说明 |
|------|------|------|
| `render/AggregateLayer.ts` | 新增 | 超点渲染层 |
| `render/EdgeLayer.ts` | 修改 | 支持边捆绑（bundling 参数） |
| `features/knowledge/graph3d/qualityTiers.ts` | 修改 | 新增 LOD 级别定义 |
| `KnowledgeGraph3DCanvas.vue` | 修改 | 集成 AggregateLayer、相机距离监听、智能重置 |
| `features/knowledge/useKnowledgeGraph.ts` | 修改 | `resetGlobalView` 加 `skipReheat` 参数 |
| `KnowledgeGraph3D.vue` | 修改 | 处理 `focus-group` 事件 |

## 6. 测试

- **单元测试**：`AggregateLayer.spec.ts`（聚合规则、超点位置/半径计算）
- **集成测试**：`KnowledgeGraph3DCanvas` 的 LOD 切换逻辑（mock 相机距离）
- **运行时验证**：浏览器打开图谱，从远/中/近三个距离观察渲染内容，点击超点验证展开

## 7. 回滚方案

所有改动通过 `props` 控制（如 `enableAggregate: boolean`），默认开启。若出现性能或渲染问题，可一键关闭回到现有逻辑。
