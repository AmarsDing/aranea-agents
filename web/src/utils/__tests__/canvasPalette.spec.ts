/**
 * 画布调色板契约：
 * 1) 代码内 fallback 必须与 _nt-tokens.css 同值，防止「token 改了代码没跟」的静默漂移；
 * 2) 运行时注入 CSS 变量后，纯函数（graphUi / layerMeta）必须真的返回新值，
 *    证明颜色解析已从纯函数里拆出来，而不是把 hex 换个地方写死。
 */
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import {
  CANVAS_PALETTE_DEFAULTS,
  canvasPalette,
  refreshCanvasPalette,
  setCanvasPaletteForTest,
} from '../canvasPalette';
import { graphDocTypeColor, graphLinkColor } from '../../features/knowledge/graphUi';
import { memoryLayerColor } from '../../features/memory/panorama/layerMeta';

const TOKEN_FILE = resolve(process.cwd(), 'src/css/theme/_nt-tokens.css');

function tokenMap(): Map<string, string> {
  const css = readFileSync(TOKEN_FILE, 'utf8');
  const map = new Map<string, string>();
  for (const match of css.matchAll(/(--nt-[\w-]+):\s*([^;]+);/g)) {
    map.set(match[1], match[2].trim());
  }
  return map;
}

/** palette 字段 → 令牌名（两者必须同值）。 */
const PALETTE_TOKENS: [string, string][] = [
  [CANVAS_PALETTE_DEFAULTS.link.explicit, '--nt-graph-link-explicit'],
  [CANVAS_PALETTE_DEFAULTS.link.entity, '--nt-graph-link-entity'],
  [CANVAS_PALETTE_DEFAULTS.link.semantic, '--nt-graph-link-semantic'],
  [CANVAS_PALETTE_DEFAULTS.link.unknown, '--nt-graph-link-unknown'],
  [CANVAS_PALETTE_DEFAULTS.nodeFallback, '--nt-graph-node-fallback'],
  [CANVAS_PALETTE_DEFAULTS.structure.ultra, '--nt-graph-structure-ultra'],
  [CANVAS_PALETTE_DEFAULTS.structure.super, '--nt-graph-structure-super'],
  [CANVAS_PALETTE_DEFAULTS.structure.regular, '--nt-graph-structure-regular'],
  [CANVAS_PALETTE_DEFAULTS.structure.leaf, '--nt-graph-structure-leaf'],
  [CANVAS_PALETTE_DEFAULTS.edgeUnified, '--nt-graph-edge-unified'],
  [CANVAS_PALETTE_DEFAULTS.edgeHighlight, '--nt-graph-edge-highlight'],
  [CANVAS_PALETTE_DEFAULTS.conflict, '--nt-graph-conflict'],
  [CANVAS_PALETTE_DEFAULTS.reticle.hover, '--nt-graph-reticle-hover'],
  [CANVAS_PALETTE_DEFAULTS.reticle.selected, '--nt-graph-reticle-selected'],
  [CANVAS_PALETTE_DEFAULTS.label.text, '--nt-graph-label'],
  [CANVAS_PALETTE_DEFAULTS.label.group, '--nt-graph-label-group'],
  [CANVAS_PALETTE_DEFAULTS.label.groupText, '--nt-graph-label-group-text'],
  [CANVAS_PALETTE_DEFAULTS.label.focus, '--nt-graph-label-focus'],
  [CANVAS_PALETTE_DEFAULTS.label.focusSelected, '--nt-graph-label-focus-selected'],
  [CANVAS_PALETTE_DEFAULTS.label.bg, '--nt-graph-label-bg'],
  [CANVAS_PALETTE_DEFAULTS.label.focusBg, '--nt-graph-label-focus-bg'],
  [CANVAS_PALETTE_DEFAULTS.label.groupBg, '--nt-graph-label-group-bg'],
  [CANVAS_PALETTE_DEFAULTS.label.ringHover, '--nt-graph-focus-ring-hover'],
  [CANVAS_PALETTE_DEFAULTS.label.ringSelected, '--nt-graph-focus-ring-selected'],
  [CANVAS_PALETTE_DEFAULTS.memoryLayer.L0, '--nt-memory-layer-0'],
  [CANVAS_PALETTE_DEFAULTS.memoryLayer.L1, '--nt-memory-layer-1'],
  [CANVAS_PALETTE_DEFAULTS.memoryLayer.L2, '--nt-memory-layer-2'],
  [CANVAS_PALETTE_DEFAULTS.memoryLayer.L3, '--nt-memory-layer-3'],
  [CANVAS_PALETTE_DEFAULTS.memoryLayer.L4, '--nt-memory-layer-4'],
  [CANVAS_PALETTE_DEFAULTS.memoryLayer.fallback, '--nt-memory-layer-fallback'],
  [CANVAS_PALETTE_DEFAULTS.hud.orbA, '--nt-hud-orb-a'],
  [CANVAS_PALETTE_DEFAULTS.hud.orbB, '--nt-hud-orb-b'],
  [CANVAS_PALETTE_DEFAULTS.hud.alertA, '--nt-hud-alert-a'],
  [CANVAS_PALETTE_DEFAULTS.hud.alertB, '--nt-hud-alert-b'],
];

afterEach(() => {
  setCanvasPaletteForTest(null);
  document.body.removeAttribute('style');
});

describe('canvasPalette 令牌一致性', () => {
  it('每个 fallback 都与 _nt-tokens.css 的令牌同值', () => {
    const tokens = tokenMap();
    const drift = PALETTE_TOKENS.filter(([fallback, name]) => tokens.get(name) !== fallback).map(
      ([fallback, name]) => `${name}: token=${tokens.get(name)} fallback=${fallback}`,
    );

    expect(drift).toEqual([]);
  });

  it('节点调色板 8 档全部登记了令牌', () => {
    const tokens = tokenMap();
    CANVAS_PALETTE_DEFAULTS.node.forEach((fallback, i) => {
      expect(tokens.get(`--nt-graph-node-${i + 1}`)).toBe(fallback);
    });
  });
});

describe('令牌注入到纯函数', () => {
  it('覆盖 --nt-graph-link-* 后 graphLinkColor 返回新值（不读死常量）', () => {
    document.body.style.setProperty('--nt-graph-link-explicit', '#112233');
    refreshCanvasPalette();

    expect(canvasPalette().link.explicit).toBe('#112233');
    expect(graphLinkColor('explicit')).toBe('#112233');
    // 未覆盖的类型仍走原令牌
    expect(graphLinkColor('semantic')).toBe(CANVAS_PALETTE_DEFAULTS.link.semantic);
  });

  it('覆盖 --nt-graph-node-* 后 doc_type 哈希取色跟着变', () => {
    document.body.style.setProperty('--nt-graph-node-1', '#aa0001');
    document.body.style.setProperty('--nt-graph-node-2', '#aa0002');
    document.body.style.setProperty('--nt-graph-node-3', '#aa0003');
    document.body.style.setProperty('--nt-graph-node-4', '#aa0004');
    document.body.style.setProperty('--nt-graph-node-5', '#aa0005');
    document.body.style.setProperty('--nt-graph-node-6', '#aa0006');
    document.body.style.setProperty('--nt-graph-node-7', '#aa0007');
    document.body.style.setProperty('--nt-graph-node-8', '#aa0008');
    refreshCanvasPalette();

    expect(graphDocTypeColor('note')).toMatch(/^#aa000[1-8]$/);
    expect(graphDocTypeColor('')).toBe(CANVAS_PALETTE_DEFAULTS.nodeFallback);
  });

  it('覆盖 --nt-memory-layer-* 后 memoryLayerColor 跟着变，未知层级走 fallback', () => {
    document.body.style.setProperty('--nt-memory-layer-2', '#00ff88');
    refreshCanvasPalette();

    expect(memoryLayerColor('L2')).toBe('#00ff88');
    expect(memoryLayerColor('L9')).toBe(CANVAS_PALETTE_DEFAULTS.memoryLayer.fallback);
  });
});
