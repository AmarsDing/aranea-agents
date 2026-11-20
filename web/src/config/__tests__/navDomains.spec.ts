import { describe, expect, it } from 'vitest';
import { flatNavLeaves } from '../navView';
import {
  assignedPaths,
  buildNavDomains,
  findDomainTrail,
  moduleLanding,
  NAV_DOMAIN_DEFS,
  NAV_MODULES,
} from '../navDomains';

const all = () => true;

describe('三个模块与领域', () => {
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

  it('全部可见时：工作台、看板各一组页签，Harness 是首页加五个领域，外加壳层', () => {
    const domains = buildNavDomains(all);
    expect(domains.map((d) => `${d.module}/${d.key}`)).toEqual([
      'workbench/workbench',
      'board/business',
      'harness/home',
      'harness/agents',
      'harness/orchestrate',
      'harness/capability',
      'harness/connect',
      'harness/ops',
      'shell/settings',
    ]);
    const primaries = domains.reduce((sum, d) => sum + d.primaries.length, 0);
    const children = domains.reduce((sum, d) => sum + d.primaries.reduce((s, p) => s + p.children.length, 0), 0);
    expect(primaries + children).toBe(flatNavLeaves().length);
    expect(children).toBeGreaterThan(0);
  });

  it('工作台是设计稿的 6 个页签，合并前的旧页面挂在子页签里', () => {
    const workbench = buildNavDomains(all).find((d) => d.key === 'workbench');
    expect(workbench?.primaries.map((p) => p.to)).toEqual([
      '/workbench',
      '/tasks',
      '/chat',
      '/schedule',
      '/mail',
      '/knowledge',
    ]);
    expect(workbench?.primaries[0].children.map((c) => c.to)).toEqual(['/approvals']);
    expect(workbench?.primaries[1].children.map((c) => c.to)).toEqual(['/coding-tasks']);
    expect(workbench?.primaries[2].children.map((c) => c.to)).toEqual(['/task-workspace', '/sessions', '/companion']);
  });

  it('任务页不在后端菜单目录里，可见性跟随今日页', () => {
    const tabs = (isVisible: (path: string) => boolean) =>
      buildNavDomains(isVisible)
        .find((d) => d.key === 'workbench')
        ?.primaries.map((p) => p.to) ?? [];
    expect(tabs((path) => path !== '/tasks')).toContain('/tasks');
    expect(tabs((path) => path !== '/workbench')).not.toContain('/tasks');
  });

  it('点模块的落点：首选 home，没权限时退到本模块第一个可见页面，整个模块不可见时为 null', () => {
    const domains = buildNavDomains(all);
    expect(NAV_MODULES.map((m) => moduleLanding(m.key, domains))).toEqual(['/workbench', '/ceo/company', '/overview']);
    expect(
      moduleLanding(
        'harness',
        buildNavDomains((path) => path !== '/overview'),
      ),
    ).toBe('/agents');
    const boardPaths = new Set(
      domains.filter((d) => d.module === 'board').flatMap((d) => d.primaries.map((p) => p.to)),
    );
    expect(
      moduleLanding(
        'board',
        buildNavDomains((path) => !boardPaths.has(path)),
      ),
    ).toBeNull();
  });

  it('无权限的页面不出现，领域下全无可见页面时整个领域隐藏', () => {
    const domains = buildNavDomains((path) => path !== '/ceo/company' && path !== '/ecommerce-loop');
    expect(domains.find((d) => d.key === 'business')).toBeUndefined();
    // 子页的可见性跟随父入口
    const noTools = buildNavDomains((path) => path !== '/tools');
    const capability = noTools.find((d) => d.key === 'capability');
    expect(capability?.primaries.some((p) => p.to === '/tools')).toBe(false);
    expect(findDomainTrail('/tools/runs', noTools)).toBeNull();
  });

  it('当前位置：一级页、子页与详情页都能定位', () => {
    const domains = buildNavDomains(all);
    expect(findDomainTrail('/workbench', domains)).toMatchObject({
      domain: { key: 'workbench', module: 'workbench' },
      child: null,
    });
    expect(findDomainTrail('/sessions', domains)).toMatchObject({
      primary: { to: '/chat' },
      child: { to: '/sessions' },
    });
    const runs = findDomainTrail('/tools/runs', domains);
    expect(runs?.domain.key).toBe('capability');
    expect(runs?.domain.module).toBe('harness');
    expect(runs?.primary.to).toBe('/tools');
    expect(runs?.child?.to).toBe('/tools/runs');
    expect(findDomainTrail('/agents/42/settings', domains)?.primary.to).toBe('/agents');
    expect(findDomainTrail('/teams/t-1/orchestrate', domains)?.primary.to).toBe('/team');
    // /settings 与 /settings/organization 不会互相吞
    expect(findDomainTrail('/settings/organization', domains)?.domain.key).toBe('agents');
    expect(findDomainTrail('/settings', domains)?.domain.module).toBe('shell');
    expect(findDomainTrail('/nowhere', domains)).toBeNull();
  });

  it('页签可以换叫法，原菜单文案不变', () => {
    const home = buildNavDomains(all).find((d) => d.key === 'home');
    expect(home?.primaries[0].tabLabelKey).toBe('shell.nav.tab.harnessHome');
    expect(home?.primaries[0].labelKey).toBe('menu.overview');
  });
});
