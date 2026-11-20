<template>
  <component
    :is="agent ? 'button' : 'div'"
    :type="agent ? 'button' : undefined"
    class="org-agent"
    :class="{ 'org-agent--empty': !agent, 'org-agent--compact': compact }"
    :title="agent ? t('orgChart.openAgent') : undefined"
    @click="agent && emit('open', agent)"
  >
    <span class="org-agent__avatar" aria-hidden="true">{{ initial }}</span>
    <span class="org-agent__text">
      <span class="org-agent__name">{{ agent ? agent.display_name : emptyLabel }}</span>
      <span v-if="agent && !compact" class="org-agent__meta org-mono">{{ agent.model || '—' }}</span>
    </span>
    <span
      v-if="agent"
      class="org-dot"
      :class="agent.status === 'active' ? 'org-dot--ok' : 'org-dot--off'"
      :aria-label="t(agent.status === 'active' ? 'orgChart.agentStatus.active' : 'orgChart.agentStatus.inactive')"
      role="img"
    />
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Agent } from '../../features/agents/types';
import { agentGlyph } from '../../features/organization/orgChart';

const props = defineProps<{ agent: Agent | null; emptyLabel?: string; compact?: boolean }>();
const emit = defineEmits<{ open: [agent: Agent] }>();
const { t } = useI18n();

const initial = computed(
  () => agentGlyph(props.agent) || (props.agent?.display_name || '?').trim().slice(0, 1).toUpperCase(),
);
</script>
