import type { RouteRecordRaw } from 'vue-router';

export const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    component: () => import('../layouts/BlankLayout.vue'),
    meta: { public: true },
    children: [
      {
        path: '',
        name: 'login',
        component: () => import('../pages/LoginPage.vue'),
        meta: { titleKey: 'shell.titles.login' },
      },
    ],
  },
  {
    // P1 补漏·强制改密门:force_reset 账号登录后唯一可达页(守卫钉住;
    // 改密成功即吊销全部会话,须用新密码重新登录)。断点守卫豁免此路径。
    path: '/force-reset-password',
    component: () => import('../layouts/BlankLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      {
        path: '',
        name: 'force-reset-password',
        component: () => import('../pages/ForceResetPasswordPage.vue'),
        meta: { titleKey: 'shell.titles.forceReset' },
      },
    ],
  },
  {
    // Mobile (Android shell) first-run backend address setup. Public: the
    // session cookie lives on the upstream that is not configured yet.
    path: '/mobile/server-setup',
    component: () => import('../layouts/BlankLayout.vue'),
    meta: { public: true },
    children: [
      {
        path: '',
        name: 'mobile-server-setup',
        component: () => import('../pages/mobile/ServerSetupPage.vue'),
        meta: { titleKey: 'shell.titles.mobileSetup' },
      },
    ],
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
      {
        path: 'sessions',
        name: 'mobile-sessions',
        component: () => import('../pages/mobile/MobileSessionsPage.vue'),
        meta: { titleKey: 'shell.titles.mobileSessions' },
      },
      {
        path: 'chat',
        name: 'mobile-chat',
        component: () => import('../pages/mobile/MobileChatPage.vue'),
        meta: { titleKey: 'shell.titles.mobileChat' },
      },
      {
        path: 'task-workspace',
        name: 'mobile-task-workspace',
        component: () => import('../pages/TaskWorkspacePage.vue'),
        meta: { titleKey: 'menu.taskWorkspace' },
      },
      {
        path: 'tasks',
        name: 'mobile-tasks',
        component: () => import('../pages/mobile/MobileTasksPage.vue'),
        meta: { titleKey: 'shell.titles.mobileTasks' },
      },
      {
        path: 'me',
        name: 'mobile-me',
        component: () => import('../pages/mobile/MobilePlaceholderPage.vue'),
        meta: { titleKey: 'shell.titles.mobileMe' },
      },
    ],
  },
  {
    // 语音伴侣 HUD（M74）：独立全屏形态，无主导航侧栏；桌面端手动访问，
    // Tauri 默认入口页可配置为此路由（设计 §7.1）。窄屏被断点守卫重定向到 /mobile。
    path: '/companion',
    component: () => import('../layouts/BlankLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      {
        path: '',
        name: 'companion',
        component: () => import('../pages/CompanionPage.vue'),
        meta: { titleKey: 'shell.titles.companion' },
      },
    ],
  },
  {
    path: '/',
    component: () => import('../layouts/MainLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      { path: '', redirect: '/workbench' },
      {
        path: 'overview',
        name: 'overview',
        component: () => import('../pages/OverviewPage.vue'),
        meta: { titleKey: 'menu.overview', breadcrumb: false, uiLayout: 'immersive', uiVisual: 'deck' },
      },
      {
        // 路由级无权限兜底：菜单白名单不含目标入口（或缺少 meta.permission 声明的权限码）时落到这里。
        // 展示层拦截，后端仍按接口硬判；菜单拉取失败时守卫放行，不会把整站锁死。
        path: '403',
        name: 'forbidden',
        component: () => import('../pages/ForbiddenPage.vue'),
        meta: { titleKey: 'shell.forbidden.title', breadcrumb: false },
      },
      {
        path: 'workbench',
        name: 'workbench',
        component: () => import('../pages/TodayPage.vue'),
        meta: { titleKey: 'menu.cockpit', breadcrumb: false, uiVisual: 'deck' },
      },
      {
        path: 'approvals',
        name: 'approvals',
        component: () => import('../pages/ApprovalsPage.vue'),
        meta: { titleKey: 'menu.approvals' },
      },
      {
        path: 'schedule',
        name: 'schedule',
        component: () => import('../pages/SchedulePage.vue'),
        meta: { titleKey: 'menu.schedule' },
      },
      { path: 'apps', name: 'apps', component: () => import('../pages/AppsPage.vue'), meta: { titleKey: 'menu.apps' } },
      { path: 'mail', name: 'mail', component: () => import('../pages/MailPage.vue'), meta: { titleKey: 'menu.mail' } },
      {
        path: 'ceo/company',
        name: 'ceo-company',
        component: () => import('../pages/CompanyTreePage.vue'),
        meta: { titleKey: 'menu.ceoCompany', breadcrumb: false, uiVisual: 'deck' },
      },
      {
        path: 'ceo/overview',
        name: 'ceo-overview',
        component: () => import('../pages/CeoOverviewPage.vue'),
        meta: { titleKey: 'menu.ceoOverview', breadcrumb: false, uiLayout: 'immersive', uiVisual: 'deck' },
      },
      {
        path: 'ceo/oncall',
        name: 'ceo-oncall',
        component: () => import('../pages/CeoOncallPage.vue'),
        meta: { titleKey: 'menu.ceoOncall', breadcrumb: false, uiLayout: 'immersive', uiVisual: 'deck' },
      },
      {
        path: 'ecommerce-loop',
        name: 'ecommerce-loop',
        component: () => import('../pages/EcommerceLoopPage.vue'),
        meta: { titleKey: 'menu.ecommerceLoop' },
      },
      {
        path: 'content-inbox',
        name: 'content-inbox',
        component: () => import('../pages/ContentInboxPage.vue'),
        meta: { titleKey: 'menu.contentInbox' },
      },
      {
        path: 'review-queue',
        name: 'review-queue',
        component: () => import('../pages/ReviewQueuePage.vue'),
        meta: { titleKey: 'menu.reviewQueue' },
      },
      {
        path: 'opportunity-board',
        name: 'opportunity-board',
        component: () => import('../pages/OpportunityBoardPage.vue'),
        meta: { titleKey: 'menu.opportunityBoard' },
      },
      {
        path: 'sourcing',
        name: 'sourcing',
        component: () => import('../pages/SourcingPage.vue'),
        meta: { titleKey: 'menu.sourcing' },
      },
      {
        path: 'usage/events',
        name: 'usage-events',
        component: () => import('../pages/UsageEventsPage.vue'),
        meta: { titleKey: 'menu.usageEvents' },
      },
      { path: 'usage/quotas', redirect: { name: 'agents' } },
      {
        path: 'chat',
        name: 'chat',
        component: () => import('../pages/ChatPage.vue'),
        meta: { titleKey: 'menu.chat', breadcrumb: false, uiLayout: 'immersive' },
      },
      {
        path: 'task-workspace',
        name: 'task-workspace',
        component: () => import('../pages/TaskWorkspacePage.vue'),
        meta: { titleKey: 'menu.taskWorkspace' },
      },
      {
        path: 'sessions',
        name: 'sessions',
        component: () => import('../pages/SessionsPage.vue'),
        meta: { titleKey: 'menu.sessions' },
      },
      {
        path: 'sessions/:sessionId',
        name: 'session-detail',
        component: () => import('../pages/SessionDetailPage.vue'),
        meta: { titleKey: 'shell.titles.sessionDetail' },
      },
      {
        path: 'memory',
        name: 'memory',
        component: () => import('../pages/MemoryCenterPage.vue'),
        meta: { titleKey: 'menu.memory' },
      },
      {
        path: 'agents',
        name: 'agents',
        component: () => import('../pages/AgentsPage.vue'),
        meta: { titleKey: 'menu.agents' },
      },
      { path: 'settings/taxonomy', redirect: '/settings/organization' },
      {
        path: 'settings/organization',
        name: 'organization',
        component: () => import('../pages/OrganizationPage.vue'),
        meta: { titleKey: 'menu.organization' },
      },
      {
        path: 'agents/:id/settings',
        name: 'agent-settings',
        component: () => import('../pages/AgentSettingsPage.vue'),
        meta: { titleKey: 'shell.titles.agentSettings' },
      },
      {
        path: 'team',
        name: 'team',
        component: () => import('../pages/TeamsPage.vue'),
        meta: { titleKey: 'menu.team' },
      },
      {
        path: 'teams/:teamId/runs/:runId/observatory',
        name: 'team-run-observatory',
        component: () => import('../pages/TeamRunObservatoryPage.vue'),
        meta: { titleKey: 'shell.titles.teamObservatory' },
      },
      {
        path: 'teams/:teamId/orchestrate',
        name: 'team-orchestrate',
        component: () => import('../pages/TeamOrchestratePage.vue'),
        meta: { titleKey: 'shell.titles.teamOrchestrate', uiLayout: 'canvas' },
      },
      {
        path: 'graphs',
        name: 'graphs',
        component: () => import('../pages/GraphsPage.vue'),
        meta: { titleKey: 'menu.graphs' },
      },
      {
        path: 'graphs/new',
        name: 'graph-editor-new',
        component: () => import('../pages/GraphEditorPage.vue'),
        meta: { titleKey: 'shell.titles.graphNew', uiLayout: 'canvas' },
      },
      {
        path: 'graphs/:id',
        name: 'graph-editor',
        component: () => import('../pages/GraphEditorPage.vue'),
        meta: { titleKey: 'shell.titles.graphEdit', uiLayout: 'canvas' },
      },
      {
        path: 'graphs/:id/run/:execId',
        name: 'graph-run',
        component: () => import('../pages/GraphRunPage.vue'),
        meta: { titleKey: 'shell.titles.graphRun' },
      },
      {
        path: 'graphs/:id/executions',
        name: 'graph-executions',
        component: () => import('../pages/GraphExecutionsPage.vue'),
        meta: { titleKey: 'shell.titles.graphExecutions' },
      },
      {
        path: 'models',
        name: 'models',
        component: () => import('../pages/ResourceManagerPage.vue'),
        meta: {
          resource: 'llm-provider-models',
          titleKey: 'menu.models',
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
          titleKey: 'menu.channels',
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
        meta: { titleKey: 'menu.mcp' },
      },
      {
        path: 'settings/coding-tools',
        name: 'coding-tools',
        component: () => import('../pages/CodingToolsPage.vue'),
        meta: { titleKey: 'menu.codingTools' },
      },
      {
        path: 'coding-tasks',
        name: 'coding-tasks',
        component: () => import('../pages/CodingTasksPage.vue'),
        meta: { titleKey: 'menu.codingTasks' },
      },
      {
        // Harness 多任务运行台（复刻 Orca：worktree + agent 终端 + diff 批注 + 编排 + 自动化）。
        path: 'fleet',
        name: 'fleet',
        component: () => import('../pages/FleetPage.vue'),
        meta: { titleKey: 'menu.fleet', breadcrumb: false },
      },
      {
        path: 'skills',
        name: 'skills',
        component: () => import('../pages/SkillsPage.vue'),
        meta: { titleKey: 'menu.skills' },
      },
      {
        path: 'skills/runs',
        name: 'skill-runs',
        component: () => import('../pages/SkillRunsPage.vue'),
        meta: { titleKey: 'shell.nav.skillRuns' },
      },
      {
        path: 'skills/evolution-suggestions',
        name: 'evolution-suggestions',
        component: () => import('../pages/EvolutionSuggestionListPage.vue'),
        meta: { titleKey: 'menu.evolutionSuggestions' },
      },
      {
        path: 'skills/experience-reports',
        name: 'experience-reports',
        component: () => import('../pages/ExperienceReportListPage.vue'),
        meta: { titleKey: 'menu.experienceReports' },
      },
      {
        path: 'skills/tags',
        name: 'skill-tags',
        component: () => import('../pages/SkillTagsPage.vue'),
        meta: { titleKey: 'menu.skillTags' },
      },
      {
        path: 'skills/:skillId',
        name: 'skill-detail',
        component: () => import('../pages/SkillDetailPage.vue'),
        meta: { titleKey: 'shell.titles.skillDetail' },
      },
      {
        path: 'plugins',
        name: 'plugins',
        component: () => import('../pages/PluginsPage.vue'),
        meta: { titleKey: 'menu.plugins' },
      },
      {
        path: 'plugins/runs',
        name: 'plugin-runs',
        component: () => import('../pages/PluginRunsPage.vue'),
        meta: { titleKey: 'shell.nav.pluginRuns' },
      },
      {
        path: 'hooks',
        name: 'hooks',
        component: () => import('../pages/HooksPage.vue'),
        meta: { titleKey: 'menu.hooks' },
      },
      {
        path: 'hooks/deliveries',
        name: 'hook-deliveries',
        component: () => import('../pages/HookDeliveriesPage.vue'),
        meta: { titleKey: 'shell.nav.hookDeliveries' },
      },
      {
        path: 'webhooks',
        name: 'webhooks',
        component: () => import('../pages/WebhooksPage.vue'),
        meta: { titleKey: 'menu.webhooks' },
      },
      {
        path: 'credentials',
        name: 'credentials',
        component: () => import('../pages/CredentialsPage.vue'),
        meta: { titleKey: 'menu.credentials' },
      },
      {
        path: 'knowledge',
        name: 'knowledge',
        component: () => import('../pages/KnowledgePage.vue'),
        meta: { titleKey: 'menu.knowledge' },
      },
      {
        path: 'artifacts',
        name: 'artifacts',
        component: () => import('../pages/ArtifactsPage.vue'),
        meta: { titleKey: 'menu.artifacts' },
      },
      {
        path: 'evaluation',
        name: 'evaluation',
        component: () => import('../pages/EvaluationPage.vue'),
        meta: { titleKey: 'menu.evaluation' },
      },
      {
        path: 'a2a',
        name: 'a2a',
        component: () => import('../pages/A2APage.vue'),
        meta: { titleKey: 'menu.a2a' },
      },
      {
        path: 'tools/audits',
        name: 'tool-audits',
        component: () => import('../pages/ToolAuditsPage.vue'),
        meta: { titleKey: 'shell.nav.toolAudits' },
      },
      {
        path: 'tools/runs',
        name: 'tool-runs',
        component: () => import('../pages/ToolRunsPage.vue'),
        meta: { titleKey: 'shell.nav.toolRuns' },
      },
      {
        path: 'tools',
        name: 'tools',
        component: () => import('../pages/ToolsPage.vue'),
        meta: { titleKey: 'menu.tools' },
      },
      {
        path: 'cron',
        name: 'cron',
        component: () => import('../pages/CronTasksPage.vue'),
        meta: { titleKey: 'menu.cron' },
      },
      {
        // 原作战总台（连线图、花名册、日志流）：属于系统运行视图，归运行监控。
        path: 'monitor/live',
        name: 'monitor-live',
        component: () => import('../pages/WorkbenchCockpitPage.vue'),
        meta: { titleKey: 'menu.monitorLive', breadcrumb: false, uiLayout: 'immersive', uiVisual: 'deck' },
      },
      {
        path: 'monitor/logs',
        name: 'monitor-logs',
        component: () => import('../pages/MonitorPage.vue'),
        meta: { titleKey: 'menu.monitor' },
      },
      {
        path: 'self-improvement',
        name: 'self-improvement',
        component: () => import('../pages/SelfImprovementPage.vue'),
        meta: { titleKey: 'menu.selfImprovement' },
      },
      {
        path: 'settings',
        name: 'settings',
        component: () => import('../pages/SystemSettingsPage.vue'),
        meta: { titleKey: 'menu.settings' },
      },
      ...(import.meta.env.DEV
        ? [
            {
              path: 'dev/theme-preview',
              name: 'theme-preview',
              component: () => import('../pages/ThemePreviewPage.vue'),
              meta: { titleKey: 'shell.titles.themePreview' },
            },
          ]
        : []),
    ],
  },
  // Catch-all：未匹配路径（如 /teams 笔误或过期书签）重定向到首页（工作台），
  // 避免 RouterView 渲染 null vnode 导致整页空白崩溃。
  { path: '/:pathMatch(.*)*', redirect: '/workbench' },
];
