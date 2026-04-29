/**
 * 遗留 REST 的 axios 实例；与 Kratos 共用 {@link syncHttpClients}。
 */
export { legacyRestApi as api, syncHttpClients as syncApiBaseURL } from "../services/axiosHandler";
