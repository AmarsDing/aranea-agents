import type { RouteRecordRaw } from 'vue-router';

// 全部路由组件懒加载（() => import(...)）。
// 背景（2026-11-09 P0-1）：此前 55+ 页面全部静态 import，入口 chunk 与路由 chunk
// 在 rolldown 分包下形成跨 chunk 循环初始化——入口 chunk 静态引用路由 chunk，
// 路由 chunk 又经共享模块（registryTableColumns 的 REGISTRY_COL_W）静态引用入口 chunk。
// 模块顶层求值读到未初始化的绑定，生产构建整页白屏
// （TypeError: Cannot read properties of undefined (reading 'category')）。
// 路由懒加载后入口不再静态依赖任何页面 chunk，从模块图层面消除该循环；
// 同时获得路由级代码分割，显著降低首屏加载体积。
// 注意：新增页面务必保持懒加载写法，不得改回静态 import。

export const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    component: () => import('../layouts/BlankLayout.vue'),
    meta: { public: true },
    children: [{ path: '', name: 'login', component: () => import('../pages/LoginPage.vue') }],
  },
  {
    // Mobile (Android shell) first-run backend address setup. Public: the
    // session cookie lives on the upstream that is not configured yet.
    path: '/mobile/server-setup',
    component: () => import('../layouts/BlankLayout.vue'),
    meta: { public: true },
    children: [{ path: '', name: 'mobile-server-setup', component: () => import('../pages/mobile/ServerSetupPage.vue') }],
  },
  {
    // Mobile layout. Sessions/chat/tasks are real views (P1); me remains a
    // placeholder until its P1 tasks land. The breakpoint guard in
    // router/index.ts redirects narrow screens here and wide screens back
    // to the desktop routes.
    path: '/mobile',
    component: () => import('../layouts/MobileLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      { path: '', redirect: '/mobile/sessions' },
      { path: 'sessions', name: 'mobile-sessions', component: () => import('../pages/mobile/MobileSessionsPage.vue') },
      { path: 'chat', name: 'mobile-chat', component: () => import('../pages/mobile/MobileChatPage.vue') },
      {
        path: 'task-workspace',
        name: 'mobile-task-workspace',
        component: () => import('../pages/TaskWorkspacePage.vue'),
      },
      { path: 'tasks', name: 'mobile-tasks', component: () => import('../pages/mobile/MobileTasksPage.vue') },
      { path: 'me', name: 'mobile-me', component: () => import('../pages/mobile/MobilePlaceholderPage.vue') },
    ],
  },
  {
    // 语音伴侣 HUD（M74）：独立全屏形态，无主导航侧栏；桌面端手动访问，
    // Tauri 默认入口页可配置为此路由（设计 §7.1）。窄屏被断点守卫重定向到 /mobile。
    path: '/companion',
    component: () => import('../layouts/BlankLayout.vue'),
    meta: { requiresAuth: true },
    children: [{ path: '', name: 'companion', component: () => import('../pages/CompanionPage.vue') }],
  },
  {
    path: '/',
    component: () => import('../layouts/MainLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      { path: '', redirect: '/overview' },
      { path: 'overview', name: 'overview', component: () => import('../pages/OverviewPage.vue') },
      { path: 'workbench', name: 'workbench', component: () => import('../pages/WorkbenchCockpitPage.vue') },
      { path: 'ecommerce-loop', name: 'ecommerce-loop', component: () => import('../pages/EcommerceLoopPage.vue') },
      { path: 'content-inbox', name: 'content-inbox', component: () => import('../pages/ContentInboxPage.vue') },
      { path: 'review-queue', name: 'review-queue', component: () => import('../pages/ReviewQueuePage.vue') },
      { path: 'opportunity-board', name: 'opportunity-board', component: () => import('../pages/OpportunityBoardPage.vue') },
      { path: 'sourcing', name: 'sourcing', component: () => import('../pages/SourcingPage.vue') },
      { path: 'usage/events', name: 'usage-events', component: () => import('../pages/UsageEventsPage.vue') },
      { path: 'usage/quotas', redirect: { name: 'agents' } },
      { path: 'chat', name: 'chat', component: () => import('../pages/ChatPage.vue') },
      {
        path: 'task-workspace',
        name: 'task-workspace',
        component: () => import('../pages/TaskWorkspacePage.vue'),
      },
      { path: 'sessions', name: 'sessions', component: () => import('../pages/SessionsPage.vue') },
      { path: 'sessions/:sessionId', name: 'session-detail', component: () => import('../pages/SessionDetailPage.vue') },
      { path: 'memory', name: 'memory', component: () => import('../pages/MemoryCenterPage.vue') },
      { path: 'agents', name: 'agents', component: () => import('../pages/AgentsPage.vue') },
      { path: 'settings/taxonomy', redirect: '/settings/organization' },
      { path: 'settings/organization', name: 'organization', component: () => import('../pages/OrganizationPage.vue') },
      { path: 'agents/:id/settings', name: 'agent-settings', component: () => import('../pages/AgentSettingsPage.vue') },
      { path: 'team', name: 'team', component: () => import('../pages/TeamsPage.vue') },
      {
        path: 'teams/:teamId/runs/:runId/observatory',
        name: 'team-run-observatory',
        component: () => import('../pages/TeamRunObservatoryPage.vue'),
      },
      { path: 'teams/:teamId/orchestrate', name: 'team-orchestrate', component: () => import('../pages/TeamOrchestratePage.vue') },
      { path: 'graphs', name: 'graphs', component: () => import('../pages/GraphsPage.vue') },
      { path: 'graphs/new', name: 'graph-editor-new', component: () => import('../pages/GraphEditorPage.vue') },
      { path: 'graphs/:id', name: 'graph-editor', component: () => import('../pages/GraphEditorPage.vue') },
      { path: 'graphs/:id/run/:execId', name: 'graph-run', component: () => import('../pages/GraphRunPage.vue') },
      { path: 'graphs/:id/executions', name: 'graph-executions', component: () => import('../pages/GraphExecutionsPage.vue') },
      {
        path: 'models',
        name: 'models',
        component: () => import('../pages/ResourceManagerPage.vue'),
        meta: {
          resource: 'llm-provider-models',
          title: '模型管理',
          subtitle: '维护 Provider/Model 可用清单与模型校验来源。',
        },
      },
      {
        path: 'channels',
        name: 'channels',
        component: () => import('../pages/ChannelsPage.vue'),
        meta: {
          resource: 'channels',
          title: 'Channel 管理',
          subtitle: '管理外部消息渠道、凭据引用与 Agent 绑定配置。',
        },
      },
      {
        path: 'mcp',
        name: 'mcp',
        redirect: '/mcp-servers',
      },
      {
        path: 'mcp-servers',
        name: 'mcp-servers',
        component: () => import('../pages/McpServersPage.vue'),
      },
      {
        path: 'settings/coding-tools',
        name: 'coding-tools',
        component: () => import('../pages/CodingToolsPage.vue'),
      },
      {
        path: 'coding-tasks',
        name: 'coding-tasks',
        component: () => import('../pages/CodingTasksPage.vue'),
      },
      {
        path: 'skills',
        name: 'skills',
        component: () => import('../pages/SkillsPage.vue'),
      },
      {
        path: 'skills/runs',
        name: 'skill-runs',
        component: () => import('../pages/SkillRunsPage.vue'),
      },
      {
        path: 'skills/evolution-suggestions',
        name: 'evolution-suggestions',
        component: () => import('../pages/EvolutionSuggestionListPage.vue'),
      },
      {
        path: 'skills/experience-reports',
        name: 'experience-reports',
        component: () => import('../pages/ExperienceReportListPage.vue'),
      },
      {
        path: 'skills/tags',
        name: 'skill-tags',
        component: () => import('../pages/SkillTagsPage.vue'),
      },
      {
        path: 'skills/:skillId',
        name: 'skill-detail',
        component: () => import('../pages/SkillDetailPage.vue'),
      },
      {
        path: 'plugins',
        name: 'plugins',
        component: () => import('../pages/PluginsPage.vue'),
      },
      {
        path: 'plugins/runs',
        name: 'plugin-runs',
        component: () => import('../pages/PluginRunsPage.vue'),
      },
      {
        path: 'hooks',
        name: 'hooks',
        component: () => import('../pages/HooksPage.vue'),
      },
      {
        path: 'hooks/deliveries',
        name: 'hook-deliveries',
        component: () => import('../pages/HookDeliveriesPage.vue'),
      },
      {
        path: 'webhooks',
        name: 'webhooks',
        component: () => import('../pages/WebhooksPage.vue'),
      },
      {
        path: 'knowledge',
        name: 'knowledge',
        component: () => import('../pages/KnowledgePage.vue'),
      },
      {
        path: 'artifacts',
        name: 'artifacts',
        component: () => import('../pages/ArtifactsPage.vue'),
      },
      {
        path: 'evaluation',
        name: 'evaluation',
        component: () => import('../pages/EvaluationPage.vue'),
      },
      {
        path: 'a2a',
        name: 'a2a',
        component: () => import('../pages/A2APage.vue'),
      },
      {
        path: 'tools/audits',
        name: 'tool-audits',
        component: () => import('../pages/ToolAuditsPage.vue'),
      },
      {
        path: 'tools/runs',
        name: 'tool-runs',
        component: () => import('../pages/ToolRunsPage.vue'),
      },
      {
        path: 'tools',
        name: 'tools',
        component: () => import('../pages/ToolsPage.vue'),
      },
      {
        path: 'cron',
        name: 'cron',
        component: () => import('../pages/CronTasksPage.vue'),
      },
      { path: 'monitor/logs', name: 'monitor-logs', component: () => import('../pages/MonitorPage.vue') },
      { path: 'self-improvement', name: 'self-improvement', component: () => import('../pages/SelfImprovementPage.vue') },
      { path: 'settings', name: 'settings', component: () => import('../pages/SystemSettingsPage.vue'), meta: { titleKey: 'menu.settings' } },
      ...(import.meta.env.DEV
        ? [{ path: 'dev/theme-preview', name: 'theme-preview', component: () => import('../pages/ThemePreviewPage.vue') }]
        : []),
    ],
  },
  // Catch-all：未匹配路径（如 /teams 笔误或过期书签）重定向到 overview，
  // 避免 RouterView 渲染 null vnode 导致整页空白崩溃。
  { path: '/:pathMatch(.*)*', redirect: '/overview' },
];
