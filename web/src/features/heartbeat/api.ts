// EP-FE-05: api layer for the heartbeat feature.
// Exposes the WebSocket health URL used by useServerHeartbeat so consumers
// can import from this module rather than reaching into config directly.
export { buildHealthWsUrl, getWsOrigin } from "../../config/runtime";
