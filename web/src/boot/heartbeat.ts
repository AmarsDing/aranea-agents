import { defineBoot } from "#q-app/wrappers";
import { useServerHeartbeat } from "../features/heartbeat/useServerHeartbeat";

export default defineBoot(() => {
  useServerHeartbeat();
});
