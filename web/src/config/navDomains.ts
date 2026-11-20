/**
 * 三个模块 → 领域 → 页面（三大模块重设计，2026-11-18；前身是 2026-11-17 的八个一级去处）。
 * 设计：E:\teamwork\docs\aranea-agents\design\2026-11-17-三大模块重设计\00-总纲.md 第 4.4、4.5 节
 *
 * 只是展示层的重新归组：入口集合、路径、权限码全部来自 sideNav.ts / navView.ts，
 * 后端菜单目录（internal/biz/menu.go）不动，URL 不变。
 *  - 模块：工作台 / 看板 / Harness，在左边的模块栏上；shell 是壳层入口（应用、设置），在模块栏底部
 *  - primaries：工作台、看板是顶栏第二行的页签；Harness 是二级侧栏里领域下的页面
 *  - children：只在父页面的子页签里出现（运行记录、审计、标签字典等）
 */
import { flatNavLeaves, navLeafVisible, type NavLeaf } from './navView';

export type NavModuleKey = 'workbench' | 'board' | 'harness' | 'shell';

/** 模块栏上的三个模块；code 是顶栏的模块代号，home 是点模块时的落点。 */
export const NAV_MODULES: Array<{ key: Exclude<NavModuleKey, 'shell'>; code: string; icon: string; home: string }> = [
  { key: 'workbench', code: 'WB', icon: 'space_dashboard', home: '/workbench' },
  { key: 'board', code: 'KB', icon: 'account_tree', home: '/ceo/company' },
  { key: 'harness', code: 'HX', icon: 'hexagon', home: '/overview' },
];

export type NavDomainKey =
  | 'workbench'
  | 'business'
  | 'home'
  | 'agents'
  | 'orchestrate'
  | 'capability'
  | 'connect'
  | 'ops'
  | 'settings';

type PrimaryDef = {
  path: string;
  /** 在页签里换一个更贴切的叫法（不改原菜单文案） */
  labelKey?: string;
  children?: string[];
};

type DomainDef = {
  key: NavDomainKey;
  module: NavModuleKey;
  icon: string;
  primaries: PrimaryDef[];
};

export const NAV_DOMAIN_DEFS: DomainDef[] = [
  // ——— 工作台：顶部 6 个页签，合并前的旧页面先挂在子页签里 ———
  {
    key: 'workbench',
    module: 'workbench',
    icon: 'space_dashboard',
    primaries: [
      { path: '/workbench', labelKey: 'shell.nav.tab.today', children: ['/approvals'] },
      { path: '/task-workspace', labelKey: 'shell.nav.tab.tasks', children: ['/coding-tasks'] },
      { path: '/chat', labelKey: 'shell.nav.tab.chat', children: ['/sessions', '/companion'] },
      { path: '/schedule' },
      { path: '/mail' },
      { path: '/knowledge', labelKey: 'shell.nav.tab.library', children: ['/artifacts'] },
    ],
  },
  // ——— 看板：总览 + 业务线（目前只有电商这一条线有页面） ———
  {
    key: 'business',
    module: 'board',
    icon: 'account_tree',
    primaries: [
      { path: '/ceo/company', labelKey: 'shell.nav.tab.boardHome', children: ['/ceo/overview'] },
      {
        path: '/ecommerce-loop',
        labelKey: 'shell.nav.tab.ecommerce',
        children: ['/content-inbox', '/review-queue', '/opportunity-board', '/sourcing'],
      },
    ],
  },
  // ——— Harness：首页 + 五个领域，在二级侧栏里 ———
  {
    key: 'home',
    module: 'harness',
    icon: 'hexagon',
    primaries: [{ path: '/overview', labelKey: 'shell.nav.tab.harnessHome' }],
  },
  {
    key: 'agents',
    module: 'harness',
    icon: 'smart_toy',
    primaries: [{ path: '/agents' }, { path: '/settings/organization' }, { path: '/team' }],
  },
  {
    key: 'orchestrate',
    module: 'harness',
    icon: 'schema',
    primaries: [{ path: '/graphs' }, { path: '/fleet' }, { path: '/cron' }],
  },
  {
    key: 'capability',
    module: 'harness',
    icon: 'extension',
    primaries: [
      { path: '/tools', children: ['/tools/runs', '/tools/audits'] },
      {
        path: '/skills',
        children: ['/skills/runs', '/skills/tags', '/skills/evolution-suggestions', '/skills/experience-reports'],
      },
      { path: '/plugins', children: ['/plugins/runs'] },
      { path: '/mcp-servers' },
      { path: '/settings/coding-tools' },
      { path: '/memory' },
    ],
  },
  {
    key: 'connect',
    module: 'harness',
    icon: 'hub',
    primaries: [
      { path: '/models' },
      { path: '/channels' },
      { path: '/credentials' },
      { path: '/webhooks' },
      { path: '/a2a' },
    ],
  },
  {
    key: 'ops',
    module: 'harness',
    icon: 'monitor_heart',
    primaries: [
      // 2026-11-19 重排：按「回答什么问题」改名，监控页拆成健康与告警 + 追踪与日志（04-Harness系统.md §12）
      { path: '/monitor/live', labelKey: 'ops.nav.live' },
      { path: '/monitor/health' },
      { path: '/monitor/logs', labelKey: 'ops.nav.traces' },
      { path: '/ceo/oncall', labelKey: 'ops.nav.flows' },
      { path: '/usage/events', labelKey: 'ops.nav.usage' },
      { path: '/evaluation', labelKey: 'ops.nav.evaluation' },
      { path: '/self-improvement', labelKey: 'ops.nav.selfImprovement' },
      { path: '/hooks', labelKey: 'ops.nav.hooks', children: ['/hooks/deliveries'] },
    ],
  },
  // ——— 壳层：模块栏底部的应用、设置 ———
  {
    key: 'settings',
    module: 'shell',
    icon: 'tune',
    primaries: [{ path: '/apps' }, { path: '/settings', labelKey: 'shell.nav.domain.settings' }],
  },
];

export type DomainPage = NavLeaf & { tabLabelKey: string };
export type DomainPrimary = DomainPage & { children: DomainPage[] };
export type NavDomain = {
  key: NavDomainKey;
  module: NavModuleKey;
  moduleLabelKey: string;
  icon: string;
  labelKey: string;
  primaries: DomainPrimary[];
};

function leafIndex(): Map<string, NavLeaf> {
  return new Map(flatNavLeaves().map((leaf) => [leaf.to, leaf]));
}

/** 已登记到某个领域的全部路径（一级 + 子页）。 */
export function assignedPaths(): Set<string> {
  const out = new Set<string>();
  for (const domain of NAV_DOMAIN_DEFS) {
    for (const primary of domain.primaries) {
      out.add(primary.path);
      for (const child of primary.children ?? []) out.add(child);
    }
  }
  return out;
}

/**
 * 按可见性构建领域树：无权限的页面不出现；一个领域下没有可见页面时整个领域隐藏。
 * 菜单里新增了却还没登记到任何领域的入口，追加到「设置」末尾，避免入口凭空消失
 * （单测会拦住这种情况，提示补登记）。
 */
export function buildNavDomains(isVisible: (path: string) => boolean): NavDomain[] {
  const leaves = leafIndex();
  const page = (path: string, labelKey?: string): DomainPage | null => {
    const leaf = leaves.get(path);
    if (!leaf || !navLeafVisible(leaf, isVisible)) return null;
    return { ...leaf, tabLabelKey: labelKey ?? leaf.labelKey };
  };
  const assigned = assignedPaths();
  const orphans = [...leaves.values()].filter((leaf) => !assigned.has(leaf.to));

  return NAV_DOMAIN_DEFS.map((def) => {
    const primaries: DomainPrimary[] = [];
    for (const p of def.primaries) {
      const head = page(p.path, p.labelKey);
      if (!head) continue;
      const children = (p.children ?? []).map((c) => page(c)).filter((c): c is DomainPage => c !== null);
      primaries.push({ ...head, children });
    }
    if (def.key === 'settings') {
      for (const leaf of orphans) {
        const extra = page(leaf.to);
        if (extra) primaries.push({ ...extra, children: [] });
      }
    }
    return {
      key: def.key,
      module: def.module,
      moduleLabelKey: `shell.nav.module.${def.module}`,
      icon: def.icon,
      labelKey: `shell.nav.domain.${def.key}`,
      primaries,
    };
  }).filter((domain) => domain.primaries.length > 0);
}

/** 点模块时的落点：首选该模块的 home；没权限看 home 时退到本模块第一个可见页面。 */
export function moduleLanding(key: NavModuleKey, domains: readonly NavDomain[]): string | null {
  const pages = domains.filter((d) => d.module === key).flatMap((d) => d.primaries.map((p) => p.to));
  const home = NAV_MODULES.find((m) => m.key === key)?.home;
  return home && pages.includes(home) ? home : (pages[0] ?? null);
}

/** 路由别名：详情页路径 → 所属入口（与 navView 保持一致）。 */
const PATH_ALIASES: Array<[string, string]> = [['/teams', '/team']];

function normalize(path: string): string {
  for (const [from, to] of PATH_ALIASES) {
    if (path === from || path.startsWith(`${from}/`)) return `${to}${path.slice(from.length)}`;
  }
  return path;
}

export type DomainTrail = {
  domain: NavDomain;
  primary: DomainPrimary;
  /** 当前所在的子页；在父页面本身时为 null */
  child: DomainPage | null;
};

/** 最长前缀匹配，详情页（/agents/42/settings）也能定位到所属领域与页签。 */
export function findDomainTrail(rawPath: string, domains: readonly NavDomain[]): DomainTrail | null {
  const path = normalize(rawPath);
  const hit = (to: string) => path === to || path.startsWith(`${to}/`);
  let best: DomainTrail | null = null;
  let bestLength = -1;
  for (const domain of domains) {
    for (const primary of domain.primaries) {
      if (hit(primary.to) && primary.to.length > bestLength) {
        best = { domain, primary, child: null };
        bestLength = primary.to.length;
      }
      for (const child of primary.children) {
        if (hit(child.to) && child.to.length > bestLength) {
          best = { domain, primary, child };
          bestLength = child.to.length;
        }
      }
    }
  }
  return best;
}
