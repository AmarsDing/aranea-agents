<template>
  <q-card
    flat
    bordered
    :class="['agent-card full-height', { 'agent-card--dark': isDark, 'agent-card--builtin': isBuiltin }]"
  >
    <q-card-section class="agent-card__header">
      <agent-avatar-q :icon="agent.icon" :alt="agent.display_name" size="40px" avatar-class="agent-card__avatar" />
      <div class="col min-width-0">
        <div class="row items-center no-wrap q-gutter-xs">
          <q-btn
            v-if="!isBuiltin"
            flat
            dense
            round
            size="xs"
            :aria-label="favorite ? '取消收藏' : '收藏 Agent'"
            :color="favorite ? 'amber-8' : 'grey-5'"
            :icon="favorite ? 'star' : 'star_border'"
            @click="$emit('toggle-favorite', agent.id)"
          />
          <div class="agent-card__name text-subtitle2 text-weight-bold ellipsis">{{ agent.display_name }}</div>
          <KindBadge :kind="agent.source" />
          <q-badge rounded :class="['agent-card__status', agent.status === 'active' ? 'is-active' : '']">{{
            statusLabel(agent.status)
          }}</q-badge>
        </div>
        <div class="row items-center q-gutter-x-sm q-mt-xxs">
          <button class="agent-handle" @click="$emit('copy-key', agent.agent_key)">{{ agent.agent_key }}</button>
          <span class="agent-card__model"
            ><q-icon name="memory" size="12px" />{{ agent.provider }} / {{ agent.model }}</span
          >
        </div>
      </div>
    </q-card-section>

    <q-card-section class="agent-card__body q-pt-none">
      <p class="agent-description">
        {{ agent.agent_description || '暂无描述'
        }}<q-tooltip v-if="agent.agent_description" class="agent-desc-tooltip" max-width="480px" :delay="300">{{
          agent.agent_description
        }}</q-tooltip>
      </p>
      <div class="row q-gutter-xs">
        <q-chip v-if="isBuiltin" dense square class="agent-card__chip is-system" icon="lock">系统</q-chip>
        <q-chip v-else dense square class="agent-card__chip">{{ taxonomyLabel }}</q-chip>
        <q-chip v-if="evolving" dense square class="agent-card__chip is-evolving" icon="auto_awesome">进化中</q-chip>
        <q-chip
          v-if="agent.agent_kind === 'a2a_proxy'"
          dense
          square
          class="agent-card__chip is-a2a-proxy"
          icon="sync_alt"
          >A2A ↗</q-chip
        >
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
    <q-card-actions align="between" class="agent-card__actions">
      <span class="agent-card__context">{{ contextLabel }}</span>
      <div class="q-gutter-xs">
        <q-btn flat dense rounded color="primary" label="设置" :to="`/agents/${agent.id}/settings`" />
        <q-btn v-if="!isBuiltin" flat dense rounded color="secondary" label="复制" @click="$emit('duplicate', agent)" />
        <q-btn v-if="!isBuiltin && agent.source !== 'system_builtin'" flat dense rounded color="negative" icon="delete" @click="$emit('delete', agent)" />
        <q-chip v-if="isBuiltin" dense square class="agent-card__readonly-chip" icon="verified_user">内置</q-chip>
      </div>
    </q-card-actions>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useQuasar } from 'quasar';
import type { Agent } from '../../features/agents/types';
import AgentAvatarQ from '../avatar/AgentAvatarQ.vue';
import KindBadge from './KindBadge.vue';
import { statusLabel } from './agentUi';

const props = defineProps<{
  agent: Agent;
  favorite: boolean;
  taxonomyLabel: string;
  contextLabel: string;
  evolving: boolean;
}>();

defineEmits<{
  'toggle-favorite': [id: string];
  'copy-key': [key: string];
  delete: [agent: Agent];
  duplicate: [agent: Agent];
}>();

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);
const isBuiltin = computed(() => props.agent.readonly === true);
</script>
