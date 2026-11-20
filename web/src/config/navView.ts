import { sideNavGroups, type NavItem } from './sideNav';

/**
 * 侧栏「组 → 段 → 项」的派生视图。
 *
 * 为什么不在 sideNav.ts 里直接改结构：
 * - 后端 internal/biz/menu.go 的目录与 sideNav.ts 有逐项漂移测试
 *   （internal/biz/menu_test.go），重排/新增会连带 Go 侧改动；
 * - 本视图只做展示分层，入口集合、顺序与权限码完全来自 sideNavGroups，
 *   并对「运行记录」类二级页追加前端派生入口（权限随父项）。
 */
export type NavLeaf = NavItem & {
  /** 前端派生入口：不在后端菜单目录中，权限跟随 derivedFrom */
  derived?: boolean;
  derivedFrom?: string;
};

export type NavSection = {
  key: string;
  /** 空字符串表示单段分组，不渲染段标题 */
  labelKey: string;
  items: NavLeaf[];
};

export type NavGroupView = {
  key: string;
  labelKey: string;
  sections: NavSection[];
};

type SectionDef = {
  key: string;
  labelKey: string;
  paths: string[];
  derived?: NavLeaf[];
};

const RUN_ENTRIES: NavLeaf[] = [
  { to: '/tools/runs', icon: 'play_circle', labelKey: 'shell.nav.toolRuns', derived: true, derivedFrom: '/tools' },
  { to: '/tools/audits', icon: 'policy', labelKey: 'shell.nav.toolAudits', derived: true, derivedFrom: '/tools' },
  { to: '/skills/runs', icon: 'play_circle', labelKey: 'shell.nav.skillRuns', derived: true, derivedFrom: '/skills' },
  {
    to: '/plugins/runs',
    icon: 'play_circle',
    labelKey: 'shell.nav.pluginRuns',
    derived: true,
    derivedFrom: '/plugins',
  },
  {
    to: '/hooks/deliveries',
    icon: 'local_shipping',
    labelKey: 'shell.nav.hookDeliveries',
    derived: true,
    derivedFrom: '/hooks',
  },
];

// 任务页与今日页同源（我的任务、交办中），不单独进后端菜单目录，可见性跟随今日页
const TASKS_ENTRY: NavLeaf = {
  to: '/tasks',
  icon: 'checklist',
  labelKey: 'shell.nav.tab.tasks',
  derived: true,
  derivedFrom: '/workbench',
};

// 健康与告警（2026-11-19 从监控页拆出）：前端派生入口，可见性跟随 /monitor/logs
const HEALTH_ENTRY: NavLeaf = {
  to: '/monitor/health',
  icon: 'health_and_safety',
  labelKey: 'ops.nav.health',
  derived: true,
  derivedFrom: '/monitor/logs',
};

const SECTION_DEFS: Record<string, SectionDef[]> = {
  'menu.groupWorkspace': [
    {
      key: 'today',
      labelKey: 'shell.nav.section.today',
      paths: ['/workbench', '/overview', '/task-workspace', '/approvals'],
      derived: [TASKS_ENTRY],
    },
    {
      key: 'collab',
      labelKey: 'shell.nav.section.collab',
      paths: ['/chat', '/sessions', '/coding-tasks', '/fleet', '/companion'],
    },
    {
      key: 'office',
      labelKey: 'shell.nav.section.office',
      paths: ['/schedule', '/apps', '/mail'],
    },
    {
      key: 'insight',
      labelKey: 'shell.nav.section.insight',
      paths: ['/ceo/company', '/ceo/overview', '/ceo/oncall', '/usage/events', '/memory'],
    },
  ],
  'menu.groupAgents': [
    { key: 'roster', labelKey: 'shell.nav.section.roster', paths: ['/agents', '/settings/organization'] },
    { key: 'orchestrate', labelKey: 'shell.nav.section.orchestrate', paths: ['/team', '/graphs'] },
  ],
  'menu.groupModels': [{ key: 'all', labelKey: '', paths: ['/models', '/channels'] }],
  'menu.groupTools': [
    { key: 'catalog', labelKey: 'shell.nav.section.catalog', paths: ['/tools', '/mcp-servers', '/skills', '/plugins'] },
    { key: 'runs', labelKey: 'shell.nav.section.runs', paths: [], derived: RUN_ENTRIES },
    {
      key: 'skillGov',
      labelKey: 'shell.nav.section.skillGov',
      paths: ['/skills/tags', '/skills/evolution-suggestions', '/skills/experience-reports'],
    },
    {
      key: 'connect',
      labelKey: 'shell.nav.section.connect',
      paths: ['/hooks', '/webhooks', '/a2a', '/credentials'],
    },
    { key: 'coding', labelKey: 'shell.nav.section.coding', paths: ['/settings/coding-tools'] },
  ],
  'menu.groupMediaCommerce': [
    { key: 'content', labelKey: 'shell.nav.section.content', paths: ['/ecommerce-loop', '/content-inbox'] },
    {
      key: 'review',
      labelKey: 'shell.nav.section.review',
      paths: ['/review-queue', '/opportunity-board', '/sourcing'],
    },
  ],
  'menu.groupKnowledge': [
    { key: 'assets', labelKey: 'shell.nav.section.assets', paths: ['/knowledge', '/artifacts'] },
    { key: 'quality', labelKey: 'shell.nav.section.quality', paths: ['/evaluation'] },
  ],
  'menu.groupOps': [
    {
      key: 'schedule',
      labelKey: 'shell.nav.section.schedule',
      paths: ['/cron', '/monitor/live', '/monitor/logs'],
      derived: [HEALTH_ENTRY],
    },
    { key: 'improve', labelKey: 'shell.nav.section.improve', paths: ['/self-improvement', '/settings'] },
  ],
};

function buildGroup(group: { labelKey: string; items: NavItem[] }, defs: SectionDef[]): NavGroupView {
  const byPath = new Map(group.items.map((item) => [item.to, item]));
  const used = new Set<string>();
  const sections: NavSection[] = defs.map((def) => {
    const items: NavLeaf[] = [];
    for (const path of def.paths) {
      const item = byPath.get(path);
      if (item) {
        items.push(item);
        used.add(path);
      }
    }
    return {
      key: `${group.labelKey}:${def.key}`,
      labelKey: def.labelKey,
      items: [...items, ...(def.derived ?? [])],
    };
  });
  const rest = group.items.filter((item) => !used.has(item.to));
  if (rest.length > 0) {
    sections.push({ key: `${group.labelKey}:other`, labelKey: 'shell.nav.section.other', items: rest });
  }
  return {
    key: group.labelKey,
    labelKey: group.labelKey,
    sections: sections.filter((section) => section.items.length > 0),
  };
}

export const navGroupsView: NavGroupView[] = sideNavGroups.map((group) =>
  buildGroup(group, SECTION_DEFS[group.labelKey] ?? []),
);

export type NavTrail = { group: NavGroupView; section: NavSection; item: NavLeaf };

/**
 * 路由前缀与导航入口不一致时的别名（详情页路径 → 导航入口前缀）。
 * 例如 /teams/:id/orchestrate 属于侧栏的 /team「协作团队」。
 */
const PATH_ALIASES: Array<[string, string]> = [['/teams', '/team']];

function normalizePath(path: string): string {
  for (const [from, to] of PATH_ALIASES) {
    if (path === from || path.startsWith(`${from}/`)) return `${to}${path.slice(from.length)}`;
  }
  return path;
}

/** 最长前缀匹配：详情页（/agents/1/settings）也能定位到所属入口与分组。 */
export function findNavTrail(rawPath: string): NavTrail | null {
  const path = normalizePath(rawPath);
  let best: NavTrail | null = null;
  let bestLength = -1;
  for (const group of navGroupsView) {
    for (const section of group.sections) {
      for (const item of section.items) {
        const hit = path === item.to || path.startsWith(`${item.to}/`);
        if (hit && item.to.length > bestLength) {
          best = { group, section, item };
          bestLength = item.to.length;
        }
      }
    }
  }
  return best;
}

/** 扁平入口列表（命令面板、侧栏筛选共用）。 */
export function flatNavLeaves(): Array<NavLeaf & { groupKey: string; sectionKey: string; sectionLabelKey: string }> {
  return navGroupsView.flatMap((group) =>
    group.sections.flatMap((section) =>
      section.items.map((item) => ({
        ...item,
        groupKey: group.labelKey,
        sectionKey: section.key,
        sectionLabelKey: section.labelKey,
      })),
    ),
  );
}

/** 派生入口的可见性跟随父入口，避免刷新后从侧栏消失。 */
export function navLeafVisible(leaf: NavLeaf, isVisible: (path: string) => boolean): boolean {
  return leaf.derived && leaf.derivedFrom ? isVisible(leaf.derivedFrom) : isVisible(leaf.to);
}

export function sectionOf(path: string): string | null {
  return findNavTrail(path)?.section.key ?? null;
}
