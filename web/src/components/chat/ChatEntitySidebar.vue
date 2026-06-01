<template>
  <transition name="chat-side">
    <aside v-show="open" class="chat-side chat-side--left column no-wrap">
      <div class="chat-side__header">
        <q-input
          :model-value="search"
          dense
          outlined
          clearable
          :dark="isDark"
          :placeholder="t('chat.searchPlaceholder')"
          class="chat-search"
          @update:model-value="$emit('update:search', String($event ?? ''))"
        >
          <template #prepend>
            <q-icon name="search" size="18px" />
          </template>
        </q-input>
      </div>

      <q-scroll-area class="col">
        <div class="chat-side__content">
          <SpiritEntry
            :active="selectedKind === 'spirit'"
            @click="$emit('select-spirit')"
          />

          <ChatSectionHeader
            icon="groups"
            :label="t('chat.groupActiveTeams', '进行中')"
            :count="activeTeamList.length"
            :collapsed="collapse.sectionCollapsed.activeTeams"
            class="q-pt-md"
            @update:collapsed="collapse.toggleSection('activeTeams')"
          />
          <template v-if="!collapse.sectionCollapsed.activeTeams">
            <TeamTaskCard
              v-for="team in activeTeamList"
              :key="team.id"
              :team="team"
              :expanded="expandedTeamIds.has(team.id)"
              :active="selectedTeamId === team.id"
              @click="$emit('select-spirit-team', team.id)"
              @toggle-expand="$emit('toggle-team-expand', team.id)"
            />
            <div v-if="activeTeamList.length === 0" class="chat-side-hint text-caption text-cream-muted">
              暂无进行中的团队
            </div>
          </template>

          <ChatSectionHeader
            icon="check_circle"
            :label="t('chat.groupCompletedTeams', '已完成')"
            :count="completedTeamList.length"
            :collapsed="collapse.sectionCollapsed.completedTeams"
            class="q-pt-md"
            @update:collapsed="collapse.toggleSection('completedTeams')"
          />
          <template v-if="!collapse.sectionCollapsed.completedTeams">
            <TeamTaskCard
              v-for="team in completedTeamList"
              :key="team.id"
              :team="team"
              :expanded="expandedTeamIds.has(team.id)"
              :active="selectedTeamId === team.id"
              @click="$emit('select-spirit-team', team.id)"
              @toggle-expand="$emit('toggle-team-expand', team.id)"
            />
            <div v-if="completedTeamList.length === 0" class="chat-side-hint text-caption text-cream-muted">
              暂无已完成的团队
            </div>
          </template>
        </div>
      </q-scroll-area>
    </aside>
  </transition>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import ChatSectionHeader from "./ChatSectionHeader.vue";
import SpiritEntry from "../spirit/SpiritEntry.vue";
import TeamTaskCard from "../spirit/TeamTaskCard.vue";
import type { SpiritTeam } from "../../features/spirit/api";
import { useChatEntityCollapse } from "../../features/chat/composables/useChatEntityCollapse";

const props = defineProps<{
  open: boolean;
  search: string;
  spiritTeams: SpiritTeam[];
  expandedTeamIds: Set<string>;
  selectedKind: string;
  selectedTeamId?: string | null;
  isDark: boolean;
}>();

defineEmits<{
  "update:search": [value: string];
  "select-spirit": [];
  "select-spirit-team": [teamId: string];
  "toggle-team-expand": [teamId: string];
}>();

const { t } = useI18n();
const collapse = useChatEntityCollapse();

const activeTeamList = computed(() =>
  props.spiritTeams.filter((t) => t.status !== "completed")
);

const completedTeamList = computed(() =>
  props.spiritTeams.filter((t) => t.status === "completed")
);
</script>

<style scoped>
.chat-side--left {
  width: var(--chat-side-left-width, 280px);
  min-width: min(var(--chat-side-left-width, 280px), 100%);
  flex: 0 0 var(--chat-side-left-width, 280px);
  overflow: hidden;
}

:global(.body--dark) .chat-side-hint {
  color: var(--chat-idle-meta);
}
</style>
