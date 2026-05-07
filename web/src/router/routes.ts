import type { RouteRecordRaw } from "vue-router";
import BlankLayout from "../layouts/BlankLayout.vue";
import LoginPage from "../pages/LoginPage.vue";
import MainLayout from "../layouts/MainLayout.vue";
import ChatPage from "../pages/ChatPage.vue";
import AgentsPage from "../pages/AgentsPage.vue";
import AgentSettingsPage from "../pages/AgentSettingsPage.vue";
import MonitorPage from "../pages/MonitorPage.vue";
import OverviewPage from "../pages/OverviewPage.vue";
import ResourceManagerPage from "../pages/ResourceManagerPage.vue";
import EcosystemPage from "../pages/EcosystemPage.vue";
import AgentCategoriesPage from "../pages/AgentCategoriesPage.vue";
import TeamsPage from "../pages/TeamsPage.vue";
import SkillsPage from "../pages/SkillsPage.vue";
import SkillRunsPage from "../pages/SkillRunsPage.vue";
import PluginsPage from "../pages/PluginsPage.vue";
import ToolsPage from "../pages/ToolsPage.vue";
import ToolRunsPage from "../pages/ToolRunsPage.vue";
import SessionsPage from "../pages/SessionsPage.vue";
import ChannelsPage from "../pages/ChannelsPage.vue";
import McpServersPage from "../pages/McpServersPage.vue";
import CronTasksPage from "../pages/CronTasksPage.vue";
import CronRunsPage from "../pages/CronRunsPage.vue";
import MemoryCenterPage from "../pages/MemoryCenterPage.vue";
import SystemSettingsPage from "../pages/SystemSettingsPage.vue";

export const routes: RouteRecordRaw[] = [
  {
    path: "/login",
    component: BlankLayout,
    meta: { public: true },
    children: [{ path: "", name: "login", component: LoginPage }]
  },
  {
    path: "/",
    component: MainLayout,
    meta: { requiresAuth: true },
    children: [
      { path: "", redirect: "/overview" },
      { path: "overview", name: "overview", component: OverviewPage },
      { path: "chat", name: "chat", component: ChatPage },
      { path: "sessions", name: "sessions", component: SessionsPage },
      { path: "sessions/:sessionId", name: "session-detail", component: SessionsPage },
      { path: "memory", name: "memory", component: MemoryCenterPage },
      { path: "agents", name: "agents", component: AgentsPage },
      { path: "settings/agent-categories", name: "agent-categories", component: AgentCategoriesPage },
      { path: "agents/:id/settings", name: "agent-settings", component: AgentSettingsPage },
      { path: "team", name: "team", component: TeamsPage },
      {
        path: "models",
        name: "models",
        component: ResourceManagerPage,
        meta: { resource: "llm-provider-models", title: "模型管理", subtitle: "维护 Provider/Model 可用清单与模型校验来源。" }
      },
      {
        path: "channels",
        name: "channels",
        component: ChannelsPage,
        meta: { resource: "channels", title: "Channel 管理", subtitle: "管理外部消息渠道、凭据引用与 Agent 绑定配置。" }
      },
      {
        path: "mcp",
        name: "mcp",
        redirect: "/mcp-servers"
      },
      {
        path: "mcp-servers",
        name: "mcp-servers",
        component: McpServersPage
      },
      {
        path: "skills",
        name: "skills",
        component: SkillsPage
      },
      {
        path: "skills/runs",
        name: "skill-runs",
        component: SkillRunsPage
      },
      {
        path: "plugins",
        name: "plugins",
        component: PluginsPage
      },
      {
        path: "tools/runs",
        name: "tool-runs",
        component: ToolRunsPage
      },
      {
        path: "tools",
        name: "tools",
        component: ToolsPage
      },
      {
        path: "cron",
        name: "cron",
        component: CronTasksPage
      },
      { path: "cron/runs", name: "cron-runs", component: CronRunsPage },
      { path: "monitor/logs", name: "monitor-logs", component: MonitorPage },
      { path: "shop", name: "shop", component: EcosystemPage },
      { path: "settings", name: "settings", component: SystemSettingsPage, meta: { titleKey: "menu.settings" } }
    ]
  }
];
