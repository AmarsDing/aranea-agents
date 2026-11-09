export type NavItem = {
  to: string;
  icon: string;
  labelKey: string;
  /** 路由 exact，默认 true */
  exact?: boolean;
  /**
   * P1-7：可见所需权限码（空 = 仅需登录）。取值与后端
   * internal/biz/menu.go 的目录逐项一致，由 Go 漂移测试守护
   * （internal/biz/menu_test.go TestNavigationCatalogMatchesWebSideNav）。
   *
   * 注意：这里只是**展示层过滤**——真正的判定在后端中间件与业务层。
   * 客户端过滤失败（接口不可用）时应放行展示，因为未授权请求仍会被后端拒绝。
   */
  permission?: string;
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
      { to: '/workbench', icon: 'track_changes', labelKey: 'menu.cockpit' },
      { to: '/ceo/overview', icon: 'monitoring', labelKey: 'menu.ceoOverview', permission: 'ceoboard.view' },
      { to: '/ceo/oncall', icon: 'engineering', labelKey: 'menu.ceoOncall', permission: 'ceoboard.view' },
      { to: '/task-workspace', icon: 'view_quilt', labelKey: 'menu.taskWorkspace' },
      { to: '/usage/events', icon: 'receipt_long', labelKey: 'menu.usageEvents', permission: 'usage.read' },
      { to: '/chat', icon: 'chat', labelKey: 'menu.chat' },
      { to: '/coding-tasks', icon: 'terminal', labelKey: 'menu.codingTasks' },
      { to: '/companion', icon: 'record_voice_over', labelKey: 'menu.companion' },
      { to: '/sessions', icon: 'history', labelKey: 'menu.sessions', exact: false },
      { to: '/memory', icon: 'psychology', labelKey: 'menu.memory', exact: false, permission: 'memory.read' },
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
      { to: '/settings/coding-tools', icon: 'code', labelKey: 'menu.codingTools' },
      { to: '/tools', icon: 'handyman', labelKey: 'menu.tools' },
      { to: '/skills', icon: 'psychology', labelKey: 'menu.skills' },
      { to: '/skills/tags', icon: 'sell', labelKey: 'menu.skillTags' },
      { to: '/skills/evolution-suggestions', icon: 'auto_fix_high', labelKey: 'menu.evolutionSuggestions' },
      { to: '/skills/experience-reports', icon: 'assessment', labelKey: 'menu.experienceReports' },
      { to: '/plugins', icon: 'tune', labelKey: 'menu.plugins' },
      { to: '/hooks', icon: 'link', labelKey: 'menu.hooks' },
      { to: '/webhooks', icon: 'webhook', labelKey: 'menu.webhooks' },
      { to: '/a2a', icon: 'swap_horiz', labelKey: 'menu.a2a' },
    ],
  },
  {
    labelKey: 'menu.groupMediaCommerce',
    items: [
      { to: '/ecommerce-loop', icon: 'shopping_bag', labelKey: 'menu.ecommerceLoop' },
      { to: '/content-inbox', icon: 'inbox', labelKey: 'menu.contentInbox' },
      { to: '/review-queue', icon: 'rate_review', labelKey: 'menu.reviewQueue' },
      { to: '/opportunity-board', icon: 'storefront', labelKey: 'menu.opportunityBoard' },
      { to: '/sourcing', icon: 'travel_explore', labelKey: 'menu.sourcing' },
    ],
  },
  {
    labelKey: 'menu.groupKnowledge',
    items: [
      { to: '/knowledge', icon: 'menu_book', labelKey: 'menu.knowledge', permission: 'knowledge.read' },
      { to: '/artifacts', icon: 'inventory_2', labelKey: 'menu.artifacts', permission: 'knowledge.read' },
      { to: '/evaluation', icon: 'fact_check', labelKey: 'menu.evaluation' },
    ],
  },
  {
    labelKey: 'menu.groupOps',
    items: [
      { to: '/cron', icon: 'schedule', labelKey: 'menu.cron', permission: 'system.cron' },
      { to: '/monitor/logs', icon: 'monitor_heart', labelKey: 'menu.monitor', exact: false, permission: 'system.monitor' },
      { to: '/self-improvement', icon: 'precision_manufacturing', labelKey: 'menu.selfImprovement', permission: 'system.self_improve' },
      { to: '/settings', icon: 'settings', labelKey: 'menu.settings', permission: 'system.setting' },
    ],
  },
];
