# 调研报告：Obsidian 式知识图谱开源方案精读与复用决策（G5 深空版）

> **日期**：2026-08-07 | **类型**：research | **关联模块**：37-knowledge（图谱 Tab G5 改造）
> **调研范围**：Obsidian 风格 3D 知识图谱渲染 + 深空科技感视觉 + 图谱业务逻辑（实体治理）
> **调研产物**：3 个仓库已克隆精读（`test/kg-graph-research/`，验证后可整目录删除）

---

## 1. TL;DR

- **选型结论**：废弃 G4 的 `3d-force-graph` 封装，按 **jkraccoon/obsidian-fast-graph** 蓝本自研渲染层（SoA typed-array 图模型 + Worker 物理 + InstancedMesh + lazy-render），视觉配方取 **c-miles/orrery**（同栈星河四件套：bloom/星云/星空/核雾）与 **Prompt-Surfer/obsidian-jarvis-ui**（HUD 美学 + 节点三层分级 + 星空分层）。
- **业务逻辑结论**：后端实体轨（`knowledge_entities` 字典表已存在）补**归一化 + 别名 + 合并治理**消歧管线，蓝本 Simple Graph Builder 的 7 级实体解析（裁剪适配项目字典模式约定）。
- **用户已裁决**：自研渲染层；深空星河 + HUD 操作台；前端 + 实体消歧一轮做完；粒子流完整复刻；`3d-force-graph` 依赖完全移除。

## 2. 候选评估

| 候选 | 定位 | 评估 | 结论 |
|------|------|------|------|
| **jkraccoon/obsidian-fast-graph** | Obsidian 插件，Three.js + WASM/JS 物理 Worker | 2 万节点 75fps；架构最干净（data/physics/render/interaction 四层）；粒子流为标志性效果 | ★ 主蓝本 |
| **c-miles/orrery** | Obsidian 插件，**同为 3d-force-graph 封装** | 星河视觉四件套（bloom/星云/星空/核雾）参数全部可调；证明同栈可达顶级观感 | ★ 视觉蓝本 |
| **Prompt-Surfer/obsidian-jarvis-ui** | React + 原生 three.js + WASM | HUD 美学体系最完整；节点三层分级；星空三档分层；timelapse 回放 | ★ HUD/分级蓝本 |
| Simple Graph Builder | Obsidian 插件，LLM 实体抽取 | 7 级实体消歧管线（缓存→精确名→别名→embedding→LLM 仲裁→新建） | ★ 后端消歧蓝本 |
| Cosmograph（@cosmograph/*） | GPU 力图引擎库 | 百万节点；但 CC-BY-NC 非商用 + 高层库闭源；cosmos.gl 引擎 MIT | 弃（ license + 闭源） |
| sigma.js + graphology | 2D WebGL 库 | ForceAtlas2 簇布局优秀但 2D；生态成熟 | 弃（项目已定 3D 深空方向） |
| Scrymap / Vault Graph 3D / obsidian-live-wallpaper | 小型 vault 可视化 | 各有亮点（18 预设皮肤等）但架构简单 | 参考，不精读 |
| vasturiano/3d-force-graph | 现用库 | 每节点一个 Object3D，万级节点撑不住；高亮/粒子可控性差 | 移除 |

## 3. obsidian-fast-graph 精读（主蓝本）

仓库：`test/kg-graph-research/obsidian-fast-graph`。核心架构一句话：**typed-array 图模型 → Worker 内 Barnes-Hut 物理（WASM 优先/JS 兜底）→ transferable buffer 回主线程 → InstancedMesh 节点 + LineSegments 边 → lazy-render**。

### 3.1 数据模型（src/data/GraphModel.ts）

SoA 布局，节点 = 整数索引，无任何热路径对象分配：

```text
{ count, edgeCount, paths[], pathToIndex: Map,
  positions: Float32Array(N*3), velocities: Float32Array(N*3),
  degree: Uint16Array(N), groupId: Uint16Array(N), edges: Int32Array(E*2) }
```

- 边为扁平索引对，无向去重键 `lo * 2^26 + hi`；自环跳过。
- **确定性初始布局**：mulberry32 种子 PRNG + 球内均匀体采样（`r = (cbrt(N)*20+1) * cbrt(rand())`，立方根纠正体积分布）——每次打开布局一致，可测试。

### 3.2 物理引擎（src/physics/PhysicsEngine.ts）— 5 力模型

| 力 | 公式 | 默认参数 |
|---|------|---------|
| BH 斥力 | `f = repulsion × mass / d²`，八叉树质心近似 | repulsion=30, theta=0.8 |
| 弹簧 | `k = linkStrength × (d - linkDistance)/d` 沿边 | linkStrength=0.05, linkDistance=30 |
| 簇凝聚 | `F += (簇质心 - p_i) × groupCohesion` | groupCohesion=0.08 |
| 簇分离 | 簇质心间 Coulomb `F += dir × separation × count_h / d²` | groupSeparation=100 |
| 向心力 | `F -= p_i × gravity` | gravity=0.011 |

积分：显式 Euler `v=(v+F·alpha)·damping(0.9)`；**maxStep 位移钳制（≤linkDistance）防 hub 节点弹簧刚度发散**——自研力布局最易踩的坑，此解法直接照搬。冷却 `alphaDecay=0.0228`、`alphaMin=0.005`（比 d3 的 0.001 提前停，省 23% 尾段 tick）。

八叉树（Octree.ts）：typed-array 池（Float32 8/cell + Int32 9/cell，容量 16N 倍增），显式栈迭代无递归，质心除法延迟到查询时。

### 3.3 Worker 协议（src/physics/protocol.ts）

- Main→Worker：`init{count, edges, positions, groupId, params}`（**先 slice 复制再 transfer**）/ `setParams` / `pin` / `unpin` / `reheat` / `stop`
- Worker→Main：`tick{positions, alpha}`（每 tick slice 出新 buffer transfer 回）/ `stopped` / `error`
- 双层兜底：Worker 创建失败 → 主线程 RAF 跑同一引擎；WASM 失败 → JS 引擎。G5 一期只移植 JS 引擎（AssemblyScript WASM 层不做，JS 引擎 2 万节点已 60fps 级）。

### 3.4 渲染层（src/render/）

- **NodeLayer**：`InstancedMesh(SphereGeometry(1,6,4), MeshBasicMaterial)` 低模球；`instanceColor` 手动建 InstancedBufferAttribute；大小 `base + sqrt(degree) × scale`；高亮 = `baseColor.lerp(white, 0.5)`，缓存 baseColors 副本 + prevSet 增量恢复。
- **EdgeLayer**：单条 LineSegments（`0x666666` opacity 0.4），每帧从 positions 拷贝端点。**边高亮不交给边层**——由粒子流表达"边被激活"，避开 LineBasicMaterial 逐顶点变色成本。
- **ParticleLayer（★ 完整复刻对象，123 行自包含）**：
  - 常量：`MAX=80` 粒子、`SPEED=0.45`/秒（~2.2s 穿一条边）、`PointsMaterial{size:8, map:glowTexture, vertexColors, depthWrite:false}`
  - 发光贴图：64×64 canvas 径向渐变 4 stop（1 → 0.85 → 0.25 → 0）
  - 发射：hover 节点的每条邻居边 1 粒子，**起始相位均布 `prog[i]=i/n`**（连续溪流而非齐发）
  - 帧更新：`prog=(prog+dt·0.45)%1`；位置 easeInOutQuad 插值；**颜色三项相位 HSL**：`hue=0.5+0.32·sin((t·0.6 + p·2.2 + i·0.12)·π)`，s=0.9 l=0.62（青↔蓝↔紫↔粉冷色循环）
- **GraphRenderer**：lazy-render——`needsRender || particles.active || autoRotate` 才过 GPU；物理收敛 + 无交互时零占用；OrbitControls damping + autoRotateSpeed 0.6；pixelRatio 钳 2。
- **Picker**：Raycaster 对 InstancedMesh 逐实例求交（包围球粗筛），mousemove 去抖（同时防粒子相位重复重置）。

### 3.5 交互（src/interaction/）

- hover 一跳邻居：全边表 O(E) 线性扫描（低频触发 + 去抖，不建邻接表）。
- localGraph：N 跳 BFS（临时邻接表），子图重建时 **groupId 沿原图复制保持跨视图颜色一致**。
- 交互状态机：`shown`（hover）与 `selected`（点击锁定）分离；位移 <5px 区分拖拽/点击；**首次点击=选中锁定，二次点击同节点=打开文档**。

## 4. orrery 精读（视觉蓝本，同为 3d-force-graph 封装）

仓库：`test/kg-graph-research/orrery`。证明星河视觉全部可在封装库上实现（其 API 扩展点 = 自研层的直接能力）：

| 模式 | 实现 | 关键参数 |
|------|------|---------|
| **bloom** | UnrealBloomPass | strength=1.1（0-3 滑杆）、radius=0.5、**threshold=0.28**——"调到只有加法亮核过阈"是灵魂 |
| **星云** | 半径 5000 反转球（BackSide）+ 3-octave FBM hash 噪声 ShaderMaterial | colA=(0.12,0.06,0.22) 紫 / colB=(0.05,0.17,0.21) 青、bright=0.5、`pow(fbm,2.2)`——**刻意低亮度保持在 bloom 阈值下防糊屏** |
| **星空** | THREE.Points 1400 颗 + **黄金角确定性散布**（无 Math.random） | 64px 径向渐变 dotTexture 把方点变柔光斑；0x9aa6ff、size=3、opacity=0.55 |
| **核雾** | 520 颗加法混合 Points，中心密外缘疏（`rad=spread·(0.18+0.82·t^1.5)`） | 0xb39dff、size=14、opacity=0.2；**布局收敛后锚定到度数最大 hub**——银河核叙事 |
| 冻结布局 | warmupTicks(220) 离屏预演 + cooldownTicks(0) 永久冻结 | 静态知识库不需要持续物理，帧预算全给 bloom |
| 离屏暂停 | IntersectionObserver → pauseAnimation/resumeAnimation | 元素级（非页面级 visibilitychange） |
| pin-and-move 拖拽 | 绕开库自带拖拽：拖拽平面（法线朝相机）+ grabOffset 防跳变 + fx 钉住 + **只重写关联边 O(度数)** | 拖拽时关 controls+autoRotate 是易漏细节 |
| 微弯边 | QuadraticBezierCurve3，控制点=中点+垂直 0.3·len，垂直轴绕边轴旋转 `hash01("s->t")·2π`（每边方向稳定各异），6 段 | vertexColors：rest=#3a5a82×0.32，hover=分组色×0.9 |

## 5. obsidian-jarvis-ui 精读（HUD/分级蓝本）

仓库：`test/kg-graph-research/obsidian-jarvis-ui`。

- **bloom 工程化**：UnrealBloomPass(strength=1.5, radius=0.4, threshold=0.2) + **半分辨率**（w/2×h/2，模糊便宜 4×）+ nMips=3；resize 后须重设 setSize；strength=0 时 `enabled=false` 整 pass 跳过；ACESFilmicToneMapping + exposure=1.2；**不透明纯黑 clear**（bloom 与透明背景不兼容）。
- **星空三档分层**（球面均匀分布 `phi=acos(2r-1)`，半径 5000-8000，`sizeAttenuation:false` 屏幕空间像素尺寸）：dim 2400 颗 size 1.2 op 0.35 / medium 4800 颗 size 1.8 op 0.60 / bright 800 颗 size 3.0 op 0.95。
- **节点三层分级**（workers/force3d.worker.ts，现行代码为绝对阈值版，比 README 宣传的 ratio 版更稳）：`supernode = degree≥15 || (moc 标签 && degree≥3)`；`ultranode = supernode 中连接 ≥4 个不同 supernode`（hub-of-hubs）。视觉：尺寸倍率 1.0/1.5/2.5；物理：分层 charge（-120/-200/-350）+ 分层碰撞半径（12/35/70）。
- **HUD 美学体系**：Courier New 等宽字体、青色 `#00d4ff` / 边色 `#1a3a4a`、`letter-spacing:0.08em`、半透明黑面板 `rgba(0,0,0,0.92)` + `box-shadow:0 0 15px #00d4ff22`、`[ LABELS ON ]` 括号式开关、全部设置 localStorage 持久化 + URL 参数覆盖。
- **zoom-to-cursor**：滚轮射线 ∩ 过 target 且面向相机的平面 → pivot，相机与 target 同步缩放 `0.95^(-ΔY·0.01)`。
- **timelapse**（备选，未纳入 G5）：按 createdAt 过滤 + 新节点 600ms scale 0→1 + 800ms 亮度脉冲 2.7×→1×，"知识库生长回放"。

## 6. 后端实体消歧蓝本（Simple Graph Builder 裁剪）

其 7 级解析管线：持久缓存 → 会话缓存 → 精确名哈希 → 别名哈希 → embedding 相似 >0.90 自动合并 → 0.80-0.90 LLM 仲裁 → 新建。裁剪适配本项目：

- 现状：`knowledge_entities(collection_id, name, entity_type)` UNIQUE(collection_id, name) 已是字典表；抽取仅 TrimSpace + 停用词过滤，**无归一化**——"AI"/"ai"/"ＡＩ" 碎为不同实体。
- G5 裁剪版：归一化（NFC + case-fold + 空白折叠）→ 精确名 → 别名 → （有 embedding 时相似 ≥0.90 自动合并 / 0.80-0.90 进建议队列）→ 新建；embedding 不可用时纯哈希管线全功能可用（对齐 NFR-15）。
- 治理：`MergeEntities` 走 `Data.ExecInTx` 重写 `knowledge_doc_entities` + `knowledge_links` 引用并返回重写条数（项目字典模式约定）。

## 7. 复用决策表

| 资产 | 来源 | 复用方式 |
|------|------|---------|
| SoA 图模型 + 确定性播种 | fast-graph | 移植（去 Obsidian 依赖，doc_id 替代 path） |
| 5 力模型 + maxStep 钳制 + alphaMin=0.005 | fast-graph | 移植公式与参数 |
| typed-array 八叉树 | fast-graph | 移植 |
| Worker 协议 + 主线程兜底 | fast-graph | 移植（仅 JS 引擎，不做 WASM） |
| InstancedMesh 节点 + lerp(white,0.5) 高亮 | fast-graph | 移植 |
| 粒子流层 | fast-graph | **1:1 完整复刻**（用户裁决：时变彩虹色保留） |
| lazy-render 三态门控 | fast-graph | 移植 |
| bloom 参数组合 + 半分辨率 + 不透明深空底 | orrery + jarvis-ui | 融合（strength≈1.2/radius 0.5/threshold 0.28，半分辨率 nMips=3） |
| FBM 星云（亮度压阈值下） | orrery | 移植 shader |
| 三档星空 + 柔光点纹理 | jarvis-ui + orrery | 融合（分层数量 + 确定性散布 + dotTexture） |
| 核雾锚定顶级 hub | orrery | 移植 |
| 微弯 Bezier 边（哈希定向） | orrery | 移植 |
| pin-and-move 拖拽 | orrery | 移植 |
| 节点三层分级 | jarvis-ui | 移植绝对阈值版（super≥15 / ultra≥4 super 邻居；MOC 规则弃——项目无 moc 概念） |
| zoom-to-cursor | jarvis-ui | 移植 |
| 单击选中/双击打开状态机 | fast-graph | 移植（双击 = 现有「在浏览中打开」） |
| local graph N 跳 | fast-graph | 移植（groupId 沿原图保色） |
| HUD 皮肤 | jarvis-ui | 移植到操作台（**限定图谱 Tab 内作用域，不改全局主题 token**） |
| 7 级实体解析 | Simple Graph Builder | 裁剪移植（见 §6） |
| WASM 物理 / timelapse / TrackballControls | fast-graph / jarvis-ui | 弃（一期不需要：JS 引擎够快；timelapse 无 createdAt 叙事需求；OrbitControls 足够） |

## 8. 反模式与坑（必须规避）

1. **UnrealBloomPass 破坏 canvas 透明背景**——必须不透明深空底 clear color（alpha:false）。
2. **星云/核雾亮度必须压在 bloom threshold 以下**（`bright=0.5` + `pow(fbm,2.2)`），否则背景整体糊掉。
3. **自研力布局必须 maxStep 钳制**——hub 节点弹簧刚度 ≈ degree×linkStrength，显式 Euler 不钳制必发散（G4 containment 力防飞散的教训同族）。
4. **Worker init 先 slice 再 transfer**——否则 detach 调用方仍要用的 buffer。
5. **拖拽时必须挂起 controls 与 autoRotate**，恢复时 hoverId 指向刚放下的节点防高亮卡死。
6. **子图重建 groupId 沿原图复制**——否则 local graph 与全局图颜色不一致。
7. **3d-force-graph 每节点 Object3D 模式**在万级节点不可行——自研 InstancedMesh 是 2 万节点 60fps 的前提（fast-graph 决策本身即资产）。

## 9. 参考链接

- fast-graph：<https://github.com/jkraccoon/obsidian-fast-graph>（插件页：<https://community.obsidian.md/plugins/fast-graph>）
- orrery：<https://github.com/c-miles/orrery>
- jarvis-ui：<https://github.com/Prompt-Surfer/obsidian-jarvis-ui>
- Simple Graph Builder：<https://community.obsidian.md/plugins/simple-graph-builder>
- Cosmograph（评估后弃用）：<https://cosmograph.app/docs-general/>
- 3D Graph Render（Galaxy 风格，参考）：<https://darcynorman.net/2026/04/10/experimental-obsidian-3d-graph-renderer/>
- obsidian-live-wallpaper（18 预设皮肤，参考）：<https://www.npmjs.com/package/obsidian-live-wallpaper>
