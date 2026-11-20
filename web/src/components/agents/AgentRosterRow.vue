<template>
  <div
    class="ar-row"
    :class="{ 'ar-row--on': active, 'ar-row--off': agent.status === 'inactive' }"
    role="button"
    tabindex="0"
    :aria-pressed="active"
    @click="$emit('open', agent)"
    @keydown.enter.self="$emit('open', agent)"
    @keydown.space.self.prevent="$emit('open', agent)"
  >
    <span class="ar-row__check" @click.stop>
      <q-checkbox
        v-if="selectable"
        dense
        :model-value="selected"
        :aria-label="t('agentRoster.actions.select', { name: agent.display_name })"
        @update:model-value="$emit('toggle-select', agent.id)"
      />
    </span>

    <span class="ar-row__who">
      <agent-avatar-q :icon="agent.icon" :alt="agent.display_name" size="30px" />
      <span class="ar-row__names">
        <span class="ar-row__name">
          {{ agent.display_name || agent.agent_key }}
          <q-icon v-if="agent.is_favorite" name="star" size="13px" class="ar-star" aria-hidden="true" />
          <q-icon v-if="agent.readonly" name="lock" size="12px" class="ar-row__lock" aria-hidden="true" />
        </span>
        <span class="ar-row__key ar-mono">{{ agent.agent_key }}</span>
      </span>
    </span>

    <span class="ar-row__pos">{{ positionName || '—' }}</span>

    <span class="ar-role" :class="`ar-role--${role}`" :title="t(`agentRoster.roleHint.${role}`)">{{
      t(`agentRoster.role.${role}`)
    }}</span>

    <span class="ar-row__model ar-mono" :title="`${agent.provider} / ${agent.model}`">{{ agent.model || '—' }}</span>

    <span class="ar-st" :class="`ar-st--${agent.status}`">
      <span class="ar-dot" aria-hidden="true" />{{
        t(`agentRoster.status.${agent.status === 'inactive' ? 'inactive' : 'active'}`)
      }}
    </span>

    <span class="ar-row__run" :class="{ 'ar-row__run--bad': agent.last_run_status === 'failed' }">
      {{ runLabel }}
    </span>

    <span class="ar-row__acts" @click.stop>
      <q-btn
        flat
        dense
        round
        size="sm"
        icon="tune"
        :to="`/agents/${agent.id}/settings`"
        :aria-label="t('agentRoster.actions.settings')"
      >
        <q-tooltip>{{ t('agentRoster.actions.settings') }}</q-tooltip>
      </q-btn>
      <q-btn flat dense round size="sm" icon="more_horiz" :aria-label="t('agentRoster.actions.more')">
        <q-menu anchor="bottom right" self="top right">
          <q-list dense class="ar-menu">
            <q-item v-close-popup clickable @click="$emit('toggle-favorite', agent)">
              <q-item-section avatar><q-icon :name="agent.is_favorite ? 'star' : 'star_border'" /></q-item-section>
              <q-item-section>{{
                t(agent.is_favorite ? 'agentRoster.actions.unfavorite' : 'agentRoster.actions.favorite')
              }}</q-item-section>
            </q-item>
            <q-item v-close-popup clickable @click="$emit('copy-key', agent.agent_key)">
              <q-item-section avatar><q-icon name="tag" /></q-item-section>
              <q-item-section>{{ t('agentRoster.actions.copyKey') }}</q-item-section>
            </q-item>
            <q-item v-close-popup clickable @click="$emit('duplicate', agent)">
              <q-item-section avatar><q-icon name="content_copy" /></q-item-section>
              <q-item-section>{{ t('agentRoster.actions.duplicate') }}</q-item-section>
            </q-item>
            <q-item
              v-if="selectable"
              v-close-popup
              clickable
              @click="$emit('set-status', agent, agent.status === 'inactive' ? 'active' : 'inactive')"
            >
              <q-item-section avatar>
                <q-icon :name="agent.status === 'inactive' ? 'play_circle' : 'pause_circle'" />
              </q-item-section>
              <q-item-section>{{
                t(agent.status === 'inactive' ? 'agentRoster.actions.enable' : 'agentRoster.actions.disable')
              }}</q-item-section>
            </q-item>
            <q-separator v-if="deletable" />
            <q-item v-if="deletable" v-close-popup clickable class="ar-menu__danger" @click="$emit('delete', agent)">
              <q-item-section avatar><q-icon name="delete" /></q-item-section>
              <q-item-section>{{ t('agentRoster.actions.delete') }}</q-item-section>
            </q-item>
          </q-list>
        </q-menu>
      </q-btn>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Agent } from '../../features/agents/types';
import type { RosterRole } from '../../features/agents/roster';
import { relativeTime } from '../../features/monitor/eventView';
import AgentAvatarQ from '../avatar/AgentAvatarQ.vue';

const props = defineProps<{
  agent: Agent;
  role: RosterRole;
  positionName: string;
  active: boolean;
  selected: boolean;
  selectable: boolean;
  deletable: boolean;
}>();

defineEmits<{
  open: [agent: Agent];
  'toggle-select': [id: string];
  'toggle-favorite': [agent: Agent];
  'copy-key': [key: string];
  duplicate: [agent: Agent];
  'set-status': [agent: Agent, status: 'active' | 'inactive'];
  delete: [agent: Agent];
}>();

const { t, te } = useI18n();

const runLabel = computed(() => {
  const s = props.agent.last_run_status ?? '';
  const when = relativeTime(t, props.agent.last_run_at);
  if (!when && (!s || s === 'idle')) return t('agentRoster.never');
  const label = te(`agentRoster.run.${s}`) ? t(`agentRoster.run.${s}`) : s;
  return when ? `${label} · ${when}` : label;
});
</script>
