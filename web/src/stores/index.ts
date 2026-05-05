import { createPinia } from "pinia";
import { store } from "quasar/wrappers";

/** Quasar 入口要求 default export 为 Pinia 实例工厂 */
export default store(() => createPinia());

export { useAppStore } from "./app";
export { useAuthStore } from "./auth";
export { useAgentsPageStore, useAgentDetailStore } from "./agents";
export { useAvatarCatalogStore } from "./avatar";
