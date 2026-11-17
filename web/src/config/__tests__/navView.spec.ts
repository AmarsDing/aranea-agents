import { describe, expect, it } from 'vitest';
import { sideNavGroups } from '../sideNav';
import { findNavTrail, flatNavLeaves, navGroupsView } from '../navView';

/**
 * 导航派生视图契约：
 * - 分层只是展示，入口集合必须与 sideNav 完全一致（否则会悄悄丢入口）；
 * - 详情页必须能回溯到父入口（面包屑/高亮）；
 * - 运行记录派生入口可被检索到。
 */
describe('navView', () => {
  it('覆盖 sideNav 的全部入口，且不重复', () => {
    const source = sideNavGroups.flatMap((group) => group.items.map((item) => item.to)).sort();
    const derived = flatNavLeaves()
      .filter((leaf) => !leaf.derived)
      .map((leaf) => leaf.to)
      .sort();
    expect(derived).toEqual(source);
  });

  it('分组数量与顺序保持与 sideNav 一致', () => {
    expect(navGroupsView.map((group) => group.labelKey)).toEqual(sideNavGroups.map((group) => group.labelKey));
  });

  it('详情页回溯到父入口', () => {
    expect(findNavTrail('/agents/42/settings')?.item.to).toBe('/agents');
    expect(findNavTrail('/sessions/s-1')?.item.to).toBe('/sessions');
    expect(findNavTrail('/graphs/g-1/executions')?.item.to).toBe('/graphs');
    expect(findNavTrail('/teams/t-1/orchestrate')?.item.to).toBe('/team');
  });

  it('最长前缀优先，技能子视图不会被 /skills 抢走', () => {
    expect(findNavTrail('/skills/tags')?.item.to).toBe('/skills/tags');
    expect(findNavTrail('/skills/evolution-suggestions')?.item.to).toBe('/skills/evolution-suggestions');
  });

  it('运行记录派生入口可定位、并归属工具与集成分组', () => {
    const trail = findNavTrail('/tools/runs');
    expect(trail?.item.to).toBe('/tools/runs');
    expect(trail?.item.derived).toBe(true);
    expect(trail?.item.derivedFrom).toBe('/tools');
    expect(trail?.section.labelKey).toBe('shell.nav.section.runs');
    expect(trail?.group.labelKey).toBe('menu.groupTools');
  });
});
