<template>
  <details class="turn-tool-strip" :open="expanded">
    <summary class="turn-tool-strip__summary" :aria-label="summaryAria">
      <q-icon name="build_circle" size="16px" class="turn-tool-strip__icon" />
      <span class="turn-tool-strip__text">{{ summaryText }}</span>
      <q-badge v-if="summary.failed > 0" color="negative" rounded class="q-ml-xs">
        {{ summary.failed }}
      </q-badge>
    </summary>
    <div class="turn-tool-strip__details">
      <ChatMessageRow
        v-for="(tool, idx) in tools"
        :key="tool.id"
        :message="tool"
        :index="idx"
        :messages="tools"
        :is-dark="isDark"
        :is-team-session="isTeamSession"
        :planner-kind="plannerKind"
        :react-tool-link-index="reactToolLinkIndex"
        @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
      />
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import ChatMessageRow from "./ChatMessageRow.vue";
import { toolStripSummary } from "../../features/chat/groupMessagesByTurn";
import type { A2UIUserActionPayload } from "../../features/chat/a2uiUserAction";
import type { Message, ReactToolLinkIndex } from "../../features/chat/types";

const props = defineProps<{
  tools: Message[];
  isDark: boolean;
  isTeamSession?: boolean;
  plannerKind?: string;
  reactToolLinkIndex: ReactToolLinkIndex;
}>();

const emit = defineEmits<{
  "a2ui-user-action": [payload: A2UIUserActionPayload];
}>();

const { t } = useI18n();
const expanded = ref(false);

const summary = computed(() => toolStripSummary(props.tools));

const summaryText = computed(() => {
  const { count, failed, totalMs } = summary.value;
  const sec = totalMs >= 1000 ? `${(totalMs / 1000).toFixed(1)}s` : `${totalMs}ms`;
  const failPart = failed > 0 ? ` · ${failed} ${t("chat.turn.block.failed", "失败")}` : "";
  return t("chat.turn.block.toolsSummary", { count, sec, failPart }, `${count} tools · ${sec}${failPart}`);
});

const summaryAria = computed(() =>
  t("chat.turn.block.toolsAria", { count: summary.value.count }, `Tools: ${summary.value.count}`)
);
</script>
