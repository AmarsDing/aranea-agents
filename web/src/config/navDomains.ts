/**
 * 八个一级去处（导航重组，2026-11-17）。
 * 设计：docs/aranea-agents/design/2026-11-17-导航重组方案.md
 *
 * 只是展示层的重新归组：入口集合、路径、权限码全部来自 sideNav.ts / navView.ts，
 * 后端菜单目录（internal/biz/menu.go）不动，URL 不变。
 *  - primaries：侧栏与页面顶部「领域页签」里出现的页面
 *  - children：只在父页面的子页签里出现（运行记录、审计、标签字典等）
 */
import { flatNavLeaves, navLeafVisible, type NavLeaf } from './navView';

export type NavDomainKey =
  | 'workbench'
  | 'business'
  | 'conversation'
  | 'agents'
  | 'capability'
  | 'knowledge'
  | 'ops'
  | 'settings';

type PrimaryDef = {
  path: string;
  /** 在领域页签里换一个更贴切的叫法（不改原菜单文案） */
  labelKey?: string;
  children?: string[];
};

type DomainDef = {
  key: NavDomainKey;
  icon: string;
  primaries: PrimaryDef[];
};

export const NAV_DOMAIN_DEFS: DomainDef[] = [
  {
    key: 'workbench',
    icon: 'track_changes',
    primaries: [
      { path: '/workbench', labelKey: 'shell.nav.tab.today' },
      { path: '/approvals', labelKey: 'shell.nav.tab.inboxHistory' },
      { path: '/schedule' },
      { path: '/mail' },
      { path: '/apps' },
    ],
  },
  {
    key: 'business',
    icon: 'account_tree',
    primaries: [
      { path: '/ceo/company' },
      { path: '/ceo/overview' },
      { path: '/ecommerce-loop' },
      { path: '/content-inbox' },
      { path: '/review-queue' },
      { path: '/opportunity-board' },
      { path: '/sourcing' },
    ],
  },
  {
    key: 'conversation',
    icon: 'forum',
    primaries: [
      { path: '/chat' },
      { path: '/task-workspace' },
      { path: '/sessions' },
      { path: '/coding-tasks' },
      { path: '/fleet' },
      { path: '/companion' },
    ],
  },
  {
    key: 'agents',
    icon: 'smart_toy',
    primaries: [{ path: '/agents' }, { path: '/settings/organization' }, { path: '/team' }, { path: '/graphs' }],
  },
  {
    key: 'capability',
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
    ],
  },
  {
    key: 'knowledge',
    icon: 'menu_book',
    primaries: [{ path: '/knowledge' }, { path: '/memory' }, { path: '/artifacts' }, { path: '/evaluation' }],
  },
  {
    key: 'ops',
    icon: 'monitor_heart',
    primaries: [
      { path: '/monitor/live' },
      { path: '/cron' },
      { path: '/monitor/logs' },
      { path: '/ceo/oncall' },
      { path: '/usage/events' },
      { path: '/overview', labelKey: 'shell.nav.tab.modelUsage' },
      { path: '/self-improvement' },
    ],
  },
  {
    key: 'settings',
    icon: 'tune',
    primaries: [
      { path: '/settings' },
      { path: '/models' },
      { path: '/channels' },
      { path: '/credentials' },
      { path: '/hooks', children: ['/hooks/deliveries'] },
      { path: '/webhooks' },
      { path: '/a2a' },
    ],
  },
];

export type DomainPage = NavLeaf & { tabLabelKey: string };
export type DomainPrimary = DomainPage & { children: DomainPage[] };
export type NavDomain = {
  key: NavDomainKey;
  icon: string;
  labelKey: string;
  shortLabelKey: string;
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
 * 菜单里新增了却还没登记到任何领域的入口，追加到「设置与接入」末尾，避免入口凭空消失
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
      icon: def.icon,
      labelKey: `shell.nav.domain.${def.key}`,
      shortLabelKey: `shell.nav.domainShort.${def.key}`,
      primaries,
    };
  }).filter((domain) => domain.primaries.length > 0);
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
