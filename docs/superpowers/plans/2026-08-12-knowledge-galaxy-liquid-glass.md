# 知识库全面升级 Implementation Plan（Liquid Glass 真折射 + Galaxy 星系视图 + 布局切换 + 能力缺口补齐）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 知识库三轨道升级——UI 轨道（M1 真折射玻璃 → M2 星系盘物理/布局切换 → M3 电影感镜头 → M4 聚焦模式+节点卡 → M5 过滤图例+透镜）、能力缺口轨道（B1 文档重嵌入 UI 入口 / B2 集合语义层单向启用）、验证轨道（C 双布局性能基准）。

**Architecture:** 前端纯增量增强既有 G5 深空图谱引擎（GPU 纹理管线 + Worker 物理 + lazy-render 不动）：布局切换用物理 alpha 再加热 morph（非坐标插值），真折射用 SVG feDisplacementMap + `@supports` 降级，零新 npm 依赖。后端 B1/B2 复用既有摄取管线（`BuildIndexedChunks` + `DeleteChunksByDocument`），单 goroutine 串行重嵌入，WS 进度复用摄取通道。

**Tech Stack:** Vue 3 + Quasar + TypeScript + Three.js + Vitest（前端）；Go + Kratos v2 + Ent + PostgreSQL（后端 B1/B2）。

**Spec:** `docs/reports/2026-08-12-plan-knowledge-galaxy-liquid-glass.md`（已评审）

**执行顺序：**

```
M1（玻璃）∥ B1-T1~T3（后端）→ M2（星系盘）→ M3（镜头）→ M4（聚焦+卡片，含 B1 入口②）
  → B1-T4~T5（前端入口①）∥ B2（语义层启用）∥ C（性能基准）→ M5（图例透镜）→ DOC（文档同步）
```

**通用门禁：**
- 前端：`cd web && pnpm vitest run <spec>` → 每里程碑收尾 `pnpm lint && pnpm test && pnpm build`
- 后端：`go test ./internal/service/ -run <Test> -count=1` → Proto 变更 `make api && make wire && make build`
- GOCACHE 纪律：后端验证用干净缓存 `$env:GOCACHE='F:\gocache'`（用户级默认值，勿在工程内设本地缓存）
- 运行时验证（R3 红线）：每里程碑起 dev 实测 + 浏览器确认

---

# 轨道 A — M1：Liquid Glass 真折射

## Task M1-T1: LiquidGlassDefs 单例组件（SVG filter 集中管理）

**Files:**
- Create: `web/src/components/knowledge/effects/LiquidGlassDefs.vue`
- Test: `web/src/components/knowledge/effects/__tests__/LiquidGlassDefs.spec.ts`

**背景**：`GlassPanel.vue:11-16` 当前内联 `kb-liquid-refract` filter（scale=10，装饰光纹用）。多实例重复定义同 id（HTML 非法但浏览器容忍）。M1 抽离为单例 Defs，并新增 `kb-liquid-bg`（背景真折射用，更细腻位移）。

- [x] **Step 1: Write the failing test**

```typescript
// web/src/components/knowledge/effects/__tests__/LiquidGlassDefs.spec.ts
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import LiquidGlassDefs from '../LiquidGlassDefs.vue';

describe('LiquidGlassDefs', () => {
  it('渲染两个单例 filter：kb-liquid-refract（光纹）与 kb-liquid-bg（背景真折射）', () => {
    const w = mount(LiquidGlassDefs);
    expect(w.find('#kb-liquid-refract').exists()).toBe(true);
    expect(w.find('#kb-liquid-bg').exists()).toBe(true);
  });

  it('kb-liquid-bg 含 feDisplacementMap（真折射核心）', () => {
    const w = mount(LiquidGlassDefs);
    const bg = w.find('#kb-liquid-bg');
    expect(bg.find('feDisplacementMap').exists()).toBe(true);
  });

  it('SVG 本体不可见且不占布局（width/height 0 + absolute）', () => {
    const w = mount(LiquidGlassDefs);
    const svg = w.find('svg');
    expect(svg.attributes('width')).toBe('0');
    expect(svg.attributes('height')).toBe('0');
    expect(svg.attributes('aria-hidden')).toBe('true');
  });
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/components/knowledge/effects/__tests__/LiquidGlassDefs.spec.ts`
Expected: FAIL（`Cannot find module '../LiquidGlassDefs.vue'`）

- [x] **Step 3: Write minimal implementation**

```vue
<!-- web/src/components/knowledge/effects/LiquidGlassDefs.vue -->
<template>
  <!-- 液态玻璃 SVG 滤镜单例（M1）：全 Workbench 挂载一次，所有 GlassPanel 共享。
       kb-liquid-refract：装饰光纹位移（原 GlassPanel 内联，迁移至此）。
       kb-liquid-bg：背景真折射（backdrop-filter: url() 用，低频细腻位移）。 -->
  <svg width="0" height="0" aria-hidden="true" focusable="false" class="kb-liquid-defs">
    <defs>
      <filter id="kb-liquid-refract" color-interpolation-filters="sRGB">
        <feTurbulence type="fractalNoise" baseFrequency="0.012 0.028" numOctaves="2" seed="7" result="noise" />
        <feDisplacementMap in="SourceGraphic" in2="noise" scale="10" xChannelSelector="R" yChannelSelector="G" />
      </filter>
      <filter id="kb-liquid-bg" color-interpolation-filters="sRGB">
        <feTurbulence type="fractalNoise" baseFrequency="0.008 0.014" numOctaves="2" seed="11" result="noise" />
        <feDisplacementMap in="SourceGraphic" in2="noise" scale="14" xChannelSelector="R" yChannelSelector="G" />
      </filter>
    </defs>
  </svg>
</template>

<style lang="sass" scoped>
.kb-liquid-defs
  position: absolute
  pointer-events: none
</style>
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/components/knowledge/effects/__tests__/LiquidGlassDefs.spec.ts`
Expected: PASS（3 tests）

- [ ] **Step 5: Commit**

```powershell
git add web/src/components/knowledge/effects/LiquidGlassDefs.vue web/src/components/knowledge/effects/__tests__/LiquidGlassDefs.spec.ts
git commit -m "feat(knowledge): M1 液态玻璃滤镜单例 LiquidGlassDefs（含真折射 kb-liquid-bg）"
```

---

## Task M1-T2: GlassPanel refract prop + deep-space.sass 真折射类

**Files:**
- Modify: `web/src/components/knowledge/effects/GlassPanel.vue`（移除内联 SVG、新增 refract prop）
- Modify: `web/src/css/deep-space.sass`（`kb-glass-surface` mixin 后新增 refract 修饰类）
- Test: `web/src/components/knowledge/effects/__tests__/GlassPanel.spec.ts`

**设计**：`backdrop-filter: url(#kb-liquid-bg)` 仅 Chromium/Firefox 支持；`@supports` 探测失败时保持现有 `blur+saturate`（`kb-glass-surface` 已有），零回归。真折射只加在 `refract` prop 显式开启的浮层面板，普通面板不受影响。

- [x] **Step 1: Write the failing test**

```typescript
// web/src/components/knowledge/effects/__tests__/GlassPanel.spec.ts
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import GlassPanel from '../GlassPanel.vue';

describe('GlassPanel（M1 refract）', () => {
  it('默认不带 refract 修饰类', () => {
    const w = mount(GlassPanel, { props: { title: 'T' } });
    expect(w.classes()).not.toContain('kb-glass-panel--refract');
  });

  it('refract prop → kb-glass-panel--refract 类', () => {
    const w = mount(GlassPanel, { props: { title: 'T', refract: true } });
    expect(w.classes()).toContain('kb-glass-panel--refract');
  });

  it('内联 filter-def 已迁移到 LiquidGlassDefs（防重复 id 回归）', () => {
    const w = mount(GlassPanel, { props: { title: 'T' } });
    expect(w.find('.kb-glass-panel__filter-def').exists()).toBe(false);
    expect(w.find('#kb-liquid-refract').exists()).toBe(false);
  });

  it('装饰层保留（sheen/highlight）', () => {
    const w = mount(GlassPanel, { props: { title: 'T', refract: true } });
    expect(w.find('.kb-glass-panel__sheen').exists()).toBe(true);
    expect(w.find('.kb-glass-panel__highlight').exists()).toBe(true);
  });
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/components/knowledge/effects/__tests__/GlassPanel.spec.ts`
Expected: FAIL（`refract` prop 未定义；`.kb-glass-panel__filter-def` 仍存在）

- [x] **Step 3: Write minimal implementation**

GlassPanel.vue 三处改动：

```vue
<!-- template：删除第 11-16 行内联 SVG（filter-def 迁移 LiquidGlassDefs），class 绑定加 refract -->
<template>
  <div
    ref="panelEl"
    class="kb-glass kb-glass-panel"
    :class="{
      'kb-glass--strong': strong,
      'kb-glass-panel--glow': glow,
      'kb-glass-panel--refract': refract,
    }"
    @pointermove="onPointerMove"
  >
    <!-- 液态玻璃装饰层：SVG 折射光纹 + 指针追随高光，均不拦截交互。
         滤镜定义由 LiquidGlassDefs 单例提供（Workbench 根挂载）。 -->
    <div class="kb-glass-panel__sheen" aria-hidden="true" />
    <div class="kb-glass-panel__highlight" aria-hidden="true" />
    <div v-if="title" class="kb-glass-panel__header">
      <q-icon v-if="icon" :name="icon" size="16px" class="kb-glass-panel__icon" />
      <span class="kb-glass-panel__title">{{ title }}</span>
      <slot name="header-actions" />
    </div>
    <div class="kb-glass-panel__body" :class="{ 'kb-glass-panel__body--flush': flush }">
      <slot />
    </div>
  </div>
</template>
```

```typescript
// script：props 增加 refract
defineProps<{
  title?: string;
  icon?: string;
  /** 更强的玻璃底色（浮层用） */
  strong?: boolean;
  /** 呼吸辉光 */
  glow?: boolean;
  /** 去掉 body 内边距（编辑器/列表等自管理padding的内容） */
  flush?: boolean;
  /** M1：背景真折射（backdrop-filter: url(#kb-liquid-bg)；@supports 降级普通 blur） */
  refract?: boolean;
}>();
```

```sass
// deep-space.sass：在 =kb-glass-surface mixin 定义（第 42 行）之后新增
// ── M1：真折射修饰类（仅显式 refract 的浮层；@supports 降级普通 blur）──
// backdrop-filter: url() 当前仅 Chromium/Firefox；不支持时 kb-glass-surface 的 blur 兜底。
.kb-glass-panel--refract
  @supports (backdrop-filter: url(#kb-liquid-bg))
    backdrop-filter: url(#kb-liquid-bg) blur(var(--kb-blur)) saturate(1.4)
    -webkit-backdrop-filter: url(#kb-liquid-bg) blur(var(--kb-blur)) saturate(1.4)
```

同时删除 GlassPanel.vue `<style>` 中不再引用的 `&__filter-def` 规则（第 96-97 行）。

- [x] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/components/knowledge/effects/__tests__/GlassPanel.spec.ts`
Expected: PASS（4 tests）

- [ ] **Step 5: Commit**

```powershell
git add web/src/components/knowledge/effects/GlassPanel.vue web/src/components/knowledge/effects/__tests__/GlassPanel.spec.ts web/src/css/deep-space.sass
git commit -m "feat(knowledge): M1 GlassPanel refract prop + 真折射 @supports 降级类"
```

---

## Task M1-T3: 挂载 LiquidGlassDefs + 三个现有浮层启用 refract + 运行时验证

**Files:**
- Modify: `web/src/components/knowledge/workbench/KnowledgeWorkbench.vue`（根挂载 LiquidGlassDefs 一次）
- Modify: `web/src/components/knowledge/workbench/QuickSwitcher.vue`（GlassPanel 加 refract）
- Modify: `web/src/components/knowledge/workbench/CommandPalette.vue`（GlassPanel 加 refract）
- Modify: `web/src/components/knowledge/workbench/SearchPanel.vue`（GlassPanel 加 refract）

**纪律**：仅浮层启用 refract（编辑器/侧栏面板禁用——大面积折射影响可读性）。三个浮层均使用 `PaletteModal.vue` 或直接使用 GlassPanel——先 Grep 确认实际使用点再改。

- [x] **Step 1: 定位 GlassPanel 使用点**

Run: `cd web && Grep -n "GlassPanel" src/components/knowledge/workbench/QuickSwitcher.vue src/components/knowledge/workbench/CommandPalette.vue src/components/knowledge/workbench/SearchPanel.vue src/components/knowledge/workbench/PaletteModal.vue`
Expected: 找到各浮层的 GlassPanel 引用行（若三者共用 PaletteModal 壳，则只在 PaletteModal 加 refract 透传 prop）

- [x] **Step 2: Workbench 根挂载 LiquidGlassDefs**

`KnowledgeWorkbench.vue` 模板根节点内（第一个子元素位置）加：

```vue
<LiquidGlassDefs />
```

script 加 import：

```typescript
import LiquidGlassDefs from '../effects/LiquidGlassDefs.vue';
```

- [x] **Step 3: 浮层启用 refract**

按 Step 1 定位结果：直接使用 GlassPanel 的浮层加 `refract` prop；若共用 PaletteModal 壳，则给 PaletteModal 加 `refract?: boolean` 透传 prop 并在三个浮层调用处开启。

- [x] **Step 4: 门禁 + 运行时验证（R3）**（门禁已绿；浏览器运行时验证由协调员执行——子代理无浏览器工具）

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: 全绿（check-i18n 无新增文案，不涉及）

运行时：起 dev，打开知识库 Workbench → ⌘K 命令面板 / ⌘O QuickSwitcher / ⌘⇧F 搜索面板 → 浏览器截图确认背景扭曲折射可见（对比 refract 前后）；DevTools 模拟禁用 `backdrop-filter: url()`（或不支持浏览器）确认降级普通 blur 无破版。

- [x] **Step 5: Commit**

```powershell
git add web/src/components/knowledge/workbench/
git commit -m "feat(knowledge): M1 Workbench 挂载 LiquidGlassDefs，浮层启用真折射"
```

---

# 轨道 A — M2：星系盘物理 + 布局切换

## Task M2-T1: forces.ts 星系盘三力（coreGravity / discFlatten / spiralSwirl）

**Files:**
- Modify: `web/src/features/knowledge/graph3d/forces.ts`（ForceParams 扩展 + tick() 注入三力）
- Test: `web/src/features/knowledge/graph3d/__tests__/forces.spec.ts`（追加 describe 块）

**设计**（坐标系：three.js 右手系，Y 轴向上，星系盘面 = XZ 平面）：
- `coreGravity`：软化径向引力 `f -= pos·k/(1+r·0.02)`，形成致密核（替代默认线性 gravity 的弱收敛）
- `discFlatten`：Y 轴单向向心 `f.y -= y·k`，压扁成盘
- `spiralSwirl`：XZ 平面切向力 `f += k·(-z,0,x)/r·envelope(r)`，螺旋悬臂；envelope = `r/(r+40)` 中心弱、边缘饱和
- 三力默认 0（纯力导向行为完全不变，向后兼容）

- [x] **Step 1: Write the failing test**

在 `__tests__/forces.spec.ts` 末尾追加（复用文件内既有 ForceEngine 构造 helper；若无 helper，用最小构造：2 节点 1 边）：

```typescript
  describe('M2 星系盘三力', () => {
    function makeGalaxyEngine(params: Partial<ForceParams>): ForceEngine {
      const count = 3;
      const positions = new Float32Array([10, 8, 0, -20, -4, 10, 30, 2, -15]);
      return new ForceEngine({
        count,
        edges: new Int32Array([0, 1]),
        positions,
        params: { ...FORCE_DEFAULTS, ...params },
      });
    }

    it('默认参数三力为 0：力导向行为不变（回归）', () => {
      expect(FORCE_DEFAULTS.coreGravity).toBe(0);
      expect(FORCE_DEFAULTS.discFlatten).toBe(0);
      expect(FORCE_DEFAULTS.spiralSwirl).toBe(0);
    });

    it('discFlatten>0：Y 坐标绝对值收敛（压向 XZ 盘面）', () => {
      const e = makeGalaxyEngine({ discFlatten: 0.12, repulsion: 0, linkStrength: 0, gravity: 0, groupCohesion: 0, groupSeparation: 0 });
      const before = Math.abs(e.positions[1]); // 节点0 的 y=8
      for (let t = 0; t < 40; t++) e.tick();
      expect(Math.abs(e.positions[1])).toBeLessThan(before);
    });

    it('spiralSwirl>0：产生 XZ 平面切向速度（角度位置变化）', () => {
      const e = makeGalaxyEngine({ spiralSwirl: 0.05, repulsion: 0, linkStrength: 0, gravity: 0, groupCohesion: 0, groupSeparation: 0 });
      const angleBefore = Math.atan2(e.positions[2], e.positions[0]); // 节点0 (10,8,0) → atan2(0,10)=0
      for (let t = 0; t < 10; t++) e.tick();
      const angleAfter = Math.atan2(e.positions[2], e.positions[0]);
      expect(angleAfter).not.toBeCloseTo(angleBefore, 5);
    });

    it('coreGravity>0：径向距离收缩快于纯线性 gravity', () => {
      const lin = makeGalaxyEngine({ gravity: 0.011, repulsion: 0, linkStrength: 0, groupCohesion: 0, groupSeparation: 0 });
      const core = makeGalaxyEngine({ gravity: 0, coreGravity: 0.08, repulsion: 0, linkStrength: 0, groupCohesion: 0, groupSeparation: 0 });
      const r0 = Math.hypot(core.positions[0], core.positions[1], core.positions[2]);
      for (let t = 0; t < 30; t++) { lin.tick(); core.tick(); }
      const rLin = Math.hypot(lin.positions[0], lin.positions[1], lin.positions[2]);
      const rCore = Math.hypot(core.positions[0], core.positions[1], core.positions[2]);
      expect(rCore).toBeLessThan(rLin);
      expect(rCore).toBeLessThan(r0);
    });

    it('GALAXY_FORCE_PARAMS 预设：三力启用且默认 gravity 减弱', () => {
      expect(GALAXY_FORCE_PARAMS.coreGravity).toBeGreaterThan(0);
      expect(GALAXY_FORCE_PARAMS.discFlatten).toBeGreaterThan(0);
      expect(GALAXY_FORCE_PARAMS.spiralSwirl).toBeGreaterThan(0);
      expect(GALAXY_FORCE_PARAMS.gravity).toBeLessThan(FORCE_DEFAULTS.gravity);
    });
  });
```

文件头部 import 需补充 `GALAXY_FORCE_PARAMS`（若既有 import 是 `import { FORCE_DEFAULTS, ForceEngine } from '../forces'` 则改为含 `GALAXY_FORCE_PARAMS` 与类型 `ForceParams`）。

- [x] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/knowledge/graph3d/__tests__/forces.spec.ts`
Expected: FAIL（`FORCE_DEFAULTS.coreGravity` undefined / `GALAXY_FORCE_PARAMS` 未导出）

- [x] **Step 3: Write minimal implementation**

`forces.ts` 改动：

```typescript
// 1) ForceParams 接口追加三字段（groupSeparation 之后）
export interface ForceParams {
  /** 斥力强度（>0）。 */
  repulsion: number;
  /** 弹簧强度。 */
  linkStrength: number;
  /** 弹簧理想距离（兼 maxStep 钳制值）。 */
  linkDistance: number;
  /** 向心力强度。 */
  gravity: number;
  /** 速度阻尼（0~1，每 tick 乘）。 */
  damping: number;
  /** Barnes-Hut 开张判据。 */
  theta: number;
  /** 簇凝聚强度（同组节点向组中心）。 */
  groupCohesion: number;
  /** 簇分离强度（组中心间 Coulomb 斥力）。 */
  groupSeparation: number;
  /** M2 星系盘：核心引力强度（0=关闭）。软化径向，形成致密核。 */
  coreGravity: number;
  /** M2 星系盘：盘压扁强度（0=关闭）。Y 轴单向向心，压向 XZ 盘面。 */
  discFlatten: number;
  /** M2 星系盘：螺旋切向力强度（0=关闭）。XZ 平面绕 Y 轴，径向包络中心弱边缘饱和。 */
  spiralSwirl: number;
}

// 2) FORCE_DEFAULTS 追加零值（纯力导向默认不变）
export const FORCE_DEFAULTS: ForceParams = {
  repulsion: 30,
  linkStrength: 0.05,
  linkDistance: 30,
  gravity: 0.011,
  damping: 0.9,
  theta: 0.8,
  groupCohesion: 0.08,
  groupSeparation: 100,
  coreGravity: 0,
  discFlatten: 0,
  spiralSwirl: 0,
};

/** M2 星系盘布局预设（布局切换 = setParams(GALAXY_FORCE_PARAMS) + reheat）。 */
export const GALAXY_FORCE_PARAMS: Partial<ForceParams> = {
  coreGravity: 0.08,
  discFlatten: 0.12,
  spiralSwirl: 0.02,
  gravity: 0.004,      // 默认向心减弱（核心引力接管）
  groupSeparation: 60, // 簇间更紧凑（盘内悬臂簇）
};
```

`tick()` 第 3 段（向心力 + Euler 积分段，现第 228-258 行）改造——在 `f[ix] -= this.pos[ix] * gravity;` 三行之后、速度积分之前注入星系盘三力：

```typescript
    // 3) 向心力 + 星系盘三力 + 显式 Euler 积分（maxStep 位移钳制防 hub 发散）
    const { coreGravity, discFlatten, spiralSwirl } = this.params;
    const galaxyMode = coreGravity > 0 || discFlatten > 0 || spiralSwirl > 0;
    const maxStep = linkDistance;
    const maxStep2 = maxStep * maxStep;
    for (let i = 0; i < this.count; i++) {
      if (this.pinned[i]) continue;
      const ix = i * 3;
      const iy = ix + 1;
      const iz = ix + 2;
      f[ix] -= this.pos[ix] * gravity;
      f[iy] -= this.pos[iy] * gravity;
      f[iz] -= this.pos[iz] * gravity;

      if (galaxyMode) {
        const px = this.pos[ix];
        const py = this.pos[iy];
        const pz = this.pos[iz];
        if (coreGravity > 0) {
          // 软化径向引力：中心不过冲，远处弱于线性（致密核成形）
          const r = Math.sqrt(px * px + py * py + pz * pz) || 1e-3;
          const k = coreGravity / (1 + r * 0.02);
          f[ix] -= px * k;
          f[iy] -= py * k;
          f[iz] -= pz * k;
        }
        if (discFlatten > 0) {
          f[iy] -= py * discFlatten; // Y 轴压向 XZ 盘面
        }
        if (spiralSwirl > 0) {
          const rxz = Math.sqrt(px * px + pz * pz);
          if (rxz > 1e-3) {
            const envelope = rxz / (rxz + 40); // 中心弱、边缘饱和
            const s = (spiralSwirl * envelope) / rxz;
            f[ix] += -pz * s;
            f[iz] += px * s;
          }
        }
      }
      // ……速度积分部分（vx/vy/vz 计算）保持不变
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/features/knowledge/graph3d/__tests__/forces.spec.ts`
Expected: PASS（既有用例 + 新 5 用例全绿）

- [x] **Step 5: Commit**

```powershell
git add web/src/features/knowledge/graph3d/forces.ts web/src/features/knowledge/graph3d/__tests__/forces.spec.ts
git commit -m "feat(knowledge): M2 星系盘三力（coreGravity/discFlatten/spiralSwirl）+ 预设参数"
```

---

## Task M2-T2: engine.setLayout 布局切换（alpha 再加热 morph）

**Files:**
- Modify: `web/src/features/knowledge/graph3d/engine.ts`（新增 `setLayout` 方法 + `GraphLayout` 类型）
- Test: `web/src/features/knowledge/graph3d/__tests__/engine.spec.ts`（追加 describe）

**设计**：布局切换 = `setParams(预设)` + `reheat()`（alpha 归 1，物理重新收敛 → 自然 morph，非坐标插值）。Worker/主线程两路径自动兼容（`setParams`/`reheat` 消息已存在，无需新协议消息；`InitMessage.params` 为 `ForceParams` 类型自动携带新字段——M2-T1 已扩展接口，Worker 构造透传不变）。

- [ ] **Step 1: Write the failing test**

在 `engine.spec.ts` 追加：

```typescript
  describe('M2 setLayout 布局切换', () => {
    function makeEngine(): GraphEngine {
      const model = buildGraphModel(
        [
          { docId: 'a', name: 'a', relPath: 'a.md', docType: 'note' },
          { docId: 'b', name: 'b', relPath: 'b.md', docType: 'note' },
        ],
        [{ source: 'a', target: 'b', type: 'explicit' }],
      );
      seedPositions(model, 1337);
      return new GraphEngine(model, {}, { workerFactory: () => null as unknown as WorkerLike });
    }

    it('setLayout("galaxy")：主线程兜底路径参数切到星系盘预设且 alpha 再加热', () => {
      const e = makeEngine();
      e.start();
      // 收敛后 alpha 衰减
      for (let t = 0; t < 400; t++) e.stepFrame();
      expect(e.settled).toBe(true);
      e.setLayout('galaxy');
      expect(e.settled).toBe(false); // reheat 唤醒
      // 参数生效：再跑若干 tick 不发散（数值护栏）
      for (let t = 0; t < 60; t++) e.stepFrame();
      const p = e.positions;
      for (let i = 0; i < p.length; i++) expect(Number.isFinite(p[i])).toBe(true);
      e.stop();
    });

    it('setLayout("force")：切回力导向默认参数', () => {
      const e = makeEngine();
      e.start();
      e.setLayout('galaxy');
      e.setLayout('force');
      expect(e.settled).toBe(false);
      for (let t = 0; t < 60; t++) e.stepFrame();
      const p = e.positions;
      for (let i = 0; i < p.length; i++) expect(Number.isFinite(p[i])).toBe(true);
      e.stop();
    });
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/knowledge/graph3d/__tests__/engine.spec.ts`
Expected: FAIL（`e.setLayout is not a function`）

- [ ] **Step 3: Write minimal implementation**

`engine.ts` 改动：

```typescript
// import 更新
import { FORCE_DEFAULTS, ForceEngine, GALAXY_FORCE_PARAMS, type ForceParams } from './forces';

/** M2：图谱布局模式。force=力导向（默认）；galaxy=星系盘。 */
export type GraphLayout = 'force' | 'galaxy';

// GraphEngine 类内新增（setParams 方法之后）：
  /** 布局切换：参数预设 + alpha 再加热 morph（非坐标插值；Worker/主线程同路径）。 */
  setLayout(layout: GraphLayout): void {
    this.setParams(layout === 'galaxy' ? { ...GALAXY_FORCE_PARAMS } : { ...FORCE_DEFAULTS });
    this.reheat();
  }
```

注：测试用 `workerFactory: () => null` 会走 `startFallback()` 主线程路径（factory 返回 null → `w` 为 falsy）。若既有 engine.spec 构造方式不同，沿用其模式。

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/features/knowledge/graph3d/__tests__/engine.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```powershell
git add web/src/features/knowledge/graph3d/engine.ts web/src/features/knowledge/graph3d/__tests__/engine.spec.ts
git commit -m "feat(knowledge): M2 GraphEngine.setLayout 布局切换（alpha 再加热 morph）"
```

---

## Task M2-T3: EdgeLayer 曲线连线（segments + curvature uniform）

**Files:**
- Modify: `web/src/components/knowledge/graph3d/render/EdgeLayer.ts`（可选细分 + 顶点着色器贝塞尔）
- Test: `web/src/components/knowledge/graph3d/render/__tests__/EdgeLayer.spec.ts`（新建或追加）

**设计**：每顶点携带 `aNodeA`/`aNodeB`（两端点索引）+ `aT`（插值参数）；顶点着色器 `mix/bezier` 统一：`curvature=0` 退化直线（力导向，segments=1，行为不变），星系盘 `curvature>0` + segments=8（弧线沿盘面弯曲）。贝塞尔控制点 = 中点 + XZ 平面法向偏移。

- [ ] **Step 1: Write the failing test**

```typescript
// EdgeLayer.spec.ts（若文件不存在则新建，沿用渲染层既有 spec 模式；无 WebGL 上下文，仅测几何构造）
import { describe, expect, it } from 'vitest';
import { EdgeLayer } from '../EdgeLayer';

describe('EdgeLayer（M2 曲线）', () => {
  const edges = new Int32Array([0, 1, 1, 2]); // 2 边
  const colors = new Float32Array([1, 0, 0, 0, 1, 0]);

  it('segments=1（默认）：每边 2 顶点，与现有一致', () => {
    const layer = new EdgeLayer(edges, colors);
    expect(layer.object.geometry.getAttribute('position').count).toBe(4);
    layer.dispose();
  });

  it('segments=8：每边 16 顶点（8 段 × 2）', () => {
    const layer = new EdgeLayer(edges, colors, 8);
    expect(layer.object.geometry.getAttribute('position').count).toBe(32);
    // 每顶点携带两端点索引 + 插值参数
    expect(layer.object.geometry.getAttribute('aNodeA')).toBeDefined();
    expect(layer.object.geometry.getAttribute('aNodeB')).toBeDefined();
    const at = layer.object.geometry.getAttribute('aT');
    expect(at.getX(0)).toBe(0);
    expect(at.getX(1)).toBeCloseTo(1 / 8, 5);
    layer.dispose();
  });

  it('setCurvature 更新 uniform', () => {
    const layer = new EdgeLayer(edges, colors);
    layer.setCurvature(0.25);
    expect((layer.object.material as { uniforms: { uCurvature: { value: number } } }).uniforms.uCurvature.value).toBe(0.25);
    layer.dispose();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/components/knowledge/graph3d/render/__tests__/EdgeLayer.spec.ts`
Expected: FAIL（构造函数不接受第三参 / `aNodeA` 属性不存在 / `setCurvature` 未定义）

- [ ] **Step 3: Write minimal implementation**

`EdgeLayer.ts` 改造（要点——保持既有 highlight/pulse 逻辑不变）：

```typescript
const EDGE_VERTEX = `
  uniform sampler2D uPosTex;
  uniform float uTexW;
  uniform float uCurvature;
  attribute float aNodeA;
  attribute float aNodeB;
  attribute float aT;
  attribute vec3 aColor;
  attribute float aHi;
  varying vec3 vColor;
  varying float vHi;
  varying float vT;
  void main() {
    int ia = int(aNodeA + 0.5);
    int ib = int(aNodeB + 0.5);
    vec3 pa = texelFetch(uPosTex, ivec2(ia % int(uTexW), ia / int(uTexW)), 0).xyz;
    vec3 pb = texelFetch(uPosTex, ivec2(ib % int(uTexW), ib / int(uTexW)), 0).xyz;
    vec3 wp = mix(pa, pb, aT);
    if (uCurvature > 0.0001) {
      // 二次贝塞尔：控制点 = 中点 + XZ 平面法向偏移（弧线沿盘面弯曲）
      vec3 mid = (pa + pb) * 0.5;
      vec3 dir = pb - pa;
      vec3 normal = normalize(vec3(-dir.z, 0.0, dir.x) + vec3(1e-4));
      vec3 ctrl = mid + normal * uCurvature * length(dir);
      float t = aT;
      wp = mix(mix(pa, ctrl, t), mix(ctrl, pb, t), t);
    }
    gl_Position = projectionMatrix * modelViewMatrix * vec4(wp, 1.0);
    vColor = aColor;
    vHi = aHi;
    vT = aT;
  }`;
```

构造函数签名 `constructor(edges: Int32Array, edgeColors: Float32Array, segmentsPerEdge = 1)`：按 `segmentsPerEdge` 展开顶点（每边 `segments×2` 顶点，逐顶点写 `aNodeA`/`aNodeB`/`aT`/`aColor`/`aHi`）；新增 uniform `uCurvature: { value: 0 }` 与方法：

```typescript
  /** M2：弧线弯曲系数（0=直线，力导向；>0 星系盘）。 */
  setCurvature(v: number): void {
    this.material.uniforms.uCurvature.value = v;
  }
```

片元着色器与 setHighlight 等其余逻辑保持不变（`vT` 语义不变）。

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/components/knowledge/graph3d/render/__tests__/EdgeLayer.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```powershell
git add web/src/components/knowledge/graph3d/render/EdgeLayer.ts web/src/components/knowledge/graph3d/render/__tests__/EdgeLayer.spec.ts
git commit -m "feat(knowledge): M2 EdgeLayer 曲线连线（贝塞尔 curvature，直线零回归）"
```

---

## Task M2-T4: Canvas layout prop + HUD 切换 + 持久化 + 运行时验证

**Files:**
- Modify: `web/src/components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue`（layout prop + watch → engine.setLayout + EdgeLayer 重建/curvature 联动）
- Modify: `web/src/components/knowledge/KnowledgeGraph3D.vue`（HUD 布局切换按钮 + emit）
- Modify: `web/src/pages/KnowledgePage.vue`（graphLayout 状态 + localStorage 持久化）
- Modify: `web/src/i18n/zh-Hans/index.ts` + `en-US/index.ts`（新增文案 `knowledgePage.graphLayoutForce` / `graphLayoutGalaxy`）

**设计**：
- Canvas props 加 `layout?: 'force' | 'galaxy'`（默认 `'force'`）；`watch(() => props.layout)` → `engine?.setLayout(v)` + 边层联动（galaxy：EdgeLayer 重建 segments=8 + `setCurvature(0.18)`；force：segments=1 + curvature 0）。EdgeLayer 重建：从 rebuildGraph 抽局部函数 `rebuildEdges()`（dispose 旧 edgeLayer → 按 currentLayout 新建 → setPositionTexture/setHighlight 恢复）。
- 布局切换时相机不动（物理 morph 自行收敛）；`pendingFit` 不重置（避免视角跳动）。
- HUD 按钮沿用 `kg-hud__switch` 模式（参照 autoRotate/showLabels，KnowledgeGraph3D.vue 第 43-56 行）。
- 持久化：`localStorage['kg3d-layout']`，KnowledgePage 初始化读取。

- [ ] **Step 1: Canvas 接线**

```typescript
// props 增加
    /** M2：布局模式（力导向/星系盘）。 */
    layout?: 'force' | 'galaxy';
// withDefaults 增加 layout: 'force'

// 局部状态
let currentLayout: 'force' | 'galaxy' = 'force';

// rebuildGraph 内 EdgeLayer 构造处（现第 288 行）改为抽出的 rebuildEdges()：
function rebuildEdges(): void {
  if (!scene || !model || !posTex) return;
  if (edgeLayer) {
    scene.remove(edgeLayer.object);
    edgeLayer.dispose();
  }
  const m = model;
  const edgeColors = new Float32Array(m.edgeCount * 3);
  for (let e = 0; e < m.edgeCount; e++) {
    const [r, g, b] = hexToRgbFloat(graphLinkColor(m.edgeTypes[e]));
    edgeColors[e * 3] = r;
    edgeColors[e * 3 + 1] = g;
    edgeColors[e * 3 + 2] = b;
  }
  edgeLayer = new EdgeLayer(m.edges, edgeColors, currentLayout === 'galaxy' ? 8 : 1);
  edgeLayer.setCurvature(currentLayout === 'galaxy' ? 0.18 : 0);
  edgeLayer.setPositionTexture(posTex.texture, posTex.width);
  scene.add(edgeLayer.object);
  applyHighlight(); // 恢复高亮状态
}

// rebuildGraph 中原 EdgeLayer 构造段替换为 rebuildEdges() 调用；
// engine 构造后（现第 307-313 行）按布局初始化参数：
  if (currentLayout === 'galaxy') engine.setLayout('galaxy');

// watch（script 尾部）：
watch(
  () => props.layout,
  (v) => {
    if (v === currentLayout) return;
    currentLayout = v;
    rebuildEdges();
    engine?.setLayout(v);
    requestRender();
  },
);
```

- [ ] **Step 2: HUD + 页面接线 + i18n**

KnowledgeGraph3D.vue：props 加 `layout: string`；emits 加 `'update:layout': [v: string]`；HUD 工具条（autoRotate 按钮旁）加：

```vue
          <button
            type="button"
            class="kg-hud__switch"
            :class="{ 'kg-hud__switch--on': layout === 'galaxy' }"
            @click="$emit('update:layout', layout === 'galaxy' ? 'force' : 'galaxy')"
          >
            <q-icon name="blur_circular" size="13px" />
            <span>{{ layout === 'galaxy' ? t('knowledgePage.graphLayoutGalaxy') : t('knowledgePage.graphLayoutForce') }}</span>
          </button>
```

KnowledgePage.vue：

```typescript
const graphLayout = ref<'force' | 'galaxy'>(
  (typeof localStorage !== 'undefined' && localStorage.getItem('kg3d-layout') === 'galaxy' ? 'galaxy' : 'force'),
);
watch(graphLayout, (v) => {
  try { localStorage.setItem('kg3d-layout', v); } catch { /* 隐私模式忽略 */ }
});
```

模板中 `<KnowledgeGraph3D v-model:layout="graphLayout" ...>`（组件 props/emits 对应 `layout` + `update:layout`）。

i18n（zh-Hans：`graphLayoutForce: '力导向'`、`graphLayoutGalaxy: '星系盘'`；en-US：`Force-directed` / `Galaxy`）。

- [ ] **Step 3: 门禁 + 运行时验证（R3）**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: 全绿

运行时：知识库图谱 → HUD 切换「星系盘」→ 确认：节点物理 morph 成盘（非瞬移）、弧线边显现、标签不重叠恶化、FPS governor 不掉档；切回力导向恢复；刷新页面布局选择保持。

- [ ] **Step 4: Commit**

```powershell
git add web/src/components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue web/src/components/knowledge/KnowledgeGraph3D.vue web/src/pages/KnowledgePage.vue web/src/i18n/
git commit -m "feat(knowledge): M2 HUD 布局切换接线 + localStorage 持久化"
```

---

# 轨道 A — M3：电影感镜头

## Task M3-T1: cameraDirector 显式状态机（AS-FSM-01）

**Files:**
- Create: `web/src/features/knowledge/graph3d/cameraDirector.ts`
- Test: `web/src/features/knowledge/graph3d/__tests__/cameraDirector.spec.ts`

**设计**：5 状态（idle/flying/orbiting/cruising/genesis）+ 合法转换表 + `canTransition` 校验 + `update(progress01)` 输出相机位姿（位置 + 看向 + revealT）。用户交互（OrbitControls start 事件）任何状态 → idle（中断镜头）。

- [ ] **Step 1: Write the failing test**

```typescript
// cameraDirector.spec.ts
import { describe, expect, it } from 'vitest';
import {
  CameraDirector,
  canTransition,
  type CameraEvent,
  type CameraState,
} from '../cameraDirector';

describe('cameraDirector 状态机（M3）', () => {
  it('合法转换表：idle→focus/genesis/cruise；flying→arrived/user-interrupt；orbiting→user-interrupt/timeout/focus；genesis→completed/user-interrupt；cruising→user-interrupt/focus', () => {
    const legal: Array<[CameraState, CameraEvent]> = [
      ['idle', 'focus'],
      ['idle', 'genesis'],
      ['idle', 'cruise'],
      ['flying', 'arrived'],
      ['flying', 'user-interrupt'],
      ['orbiting', 'user-interrupt'],
      ['orbiting', 'timeout'],
      ['orbiting', 'focus'],
      ['genesis', 'completed'],
      ['genesis', 'user-interrupt'],
      ['cruising', 'user-interrupt'],
      ['cruising', 'focus'],
    ];
    for (const [s, e] of legal) expect(canTransition(s, e), `${s}+${e}`).toBe(true);
  });

  it('非法转换拒绝：idle+arrived / orbiting+genesis / cruising+completed', () => {
    expect(canTransition('idle', 'arrived')).toBe(false);
    expect(canTransition('orbiting', 'genesis')).toBe(false);
    expect(canTransition('cruising', 'completed')).toBe(false);
  });

  it('dispatch 驱动状态迁移：idle→focus→flying→arrived→orbiting→user-interrupt→idle', () => {
    const d = new CameraDirector();
    expect(d.state).toBe('idle');
    expect(d.dispatch('focus', { target: [10, 0, 0], distance: 60 })).toBe(true);
    expect(d.state).toBe('flying');
    expect(d.dispatch('arrived')).toBe(true);
    expect(d.state).toBe('orbiting');
    expect(d.dispatch('user-interrupt')).toBe(true);
    expect(d.state).toBe('idle');
  });

  it('非法 dispatch 返回 false 且状态不变', () => {
    const d = new CameraDirector();
    expect(d.dispatch('arrived')).toBe(false);
    expect(d.state).toBe('idle');
  });

  it('flying 插值：update(0) 在起点附近，update(1) 距目标 distance 处且看向目标', () => {
    const d = new CameraDirector();
    d.dispatch('focus', { target: [100, 0, 0], distance: 50, from: [0, 0, 400] });
    const p0 = d.update(0);
    const p1 = d.update(1);
    expect(p0.position[2]).toBeCloseTo(400, 1);
    expect(Math.hypot(p1.position[0] - 100, p1.position[1], p1.position[2])).toBeCloseTo(50, 0);
    expect(p1.lookAt).toEqual([100, 0, 0]);
  });

  it('genesis：update 输出 revealT 0→1（供 NodeLayer uRevealT uniform）', () => {
    const d = new CameraDirector();
    d.dispatch('genesis', { duration: 1200 });
    expect(d.update(0).revealT).toBe(0);
    expect(d.update(0.5).revealT).toBeGreaterThan(0);
    expect(d.update(0.5).revealT).toBeLessThan(1);
    expect(d.update(1).revealT).toBe(1);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/knowledge/graph3d/__tests__/cameraDirector.spec.ts`
Expected: FAIL（模块不存在）

- [ ] **Step 3: Write minimal implementation**

```typescript
// cameraDirector.ts
/**
 * cameraDirector：M3 电影感镜头状态机（AS-FSM-01 显式状态机，纯 TS 零 three 依赖）。
 *
 * 5 状态：idle（用户自由操控）/ flying（飞往目标节点）/ orbiting（绕目标缓转）
 *         / cruising（全局巡游）/ genesis（创世绽放，驱动 NodeLayer uRevealT）
 * 用户交互（user-interrupt）任何进行态 → idle（镜头让位手控）。
 * update(progress01) 输出当前帧相机指令（位置/看向/revealT），由 Canvas RAF 驱动。
 */

export type CameraState = 'idle' | 'flying' | 'orbiting' | 'cruising' | 'genesis';
export type CameraEvent = 'focus' | 'genesis' | 'cruise' | 'arrived' | 'timeout' | 'completed' | 'user-interrupt';

export type Vec3 = [number, number, number];

export interface CameraPose {
  position: Vec3;
  lookAt: Vec3;
  /** 创世进度（非 genesis 状态恒 1）。 */
  revealT: number;
}

export interface FocusPayload {
  target: Vec3;
  distance: number;
  from?: Vec3;
}

export interface GenesisPayload {
  duration: number;
}

/** 合法转换表：state → 允许的事件集。 */
const TRANSITIONS: Record<CameraState, ReadonlySet<CameraEvent>> = {
  idle: new Set(['focus', 'genesis', 'cruise']),
  flying: new Set(['arrived', 'user-interrupt']),
  orbiting: new Set(['user-interrupt', 'timeout', 'focus']),
  cruising: new Set(['user-interrupt', 'focus']),
  genesis: new Set(['completed', 'user-interrupt']),
};

/** 事件 → 目标状态。 */
const EVENT_TARGET: Record<CameraEvent, CameraState> = {
  focus: 'flying',
  genesis: 'genesis',
  cruise: 'cruising',
  arrived: 'orbiting',
  timeout: 'idle',
  completed: 'idle',
  'user-interrupt': 'idle',
};

export function canTransition(from: CameraState, event: CameraEvent): boolean {
  return TRANSITIONS[from].has(event);
}

function easeInOutQuad(t: number): number {
  return t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2;
}

export class CameraDirector {
  private _state: CameraState = 'idle';
  private focusTarget: FocusPayload | null = null;
  private orbitAngle = 0;

  get state(): CameraState {
    return this._state;
  }

  dispatch(event: CameraEvent, payload?: FocusPayload | GenesisPayload): boolean {
    if (!canTransition(this._state, event)) return false;
    if (event === 'focus') this.focusTarget = payload as FocusPayload;
    this._state = EVENT_TARGET[event];
    return true;
  }

  /** progress01：当前状态内的归一化进度（flying/genesis 用；orbiting 内部累计角度）。 */
  update(progress01: number): CameraPose {
    const t = Math.min(1, Math.max(0, progress01));
    if (this._state === 'flying' && this.focusTarget) {
      const { target, distance, from = [0, 0, 400] } = this.focusTarget;
      const e = easeInOutQuad(t);
      // 二次贝塞尔甩镜：控制点侧向抬升（电影感弧度）
      const mid: Vec3 = [(from[0] + target[0]) / 2, (from[1] + target[1]) / 2 + distance * 0.6, (from[2] + target[2]) / 2];
      const quad = (a: number, c: number, b: number): number => (1 - e) * ((1 - e) * a + e * c) + e * ((1 - e) * c + e * b);
      const end: Vec3 = [target[0], target[1], target[2] + distance];
      return {
        position: [quad(from[0], mid[0], end[0]), quad(from[1], mid[1], end[1]), quad(from[2], mid[2], end[2])],
        lookAt: [from[0] + (target[0] - from[0]) * e, from[1] + (target[1] - from[1]) * e, from[2] + (target[2] - from[2]) * e],
        revealT: 1,
      };
    }
    if (this._state === 'orbiting' && this.focusTarget) {
      this.orbitAngle += 0.004;
      const { target, distance } = this.focusTarget;
      return {
        position: [target[0] + Math.sin(this.orbitAngle) * distance, target[1] + distance * 0.25, target[2] + Math.cos(this.orbitAngle) * distance],
        lookAt: target,
        revealT: 1,
      };
    }
    if (this._state === 'genesis') {
      return { position: [0, 0, 400], lookAt: [0, 0, 0], revealT: t === 1 ? 1 : easeInOutQuad(t) };
    }
    // idle/cruising：不干预（Canvas 忽略输出）
    return { position: [0, 0, 0], lookAt: [0, 0, 0], revealT: 1 };
  }

  reset(): void {
    this._state = 'idle';
    this.focusTarget = null;
    this.orbitAngle = 0;
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/features/knowledge/graph3d/__tests__/cameraDirector.spec.ts`
Expected: PASS（6 tests）

- [ ] **Step 5: Commit**

```powershell
git add web/src/features/knowledge/graph3d/cameraDirector.ts web/src/features/knowledge/graph3d/__tests__/cameraDirector.spec.ts
git commit -m "feat(knowledge): M3 CameraDirector 电影感镜头状态机（AS-FSM-01）"
```

---

## Task M3-T2: Canvas 集成（genesis 驱动 NodeLayer reveal + user-interrupt 接线）+ 运行时验证

**Files:**
- Modify: `web/src/components/knowledge/graph3d/render/NodeLayer.ts`（新增 `uRevealT` uniform + `setRevealT`）
- Modify: `web/src/components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue`（genesis 首载动画 + OrbitControls start → user-interrupt）
- Test: `web/src/components/knowledge/graph3d/render/__tests__/NodeLayer.spec.ts`（新建或追加）

**范围纪律**：Canvas 已有 zoomToFit/聚焦 tween（第 157-165 行）。M3 落地两件事：① 创世动画（首载/代际变化时 uRevealT 0→1，节点从核心绽放，1.2s）；② OrbitControls `start` 事件 → 立即完成动画（镜头让位）。flying/orbiting 复用既有 tween + CameraDirector 位姿输出的完整接线**降级为后续可选增强**（YAGNI：创世是视觉主菜，飞入已有 tween 覆盖）——CameraDirector 状态机保留供聚焦链路后续接入。LOW 画质档跳过 genesis。

- [ ] **Step 1: NodeLayer reveal 测试先行**

```typescript
// NodeLayer.spec.ts（无 WebGL 上下文，仅测 uniform 接线）
import { describe, expect, it } from 'vitest';
import { NodeLayer } from '../NodeLayer';

describe('NodeLayer（M3 reveal）', () => {
  it('uRevealT 默认 1（无动画时全显现）', () => {
    const layer = new NodeLayer(4);
    expect((layer.points.material as { uniforms: { uRevealT: { value: number } } }).uniforms.uRevealT.value).toBe(1);
    layer.dispose();
  });

  it('setRevealT 更新 uRevealT uniform', () => {
    const layer = new NodeLayer(4);
    layer.setRevealT(0.3);
    expect((layer.points.material as { uniforms: { uRevealT: { value: number } } }).uniforms.uRevealT.value).toBe(0.3);
    layer.dispose();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/components/knowledge/graph3d/render/__tests__/NodeLayer.spec.ts`
Expected: FAIL（`setRevealT is not a function`）

- [ ] **Step 3: 实现**

NodeLayer.ts：
- uniforms 加 `uRevealT: { value: 1 }`
- 顶点着色器 `void main()` 开头加 `uniform float uRevealT;` 声明（uniform 声明在函数外），`gl_PointSize` 赋值后追加：`gl_PointSize *= (0.2 + 0.8 * uRevealT);`，`vFade` 赋值后追加：`vFade *= uRevealT;`
- 新增方法：

```typescript
  /** M3 创世绽放：0=收拢于核心，1=完全显现（默认 1 无动画）。 */
  setRevealT(t: number): void {
    this.material.uniforms.uRevealT.value = t;
  }
```

KnowledgeGraph3DCanvas.vue：
- 局部状态加（非响应式）：`let revealT = 1; let genesisStart = 0;`
- `rebuildGraph()` 尾部（`engine.start()` 与 `requestRender()` 之间）：

```typescript
  // M3 创世绽放：LOW 档跳过（uniform 直接 1）
  if (QUALITY_SPECS[tier].label !== 'LOW') {
    revealT = 0;
    genesisStart = performance.now();
    nodeLayer?.setRevealT(0);
  } else {
    revealT = 1;
    nodeLayer?.setRevealT(1);
  }
```

- RAF 渲染循环内（`handleTick`/渲染驱动处，保证 revealT<1 时持续 requestRender）：

```typescript
  if (revealT < 1) {
    revealT = Math.min(1, (performance.now() - genesisStart) / 1200);
    nodeLayer?.setRevealT(revealT);
    requestRender();
  }
```

- `initScene` 中 controls 创建后（第 204 行 `addEventListener('change', ...)` 旁）：

```typescript
  controls.addEventListener('start', () => {
    if (revealT < 1) {
      revealT = 1;
      nodeLayer?.setRevealT(1);
    }
  });
```

- `disposeGraph()` 中重置：`revealT = 1;`

- [ ] **Step 4: 测试 + 门禁 + 运行时验证**

Run: `cd web && pnpm vitest run src/components/knowledge/graph3d/render/__tests__/NodeLayer.spec.ts && pnpm lint && pnpm test && pnpm build`
Expected: 全绿

运行时：打开知识库图谱（首载/切库）→ 节点从核心 1.2s 绽放（HIGH/MID 档）；拖拽立刻接管；LOW 档直接全显；FPS 无异常掉档；切到 LOW 档再切回无残留动画状态。

- [ ] **Step 5: Commit**

```powershell
git add web/src/components/knowledge/graph3d/render/NodeLayer.ts web/src/components/knowledge/graph3d/render/__tests__/NodeLayer.spec.ts web/src/components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue
git commit -m "feat(knowledge): M3 创世绽放动画（uRevealT）+ 用户交互让位"
```

---

# 轨道 A — M4：聚焦模式 + 节点卡

## Task M4-T1: nHop BFS + 聚焦锁定状态（interaction.ts 扩展）

**Files:**
- Modify: `web/src/features/knowledge/graph3d/interaction.ts`（新增 `nHop` BFS + 聚焦锁定状态管理）
- Test: `web/src/features/knowledge/graph3d/__tests__/interaction.spec.ts`（追加 describe）

**设计**：现有 `oneHop` 返回一跳邻居集驱动 hover 高亮。M4 聚焦模式：点击节点 → BFS N 跳集合（默认 2 跳）→ 复用 `NodeLayer.setHighlight`（集内 1.6 / 集外 0.15 dim）→ 锁定（hover 不覆盖，点击空白解除）。状态优先级：**聚焦锁定 > hover 高亮 > 默认**。

- [ ] **Step 1: Write the failing test**

```typescript
// interaction.spec.ts 追加
  describe('M4 聚焦模式', () => {
    // 图：0-1-2-3 链 + 0-4 支链
    const edges = new Int32Array([0, 1, 1, 2, 2, 3, 0, 4]);
    const edgeCount = 4;

    it('nHop(root, 1) = 一跳邻居（与 oneHop 一致）', () => {
      const { nodes } = nHop(edges, edgeCount, 0, 1);
      expect([...nodes].sort()).toEqual([0, 1, 4]);
    });

    it('nHop(root, 2) = 二跳邻居（含 2）', () => {
      const { nodes } = nHop(edges, edgeCount, 0, 2);
      expect([...nodes].sort()).toEqual([0, 1, 2, 4]);
    });

    it('nHop(root, 0) = 仅根节点', () => {
      const { nodes } = nHop(edges, edgeCount, 0, 0);
      expect([...nodes]).toEqual([0]);
    });

    it('nHop 边集 = 两端点都在节点集内的边', () => {
      const { edges: edgeSet } = nHop(edges, edgeCount, 0, 1);
      expect(edgeSet.has(0)).toBe(true);  // 0-1
      expect(edgeSet.has(3)).toBe(true);  // 0-4
      expect(edgeSet.has(1)).toBe(false); // 1-2 出圈
    });

    it('GraphInteraction 聚焦锁定：focus 后 hover 不覆盖；clearFocus 恢复 hover 驱动', () => {
      const gi = new GraphInteraction();
      gi.setFocus(0, 2);
      expect(gi.focused).toBe(0);
      expect(gi.focusHops).toBe(2);
      gi.setHover(1);
      expect(gi.focused).toBe(0); // hover 不覆盖锁定
      gi.clearFocus();
      expect(gi.focused).toBeNull();
    });
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/knowledge/graph3d/__tests__/interaction.spec.ts`
Expected: FAIL（`nHop is not exported` / `gi.setFocus is not a function`）

- [ ] **Step 3: Write minimal implementation**

`interaction.ts` 追加：

```typescript
/** M4：BFS N 跳邻居集（聚焦模式）。hops=0 仅根；边集 = 两端点都在节点集内的边。 */
export function nHop(
  edges: Int32Array,
  edgeCount: number,
  root: number,
  hops: number,
): { nodes: Set<number>; edges: Set<number> } {
  const nodes = new Set<number>([root]);
  if (hops > 0) {
    let frontier = [root];
    for (let h = 0; h < hops; h++) {
      const next: number[] = [];
      for (let e = 0; e < edgeCount; e++) {
        const a = edges[e * 2];
        const b = edges[e * 2 + 1];
        if (frontier.includes(a) && !nodes.has(b)) {
          nodes.add(b);
          next.push(b);
        } else if (frontier.includes(b) && !nodes.has(a)) {
          nodes.add(a);
          next.push(a);
        }
      }
      frontier = next;
      if (frontier.length === 0) break;
    }
  }
  const edgeSet = new Set<number>();
  for (let e = 0; e < edgeCount; e++) {
    if (nodes.has(edges[e * 2]) && nodes.has(edges[e * 2 + 1])) edgeSet.add(e);
  }
  return { nodes, edges: edgeSet };
}
```

`GraphInteraction` 类追加聚焦锁定（字段 + 方法）：

```typescript
  /** M4 聚焦锁定：非 null 时 active 由 focused 驱动（hover 不覆盖）。 */
  private _focused: number | null = null;
  private _focusHops = 2;

  get focused(): number | null {
    return this._focused;
  }

  get focusHops(): number {
    return this._focusHops;
  }

  setFocus(index: number, hops: number): void {
    this._focused = index;
    this._focusHops = hops;
  }

  clearFocus(): void {
    this._focused = null;
  }
```

并调整 `active` getter（既有）：`active = focused ?? hover ?? selected`（聚焦锁定优先级最高——具体以既有 active 语义为准微调，保证 hover 高亮在聚焦时不覆盖 dim 集）。

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/features/knowledge/graph3d/__tests__/interaction.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```powershell
git add web/src/features/knowledge/graph3d/interaction.ts web/src/features/knowledge/graph3d/__tests__/interaction.spec.ts
git commit -m "feat(knowledge): M4 nHop BFS + 聚焦锁定状态（优先级高于 hover）"
```

---

## Task M4-T2: FocusCard.vue 节点信息卡（真折射玻璃 + B1 入口②插槽）

**Files:**
- Create: `web/src/components/knowledge/graph3d/FocusCard.vue`
- Test: `web/src/components/knowledge/graph3d/__tests__/FocusCard.spec.ts`（组件挂载测试）

**设计**：复用 M1 `GlassPanel refract`；内容 = 文档标题 + doc_type 色点 + 度数 + rel_path；操作区 =「在编辑器打开」（既有 open-in-explorer 链路）+「重新向量化」（B1 入口②，emit 事件由 KnowledgePage 接线调 B1 RPC，M4 阶段先 emit 占位）。可拖动（pointer 拖拽）+ 可收起。

- [ ] **Step 1: Write the failing test**

```typescript
// FocusCard.spec.ts
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import FocusCard from '../FocusCard.vue';

const node = {
  docId: 'd1',
  name: '架构设计',
  relPath: 'docs/arch.md',
  docType: 'note',
  degree: 7,
};

describe('FocusCard（M4）', () => {
  it('渲染节点信息：标题/doc_type/度数/路径', () => {
    const w = mount(FocusCard, { props: { node, canReembed: true } });
    expect(w.text()).toContain('架构设计');
    expect(w.text()).toContain('docs/arch.md');
    expect(w.text()).toContain('7');
  });

  it('「在编辑器打开」emit open-in-explorer', async () => {
    const w = mount(FocusCard, { props: { node, canReembed: true } });
    await w.find('[data-test="focus-open"]').trigger('click');
    expect(w.emitted('open-in-explorer')).toEqual([[{ docId: 'd1', relPath: 'docs/arch.md' }]]);
  });

  it('「重新向量化」emit reembed（B1 入口②）；canReembed=false 时禁用', async () => {
    const w = mount(FocusCard, { props: { node, canReembed: true } });
    await w.find('[data-test="focus-reembed"]').trigger('click');
    expect(w.emitted('reembed')).toEqual([['d1']]);
    const w2 = mount(FocusCard, { props: { node, canReembed: false } });
    expect(w2.find('[data-test="focus-reembed"]').attributes('disabled')).toBeDefined();
  });

  it('收起态只显示标题条', async () => {
    const w = mount(FocusCard, { props: { node, canReembed: true } });
    await w.find('[data-test="focus-collapse"]').trigger('click');
    expect(w.find('[data-test="focus-body"]').exists()).toBe(false);
    expect(w.text()).toContain('架构设计');
  });

  it('关闭 emit close', async () => {
    const w = mount(FocusCard, { props: { node, canReembed: true } });
    await w.find('[data-test="focus-close"]').trigger('click');
    expect(w.emitted('close')).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/components/knowledge/graph3d/__tests__/FocusCard.spec.ts`
Expected: FAIL（模块不存在）

- [ ] **Step 3: Write minimal implementation**

```vue
<!-- web/src/components/knowledge/graph3d/FocusCard.vue -->
<template>
  <!-- M4 聚焦节点信息卡：真折射玻璃（M1），画布右侧浮层，可拖动/可收起。
       操作：在编辑器打开（open-in-explorer 既有链路）+ 重新向量化（B1 入口②）。 -->
  <div class="kg3d-focus-card" :style="{ left: `${pos.x}px`, top: `${pos.y}px` }">
    <GlassPanel strong refract :title="collapsed ? node.name : t('knowledgePage.graphFocusCardTitle')" icon="my_location">
      <template #header-actions>
        <q-btn flat dense round size="xs" :icon="collapsed ? 'expand_more' : 'expand_less'" data-test="focus-collapse" @click="collapsed = !collapsed" />
        <q-btn flat dense round size="xs" icon="close" data-test="focus-close" @click="$emit('close')" />
      </template>
      <div v-if="!collapsed" data-test="focus-body" class="kg3d-focus-card__body">
        <div class="kg3d-focus-card__name">{{ node.name }}</div>
        <div class="kg3d-focus-card__meta">
          <span class="kg3d-focus-card__type">{{ node.docType }}</span>
          <span class="kg3d-focus-card__degree">{{ t('knowledgePage.graphFocusDegree', { n: node.degree }) }}</span>
        </div>
        <div class="kg3d-focus-card__path">{{ node.relPath }}</div>
        <div class="kg3d-focus-card__actions">
          <q-btn dense outline size="sm" icon="open_in_new" :label="t('knowledgePage.graphOpenInEditor')" data-test="focus-open"
                 @click="$emit('open-in-explorer', { docId: node.docId, relPath: node.relPath })" />
          <q-btn dense outline size="sm" icon="psychology" :label="t('knowledgePage.graphReembed')" data-test="focus-reembed"
                 :disable="!canReembed" @click="$emit('reembed', node.docId)" />
        </div>
      </div>
    </GlassPanel>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import GlassPanel from '../effects/GlassPanel.vue';

export interface FocusCardNode {
  docId: string;
  name: string;
  relPath: string;
  docType: string;
  degree: number;
}

defineProps<{
  node: FocusCardNode;
  /** B1：所属集合有语义层时可重新向量化。 */
  canReembed: boolean;
}>();

defineEmits<{
  'open-in-explorer': [payload: { docId: string; relPath: string }];
  reembed: [docId: string];
  close: [];
}>();

const { t } = useI18n();
const collapsed = ref(false);
/** 画布右侧浮层初始位（拖动由 pointer 事件更新，M4-T3 接线拖动）。 */
const pos = reactive({ x: 16, y: 76 });
</script>

<style lang="sass" scoped>
.kg3d-focus-card
  position: absolute
  right: 16px
  left: auto !important
  z-index: 5
  width: 280px

  &__name
    font-size: 14px
    font-weight: 600
    margin-bottom: 6px

  &__meta
    display: flex
    gap: 8px
    font-size: 11px
    color: var(--kb-text-dim)
    margin-bottom: 4px

  &__path
    font-size: 11px
    color: var(--kb-text-dim)
    opacity: 0.7
    margin-bottom: 10px
    word-break: break-all

  &__actions
    display: flex
    gap: 8px
</style>
```

i18n 文案（zh-Hans / en-US）：`graphFocusCardTitle: '聚焦节点'` / `Focus node`；`graphFocusDegree: '度数 {n}'` / `Degree {n}`；`graphOpenInEditor: '在编辑器打开'` / `Open in editor`；`graphReembed: '重新向量化'` / `Re-embed`。

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/components/knowledge/graph3d/__tests__/FocusCard.spec.ts`
Expected: PASS（5 tests）

- [ ] **Step 5: Commit**

```powershell
git add web/src/components/knowledge/graph3d/FocusCard.vue web/src/components/knowledge/graph3d/__tests__/FocusCard.spec.ts web/src/i18n/
git commit -m "feat(knowledge): M4 FocusCard 节点信息卡（真折射 + B1 入口②插槽）"
```

---

## Task M4-T3: Canvas 聚焦集成（单击锁定 dim + 空白解除 + FocusCard 挂载）+ 运行时验证

**Files:**
- Modify: `web/src/components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue`（applyHighlight 聚焦分支 + 点击锁定/解除 + emits）
- Modify: `web/src/components/knowledge/KnowledgeGraph3D.vue`（FocusCard 挂载 + 事件透传）
- Modify: `web/src/pages/KnowledgePage.vue`（reembed 事件接线 → B1 RPC 调用占位，B1-T4 完成后接通）

**设计**：
- `applyHighlight()` 扩展聚焦分支：`interaction.focused !== null` → `nHop(model.edges, model.edgeCount, focused, focusHops)` 驱动 NodeLayer/EdgeLayer setHighlight（锁定态 hover 不覆盖）
- 单击节点（既有 node-click）→ `interaction.setFocus(index, 2)`；单击空白（既有 background-click）→ `interaction.clearFocus()`
- Canvas emit 新增 `focus-change: [docId: string]`（聚焦锁定/解除时抛出，''=解除），KnowledgeGraph3D 据此挂载/卸载 FocusCard
- FocusCard 的 `canReembed`：由 KnowledgePage 传入（当前集合 `embedding_model !== ''`，B1-T4 后接通真实 RPC）

- [ ] **Step 1: Canvas 集成**

```typescript
// applyHighlight() 改造：聚焦分支优先（现第 367-393 行函数内开头插入）
function applyHighlight(): void {
  if (!model || !nodeLayer || !edgeLayer) return;
  // M4 聚焦锁定：BFS N 跳 dim（优先级高于 hover）
  const focused = interaction.focused;
  if (focused !== null) {
    const { nodes, edges } = nHop(model.edges, model.edgeCount, focused, interaction.focusHops);
    nodeLayer.setHighlight(nodes);
    edgeLayer.setHighlight(edges);
    particleLayer.setSource(null, []);
    if (reticle) {
      const sel = interaction.selected;
      reticle.setHover(null, 0);
      reticle.setSelected(sel, sel !== null ? nodeLayer.nodeSize(sel) : 0);
    }
    labelVis.extraVisible = new Set([focused]);
    requestRender();
    return;
  }
  // ……既有 hover/selected 分支保持不变
}
```

```typescript
// import 增加 nHop
import { GraphInteraction, isClickMovement, nHop, oneHop, wheelZoomFactor } from '../../../features/knowledge/graph3d/interaction';

// emits 增加
  /** M4：聚焦锁定变化（''=解除）。 */
  'focus-change': [docId: string];

// 单击节点处理处（既有 node-click emit 旁）：interaction.setFocus(index, 2) + emit('focus-change', model.docIds[index])
// 单击空白处理处（既有 background-click emit 旁）：interaction.clearFocus() + emit('focus-change', '')
```

- [ ] **Step 2: KnowledgeGraph3D 挂载 FocusCard**

```vue
<!-- 画布容器内（KnowledgeGraph3DCanvas 同级浮层） -->
<FocusCard
  v-if="focusedNode"
  :node="focusedNode"
  :can-reembed="canReembed"
  @open-in-explorer="(p) => $emit('open-in-explorer', p)"
  @reembed="(docId) => $emit('reembed-node', docId)"
  @close="onFocusClose"
/>
```

props 加 `focused-doc-id: string`、`can-reembed: boolean`；emits 加 `'reembed-node': [docId: string]`；`focusedNode` computed 由 nodes + focusedDocId + degree（从 edges 统计或 Canvas 透传）合成；`onFocusClose` 转发 background-click 语义（通知 Canvas clearFocus——通过 `select-node ''` 既有事件即可，因为 background-click 已接 clearFocus）。

简化：`focus-change` 在 KnowledgeGraph3D 监听并更新局部 `focusedDocId`；关闭卡片 = 调 Canvas 的 clearFocus——经由 `$emit('select-node', '')` 触发 KnowledgePage 清除选中，同时 KnowledgeGraph3D 内部将 focusedDocId 置 ''。**注意保持与既有 select-node 链路兼容**：focusedDocId 是独立局部状态，不与 selectedNodeId 耦合。

- [ ] **Step 3: 门禁 + 运行时验证（R3）**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: 全绿

运行时：单击节点 → 2 跳内节点高亮、其余压暗 + FocusCard 出现（标题/度数/路径正确）；hover 其他节点 dim 集不变（锁定）；单击空白 → dim 解除 + 卡片关闭；「在编辑器打开」跳转工作台正确 tab；「重新向量化」按钮在无语义层集合禁用。

- [ ] **Step 4: Commit**

```powershell
git add web/src/components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue web/src/components/knowledge/KnowledgeGraph3D.vue web/src/pages/KnowledgePage.vue
git commit -m "feat(knowledge): M4 聚焦锁定 dim + FocusCard 画布集成"
```

---

# 轨道 A — M5：过滤图例 + 透镜

## Task M5-T1: model.ts 过滤管线（filterGraphByGroups）

**Files:**
- Modify: `web/src/features/knowledge/graph3d/model.ts`（新增 `filterGraphByGroups` 纯函数）
- Test: `web/src/features/knowledge/graph3d/__tests__/model.spec.ts`（追加 describe）

**设计**：`hiddenGroups: ReadonlySet<string>`（doc_type 集合）→ 过滤 nodes（doc_type 命中即排除）→ 边级联（端点被排除则边排除）→ 输出过滤后的 `GraphNodeInput[]/GraphEdgeInput[]`（在 `buildGraphModel` 之前过滤，引擎零感知）。

- [ ] **Step 1: Write the failing test**

```typescript
// model.spec.ts 追加
  describe('M5 filterGraphByGroups', () => {
    const nodes = [
      { docId: 'a', name: 'a', relPath: 'a.md', docType: 'note' },
      { docId: 'b', name: 'b', relPath: 'b.md', docType: 'note' },
      { docId: 'c', name: 'c', relPath: 'c.md', docType: 'image' },
    ];
    const edges = [
      { source: 'a', target: 'b', type: 'explicit' },
      { source: 'b', target: 'c', type: 'semantic' },
    ];

    it('空 hiddenGroups：原样返回（引用相等，零开销）', () => {
      const out = filterGraphByGroups(nodes, edges, new Set());
      expect(out.nodes).toBe(nodes);
      expect(out.edges).toBe(edges);
    });

    it('隐藏 image 组：节点 c 排除 + 边 b-c 级联排除', () => {
      const out = filterGraphByGroups(nodes, edges, new Set(['image']));
      expect(out.nodes.map((n) => n.docId)).toEqual(['a', 'b']);
      expect(out.edges).toEqual([{ source: 'a', target: 'b', type: 'explicit' }]);
    });

    it('隐藏全部组：空图', () => {
      const out = filterGraphByGroups(nodes, edges, new Set(['note', 'image']));
      expect(out.nodes).toEqual([]);
      expect(out.edges).toEqual([]);
    });
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/knowledge/graph3d/__tests__/model.spec.ts`
Expected: FAIL（`filterGraphByGroups is not exported`）

- [ ] **Step 3: Write minimal implementation**

```typescript
/** M5：按 doc_type 组过滤（隐藏组节点排除 + 边级联排除）。空集合零开销原样返回。 */
export function filterGraphByGroups(
  nodes: GraphNodeInput[],
  edges: GraphEdgeInput[],
  hiddenGroups: ReadonlySet<string>,
): { nodes: GraphNodeInput[]; edges: GraphEdgeInput[] } {
  if (hiddenGroups.size === 0) return { nodes, edges };
  const kept = nodes.filter((n) => !hiddenGroups.has(n.docType));
  const keptIds = new Set(kept.map((n) => n.docId));
  const keptEdges = edges.filter((e) => keptIds.has(e.source) && keptIds.has(e.target));
  return { nodes: kept, edges: keptEdges };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/features/knowledge/graph3d/__tests__/model.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```powershell
git add web/src/features/knowledge/graph3d/model.ts web/src/features/knowledge/graph3d/__tests__/model.spec.ts
git commit -m "feat(knowledge): M5 filterGraphByGroups 过滤管线（边级联排除）"
```

---

## Task M5-T2: GraphLegend.vue 过滤图例 + 透镜悬停

**Files:**
- Create: `web/src/components/knowledge/graph3d/GraphLegend.vue`
- Test: `web/src/components/knowledge/graph3d/__tests__/GraphLegend.spec.ts`

**设计**：图例列出全部 doc_type 组（颜色点 + 组名 + 节点计数）；**点击** = 切换隐藏（emit toggle-group）；**悬停** = 透镜（emit lens-enter/lens-leave，Canvas 临时 dim 其他组——复用 setHighlight：组内节点集 1.6 / 组外 0.15）。隐藏组行置灰 + 斜体。复用 M1 GlassPanel refract。

- [ ] **Step 1: Write the failing test**

```typescript
// GraphLegend.spec.ts
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import GraphLegend from '../GraphLegend.vue';

const groups = [
  { docType: 'note', color: '#4fc3f7', count: 12 },
  { docType: 'image', color: '#ba68c8', count: 3 },
];

describe('GraphLegend（M5）', () => {
  it('渲染组行：颜色点 + 组名 + 计数', () => {
    const w = mount(GraphLegend, { props: { groups, hiddenGroups: [] } });
    expect(w.text()).toContain('note');
    expect(w.text()).toContain('12');
    expect(w.text()).toContain('image');
  });

  it('点击组行 emit toggle-group', async () => {
    const w = mount(GraphLegend, { props: { groups, hiddenGroups: [] } });
    await w.find('[data-test="legend-row-note"]').trigger('click');
    expect(w.emitted('toggle-group')).toEqual([['note']]);
  });

  it('隐藏组行带隐藏样式类', () => {
    const w = mount(GraphLegend, { props: { groups, hiddenGroups: ['image'] } });
    expect(w.find('[data-test="legend-row-image"]').classes()).toContain('kg3d-legend__row--hidden');
  });

  it('悬停 emit lens-enter / lens-leave（透镜）', async () => {
    const w = mount(GraphLegend, { props: { groups, hiddenGroups: [] } });
    const row = w.find('[data-test="legend-row-note"]');
    await row.trigger('pointerenter');
    expect(w.emitted('lens-enter')).toEqual([['note']]);
    await row.trigger('pointerleave');
    expect(w.emitted('lens-leave')).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/components/knowledge/graph3d/__tests__/GraphLegend.spec.ts`
Expected: FAIL（模块不存在）

- [ ] **Step 3: Write minimal implementation**

```vue
<!-- web/src/components/knowledge/graph3d/GraphLegend.vue -->
<template>
  <!-- M5 过滤图例：点击切换组隐藏；悬停透镜（临时 dim 其他组）。真折射玻璃（M1）。 -->
  <div class="kg3d-legend">
    <GlassPanel strong refract :title="t('knowledgePage.graphLegendTitle')" icon="filter_list">
      <div
        v-for="g in groups"
        :key="g.docType"
        class="kg3d-legend__row"
        :class="{ 'kg3d-legend__row--hidden': hiddenGroups.includes(g.docType) }"
        :data-test="`legend-row-${g.docType}`"
        role="button"
        tabindex="0"
        @click="$emit('toggle-group', g.docType)"
        @keydown.enter="$emit('toggle-group', g.docType)"
        @pointerenter="$emit('lens-enter', g.docType)"
        @pointerleave="$emit('lens-leave')"
      >
        <span class="kg3d-legend__dot" :style="{ background: g.color }" />
        <span class="kg3d-legend__name">{{ g.docType }}</span>
        <span class="kg3d-legend__count">{{ g.count }}</span>
      </div>
      <div v-if="!groups.length" class="kg3d-legend__empty">{{ t('knowledgePage.graphLegendEmpty') }}</div>
    </GlassPanel>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import GlassPanel from '../effects/GlassPanel.vue';

export interface LegendGroup {
  docType: string;
  color: string;
  count: number;
}

defineProps<{
  groups: LegendGroup[];
  /** 已隐藏的 doc_type 列表。 */
  hiddenGroups: string[];
}>();

defineEmits<{
  'toggle-group': [docType: string];
  'lens-enter': [docType: string];
  'lens-leave': [];
}>();

const { t } = useI18n();
</script>

<style lang="sass" scoped>
.kg3d-legend
  position: absolute
  left: 16px
  bottom: 16px
  z-index: 5
  min-width: 160px

  &__row
    display: flex
    align-items: center
    gap: 8px
    padding: 4px 6px
    border-radius: 6px
    cursor: pointer
    font-size: 12px
    &:hover
      background: rgba(255, 255, 255, 0.06)
    &--hidden
      opacity: 0.4
      font-style: italic

  &__dot
    width: 8px
    height: 8px
    border-radius: 50%
    flex: none

  &__name
    flex: 1

  &__count
    color: var(--kb-text-dim)
    font-size: 11px

  &__empty
    font-size: 12px
    color: var(--kb-text-dim)
</style>
```

i18n：`graphLegendTitle: '图例'` / `Legend`；`graphLegendEmpty: '暂无分组'` / `No groups`。

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/components/knowledge/graph3d/__tests__/GraphLegend.spec.ts`
Expected: PASS（4 tests）

- [ ] **Step 5: Commit**

```powershell
git add web/src/components/knowledge/graph3d/GraphLegend.vue web/src/components/knowledge/graph3d/__tests__/GraphLegend.spec.ts web/src/i18n/
git commit -m "feat(knowledge): M5 GraphLegend 过滤图例（点击隐藏 + 悬停透镜）"
```

---

## Task M5-T3: 集成（KnowledgePage 过滤状态 + Canvas 透镜 dim + localStorage）+ 运行时验证

**Files:**
- Modify: `web/src/pages/KnowledgePage.vue`（hiddenGroups 状态 + localStorage + 过滤管线接线）
- Modify: `web/src/components/knowledge/KnowledgeGraph3D.vue`（GraphLegend 挂载 + lens 事件透传 + groups 统计）
- Modify: `web/src/components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue`（lens dim：按 doc_type 组集合 setHighlight）

**设计**：
- KnowledgePage：`graphHiddenGroups = ref<string[]>(JSON.parse(localStorage['kg3d-hidden-groups'] ?? '[]'))`；`graphRenderNodes/Edges` computed 源头经 `filterGraphByGroups`（在 `graphViewGraph` 之后套一层）
- groups 统计：从 `graphAllNodes` 按 doc_type 聚合计数 + 颜色（`buildGroupPalette` 同款色序——复用 palette 模块按排序组名取色，保证与节点色一致）
- 透镜：`lens-enter(docType)` → Canvas 收到组名 → 该组节点索引集 setHighlight（1.6/0.15 dim）；`lens-leave` → 恢复（回到聚焦/hover 驱动）
- **互斥纪律**：透镜激活时清除 hover（`interaction.setHover(null)`），透镜优先于 hover 但低于聚焦锁定（聚焦锁定时透镜不生效）

- [ ] **Step 1: Canvas lens 接口**

```typescript
// Canvas expose 或 emits 方案：KnowledgeGraph3D 持有 canvas ref，调 expose 的 setLens(docType|null)
/** M5 透镜：按 doc_type 组临时 dim（null 解除）。聚焦锁定时忽略。 */
function setLens(docType: string | null): void {
  if (!model || !nodeLayer || !edgeLayer) return;
  if (interaction.focused !== null) return; // 聚焦锁定优先
  if (docType === null) {
    applyHighlight(); // 恢复 hover/selected 驱动
    return;
  }
  interaction.setHover(null);
  const gid = model.groups.indexOf(docType);
  if (gid < 0) return;
  const nodes = new Set<number>();
  for (let i = 0; i < model.count; i++) if (model.groupId[i] === gid) nodes.add(i);
  const edges = new Set<number>();
  for (let e = 0; e < model.edgeCount; e++) {
    if (nodes.has(model.edges[e * 2]) && nodes.has(model.edges[e * 2 + 1])) edges.add(e);
  }
  nodeLayer.setHighlight(nodes);
  edgeLayer.setHighlight(edges);
  requestRender();
}
defineExpose({ setLens });
```

- [ ] **Step 2: KnowledgeGraph3D 挂载 + KnowledgePage 接线**

KnowledgeGraph3D.vue：props 加 `hidden-groups: string[]`；emits 加 `'toggle-group': [docType: string]`；groups computed（从 props.nodes 聚合 + palette 取色）；GraphLegend 挂载（HUD 区域左下）；lens-enter/leave → canvas ref 调 setLens。

KnowledgePage.vue：

```typescript
const graphHiddenGroups = ref<string[]>(readJsonArray('kg3d-hidden-groups'));
watch(graphHiddenGroups, (v) => {
  try { localStorage.setItem('kg3d-hidden-groups', JSON.stringify(v)); } catch { /* 隐私模式忽略 */ }
});
function onGraphToggleGroup(docType: string) {
  const i = graphHiddenGroups.value.indexOf(docType);
  if (i >= 0) graphHiddenGroups.value.splice(i, 1);
  else graphHiddenGroups.value.push(docType);
}
// 过滤管线：graphRenderNodes/Edges 源头加 filterGraphByGroups
const graphFiltered = computed(() =>
  filterGraphByGroups(graphViewGraph.value.nodes, graphViewGraph.value.edges, new Set(graphHiddenGroups.value)),
);
const graphRenderNodes = computed(() => graphFiltered.value.nodes);
const graphRenderEdges = computed(() => graphFiltered.value.edges);
```

- [ ] **Step 3: 门禁 + 运行时验证（R3）**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: 全绿

运行时：图例显示全部 doc_type 组与计数（与图谱节点色一致）；点击隐藏组 → 节点+边从图谱消失、图例行置灰；悬停组 → 该组高亮其余压暗（透镜），移开恢复；聚焦锁定节点后悬停图例不生效（互斥）；刷新页面隐藏状态保持。

- [ ] **Step 4: Commit**

```powershell
git add web/src/pages/KnowledgePage.vue web/src/components/knowledge/KnowledgeGraph3D.vue web/src/components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue
git commit -m "feat(knowledge): M5 过滤图例集成 + 透镜 dim + 隐藏状态持久化"
```

---

# 轨道 B — B1：文档重新 embedding（能力缺口①）

> 设计依据：spec §8。核心事实：UI 上传文档 `content_text` 已存 DB，重嵌入**无需原始文件**——删旧 chunks → `content_text` 重新分块 → EmbedBatch → 插入新 chunks，复用 `knowledge.BuildIndexedChunks`（[knowledge.go:579](file:///f:/aranea-agents/internal/service/knowledge.go#L579) 同款调用）与 `DeleteChunksByDocument`（vault sync 既有接口）。

## Task B1-T1: Proto ReembedDocuments 定义 + make api

**Files:**
- Modify: `api/kratos/knowledge/v1/knowledge.proto`（message 插到 `DeleteDocumentRequest`（:409-411）之后；rpc 插到 `MoveDocumentToDir` rpc（:553 区域）之后）
- Modify: `api/kratos/knowledge/v1/*.pb.go`（生成物，`make api` 产出）

- [x] **Step 1: Proto 定义**

```proto
// ReembedDocuments re-chunks and re-embeds documents from their stored
// content_text (B1: heals UI-uploaded docs whose embeddings were nulled by
// reconcileEmbeddingDim; vault docs self-heal via vault_sync and are skipped).
rpc ReembedDocuments(ReembedDocumentsRequest) returns (ReembedDocumentsResponse) {
  option (google.api.http) = { post: "/v1/knowledge/collections/{collection_id}/documents:reembed", body: "*" };
}

message ReembedDocumentsRequest {
  string collection_id = 1 [(google.api.field_behavior) = REQUIRED];
  repeated string doc_ids = 2;  // 空 = 全集合待重嵌入文档（chunks embedding IS NULL 或无 chunks）
  int32 chunk_size = 3;
  int32 chunk_overlap = 4;
}

message ReembedDocumentsResponse {
  int32 accepted_count = 1;  // 已受理进入重嵌入队列的文档数
  int32 skipped_count = 2;   // 跳过数（content_text 空 / 正在 indexing / vault 文档走 sync 自愈）
}
```

- [x] **Step 2: 生成 + 构建验证**

Run: `make api && go build ./api/...`
Expected: 生成成功，零编译错误

- [x] **Step 3: Commit**

```powershell
git add api/kratos/knowledge/v1/
git commit -m "feat(knowledge): B1 ReembedDocuments proto 定义"
```

---

## Task B1-T2: data ListDocumentsPendingReembed + biz DocumentRepo/Usecase 扩展

**Files:**
- Modify: `internal/biz/knowledge/knowledge.go`（`DocumentRepo`（:131-148）加方法 + `Usecase` 透传，模式见 `UpdateDocumentStatus`（:460-464)）
- Modify: `internal/data/knowledge.go`（`ListDocuments`（:560）之后新增实现，复用 `scanDocumentSummary`）
- Test: `internal/data/knowledge_reembed_test.go`（PG 集成，`testhelper.SetupTestPG`，同款见 `knowledge_dim_reconcile_test.go`）

**背景**：DB-N3 接口 ≤5 方法属既有债务区（DB-DEBT-02/05，`DocumentRepo` 已 10+ 方法），本次随既有复合接口走（Wire 绑定用 `Repo` 复合接口），不新增拆分。

- [x] **Step 1: Write the failing test**

```go
// internal/data/knowledge_reembed_test.go
// TestKnowledgeRepo_ListDocumentsPendingReembed 覆盖筛选正确性：
// - 命中：chunks embedding IS NULL 的文档；无任何 chunks 但有 content_text 的文档
// - 排除：content_text='' 的文档；status='indexing' 的文档；embedding 非 NULL 的正常文档
func TestKnowledgeRepo_ListDocumentsPendingReembed(t *testing.T) {
	// SetupTestPG 建集合 + 4 篇文档（null-embedding / no-chunks / indexing / healthy）
	// 断言返回 id 集合 = {null-embedding, no-chunks}，按 created_at ASC
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/data/ -run TestKnowledgeRepo_ListDocumentsPendingReembed -count=1`
Expected: FAIL（`ListDocumentsPendingReembed` 未定义）

- [x] **Step 3: Write minimal implementation**

```go
// biz/knowledge/knowledge.go — DocumentRepo 接口加：
// ListDocumentsPendingReembed 列出待重嵌入文档（B1）：有正文、非 indexing、
// 且（chunks embedding IS NULL 或无任何 chunks）。按 created_at ASC（先入队先处理）。
ListDocumentsPendingReembed(ctx context.Context, collectionID string) ([]Document, error)

// Usecase 透传（knowledge.go:460 同款）：
func (u *Usecase) ListDocumentsPendingReembed(ctx context.Context, collectionID string) ([]Document, error) {
	if strings.TrimSpace(collectionID) == "" {
		return nil, ErrCollectionIDRequired
	}
	return u.documents.ListDocumentsPendingReembed(ctx, collectionID)
}
```

```go
// internal/data/knowledge.go — knowledgeRepo 实现（读路径 RW().Read 同款 Postgres() 读取，
// 错误处理遵循 ListDocuments 既有 raw-SQL 模式）：
func (r *knowledgeRepo) ListDocumentsPendingReembed(ctx context.Context, collectionID string) ([]biz.KnowledgeDocument, error) {
	q := `SELECT id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
		         organized, asset_uri,
		         rel_path, content_hash, summary, summary_hash, tags, doc_type,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_documents d
		  WHERE collection_id = $1
		    AND content_text <> ''
		    AND status <> 'indexing'
		    AND (id IN (SELECT doc_id FROM knowledge_chunks WHERE embedding IS NULL)
		         OR NOT EXISTS (SELECT 1 FROM knowledge_chunks c WHERE c.doc_id = d.id))
		  ORDER BY created_at ASC`
	// rows → scanDocumentSummary（复用 :580 既有扫描器）
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/data/ -run TestKnowledgeRepo_ListDocumentsPendingReembed -count=1`
Expected: PASS

- [x] **Step 5: Commit**

```powershell
git add internal/biz/knowledge/knowledge.go internal/data/knowledge.go internal/data/knowledge_reembed_test.go
git commit -m "feat(knowledge): B1 ListDocumentsPendingReembed 待重嵌入文档筛选（data+biz）"
```

---

## Task B1-T3: service knowledge_reembed.go（串行重嵌入管线 + flow log 登记）+ 单测

**Files:**
- Create: `internal/service/knowledge_reembed.go`
- Test: `internal/service/knowledge_reembed_test.go`
- Modify: `internal/event/flow_log.go`（`stepTitleRegistry`（:233-241 knowledge.* 区）登记 2 个 step）
- Modify: `docs/development/52-flow-logger.design.md` §5.1 步骤注册表（同步登记，红线）

**设计**（spec §8.2）：
- RPC 同步返回 accepted/skipped 计数（异步模式与 IngestDocument 一致）
- **单后台 goroutine 串行处理**（不打爆 embedder API）：per doc `DeleteChunksByDocument` → `UpdateDocumentStatus("indexing")` + `publishKnowledgeIngest` WS → `BuildIndexedChunks(content_text)` → `InsertChunks` → `UpdateDocumentStatus("indexed", chunk_count)` + WS；单文档失败置 error 继续下一篇
- **不触发** `RebuildBlockIndex`（content_text 未变，块/边不变——与 IngestDocument 的 SP1-C 钩子区分）
- K7 进程日志：goroutine 启动/每文档 done/退出/panic 各一条

- [x] **Step 1: Write the failing test**

```go
// internal/service/knowledge_reembed_test.go
// 构造模式参照 knowledge_us14_test.go：biz.NewKnowledgeUsecase(memRepo...) + &KnowledgeService{uc, embedder, lg: loggateway.NewNoop()}
// 异步管线用 require.Eventually 轮询 memRepo 中文档 status 落定为 indexed。

func TestReembedDocuments_LexicalCollectionRejected(t *testing.T) {
	// embedding_model='' 集合 → CodeBadRequest
}
func TestReembedDocuments_MutateAccessDenied(t *testing.T) {
	// 共享/跨租户集合 → 权限错误（assertCollectionMutateAccess 路径）
}
func TestReembedDocuments_ExplicitDocIdsSkipsRules(t *testing.T) {
	// 显式 doc_ids：content_text 空 / status=indexing 计 skipped；正常文档 accepted
}
func TestReembedDocuments_DefaultSelectsPending(t *testing.T) {
	// doc_ids 空 → 走 ListDocumentsPendingReembed；accepted = 待重嵌入数
}
func TestReembedDocuments_PipelineReembedsFromContentText(t *testing.T) {
	// stub embedder（EmbedBatch 返固定向量）；Eventually 断言：
	// DeleteChunksByDocument 被调 → status 终态 indexed + chunk_count>0
	// 且 RebuildBlockIndex 未被调用（spy 断言）
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestReembedDocuments -count=1`
Expected: FAIL（`ReembedDocuments` 未实现）

- [x] **Step 3: Write minimal implementation**

`internal/event/flow_log.go` stepTitleRegistry 登记（紧随 `knowledge.ingest.*` 区）：

```go
"knowledge.reembed.start":  "文档重嵌入开始",
"knowledge.reembed.done":   "文档重嵌入完成",
```

`internal/service/knowledge_reembed.go` 骨架（关键调用锚点，完整错误处理同 IngestDocument 模式）：

```go
package service

// ReembedDocuments B1：从已存 content_text 重建 chunks+embedding（无需原始文件）。
// 同步返回受理计数；重嵌入在单后台 goroutine 串行执行（复用摄取管线路径）。
func (s *KnowledgeService) ReembedDocuments(ctx context.Context, req *v1.ReembedDocumentsRequest) (*v1.ReembedDocumentsResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	// 词法库（无 embedding_model）：重嵌入无语义层无意义。
	if strings.TrimSpace(col.EmbeddingModel) == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "collection has no semantic layer (embedding_model is empty)")
	}
	if s.embedder == nil {
		return nil, apierror.BadRequest("KNOWLEDGE", "embedder not configured")
	}

	// 筛选目标：显式 doc_ids（per-id GetDocument + 跳过规则）或默认全集合待重嵌入。
	var docs []biz.KnowledgeDocument
	skipped := 0
	if len(req.GetDocIds()) > 0 {
		for _, id := range req.GetDocIds() {
			d, getErr := s.uc.GetDocument(ctx, id)
			if getErr != nil || d.CollectionID != col.ID ||
				strings.TrimSpace(d.ContentText) == "" || d.Status == "indexing" {
				skipped++
				continue
			}
			docs = append(docs, d)
		}
	} else {
		pending, listErr := s.uc.ListDocumentsPendingReembed(ctx, col.ID)
		if listErr != nil {
			return nil, listErr
		}
		docs = pending
	}
	if len(docs) == 0 {
		return &v1.ReembedDocumentsResponse{AcceptedCount: 0, SkippedCount: int32(skipped)}, nil
	}

	flow := s.knowledgeFlow(ctx)
	flow.LogStart("knowledge.reembed.start", "文档重嵌入开始",
		event.P("collection_id", col.ID),
		event.P("doc_count", len(docs)))

	embedder := s.embedder
	uc := s.uc
	reembedCtx := appctx.Ctx()
	safego.Go(reembedCtx, "knowledge-reembed", func() {
		s.lg.Info("knowledge reembed worker started", // K7 启动
			loggateway.StepID("knowledge.reembed.worker_start"),
			loggateway.Str("collection_id", col.ID),
			loggateway.Int("doc_count", len(docs)))
		defer s.lg.Info("knowledge reembed worker exited", // K7 退出
			loggateway.StepID("knowledge.reembed.worker_exit"),
			loggateway.Str("collection_id", col.ID))
		for _, doc := range docs {
			s.reembedOneDocument(reembedCtx, uc, embedder, col, doc, req.GetChunkSize(), req.GetChunkOverlap(), flow)
		}
		flow.LogDone("knowledge.reembed.done", "文档重嵌入完成",
			event.P("collection_id", col.ID),
			event.P("doc_count", len(docs)))
	})
	return &v1.ReembedDocumentsResponse{AcceptedCount: int32(len(docs)), SkippedCount: int32(skipped)}, nil
}

// reembedOneDocument 单文档串行管线：DeleteChunksByDocument → indexing+WS →
// BuildIndexedChunks(content_text) → InsertChunks → indexed+WS。失败置 error 由调用方继续下一篇。
// 不触发 RebuildBlockIndex（content_text 未变，块/边不变）。
func (s *KnowledgeService) reembedOneDocument(ctx context.Context, uc *biz.KnowledgeUsecase, embedder knowledge.Embedder, col biz.KnowledgeCollection, doc biz.KnowledgeDocument, chunkSize, chunkOverlap int32, flow *event.TraceEmitter) {
	if err := uc.DeleteChunksByDocument(ctx, doc.ID); err != nil { /* Warn + 置 error + WS，return */ }
	if err := uc.UpdateDocumentStatus(ctx, doc.ID, "indexing", "", 0); err != nil { /* Error 日志 */ }
	s.publishKnowledgeIngest(col.ID, doc.ID, "indexing", "", 0)
	params := knowledge.IngestParams{
		DocID: doc.ID, CollectionID: col.ID, Text: doc.ContentText,
		ChunkSize: int(chunkSize), ChunkOverlap: int(chunkOverlap),
	}
	params.ApplyDefaults()
	bizChunks, err := knowledge.BuildIndexedChunks(ctx, embedder, params, flow)
	if err != nil { /* 置 error + WS + flow.LogError，return */ }
	if err := uc.InsertChunks(ctx, bizChunks); err != nil { /* 同上 */ }
	if err := uc.UpdateDocumentStatus(ctx, doc.ID, "indexed", "", len(bizChunks)); err != nil { /* Error 日志 + WS error */ }
	s.publishKnowledgeIngest(col.ID, doc.ID, "indexed", "", len(bizChunks))
	s.lg.Info("knowledge reembed document done", // K7 每文档 done
		loggateway.StepID("knowledge.reembed.doc_done"),
		loggateway.Str("doc_id", doc.ID),
		loggateway.Int("chunk_count", len(bizChunks)))
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/ -run TestReembedDocuments -count=1`
Expected: PASS（5 tests）

- [ ] **Step 5: 门禁 + Commit**

Run: `make build && go test ./internal/service/ ./internal/event/ -count=1`（干净 GOCACHE）
Expected: 全绿（已知环境受限失败 `TestModelCatalogService_SyncModelCatalog_*` 除外）

```powershell
git add internal/service/knowledge_reembed.go internal/service/knowledge_reembed_test.go internal/event/flow_log.go docs/development/52-flow-logger.design.md
git commit -m "feat(knowledge): B1 ReembedDocuments 串行重嵌入管线（复用摄取链路 + flow log 双轨）"
```

---

## Task B1-T4: 前端入口（api/store + WorkbenchSidebar 菜单 + FocusCard 接线）

**Files:**
- Modify: `web/src/features/knowledge/api.ts`（`deleteDocument`（:230）后新增 `reembedDocuments`）
- Modify: `web/src/stores/knowledge.ts`（`removeDocument` 旁新增 store action）
- Modify: `web/src/components/knowledge/workbench/WorkbenchSidebar.vue`（文件行菜单（:85-94 move/download/delete 区）加「重新向量化」项）
- Modify: `web/src/features/knowledge/useKnowledgePage.ts`（`onFileAction`（KnowledgePage.vue:274 路由至此）加 `reembed` 分支 + `confirmReembed` 处理器）
- Modify: `web/src/components/knowledge/KnowledgeGraph3D.vue`（FocusCard `@reembed` 透传为 `reembed` emit + `:can-reembed` 绑定集合语义层判定）
- Modify: `web/src/pages/KnowledgePage.vue`（`@reembed` handler → 同一 `confirmReembed`）
- Test: `web/src/components/knowledge/workbench/__tests__/WorkbenchSidebar.spec.ts`（新增或追加）
- Test: `web/src/stores/__tests__/knowledge.spec.ts`（新增或追加 reembedDocuments action）

**设计**（spec §8.3 落地修正）：spec 原述「文档面板批量操作栏」——SP2-8 工作台时代该 UI 已不存在，文档操作实际位于 WorkbenchSidebar 文件行右键菜单（move/download/delete 同模式）。**入口①修正为单文档菜单项**（与既有交互一致；批量留待后续多选能力）。入口②（FocusCard 按钮）已在 M4-T2 落组件、本任务接线。

- [ ] **Step 1: Write the failing test**

```typescript
// WorkbenchSidebar.spec.ts 追加：
it('文件行菜单含「重新向量化」项并发射 file-action reembed', async () => {
  // mount with 文件节点 → 打开菜单 → 点击 data-test="file-reembed"
  // 断言 emitted('file-action')[0] = ['reembed', node]
});

// knowledge.spec.ts 追加：
it('reembedDocuments action 调用 api 并返回受理计数', async () => {
  // mock api.reembedDocuments resolve { accepted_count: 1, skipped_count: 0 }
  // 断言 store.reembedDocuments('col-1', ['doc-1']) 返回计数且 api 入参正确
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/components/knowledge/workbench/__tests__/WorkbenchSidebar.spec.ts src/stores/__tests__/knowledge.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```typescript
// api.ts：
export async function reembedDocuments(
  collectionId: string,
  docIds?: string[],
  chunkSize?: number,
  chunkOverlap?: number,
): Promise<{ accepted_count: number; skipped_count: number }> {
  const { data } = await apiClient.post(`/v1/knowledge/collections/${collectionId}/documents:reembed`, {
    doc_ids: docIds ?? [],
    chunk_size: chunkSize ?? 0,
    chunk_overlap: chunkOverlap ?? 0,
  });
  return data;
}
```

```typescript
// useKnowledgePage.ts — onFileAction 增加分支 + 处理器（对话框在 T5 完善，本步直调）：
function confirmReembed(docIds: string[]) {
  if (!selectedId.value || !docIds.length) return;
  void knowledgeStore.reembedDocuments(selectedId.value, docIds).then((r) => {
    $q.notify({ type: 'positive', message: t('knowledgePage.reembedAccepted', { n: r.accepted_count }) });
    void loadDocuments(); // 状态经摄取 WS 实时刷新（零新订阅）
  }).catch((e) => $q.notify({ type: 'negative', message: friendlyError(e) }));
}
// onFileAction: } else if (action === 'reembed') confirmReembed([node.doc_id]);
```

```vue
<!-- WorkbenchSidebar.vue 文件菜单 download 与 delete 之间： -->
<q-item clickable data-test="file-reembed" @click="$emit('file-action', 'reembed', f)">
  <q-item-section avatar><q-icon name="psychology" size="18px" /></q-item-section>
  <q-item-section>{{ t('knowledgePage.reembedDocument') }}</q-item-section>
</q-item>
```

KnowledgeGraph3D.vue：FocusCard 挂载处加 `@reembed="(docId: string) => $emit('reembed', docId)"`，`:can-reembed="collectionHasSemantic"`（从 `collections` prop 按 `collection-id` 查 `embedding_model` 非空）；emits 加 `'reembed': [docId: string]`。KnowledgePage.vue：`<knowledge-graph-3-d ... @reembed="(docId) => confirmReembed([docId])" />`。

- [ ] **Step 4: Run test to verify it passes**

Run: 同 Step 2 命令
Expected: PASS

- [ ] **Step 5: Commit**

```powershell
git add web/src/features/knowledge/api.ts web/src/stores/knowledge.ts web/src/components/knowledge/workbench/WorkbenchSidebar.vue web/src/features/knowledge/useKnowledgePage.ts web/src/components/knowledge/KnowledgeGraph3D.vue web/src/pages/KnowledgePage.vue web/src/components/knowledge/workbench/__tests__/ web/src/stores/__tests__/
git commit -m "feat(knowledge): B1 前端重嵌入入口（文件菜单① + FocusCard② 接线）"
```

---

## Task B1-T5: 确认对话框 + 词法库置灰 + i18n + 运行时验证

**Files:**
- Modify: `web/src/features/knowledge/useKnowledgePage.ts`（`confirmReembed` 包 `$q.dialog` 确认层）
- Modify: `web/src/components/knowledge/workbench/WorkbenchSidebar.vue`（词法库菜单项置灰 + tooltip）
- Modify: `web/src/components/knowledge/workbench/KnowledgeWorkbench.vue`（如需透传当前集合语义层标记给 Sidebar）
- Modify: `web/src/i18n/`（zh-Hans / en-US 文案）
- Test: 上述两 spec 追加对话框/置灰用例

- [ ] **Step 1: Write the failing test**

```typescript
// useKnowledgePage spec 或 sidebar spec 追加：
it('词法库（embedding_model 空）时「重新向量化」菜单项置灰', () => {
  // mount sidebar with 当前集合 embedding_model='' → data-test="file-reembed" 项 disabled
});
it('confirmReembed 先弹确认对话框（列出文档数），确认后才调 store', async () => {
  // $q.dialog spy；未确认不调 store；确认后调且 notify
});
```

- [ ] **Step 2: Run test to verify it fails** → **Step 3: Write minimal implementation**

- 词法库置灰：Sidebar 由 `current-vault-id` + `collections`（Workbench 已有 props）computed `currentHasSemantic`；菜单项 `:disable="!currentHasSemantic"` + `q-tooltip` 说明「词法库无语义层，先启用语义检索」
- 确认对话框（复用 M1 真折射玻璃风格类）：列出文档数 + 「从已存正文重建向量，无需原文件」说明；确认后调 store
- i18n 文案（zh-Hans / en-US）：`reembedDocument: '重新向量化' / 'Re-embed'`；`reembedConfirmTitle: '重新向量化文档' / 'Re-embed documents'`；`reembedConfirmBody: '将从已存正文为 {n} 篇文档重建向量索引（无需原文件）。' / 'Rebuilds vectors from stored text for {n} document(s).'`；`reembedAccepted: '已受理 {n} 篇重嵌入' / '{n} document(s) queued'`；`reembedNoSemantic: '词法库无语义层' / 'No semantic layer'`

- [ ] **Step 4: Run test to verify it passes + 门禁**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: 全绿（含 check-i18n）

- [ ] **Step 5: 运行时验证（R3 红线，spec §8.4）**

1. 起 dev（`make build` pgvector tag + 前端 dev）
2. 事故复现：DB 执行 `UPDATE knowledge_chunks SET embedding = NULL WHERE doc_id = '<某 UI 上传文档>'`
3. 工作台文件行菜单点「重新向量化」→ 确认对话框 → 受理 notify
4. 文档列表状态实时 indexing → indexed（摄取 WS 复用生效）
5. 语义检索该文档内容 → 命中恢复
6. 图谱 FocusCard「重新向量化」按钮同链路验证（入口②）

- [ ] **Step 6: Commit**

```powershell
git add web/src/features/knowledge/useKnowledgePage.ts web/src/components/knowledge/workbench/ web/src/i18n/
git commit -m "feat(knowledge): B1 重嵌入确认对话框 + 词法库置灰 + i18n"
```

---

# 轨道 B — B2：集合语义层启用（空 → 启用单向，能力缺口②）

> 设计依据：spec §9。**仅支持「空语义层 → 启用」单向**（绑定当前全局 embedder）；换模型/降维不走 UI，仍走配置文件 + 重启 reconcile 既有路径。

## Task B2-T1: 后端 EnableCollectionSemantic（Proto + data/biz/service，复用 B1 管线）

**Files:**
- Modify: `api/kratos/knowledge/v1/knowledge.proto` + `make api`（message 紧随 B1-T1 区块；rpc 紧随 ReembedDocuments）
- Modify: `internal/biz/knowledge/knowledge.go`（`CollectionRepo`（:112-120）加 `EnableCollectionSemantic` + Usecase 透传含 Conflict 守卫）
- Modify: `internal/data/knowledge.go`（knowledgeRepo 实现，写路径 `RW().Write` 区域既有 UPDATE 模式）
- Modify: `internal/service/knowledge_reembed.go`（同文件加 RPC——B1/B2 共用重嵌入管线）
- Modify: `internal/event/flow_log.go` stepTitleRegistry + `docs/development/52-flow-logger.design.md` §5.1
- Test: `internal/service/knowledge_reembed_test.go` 追加；`internal/data/knowledge_reembed_test.go` 追加 PG 用例

- [ ] **Step 1: Write the failing test**

```go
func TestEnableCollectionSemantic_ConflictWhenAlreadyEnabled(t *testing.T) {
	// embedding_model 非空集合 → CodeConflict
}
func TestEnableCollectionSemantic_BadRequestWhenEmbedderNotConfigured(t *testing.T) {
	// s.embedder.Config() configured=false → CodeBadRequest
}
func TestEnableCollectionSemantic_EnqueuesAllContentDocs(t *testing.T) {
	// 词法库 + 3 篇有正文文档 → resp.EnqueuedDocs=3、EmbeddingModel/Dim = 全局 embedder 值
	// Eventually：全部文档 status 终态 indexed（复用 B1 串行管线）
}
func TestEnableCollectionSemantic_MutateAccessDenied(t *testing.T) { /* 权限拒绝 */ }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestEnableCollectionSemantic -count=1`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```proto
// knowledge.proto：
rpc EnableCollectionSemantic(EnableCollectionSemanticRequest) returns (EnableCollectionSemanticResponse) {
  option (google.api.http) = { post: "/v1/knowledge/collections/{collection_id}:enable-semantic", body: "*" };
}
message EnableCollectionSemanticRequest { string collection_id = 1 [(google.api.field_behavior) = REQUIRED]; }
message EnableCollectionSemanticResponse {
  int32 enqueued_docs = 1;    // 进入重嵌入队列的文档数
  string embedding_model = 2; // 绑定的模型名（当前全局 embedder）
  int32 dim = 3;
}
```

```go
// data knowledge.go（守卫式 UPDATE：仅当仍为空语义层才绑定，返 bool=是否生效）：
func (r *knowledgeRepo) EnableCollectionSemantic(ctx context.Context, id, model string, dim int) (bool, error) {
	res, err := r.data.RW().Write(ctx).ExecContext(ctx,
		`UPDATE knowledge_collections SET embedding_model = $2, dim = $3, updated_at = now()
		 WHERE id = $1 AND embedding_model = ''`, id, model, dim)
	// RowsAffected==0 → false（并发/已启用）
}

// biz Usecase 透传：false → ErrCollectionSemanticConflict（apierror.Conflict）

// service knowledge_reembed.go：
func (s *KnowledgeService) EnableCollectionSemantic(ctx context.Context, req *v1.EnableCollectionSemanticRequest) (*v1.EnableCollectionSemanticResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())        // 存在性
	if err != nil { return nil, err }
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil { return nil, err }
	if strings.TrimSpace(col.EmbeddingModel) != "" {
		return nil, apierror.Conflict("KNOWLEDGE", "semantic layer already enabled")
	}
	_, _, model, dim, configured, _ := s.embedder.Config()           // 全局 embedder 快照
	if s.embedder == nil || !configured {
		return nil, apierror.BadRequest("KNOWLEDGE", "embedder not configured")
	}
	if err := s.uc.EnableCollectionSemantic(ctx, col.ID, model, dim); err != nil { return nil, err }
	// 全集合有正文文档入队（复用 B1 同一串行管线 goroutine 函数）
	docs, err := s.uc.ListDocumentsPendingReembed(ctx, col.ID)       // 启用后全部 chunks 缺失/NULL，恰为 pending 集
	if err != nil { return nil, err }
	flow := s.knowledgeFlow(ctx)
	flow.LogStart("knowledge.collection.enable_semantic", "集合语义层启用",
		event.P("collection_id", col.ID), event.P("embedding_model", model), event.P("dim", dim))
	// safego.Go 串行循环 reembedOneDocument（K7 日志同款），done 时 flow.LogDone 同 step
	return &v1.EnableCollectionSemanticResponse{
		EnqueuedDocs: int32(len(docs)), EmbeddingModel: model, Dim: int32(dim),
	}, nil
}
```

stepTitleRegistry 登记：`"knowledge.collection.enable_semantic": "集合语义层启用"`。

- [ ] **Step 4: Run test to verify it passes + 门禁**

Run: `make api && make build && go test ./internal/service/ ./internal/data/ -run 'TestEnableCollectionSemantic|TestKnowledgeRepo_EnableCollectionSemantic' -count=1`
Expected: 全绿

- [ ] **Step 5: Commit**

```powershell
git add api/kratos/knowledge/v1/ internal/biz/knowledge/knowledge.go internal/data/knowledge.go internal/data/knowledge_reembed_test.go internal/service/knowledge_reembed.go internal/service/knowledge_reembed_test.go internal/event/flow_log.go docs/development/52-flow-logger.design.md
git commit -m "feat(knowledge): B2 EnableCollectionSemantic 空语义层单向启用（绑定全局 embedder + 批量重嵌入）"
```

---

## Task B2-T2: 前端 vault 树根菜单「启用语义检索」+ 确认对话框 + 运行时验证

**Files:**
- Modify: `web/src/components/knowledge/KnowledgeVaultTree.vue`（vault 根节点菜单（:96-105 refresh/delete-vault 区）加项，仅词法库显示）
- Modify: `web/src/features/knowledge/api.ts` + `web/src/stores/knowledge.ts`（`enableCollectionSemantic`）
- Modify: `web/src/features/knowledge/useKnowledgePage.ts`（`onTreeNodeAction`（:499）加 `enable-semantic` 分支 + 确认对话框）
- Modify: `web/src/pages/KnowledgePage.vue`（向树传 `lexical-vault-ids` computed：collections 中 `embedding_model===''` 的 id 集）
- Modify: `web/src/i18n/`
- Test: `web/src/components/knowledge/__tests__/KnowledgeVaultTree.spec.ts` 追加；store spec 追加

**设计**：
- 树节点不复制 collection 字段（单一事实源在 `collections`）；树经 `lexical-vault-ids` prop 判定菜单可见性
- 确认对话框（M1 真折射玻璃）：展示将绑定的全局 embedder 模型名/dim（`embedderConfig` store computed）+ 文档数耗时提示 +「启用后词法检索自动升级为混合检索」
- 启用后 `loadCollections()` 刷新；重嵌入进度复用摄取 WS（文档状态实时流转）

- [ ] **Step 1: Write the failing test**

```typescript
it('词法库 vault 根菜单显示「启用语义检索」，语义库不显示', () => {
  // lexicalVaultIds 含目标 vault → 菜单项存在；不含 → 不存在
});
it('enable-semantic action 确认后调 store 并刷新集合', async () => {
  // dialog 确认 → store.enableCollectionSemantic 被调 + loadCollections 被调
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/components/knowledge/__tests__/KnowledgeVaultTree.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```vue
<!-- KnowledgeVaultTree.vue：delete-vault 菜单项之前，v-if="lexicalVaultIds.includes(vaultIdOf(scope.node))" -->
<q-item clickable data-test="vault-enable-semantic" @click="emitAction('enable-semantic', scope.node)">
  <q-item-section avatar><q-icon name="travel_explore" size="18px" /></q-item-section>
  <q-item-section>{{ t('knowledgePage.enableSemantic') }}</q-item-section>
</q-item>
```

```typescript
// useKnowledgePage.ts — onTreeNodeAction 分支：
} else if (action === 'enable-semantic') {
  const col = collections.value.find((c) => vaultNodeKey(c.id) === node.key);
  if (!col) return;
  $q.dialog({
    title: t('knowledgePage.enableSemanticTitle'),
    message: t('knowledgePage.enableSemanticBody', {
      model: embedderConfig.value?.model ?? '', dim: embedderConfig.value?.dim ?? 0,
    }),
    cancel: true,
  }).onOk(async () => {
    try {
      const r = await knowledgeStore.enableCollectionSemantic(col.id);
      $q.notify({ type: 'positive', message: t('knowledgePage.enableSemanticAccepted', { n: r.enqueued_docs }) });
      await loadCollections();
    } catch (e) { $q.notify({ type: 'negative', message: friendlyError(e) }); }
  });
}
```

i18n（zh-Hans / en-US）：`enableSemantic: '启用语义检索' / 'Enable semantic search'`；`enableSemanticTitle: '启用语义检索' / 'Enable semantic search'`；`enableSemanticBody: '将绑定当前 Embedder（{model}，{dim} 维）并为全部文档重建向量；启用后词法检索自动升级为混合检索。' / 'Binds current embedder ({model}, {dim}d) and re-embeds all documents; lexical search upgrades to hybrid.'`；`enableSemanticAccepted: '已启用，{n} 篇文档进入重嵌入队列' / 'Enabled; {n} document(s) queued'`。

- [ ] **Step 4: Run test to verify it passes + 门禁**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: 全绿

- [ ] **Step 5: 运行时验证（R3 红线，spec §9.4）**

词法库（embedding_model 空）vault 根菜单点「启用语义检索」→ 对话框显示模型名/dim → 确认 → 文档批量 indexing → indexed → 检索从 BM25-only 升级为混合检索命中；已启用集合菜单项消失。

- [ ] **Step 6: Commit**

```powershell
git add web/src/components/knowledge/KnowledgeVaultTree.vue web/src/features/knowledge/api.ts web/src/stores/knowledge.ts web/src/features/knowledge/useKnowledgePage.ts web/src/pages/KnowledgePage.vue web/src/i18n/ web/src/components/knowledge/__tests__/ web/src/stores/__tests__/
git commit -m "feat(knowledge): B2 前端语义层启用入口（vault 菜单 + 确认对话框）"
```

---

# 轨道 C — G5-G G-3 性能基准（双布局矩阵）

> 设计依据：spec §10。与 M2 协同：一次造数双布局复用。关闭 37-knowledge.development.md:1046 的 G-3 📋。

## Task C-T1: 2 万节点/5 万边双布局基准录制 + 落档

**Files:**
- Create: `test/graph3d-perf/gen-dataset.ts`（合成数据集生成器，一次性工具，test/ 目录纪律）
- Create: `docs/testing/reports/perf-2026-08-12-graph3d-dual-layout.md`（基准记录，沿用 reports/ acceptance-* 位置与结构）
- Modify: `docs/development/37-knowledge.development.md`（G-3 📋→✅，DOC-T1 亦可合并收尾）

- [ ] **Step 1: 合成数据集 + dev 注入**

`test/graph3d-perf/gen-dataset.ts`：确定性 seed 生成 20,000 节点 / 50,000 边（doc_type 分布 ≥6 组，度数幂律逼近真实 vault）；输出 JSON 供 dev 控制台注入图谱全屏（`generation` +1 触发重建）。双布局各测一轮：force（FORCE_DEFAULTS）→ HUD 切 galaxy（GALAXY_FORCE_PARAMS 再加热）。

- [ ] **Step 2: 基准矩阵录制**

| 项 | 指标 | 方法 |
|----|------|------|
| 交互帧率 | hover/拖拽/缩放 FPS（HIGH/MID/LOW 三档 × 双布局） | DevTools Performance 录制 |
| Worker tick | 物理单 tick 耗时（双布局） | performance.now 采样（worker onmessage 计时注入） |
| 布局收敛 | alpha 收敛时间（双布局） | engine onSettled 计时 |
| 静置零占用 | 收敛后 CPU/GPU 零占用断言 | lazy-render 验证（needsRender=false 时无 RAF；Performance 静置 10s 无长任务） |

- [ ] **Step 3: 落档 + G-3 关闭**

结果写 `docs/testing/reports/perf-2026-08-12-graph3d-dual-layout.md`（环境：CPU/GPU/浏览器版本；矩阵表格；结论与档位建议）；37-knowledge.development.md G-3 行 📋→✅（As-built 引用基准文档路径）。

- [ ] **Step 4: Commit**

```powershell
git add test/graph3d-perf/ docs/testing/reports/perf-2026-08-12-graph3d-dual-layout.md docs/development/37-knowledge.development.md
git commit -m "test(knowledge): C 双布局性能基准（2 万节点/5 万边）落档，G-3 关闭"
```

---

# 轨道 DOC — 文档同步（DOC-SYNC 红线）

## Task DOC-T1: 37-knowledge 三件套 + 交叉参考同步

**Files:**
- Modify: `docs/development/37-knowledge.design.md`（增补 V12.9 章节：M1 真折射 / M2 星系盘+布局切换 / M3 电影感镜头 / M4 聚焦+节点卡 / M5 图例透镜 / B1 重嵌入 / B2 语义层启用；API 端点表加 2 个 RPC——DOC-SYNC-7）
- Modify: `docs/development/37-knowledge.development.md`（M1-M5/B1/B2/C 任务清单 ✅ + G-3 关闭 + 代码锚点——DOC-SYNC-5/6）
- Modify: `docs/development/37-knowledge.md`（需求文档增补 B1/B2 需求条目与验收标准；验收 34 性能基准引用更新——DOC-SYNC-2 边界：只写需求/验收，实现细节留在 design）
- Modify: `docs/development/65-module-cross-reference-full.md`（knowledge 模块卡片：新增 RPC/前端组件关联——若该手册含 API/组件清单）
- 归档说明：本计划与 spec（`docs/reports/2026-08-12-plan-knowledge-galaxy-liquid-glass.md`）保留为方案档案

- [ ] **Step 1: 三件套同步**（按上方 Files 各点；三件套内容边界红线：需求/.md、设计/.design.md、进度/.development.md 不跨类混写）

- [ ] **Step 2: 一致性自检**

- design.md API 端点表 vs `api/kratos/knowledge/v1/knowledge.proto`（ReembedDocuments / EnableCollectionSemantic）
- development.md 代码锚点路径全部真实存在（含本计划全部新增文件）
- 52-flow-logger.design.md §5.1 已含 `knowledge.reembed.*` / `knowledge.collection.enable_semantic`（B1-T3/B2-T1 已同步，此处复核）

- [ ] **Step 3: Commit**

```powershell
git add docs/development/37-knowledge.design.md docs/development/37-knowledge.development.md docs/development/37-knowledge.md docs/development/65-module-cross-reference-full.md
git commit -m "docs(knowledge): 全面升级文档同步（V12.9 + B1/B2 + G-3 关闭）"
```

---
