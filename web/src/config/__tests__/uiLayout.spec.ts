import { describe, expect, it } from 'vitest';
import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { resolveUiLayout, resolveUiVisual } from '../uiLayout';
import { routes } from '../../router/routes';

function routeByName(name: string) {
  const main = routes.find((route) => route.path === '/');
  return main?.children?.find((route) => route.name === name);
}

describe('整站布局属主', () => {
  it('默认使用 fluid，并保留四种明确布局', () => {
    expect(resolveUiLayout(undefined)).toBe('fluid');
    expect(resolveUiLayout('measure')).toBe('measure');
    expect(resolveUiLayout('canvas')).toBe('canvas');
    expect(resolveUiLayout('immersive')).toBe('immersive');
    expect(resolveUiLayout('unknown')).toBe('fluid');
  });

  it('视觉档位：指挥舱页面显式声明，其余缺省为工作区', () => {
    expect(resolveUiVisual(undefined)).toBe('workspace');
    expect(resolveUiVisual('deck')).toBe('deck');
    expect(resolveUiVisual('other')).toBe('workspace');
    for (const name of ['workbench', 'overview', 'ceo-company', 'ceo-overview', 'chat']) {
      expect(routeByName(name)?.meta?.uiVisual, name).toBe('deck');
    }
    // 运行与治理各页（含实时运转、流程健康）2026-11-19 起统一用标准页面骨架
    for (const name of ['agents', 'tools', 'plugins', 'settings', 'monitor-live', 'ceo-oncall']) {
      expect(resolveUiVisual(routeByName(name)?.meta?.uiVisual), name).toBe('workspace');
    }
  });

  it('并行面板和编排画布不会被标准页壳改形', () => {
    for (const name of ['overview', 'ceo-overview', 'chat']) {
      expect(routeByName(name)?.meta?.uiLayout, name).toBe('immersive');
    }
    for (const name of ['team-orchestrate', 'graph-editor-new', 'graph-editor']) {
      expect(routeByName(name)?.meta?.uiLayout, name).toBe('canvas');
    }
  });

  it('页面铺满、正文限宽且航仪表面地基已全局接入', () => {
    const darkPath = resolve(process.cwd(), 'src/css/theme/_css-vars-dark.sass');
    const lightPath = resolve(process.cwd(), 'src/css/theme/_css-vars-light.sass');
    const layoutPath = resolve(process.cwd(), 'src/css/theme/_layout-fluid.sass');
    const entryPath = resolve(process.cwd(), 'src/css/app-global.sass');

    const light = readFileSync(lightPath, 'utf8');
    expect(readFileSync(darkPath, 'utf8')).toContain('--layout-max-width: 100%');
    expect(light).toContain('--layout-max-width: 100%');
    expect(light).toMatch(/body:not\(\.body--dark\)\s+--q-primary/);
    expect(existsSync(layoutPath)).toBe(true);
    if (existsSync(layoutPath)) {
      const layout = readFileSync(layoutPath, 'utf8');
      expect(layout).toContain('.app-surface-l0');
      expect(layout).toContain('.app-coordinate-hero');
      expect(layout).toContain('--layout-measure: 72ch');
    }
    expect(readFileSync(entryPath, 'utf8')).toContain('@import ./theme/layout-fluid');
  });
});
