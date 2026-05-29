<template>
  <q-card flat bordered :class="['agent-card full-height', { 'agent-card--dark': isDark }]">
    <q-card-section class="agent-card__header">
      <agent-avatar-q :icon="agent.icon" :alt="agent.display_name" size="56px" avatar-class="agent-card__avatar" />
      <div class="col min-width-0">
        <div class="row items-center no-wrap q-gutter-xs">
          <q-btn
            flat
            dense
            round
            size="sm"
            :aria-label="favorite ? '取消收藏' : '收藏 Agent'"
            :color="favorite ? 'amber-8' : 'grey-5'"
            :icon="favorite ? 'star' : 'star_border'"
            @click="$emit('toggle-favorite', agent.id)"
          />
          <div class="agent-card__name text-subtitle1 text-weight-bold ellipsis">{{ agent.display_name }}</div>
        </div>
        <button class="agent-handle" @click="$emit('copy-key', agent.agent_key)">{{ agent.agent_key }}</button>
      </div>
      <q-badge rounded :class="['agent-card__status', agent.status === 'active' ? 'is-active' : '']">{{ statusLabel(agent.status) }}</q-badge>
    </q-card-section>

    <q-card-section class="q-pt-none">
      <div class="agent-card__model"><q-icon name="memory" size="14px" />{{ agent.provider }} / {{ agent.model }}</div>
      <p class="agent-description">{{ agent.agent_description || "暂无描述，可在设置中补充能力边界与使用场景。" }}</p>
      <div class="row q-gutter-xs">
        <q-chip dense square class="agent-card__chip">{{ categoryLabel }}</q-chip>
        <q-chip v-if="evolving" dense square class="agent-card__chip is-evolving" icon="auto_awesome">进化中</q-chip>
        <q-chip v-if="agent.agent_kind === 'a2a_proxy'" dense square class="agent-card__chip is-a2a-proxy" icon="sync_alt">A2A ↗</q-chip>
        <q-chip
          v-else-if="agent.a2a_endpoint_enabled"
          dense
          square
          class="agent-card__chip is-a2a-endpoint"
          icon="call_received"
        >
          A2A ↙
        </q-chip>
      </div>
    </q-card-section>

    <q-space />
    <q-separator />
    <q-card-actions align="between" class="agent-card__actions">
      <span class="agent-card__context">{{ contextLabel }}</span>
      <div class="q-gutter-xs">
        <q-btn flat dense rounded color="primary" label="设置" :to="`/agents/${agent.id}/settings`" />
        <q-btn flat dense rounded color="secondary" label="复制" @click="$emit('duplicate', agent)" />
        <q-btn flat dense rounded color="negative" icon="delete" label="删除" @click="$emit('delete', agent)" />
      </div>
    </q-card-actions>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useQuasar } from "quasar";
import type { Agent } from "../../features/agents/types";
import AgentAvatarQ from "../avatar/AgentAvatarQ.vue";
import { statusLabel } from "./agentUi";

const props = defineProps<{
  agent: Agent;
  favorite: boolean;
  categoryLabel: string;
  contextLabel: string;
  evolving: boolean;
}>();

defineEmits<{
  "toggle-favorite": [id: string];
  "copy-key": [key: string];
  delete: [agent: Agent];
  duplicate: [agent: Agent];
}>();

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);
</script>
