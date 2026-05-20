import { useRouter } from "vue-router";

export type MonitorRunNavQuery = {
  tab?: string;
  session?: string;
  trace?: string;
  usageEventId?: string;
};

export function useMonitorRunNavigation() {
  const router = useRouter();

  function openChatSession(sessionId: string) {
    const sid = sessionId.trim();
    if (!sid) return;
    void router.push({ path: "/chat", query: { session: sid } });
  }

  function openRunsTab(extra: MonitorRunNavQuery = {}) {
    void router.push({
      path: "/monitor/logs",
      query: {
        tab: extra.tab || "traces",
        ...(extra.session ? { session: extra.session } : {}),
        ...(extra.trace ? { trace: extra.trace } : {}),
        ...(extra.usageEventId ? { usage_event_id: extra.usageEventId } : {})
      }
    });
  }

  return { openChatSession, openRunsTab };
}
