import { computed, ref, watch, type ComputedRef, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import { useArtifactStore } from "../../../stores/artifact";
import { artifactDownloadHref, signDownloadUrl } from "../../artifact/api";
import type { ArtifactMeta } from "../../artifact/types";
import type { ChatEntityKind } from "../../../components/chat/types";
import type { useChatStreamManager } from "./useChatStreamManager";

type StreamManager = ReturnType<typeof useChatStreamManager>;

export function useChatTraceDialog(
  selectedEntityKind: Ref<ChatEntityKind>,
  displaySessions: ComputedRef<Array<{ id: string; title?: string }>>,
  streamManager: StreamManager
) {
  const { t } = useI18n();
  const traceOpen = ref(false);
  const traceSessionId = ref<string | null>(null);
  const traceSessionTitle = ref("");
  const traceInitialTab = ref<"trace" | "events">("trace");
  const traceSessionOwnerKind = ref<"agent" | "team">("agent");

  function openSessionTrace(sessionId: string, tab: "trace" | "events" = "trace") {
    const session = displaySessions.value.find((item) => item.id === sessionId);
    traceSessionId.value = sessionId;
    traceSessionTitle.value = session?.title ?? t("chat.untitledSession");
    traceInitialTab.value = tab;
    traceSessionOwnerKind.value = selectedEntityKind.value === "team" ? "team" : "agent";
    traceOpen.value = true;
  }

  const traceStreamDeps = computed(() => ({
    ownerKind: traceSessionOwnerKind.value,
    subscribe: streamManager.subscribeSessionStream,
  }));

  function openSessionEvents(selectedSessionId: string | undefined) {
    if (!selectedSessionId) return;
    openSessionTrace(selectedSessionId, "events");
  }

  return {
    traceOpen,
    traceSessionId,
    traceSessionTitle,
    traceInitialTab,
    traceStreamDeps,
    openSessionTrace,
    openSessionEvents,
  };
}

export function useChatSessionArtifacts(sessionId: ComputedRef<string | undefined>) {
  const router = useRouter();
  const artifactStore = useArtifactStore();
  const sessionArtifacts = ref<ArtifactMeta[]>([]);
  const sessionArtifactsLoading = ref(false);

  async function loadSessionArtifacts(sid: string) {
    if (!sid) {
      sessionArtifacts.value = [];
      return;
    }
    sessionArtifactsLoading.value = true;
    try {
      const res = await artifactStore.loadArtifacts({ session_id: sid, limit: 20 });
      sessionArtifacts.value = res.items;
    } finally {
      sessionArtifactsLoading.value = false;
    }
  }

  watch(
    () => sessionId.value ?? "",
    (sid) => void loadSessionArtifacts(sid),
    { immediate: true }
  );

  function openSessionArtifact(id: string) {
    void (async () => {
      try {
        const signed = await signDownloadUrl(id);
        if (signed.url) {
          window.open(artifactDownloadHref(signed.url), "_blank", "noopener,noreferrer");
          return;
        }
      } catch {
        // fall through to artifacts page
      }
      void router.push({ path: "/artifacts", query: { id } });
    })();
  }

  return {
    sessionArtifacts,
    sessionArtifactsLoading,
    openSessionArtifact,
  };
}
