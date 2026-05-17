import { createPinia } from "pinia";
import { store } from "quasar/wrappers";

/** Quasar 入口要求 default export 为 Pinia 实例工厂 */
export default store(() => createPinia());

export { useAppStore } from "./app";
export { useAuthStore } from "./auth";
export { useAgentsPageStore, useAgentDetailStore } from "./agents";
export { useAvatarCatalogStore } from "./avatar";
export { useAdminStore } from "./admin";
export { useChannelsStore } from "./channels";
export { useChatStore } from "./chat";
export { useCronStore } from "./cron";
export { useGraphStore } from "./graph";
export { useHeartbeatStore } from "./heartbeat";
export { useMcpStore } from "./mcp";
export { useMemoryStore } from "./memory";
export { useMonitorStore } from "./monitor";
export { usePlatformStore } from "./platform";
export { usePluginsStore } from "./plugins";
export { useSessionStore } from "./session";
export { useSkillsStore } from "./skills";
export { useSystemSettingsStore } from "./system-settings";
export { useTeamsStore } from "./teams";
export { useToolsStore } from "./tools";
export { useUsageStore } from "./usage";
