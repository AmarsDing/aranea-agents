# UI 提升综合方案：流体卡片 / 粒子容器 / 3D 旋转（对标抖音「清晨方白晓」知识库 Demo）

> **类型**：research + 方案 | **日期**：2026-08-11 | **触发**：用户分享抖音视频（`https://v.douyin.com/hxGaJf1fXXs/`），要求学习其配色/样式/操作并给出综合方案
> **视频内容**：Obsidian 风格知识库 UI Demo——**透明流体卡片、3D 环形文件夹（高亮聚焦）、粒子容器**，尺寸/位置/动画速度可自定义。作者将 3D 旋转效果单独剥离重做（自认旧版"粗糙"）。
> **约束**：延续 SP2-ADR-4/ADR-8 自研零依赖原则（不引 tsParticles/Swiper 等库）；全部手法纯 CSS + Canvas2D + 轻量 JS；reduced-motion 必须降级。

---

## 一、视频手法拆解 → Web 落地技术映射

| 视频特性 | 视觉本质 | Web 落地技术（零依赖） |
|---------|---------|----------------------|
| 透明流体卡片 | 卡片内有缓慢流动的彩色光斑，玻璃质感叠加 | ① 卡内 2~3 个 `blur(40-60px)` 色团 div 做 20-30s `translate3d` 关键帧漂移 + `overflow:hidden` 裁剪 + 玻璃 tint 覆盖；② 进阶：`@property --angle` + `conic-gradient` 旋转渐变边框（外发光用 `blur` 复制层） |
| 3D 环形文件夹 | 卡片绕 Y 轴环形排布持续旋转 | `perspective` + `preserve-3d` + `rotateY(iθ) translateZ(r)`——**我们已有**（RingCarousel），但需 JS 驱动化改造（见 §三） |
| 高亮聚焦 | 转到正面的卡片亮起/放大，背侧卡片压暗 | **JS 驱动旋转角度** → 每帧计算各卡与正面角差 → CSS 变量输出 opacity/blur/scale；纯 CSS keyframes 做不到（角度不可读） |
| 粒子容器 | 深空粒子背景 | 已有 ParticleField（斥力+连线）；升级方向见 §四 |
| 尺寸/位置/速度可自定义 | 参数化 | 组件 props 化（radius/speed/cardSize/autoPlay） |

### 关键调研结论

1. **聚焦高亮是"粗糙→精致"的分水岭**。纯 CSS `animation: spin` 的环形旋转（我们现状）无法感知"哪张卡在正面"，所有卡一视同仁 = 视觉平。CoverFlow 经典做法：正面卡 `z-index/scale/opacity` 拉满，侧卡 `blur(2-5px)` + `opacity 0.5-0.6` 递减（出处：addyosmani.com Cover Flow 现代 CSS 分解；Swiper coverflow 参数系 rotate/depth/modifier）。
2. **JS 驱动角度是唯一正解**：`requestAnimationFrame` 累加 `--rot` 自定义属性（`@property` 注册 `<angle>` 或直接内联 transform），同时获得：拖拽旋转、惯性、吸附正面、聚焦高亮四项能力。纯 CSS keyframes 一条都做不到。
3. **流体感 = 慢 + 大 + 糊**：色团直径 ≥ 卡片 60%、blur ≥ 40px、周期 ≥ 20s、透明度 0.15-0.3。快而小 = 廉价噪动感。
4. **旋转渐变边框**（`@property --angle` + conic-gradient + blur 复制层发光）是 2026 年标准手法（theosoti.com），与我们的 `kb-gradient-edge` 静态渐变环互补：静态环给所有玻璃面，**旋转光环只给聚焦卡/激活态**——克制使用。
5. **粒子性能纪律**（tsParticles 官方性能指南）：数量按密度面积换算而非定值、`detectRetina`、页面不可见停帧、移动端降 fpsLimit——我们 ParticleField 已具备分级与停帧，升级时保持。

---

## 二、配色 / 样式 / 操作规格（对齐视频美学 + 深空令牌）

| 维度 | 规格 |
|------|------|
| 配色 | 流体色团走 `--kb-accent-cyan` → `--kb-accent-violet` 双色团（不引入第三彩色，守单 accent 铁律的例外仅限背景装饰）；聚焦卡光晕 = accent 25% 透明 glow |
| 样式 | 卡片保持 `kb-glass` 玻璃底，流体层在玻璃**之下**（z-index 叠序：流体色团 → 玻璃 tint → 内容）；聚焦卡追加旋转渐变光环 |
| 操作 | 拖拽旋转（pointer 事件，水平位移 → 角速度）、滚轮旋转、松手惯性 + 吸附最近卡正面、点击聚焦卡进入、hover 暂停自动旋转 |
| 参数 | `radius`（默认 220）、`speed`（圈/秒，默认 24s/圈）、`cardSize`、`autoPlay` 全 props 化 |

---

## 三、落地 Phase（按性价比排序）

### V1 — RingCarousel 重做为「3D 聚焦环」（对标视频核心，改动最大收益最高）

文件：`components/knowledge/effects/RingCarousel.vue`（重写）、`css/deep-space.sass`（新增流体卡片 mixin）

1. **JS 驱动旋转**：rAF 累加角度 → 内联 `transform: rotateY(...)` 于 ring；卡片按 `rotateY(iθ) translateZ(r)` 不变。
2. **聚焦高亮**：每帧计算各卡与 0°（正面）的角差 Δ，输出 CSS 变量 `--focus`(0~1) 到卡片 style；卡内样式映射：`opacity: 0.45+0.55*focus`、`filter: blur((1-focus)*3px)`、`scale: 0.92+0.08*focus`；正面卡（Δ<步进角/2）追加 `--kb-accent-glow` 光晕 + 旋转渐变光环。
3. **交互**：pointerdown/move/up 拖拽（位移→角速度）、松手惯性衰减 + 吸附、wheel 旋转、hover 暂停（保留）。
4. **流体卡片 mixin**（`=kb-fluid-card`）：卡内 `::before` 双色团（cyan/violet 径向渐变，blur 48px）20-30s 反向漂移关键帧，`overflow:hidden` + 玻璃 tint 覆盖；施加点：环形卡、空态卡。
5. **props 化**：`radius/speed/cardSize/autoplay`；reduced-motion 仍退化为纵向列表（现状保留）。

### V2 — ParticleField 升级（粒子容器）

文件：`components/knowledge/effects/ParticleField.vue`

1. **闪烁（twinkle）**：粒子透明度正弦振荡（相位随机），深空"星光"感。
2. **流星**：每 4-8s 一颗 streak（短尾迹线段，300ms 生命周期），只画 1 颗在屏。
3. **视差双层**：远层小星慢速 + 近层大星快速，鼠标移动视差偏移（非斥力的补充）。
4. 保持既有分级/停帧/斥力/连线；全部新行为走同一 rAF 循环，reduced-motion 不渲染（现状契约不变）。

### V3 — 细节补完

1. **TiltCard 弹簧回正**：`cubic-bezier(0.34,1.56,0.64,1)` 近似弹簧（调研 D2），替代现 180ms 线性回正。
2. **GlowButton 磁吸 + 流体底**复用到工作台主 CTA。
3. （可选）工作台设置面板暴露环转速/粒子密度开关——对齐视频"可自定义"卖点。

---

## 四、不做清单（YAGNI / 风险隔离）

- 不引 tsParticles / Swiper / Three.js（违反 SP2-ADR-4 零依赖；现有 Canvas2D 足够）
- 不做 WebGL 流体模拟（性能/复杂度远超收益，SP2-ADR-8 已否决）
- 旋转渐变光环**只给聚焦卡**，不给所有玻璃面（大面积旋转光 = 廉价游戏感，调研 B 表霓虹克制原则）
- 粒子不加碰撞/物理引擎（tsParticles 性能指南：全屏 links+collisions 组合是掉帧首因）

## 五、验证方案

1. `pnpm vitest run src/components/knowledge src/features/knowledge` 全绿（RingCarousel 现有测试需随 JS 驱动化更新）
2. `pnpm build` + `pnpm lint`（新增 i18n 文案同步双语）
3. **运行时目视**（R3）：启动应用截图验证——聚焦卡高亮/侧卡压暗、流体漂移、拖拽惯性、reduced-motion 降级列表
4. 性能：rAF 循环内零布局读写（只写 transform/opacity CSS 变量）；`document.hidden` 停帧

**主要出处**：抖音视频描述（2026-08-02，透明流体卡片/3D 环形文件夹/高亮聚焦/可自定义）；addyosmani.com《Cover Flow with Modern CSS》（z-index/聚焦手法）；theosoti.com《Animated Gradient Borders》（@property + conic-gradient 旋转光环）；aduok.in 3D Card Carousel（多边形半径公式 tan()、transform 链顺序）；particles.js.org 性能指南（密度/retina/停帧）；hsarticle 2026 渐变指南（动画周期 6-10s、background-position 技法）。
