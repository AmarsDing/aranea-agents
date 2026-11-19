/**
 * canvasPalette：three.js / Canvas2D / Sprite 的取色口。
 *
 * 为什么单独一层：图谱配色被纯函数（graphUi / structurePalette / layerMeta / hudParams）
 * 直接返回给材质与 BufferGeometry，这些函数必须可单测（无 DOM）。
 * 因此把「颜色解析」抽到这里：函数只问 palette，palette 负责读 --nt-graph-* 令牌，
 * 读不到时回退到与 tokens 同值的默认值（单测 / SSR 场景）。
 *
 * 约定：生产代码不得再出现画布字面色；新增颜色先加 token，再加 fallback。
 */
import { readCssVar } from './cssVar';

export interface CanvasPalette {
  link: Record<'explicit' | 'entity' | 'semantic' | 'unknown', string>;
  node: string[];
  nodeFallback: string;
  structure: Record<'ultra' | 'super' | 'regular' | 'leaf' | 'fallback', string>;
  edgeUnified: string;
  edgeHighlight: string;
  conflict: string;
  reticle: Record<'hover' | 'selected', string>;
  label: Record<
    | 'text'
    | 'group'
    | 'groupText'
    | 'focus'
    | 'focusSelected'
    | 'bg'
    | 'focusBg'
    | 'groupBg'
    | 'ringHover'
    | 'ringSelected',
    string
  >;
  memoryLayer: Record<'L0' | 'L1' | 'L2' | 'L3' | 'L4' | 'fallback', string>;
  hud: Record<'orbA' | 'orbB' | 'alertA' | 'alertB', string>;
}

/** 与 _nt-tokens.css 同步的兜底值（改值先改 token，再同步此处）。 */
export const CANVAS_PALETTE_DEFAULTS: CanvasPalette = {
  link: {
    explicit: '#4c8dff',
    entity: '#a066d3',
    semantic: '#3bbdbd',
    unknown: '#8a93a6',
  },
  node: ['#4c8dff', '#3bbdbd', '#a066d3', '#d3864a', '#c45b8a', '#7fb069', '#d3b04a', '#5bc4e5'],
  nodeFallback: '#8a93a6',
  structure: {
    ultra: '#f472b6',
    super: '#f5c542',
    regular: '#35c8d0',
    leaf: '#16434f',
    fallback: '#8a93a6',
  },
  edgeUnified: '#4f6b7d',
  edgeHighlight: '#39e6ff',
  conflict: '#ef5350',
  reticle: { hover: '#54e6ff', selected: '#a855f7' },
  label: {
    text: '#8fb9d9',
    group: '#9fdcff',
    groupText: '#d8ecff',
    focus: '#e8f7ff',
    focusSelected: '#ffffff',
    bg: 'rgba(5, 8, 16, 0.55)',
    focusBg: 'rgba(5, 8, 16, 0.78)',
    groupBg: 'rgba(5, 8, 16, 0.72)',
    ringHover: 'rgba(84, 230, 255, 0.9)',
    ringSelected: 'rgba(168, 85, 247, 0.95)',
  },
  memoryLayer: {
    L0: '#90a4ae',
    L1: '#7986cb',
    L2: '#4db6ac',
    L3: '#ba68c8',
    L4: '#ff8a65',
    fallback: '#9e9e9e',
  },
  hud: {
    orbA: '#123a6e',
    orbB: '#4dd8e8',
    alertA: '#4c0519',
    alertB: '#f87171',
  },
};

/** 从令牌解析一次（不缓存）：主题切换后调用以取新值。 */
export function resolveCanvasPalette(): CanvasPalette {
  const d = CANVAS_PALETTE_DEFAULTS;
  const read = (name: string, fallback: string) => readCssVar(name, fallback);
  return {
    link: {
      explicit: read('--nt-graph-link-explicit', d.link.explicit),
      entity: read('--nt-graph-link-entity', d.link.entity),
      semantic: read('--nt-graph-link-semantic', d.link.semantic),
      unknown: read('--nt-graph-link-unknown', d.link.unknown),
    },
    node: d.node.map((fallback, i) => read(`--nt-graph-node-${i + 1}`, fallback)),
    nodeFallback: read('--nt-graph-node-fallback', d.nodeFallback),
    structure: {
      ultra: read('--nt-graph-structure-ultra', d.structure.ultra),
      super: read('--nt-graph-structure-super', d.structure.super),
      regular: read('--nt-graph-structure-regular', d.structure.regular),
      leaf: read('--nt-graph-structure-leaf', d.structure.leaf),
      fallback: read('--nt-graph-structure-fallback', d.structure.fallback),
    },
    edgeUnified: read('--nt-graph-edge-unified', d.edgeUnified),
    edgeHighlight: read('--nt-graph-edge-highlight', d.edgeHighlight),
    conflict: read('--nt-graph-conflict', d.conflict),
    reticle: {
      hover: read('--nt-graph-reticle-hover', d.reticle.hover),
      selected: read('--nt-graph-reticle-selected', d.reticle.selected),
    },
    label: {
      text: read('--nt-graph-label', d.label.text),
      group: read('--nt-graph-label-group', d.label.group),
      groupText: read('--nt-graph-label-group-text', d.label.groupText),
      focus: read('--nt-graph-label-focus', d.label.focus),
      focusSelected: read('--nt-graph-label-focus-selected', d.label.focusSelected),
      bg: read('--nt-graph-label-bg', d.label.bg),
      focusBg: read('--nt-graph-label-focus-bg', d.label.focusBg),
      groupBg: read('--nt-graph-label-group-bg', d.label.groupBg),
      ringHover: read('--nt-graph-focus-ring-hover', d.label.ringHover),
      ringSelected: read('--nt-graph-focus-ring-selected', d.label.ringSelected),
    },
    memoryLayer: {
      L0: read('--nt-memory-layer-0', d.memoryLayer.L0),
      L1: read('--nt-memory-layer-1', d.memoryLayer.L1),
      L2: read('--nt-memory-layer-2', d.memoryLayer.L2),
      L3: read('--nt-memory-layer-3', d.memoryLayer.L3),
      L4: read('--nt-memory-layer-4', d.memoryLayer.L4),
      fallback: read('--nt-memory-layer-fallback', d.memoryLayer.fallback),
    },
    hud: {
      orbA: read('--nt-hud-orb-a', d.hud.orbA),
      orbB: read('--nt-hud-orb-b', d.hud.orbB),
      alertA: read('--nt-hud-alert-a', d.hud.alertA),
      alertB: read('--nt-hud-alert-b', d.hud.alertB),
    },
  };
}

let cached: CanvasPalette | null = null;

/** 当前调色板（首次读取后缓存；主题切换调用 refreshCanvasPalette 失效）。 */
export function canvasPalette(): CanvasPalette {
  if (!cached) cached = resolveCanvasPalette();
  return cached;
}

/** 主题/密度切换后重新解析令牌。 */
export function refreshCanvasPalette(): CanvasPalette {
  cached = resolveCanvasPalette();
  return cached;
}

/** 单测注入（传 null 复位到令牌/兜底）。 */
export function setCanvasPaletteForTest(palette: CanvasPalette | null): void {
  cached = palette;
}
