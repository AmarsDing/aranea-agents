export type UiLayout = 'fluid' | 'measure' | 'canvas' | 'immersive';

/**
 * 视觉档位：deck = 指挥舱（工作台、公司树、经营看板、实时运转、概览），保留网格底、
 * 光效与掠光动效，数字大、信息少；workspace = 工作区（登记页、设置、编辑器等其余页面），
 * 去掉装饰、实色背板、信息密度优先。路由 meta.uiVisual 声明，缺省为 workspace。
 */
export type UiVisual = 'deck' | 'workspace';

export function resolveUiVisual(value: unknown): UiVisual {
  return value === 'deck' ? 'deck' : 'workspace';
}

export function resolveUiLayout(value: unknown): UiLayout {
  return value === 'measure' || value === 'canvas' || value === 'immersive' ? value : 'fluid';
}
