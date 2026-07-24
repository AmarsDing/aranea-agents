import type { RouteRecordRaw } from 'vue-router';
import BlankLayout from '../layouts/BlankLayout.vue';
import LoginPage from '../pages/LoginPage.vue';
import MainLayout from '../layouts/MainLayout.vue';
import ChatPage from '../pages/ChatPage.vue';
import AgentsPage from '../pages/AgentsPage.vue';
import AgentSettingsPage from '../pages/AgentSettingsPage.vue';
import MonitorPage from '../pages/MonitorPage.vue';
import OverviewPage from '../pages/OverviewPage.vue';
import UsageEventsPage from '../pages/UsageEventsPage.vue';
import ResourceManagerPage from '../pages/ResourceManagerPage.vue';
import EcosystemPage from '../pages/EcosystemPage.vue';
import OrganizationPage from '../pages/OrganizationPage.vue';
import TeamsPage from '../pages/TeamsPage.vue';
import SkillsPage from '../pages/SkillsPage.vue';
import SkillDetailPage from '../pages/SkillDetailPage.vue';
import SkillRunsPage from '../pages/SkillRunsPage.vue';
import EvolutionSuggestionListPage from '../pages/EvolutionSuggestionListPage.vue';
import ExperienceReportListPage from '../pages/ExperienceReportListPage.vue';
import PluginsPage from '../pages/PluginsPage.vue';
import PluginRunsPage from '../pages/PluginRunsPage.vue';
import HooksPage from '../pages/HooksPage.vue';
import HookDeliveriesPage from '../pages/HookDeliveriesPage.vue';
import WebhooksPage from '../pages/WebhooksPage.vue';
import KnowledgePage from '../pages/KnowledgePage.vue';
import ArtifactsPage from '../pages/ArtifactsPage.vue';
import EvaluationPage from '../pages/EvaluationPage.vue';
import A2APage from '../pages/A2APage.vue';
import ToolsPage from '../pages/ToolsPage.vue';
import ToolRunsPage from '../pages/ToolRunsPage.vue';
import ToolAuditsPage from '../pages/ToolAuditsPage.vue';
import SessionsPage from '../pages/SessionsPage.vue';
import SessionDetailPage from '../pages/SessionDetailPage.vue';
import ChannelsPage from '../pages/ChannelsPage.vue';
import McpServersPage from '../pages/McpServersPage.vue';
import CronTasksPage from '../pages/CronTasksPage.vue';
import MemoryCenterPage from '../pages/MemoryCenterPage.vue';
import SystemSettingsPage from '../pages/SystemSettingsPage.vue';
import GraphsPage from '../pages/GraphsPage.vue';
import GraphEditorPage from '../pages/GraphEditorPage.vue';
import GraphRunPage from '../pages/GraphRunPage.vue';
import GraphExecutionsPage from '../pages/GraphExecutionsPage.vue';
import TeamRunObservatoryPage from '../pages/TeamRunObservatoryPage.vue';
import TeamOrchestratePage from '../pages/TeamOrchestratePage.vue';
import ObservabilityDashboardPage from '../pages/ObservabilityDashboardPage.vue';
import ThemePreviewPage from '../pages/ThemePreviewPage.vue';

export const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    component: BlankLayout,
    meta: { public: true },
    children: [{ path: '', name: 'login', component: LoginPage }],
  },
  {
    path: '/',
    component: MainLayout,
    meta: { requiresAuth: true },
    children: [
      { path: '', redirect: '/overview' },
      { path: 'overview', name: 'overview', component: OverviewPage },
      { path: 'usage/events', name: 'usage-events', component: UsageEventsPage },
      { path: 'usage/quotas', redirect: { name: 'agents' } },
      { path: 'chat', name: 'chat', component: ChatPage },
      { path: 'sessions', name: 'sessions', component: SessionsPage },
      { path: 'sessions/:sessionId', name: 'session-detail', component: SessionDetailPage },
      { path: 'memory', name: 'memory', component: MemoryCenterPage },
      { path: 'agents', name: 'agents', component: AgentsPage },
      { path: 'settings/taxonomy', redirect: '/settings/organization' },
      { path: 'settings/organization', name: 'organization', component: OrganizationPage },
      { path: 'agents/:id/settings', name: 'agent-settings', component: AgentSettingsPage },
      { path: 'team', name: 'team', component: TeamsPage },
      {
        path: 'teams/:teamId/runs/:runId/observatory',
        name: 'team-run-observatory',
        component: TeamRunObservatoryPage,
      },
      { path: 'teams/:teamId/orchestrate', name: 'team-orchestrate', component: TeamOrchestratePage },
      { path: 'graphs', name: 'graphs', component: GraphsPage },
      { path: 'graphs/new', name: 'graph-editor-new', component: GraphEditorPage },
      { path: 'graphs/:id', name: 'graph-editor', component: GraphEditorPage },
      { path: 'graphs/:id/run/:execId', name: 'graph-run', component: GraphRunPage },
      { path: 'graphs/:id/executions', name: 'graph-executions', component: GraphExecutionsPage },
      {
        path: 'models',
        name: 'models',
        component: ResourceManagerPage,
        meta: {
          resource: 'llm-provider-models',
          title: '模型管理',
          subtitle: '维护 Provider/Model 可用清单与模型校验来源。',
        },
      },
      {
        path: 'channels',
        name: 'channels',
        component: ChannelsPage,
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
        component: McpServersPage,
      },
      {
        path: 'skills',
        name: 'skills',
        component: SkillsPage,
      },
      {
        path: 'skills/runs',
        name: 'skill-runs',
        component: SkillRunsPage,
      },
      {
        path: 'skills/evolution-suggestions',
        name: 'evolution-suggestions',
        component: EvolutionSuggestionListPage,
      },
      {
        path: 'skills/experience-reports',
        name: 'experience-reports',
        component: ExperienceReportListPage,
      },
      {
        path: 'skills/:skillId',
        name: 'skill-detail',
        component: SkillDetailPage,
      },
      {
        path: 'plugins',
        name: 'plugins',
        component: PluginsPage,
      },
      {
        path: 'plugins/runs',
        name: 'plugin-runs',
        component: PluginRunsPage,
      },
      {
        path: 'hooks',
        name: 'hooks',
        component: HooksPage,
      },
      {
        path: 'hooks/deliveries',
        name: 'hook-deliveries',
        component: HookDeliveriesPage,
      },
      {
        path: 'webhooks',
        name: 'webhooks',
        component: WebhooksPage,
      },
      {
        path: 'knowledge',
        name: 'knowledge',
        component: KnowledgePage,
      },
      {
        path: 'artifacts',
        name: 'artifacts',
        component: ArtifactsPage,
      },
      {
        path: 'evaluation',
        name: 'evaluation',
        component: EvaluationPage,
      },
      {
        path: 'a2a',
        name: 'a2a',
        component: A2APage,
      },
      {
        path: 'tools/audits',
        name: 'tool-audits',
        component: ToolAuditsPage,
      },
      {
        path: 'tools/runs',
        name: 'tool-runs',
        component: ToolRunsPage,
      },
      {
        path: 'tools',
        name: 'tools',
        component: ToolsPage,
      },
      {
        path: 'cron',
        name: 'cron',
        component: CronTasksPage,
      },
      { path: 'monitor/logs', name: 'monitor-logs', component: MonitorPage },
      { path: 'observability', name: 'observability', component: ObservabilityDashboardPage },
      { path: 'shop', name: 'shop', component: EcosystemPage },
      { path: 'settings', name: 'settings', component: SystemSettingsPage, meta: { titleKey: 'menu.settings' } },
      ...(import.meta.env.DEV
        ? [{ path: 'dev/theme-preview', name: 'theme-preview', component: ThemePreviewPage }]
        : []),
    ],
  },
  // Catch-all：未匹配路径（如 /teams 笔误或过期书签）重定向到 overview，
  // 避免 RouterView 渲染 null vnode 导致整页空白崩溃。
  { path: '/:pathMatch(.*)*', redirect: '/overview' },
];
