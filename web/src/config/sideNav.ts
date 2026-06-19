export type NavItem = {
  to: string;
  icon: string;
  labelKey: string;
  /** 路由 exact，默认 true */
  exact?: boolean;
};

export type NavGroup = {
  labelKey: string;
  items: NavItem[];
};

export const sideNavGroups: NavGroup[] = [
  {
    labelKey: 'menu.groupWorkspace',
    items: [
      { to: '/overview', icon: 'dashboard', labelKey: 'menu.overview' },
      { to: '/usage/events', icon: 'receipt_long', labelKey: 'menu.usageEvents' },
      { to: '/chat', icon: 'chat', labelKey: 'menu.chat' },
      { to: '/sessions', icon: 'history', labelKey: 'menu.sessions', exact: false },
      { to: '/memory', icon: 'psychology', labelKey: 'menu.memory', exact: false },
    ],
  },
  {
    labelKey: 'menu.groupAgents',
    items: [
      { to: '/agents', icon: 'smart_toy', labelKey: 'menu.agents' },
      { to: '/settings/organization', icon: 'business', labelKey: 'menu.organization' },
      { to: '/team', icon: 'groups', labelKey: 'menu.team' },
      { to: '/graphs', icon: 'hub', labelKey: 'menu.graphs' },
    ],
  },
  {
    labelKey: 'menu.groupModels',
    items: [
      { to: '/models', icon: 'model_training', labelKey: 'menu.models' },
      { to: '/channels', icon: 'hub', labelKey: 'menu.channels' },
    ],
  },
  {
    labelKey: 'menu.groupTools',
    items: [
      { to: '/mcp-servers', icon: 'extension', labelKey: 'menu.mcp' },
      { to: '/tools', icon: 'handyman', labelKey: 'menu.tools' },
      { to: '/skills', icon: 'psychology', labelKey: 'menu.skills' },
      { to: '/skills/evolution-suggestions', icon: 'auto_fix_high', labelKey: 'menu.evolutionSuggestions' },
      { to: '/skills/experience-reports', icon: 'assessment', labelKey: 'menu.experienceReports' },
      { to: '/plugins', icon: 'tune', labelKey: 'menu.plugins' },
      { to: '/hooks', icon: 'link', labelKey: 'menu.hooks' },
      { to: '/webhooks', icon: 'webhook', labelKey: 'menu.webhooks' },
      { to: '/a2a', icon: 'swap_horiz', labelKey: 'menu.a2a' },
    ],
  },
  {
    labelKey: 'menu.groupKnowledge',
    items: [
      { to: '/knowledge', icon: 'menu_book', labelKey: 'menu.knowledge' },
      { to: '/artifacts', icon: 'inventory_2', labelKey: 'menu.artifacts' },
      { to: '/evaluation', icon: 'fact_check', labelKey: 'menu.evaluation' },
    ],
  },
  {
    labelKey: 'menu.groupOps',
    items: [
      { to: '/observability', icon: 'insights', labelKey: 'menu.observability' },
      { to: '/cron', icon: 'schedule', labelKey: 'menu.cron' },
      { to: '/monitor/logs', icon: 'monitor_heart', labelKey: 'menu.monitor', exact: false },
      { to: '/shop', icon: 'storefront', labelKey: 'menu.shop' },
      { to: '/settings', icon: 'settings', labelKey: 'menu.settings' },
    ],
  },
];
