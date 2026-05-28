import { ref } from "vue";
import type { Message } from "../features/chat/types";
import { createPlaceholderMessage } from "../features/chat/streamHandlers";
import { patchStreamingMessage } from "../features/chat/streamContentPatch";

export type ChatStreamingSnapshot = {
  reasoning: string;
  partialText: string;
  updatedAt: number;
};

const snapshots = ref<Record<string, ChatStreamingSnapshot>>({});

export function useChatStreamingSnapshots() {
  function put(
    sessionId: string,
    patch: Partial<Pick<ChatStreamingSnapshot, "reasoning" | "partialText">> & { replace?: boolean }
  ) {
    const sid = sessionId.trim();
    if (!sid) return;
    if (patch.replace) {
      snapshots.value[sid] = {
        reasoning: patch.reasoning ?? "",
        partialText: patch.partialText ?? "",
        updatedAt: Date.now(),
      };
      return;
    }
    const cur = snapshots.value[sid] ?? { reasoning: "", partialText: "", updatedAt: 0 };
    snapshots.value[sid] = {
      reasoning:
        patch.reasoning !== undefined ? `${cur.reasoning}${patch.reasoning}` : cur.reasoning,
      partialText:
        patch.partialText !== undefined ? `${cur.partialText}${patch.partialText}` : cur.partialText,
      updatedAt: Date.now(),
    };
  }

  function get(sessionId: string): ChatStreamingSnapshot | undefined {
    return snapshots.value[sessionId.trim()];
  }

  function clear(sessionId: string) {
    const sid = sessionId.trim();
    if (!sid) return;
    const next = { ...snapshots.value };
    delete next[sid];
    snapshots.value = next;
  }

  return { snapshots, put, get, clear };
}

export function applyStreamingSnapshotToSession(
  getMessages: (sessionId: string) => Message[],
  setMessages: (sessionId: string, rows: Message[]) => void,
  sessionId: string
) {
  const store = useChatStreamingSnapshots();
  const snap = store.get(sessionId);
  if (!snap || (!snap.reasoning && !snap.partialText)) return;

  const streamId = `ws-stream-${sessionId}`;
  let rows = getMessages(sessionId);
  const existingStream = rows.find((m) => m.id === streamId);

  if (!existingStream) {
    const lastPersistedAssistant = [...rows]
      .reverse()
      .find(
        (m) =>
          m.role === "assistant" &&
          !String(m.id).startsWith("ws-stream-") &&
          !String(m.id).startsWith("ws-team-stream-")
      );
    if (lastPersistedAssistant?.status === "ok" && lastPersistedAssistant.content_markdown?.trim()) {
      store.clear(sessionId);
      return;
    }
    rows = [
      ...rows,
      {
        ...createPlaceholderMessage(streamId, sessionId, "assistant", ""),
        status: "streaming",
      },
    ];
  }

  setMessages(
    sessionId,
    patchStreamingMessage(rows, streamId, {
      reasoning: snap.reasoning || undefined,
      text: snap.partialText || undefined,
      status: existingStream?.status === "ok" ? "ok" : "streaming",
    })
  );
}
