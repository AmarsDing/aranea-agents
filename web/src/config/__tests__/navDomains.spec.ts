import { describe, expect, it } from 'vitest';
import { flatNavLeaves } from '../navView';
import { assignedPaths, buildNavDomains, findDomainTrail, NAV_DOMAIN_DEFS } from '../navDomains';

const all = () => true;

describe('八个一级去处', () => {
  it('每个侧栏入口都登记到了某个领域，且只登记一次', () => {
    const leaves = flatNavLeaves().map((leaf) => leaf.to);
    const assigned = assignedPaths();
    const missing = leaves.filter((path) => !assigned.has(path));
    expect(missing, '新增菜单入口后需要在 navDomains.ts 里登记').toEqual([]);

    const seen = new Map<string, number>();
    for (const domain of NAV_DOMAIN_DEFS) {
      for (const p of domain.primaries) {
        for (const path of [p.path, ...(p.children ?? [])]) seen.set(path, (seen.get(path) ?? 0) + 1);
      }
    }
    expect([...seen].filter(([, n]) => n > 1)).toEqual([]);
  });

  it('登记的路径都是真实存在的入口', () => {
    const leaves = new Set(flatNavLeaves().map((leaf) => leaf.to));
    expect([...assignedPaths()].filter((path) => !leaves.has(path))).toEqual([]);
  });

  it('全部可见时恰好八个领域，侧栏一级页面不超过 45 个', () => {
    const domains = buildNavDomains(all);
    expect(domains.map((d) => d.key)).toEqual([
      'workbench',
      'business',
      'conversation',
      'agents',
      'capability',
      'knowledge',
      'ops',
      'settings',
    ]);
    const primaries = domains.reduce((sum, d) => sum + d.primaries.length, 0);
    const children = domains.reduce((sum, d) => sum + d.primaries.reduce((s, p) => s + p.children.length, 0), 0);
    expect(primaries + children).toBe(flatNavLeaves().length);
    expect(children).toBeGreaterThan(0);
  });

  it('无权限的页面不出现，领域下全无可见页面时整个领域隐藏', () => {
    const hidden = new Set([
      '/ceo/company',
      '/ceo/overview',
      '/ecommerce-loop',
      '/content-inbox',
      '/review-queue',
      '/opportunity-board',
      '/sourcing',
    ]);
    const domains = buildNavDomains((path) => !hidden.has(path));
    expect(domains.find((d) => d.key === 'business')).toBeUndefined();
    // 子页的可见性跟随父入口
    const noTools = buildNavDomains((path) => path !== '/tools');
    const capability = noTools.find((d) => d.key === 'capability');
    expect(capability?.primaries.some((p) => p.to === '/tools')).toBe(false);
    expect(findDomainTrail('/tools/runs', noTools)).toBeNull();
  });

  it('当前位置：一级页、子页与详情页都能定位', () => {
    const domains = buildNavDomains(all);
    expect(findDomainTrail('/workbench', domains)).toMatchObject({ domain: { key: 'workbench' }, child: null });
    const runs = findDomainTrail('/tools/runs', domains);
    expect(runs?.domain.key).toBe('capability');
    expect(runs?.primary.to).toBe('/tools');
    expect(runs?.child?.to).toBe('/tools/runs');
    expect(findDomainTrail('/agents/42/settings', domains)?.primary.to).toBe('/agents');
    expect(findDomainTrail('/teams/t-1/orchestrate', domains)?.primary.to).toBe('/team');
    // /settings 与 /settings/organization 不会互相吞
    expect(findDomainTrail('/settings/organization', domains)?.domain.key).toBe('agents');
    expect(findDomainTrail('/settings', domains)?.domain.key).toBe('settings');
    expect(findDomainTrail('/nowhere', domains)).toBeNull();
  });

  it('领域页签可以换叫法，原菜单文案不变', () => {
    const ops = buildNavDomains(all).find((d) => d.key === 'ops');
    const overview = ops?.primaries.find((p) => p.to === '/overview');
    expect(overview?.tabLabelKey).toBe('shell.nav.tab.modelUsage');
    expect(overview?.labelKey).toBe('menu.overview');
  });
});
