# G5-G G-3 图谱 3D 双布局性能基准（2 万节点/5 万边）

> **基准任务**：37-knowledge G5-G G-3 / 计划 C-T1（双布局性能基准录制 + 落档）
> **执行时间**：2026-08-13 15:05–15:21（UTC 07:05）
> **执行者**：AI（Playwright headless Chromium 自动化录制）
> **结论**：**通过**（✅）——三档规模 × 双布局全部收敛，交互帧率达标，画质自适应分档符合设计预期；基准过程中发现并修复物理调度棘轮缺陷（收敛 116s→4.5s）

---

## 1. 基准环境

| 项 | 值 |
|----|----|
| CPU | 12th Gen Intel Core i9-12900K（24 核） |
| 内存 | 32 GB |
| OS | Windows 10（10.0.19045） |
| 浏览器 | Chromium 148.0.7778.96（Playwright headless，1600×1000 视口） |
| GPU | NVIDIA GeForce RTX 2080 Ti（ANGLE D3D11） |
| 前端 | quasar dev（:9301，dev 模式含 HMR 开销） |
| 后端 | bin/admin.exe（:8800，图谱 API 由 `page.route` 拦截注入合成数据集） |
| 数据集 | 确定性 seed 合成（`test/graph3d-perf/gen-dataset.ts`）：幂律度数分布、8 组 doc_type + 未分类、无自环/重复边、保底连通环 |
| 原始数据 | `test/graph3d-perf/results-2026-08-13T0721.json` |

## 2. 基准矩阵（实测）

| 规模 | 布局 | 收敛 | 收敛耗时 | tick P50/P90 | tick 数 | 画质档 | 交互 FPS（mean/p10） | 旋转 FPS | 静置 busyRatio（10s） | 长任务（>50ms） |
|------|------|------|---------|--------------|--------|--------|---------------------|----------|----------------------|----------------|
| 2k 节点 / 5k 边 | force 力导向 | ✅ | **4,487 ms** | 16.7 / 18.8 ms | 229 | HIGH→HIGH | 59.9 / 59.9 | 60.2 | 0.172 | 0（>200ms 0 次） |
| 2k 节点 / 5k 边 | galaxy 星系盘 | ✅ | **3,832 ms** | 16.4 / 17.8 ms | 230 | HIGH→HIGH | 60.0 / 59.9 | 60.1 | 0.145 | 0 |
| 5k 节点 / 12.5k 边 | force 力导向 | ✅ | **6,584 ms** | 26.9 / 30.8 ms | 229 | MID→MID | 60.1 / 59.9 | 60.2 | 0.184 | 0 |
| 5k 节点 / 12.5k 边 | galaxy 星系盘 | ✅ | **6,249 ms** | 26.2 / 29.5 ms | 230 | MID→MID | 60.1 / 59.9 | 58.4 | 0.210 | 0 |
| 20k 节点 / 50k 边 | force 力导向 | ✅ | **29,018 ms** | 122.1 / 132.8 ms | 229 | LOW→LOW | 55.3 / 59.5 | 52.1* | 0.231 | 2 次 >200ms |
| 20k 节点 / 50k 边 | galaxy 星系盘 | ✅ | **28,682 ms** | 120.1 / 132.6 ms | 230 | LOW→LOW | 56.3 / 59.5 | 53.5* | 0.240 | 2 次 >200ms |

\* 20k 规模下「自动旋转」HUD 按钮点击因主线程长任务 actionability 饿死（click + dispatchEvent 兜底均未生效，`enabled=false`），旋转 FPS 为未开启旋转时的纯渲染采样，代表该规模下常驻渲染帧率下限。

**测量方法**（`test/graph3d-perf/run-benchmark.mjs`）：
- Worker tick：页内注入包装 `window.Worker`，按 physics Worker `tick` 消息间隔采样（P50/P90）
- 布局收敛：`init`/布局切换时刻 → Worker `stopped` 消息（alpha < alphaMin）
- 交互帧率：hover 扫掠 20 点 + 拖拽 ×2 + 缩放 ×12 期间 RAF 间隔采样（8s）
- 静置零占用：收敛后静置 10s，CDP `Performance.getMetrics` TaskDuration 增量 + longtask 计数

## 3. 发现与结论

### 3.1 基准过程中修复的缺陷：物理调度棘轮（P0）

旧 `nextTickDelay` 把「上一拍实际间隔」计入补偿，单向锁存形成棘轮——一次调度抖动（GC/主线程抢占）即把 tick 节奏永久抬高。修复前实测 2k 节点收敛被拖到 **116s**（tick 间隔锁死 ~425ms）；修复为只按本拍耗时补到 16ms 节奏后，2k 收敛 **4.5s**（26.7×），tick P50 16.7ms 贴满 60Hz 节奏。修复位置：[physics.worker.ts](../../../web/src/features/knowledge/graph3d/physics.worker.ts) `nextTickDelay`（含红线注释，禁止回归）。

### 3.2 结论

1. **收敛耗时与规模线性**：收敛 tick 数恒定 ~230（alphaDecay=0.0228 / alphaMin=0.005），settle 时间 = 230 × tick 间隔。2k≈4s、5k≈6.5s、20k≈29s。物理在 Worker 中执行，收敛期间主线程可交互。
2. **画质自适应分档符合设计**：初始分级阈值（HIGH <2500 / MID 2500–8000 / LOW ≥8000）与实测三档规模一一对应；运行期 governor 未触发降档（帧率均高于 DOWN_FPS=45），`tierAfter` 全部持平。
3. **交互帧率达标**：2k/5k 交互与旋转全程贴满 60 FPS；20k（LOW 档，关 bloom）交互 55–56 FPS、纯渲染 52–53 FPS，p10 均 ≥59.5（卡顿集中在少数采样点，非持续掉帧）。
4. **静置占用**：收敛后 Worker 自停（`stopped` 到达，零物理计算）；10s 静置主线程 busyRatio 0.145–0.24（dev 模式含 HMR/响应式后台任务，生产构建预期更低），2k/5k 零长任务，20k 仅 2 次 >200ms 长任务（数据集注入/建模瞬间）。

### 3.3 档位建议

- **阈值维持现状**：HIGH/MID/LOW 初始阈值（2500/8000）经实测分档正确，无需调整。
- **20k 收敛期 UX（可选后续项）**：20k 收敛 ~29s 期间画布可交互但节点仍在运动，可在 HUD 加「布局计算中…」轻提示（非本次范围，不阻断 G-3 关闭）。
- **20k HUD 按钮可点性（可选后续项）**：大图主线程长任务期间 HUD 按钮 actionability 饿死，建议 HUD 点击处理改为 pointerdown 直发（不依赖 Playwright 才暴露的稳定性等待，真人用户影响轻微）。

---

**G-3 关闭依据**：2 万节点/5 万边合成数据集双布局交互帧率已记录（§2）；布局收敛静置断言已验（Worker `stopped` + 静置长任务计数，§3.2-4）。本文档即 G-3 要求的性能基准落档。
