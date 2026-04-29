/**
 * @deprecated 新代码请使用 `web/src/services`：
 * - Kratos：`createAdminService()`、`createAvatarService()` 等；
 * - 遗留 REST（`/api/v1/*`）：`import { … } from "@/api/client"` 或 `from "@/services/clientLegacy"`（勿从 `@/services` 误收 legacy）。
 */
export * from "../services/clientLegacy";
export { api, syncApiBaseURL } from "./http";
