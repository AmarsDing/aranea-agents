/**
 * @deprecated 新代码请优先：
 * - Kratos：`import { createAdminService() … } from "@/services"` 或与 domain 对齐的 **`features/<domain>/api.ts`**（如会话：**`@/features/session/api`**）；
 * - 遗留 REST：`clientLegacy` 仅收口 **`legacyRestApi`**（`/api/v1/*`）；勿向其追加 **已迁 Kratos** 的实现（见 vue-design-agent-rules.md）。
 */
export * from "../services/clientLegacy";
export { api, syncApiBaseURL } from "./http";
