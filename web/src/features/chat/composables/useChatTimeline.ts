import { computed, type ComputedRef } from "vue";
import type { Message, ReactToolLinkIndex } from "../types";
import type { TeamMemberLane } from "../../../components/chat/ChatTeamMemberStrip";
import { useChatMessageRow } from "../useChatMessageRow";
import { groupMessagesByTurn, type TurnBlockGroup } from "../groupMessagesByTurn";
import { useTurnBlockEnabled } from "../useTurnBlock";

export type TimelineItem =
  | { kind: "block"; block: TurnBlockGroup }
  | { kind: "message"; message: Message };

export function useChatTimeline(deps: {
  messages: ComputedRef<Message[]>;
  isTeamSession?: boolean;
}) {
  const messageRow = useChatMessageRow(deps.messages);

  const teamMemberLanes = computed((): TeamMemberLane[] => {
    if (!deps.isTeamSession) return [];
    const lanes = new Map<string, TeamMemberLane>();
    for (const message of deps.messages.value) {
      if (!messageRow.isTeamMember(message)) continue;
      const key = messageRow.messageIdentityKey(message);
      const meta = messageRow.teamMemberMeta(message);
      const label = meta?.name || meta?.agent_key || messageRow.displayMessageName(message);
      const streaming = message.status === "streaming" || message.status === "tool_running";
      const prev = lanes.get(key);
      lanes.set(key, {
        key,
        label,
        streaming: (prev?.streaming ?? false) || streaming,
      });
    }
    return [...lanes.values()];
  });

  const useTurnBlockMode = computed(() => useTurnBlockEnabled() && !deps.isTeamSession);

  const turnBlocks = computed((): TurnBlockGroup[] =>
    useTurnBlockMode.value ? groupMessagesByTurn(deps.messages.value) : []
  );

  const timelineItems = computed((): TimelineItem[] => {
    if (useTurnBlockMode.value) {
      return turnBlocks.value.map((block) => ({ kind: "block" as const, block }));
    }
    return deps.messages.value.map((message) => ({ kind: "message" as const, message }));
  });

  return {
    messageRow,
    teamMemberLanes,
    useTurnBlockMode,
    turnBlocks,
    timelineItems,
  };
}
