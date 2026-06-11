<template>
  <div class="member-readonly-panel column no-wrap">
    <!-- Header -->
    <div class="member-readonly-panel__header q-px-md q-py-sm">
      <div class="row items-center q-gutter-sm">
        <q-avatar size="32px">
          <img v-if="member.avatarUrl" :src="member.avatarUrl" alt="" />
          <q-icon v-else name="person" size="20px" color="grey" />
        </q-avatar>
        <div class="col min-width-0">
          <div class="text-subtitle2 ellipsis">{{ member.displayName }}</div>
          <div class="text-caption text-grey ellipsis">{{ member.role }}</div>
        </div>
        <AgentStatusLabel :label="statusLabel" />
      </div>
    </div>

    <q-separator />

    <!-- Member info section -->
    <div class="q-pa-md">
      <div class="text-caption text-weight-medium text-grey q-mb-sm">{{ t('spirit.memberInfo') }}</div>
      <div class="member-readonly-panel__info-grid">
        <div class="text-caption text-grey">Agent Key</div>
        <div class="text-body2">{{ member.agentKey }}</div>
        <div class="text-caption text-grey">{{ t('spirit.status') }}</div>
        <div class="text-body2">{{ statusConfig.text }}</div>
        <div class="text-caption text-grey">{{ t('spirit.belongingTeam') }}</div>
        <div class="text-body2 ellipsis">{{ teamName }}</div>
      </div>
    </div>

    <q-separator />

    <!-- Execution statistics -->
    <div class="q-pa-md">
      <div class="text-caption text-weight-medium text-grey q-mb-sm">{{ t('spirit.executionStats') }}</div>
      <div class="member-readonly-panel__stats row q-gutter-md">
        <div class="member-readonly-panel__stat">
          <q-icon name="build" size="14px" class="q-mr-xs member-readonly-panel__stat-icon--accent" />
          <span class="text-body2">{{ toolCallCount }}</span>
          <span class="text-caption text-grey q-ml-xs">{{ t('spirit.toolCalls') }}</span>
        </div>
        <div v-if="durationLabel" class="member-readonly-panel__stat">
          <q-icon name="schedule" size="14px" class="q-mr-xs member-readonly-panel__stat-icon--warning" />
          <span class="text-body2">{{ durationLabel }}</span>
        </div>
        <div v-if="tokenLabel" class="member-readonly-panel__stat">
          <q-icon name="data_usage" size="14px" class="q-mr-xs member-readonly-panel__stat-icon--tertiary" />
          <span class="text-body2">{{ tokenLabel }}</span>
        </div>
      </div>
    </div>

    <q-separator />

    <!-- Read-only message stream -->
    <div class="member-readonly-panel__messages col q-pa-md">
      <div class="text-caption text-weight-medium text-grey q-mb-sm">{{ t('spirit.executionRecordsReadonly') }}</div>
      <template v-if="memberMessages.length > 0">
        <ChatExecutionCard
          v-for="msg in memberMessages"
          :key="msg.id"
          :event="msg"
          :show-member-label="false"
          :initial-collapsed="isCompleted(msg)"
        />
      </template>
      <div v-else class="text-caption text-grey">{{ t('spirit.noExecutionRecords') }}</div>
    </div>

    <q-separator />

    <!-- Assistant messages -->
    <div v-if="assistantMessages.length > 0" class="member-readonly-panel__assistant q-pa-md">
      <div class="text-caption text-weight-medium text-grey q-mb-sm">{{ t('spirit.dialogOutput') }}</div>
      <div v-for="msg in assistantMessages" :key="msg.id" class="member-readonly-panel__assistant-item">
        <div class="chat-message-prose" v-html="renderMarkdown(msg.content_markdown)" />
      </div>
    </div>

    <q-separator v-if="assistantMessages.length > 0" />

    <!-- Back buttons -->
    <div class="q-pa-md row items-center q-gutter-sm">
      <q-btn flat dense no-caps icon="arrow_back" :label="t('spirit.backToTeam')" color="accent" @click="emit('return-to-team')" />
      <q-btn flat dense no-caps icon="auto_awesome" :label="t('spirit.backToSpirit')" color="accent" @click="emit('return-to-spirit')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AgentStatusLabel from './AgentStatusLabel.vue';
import ChatExecutionCard from '../chat/ChatExecutionCard.vue';
import { spiritMemberStatusToLabel, STATUS_LABEL_CONFIG, formatDuration, formatTokenCount } from '../../features/spirit/spiritUi';
import type { SpiritMember, SpiritTeam } from '../../features/spirit/types';
import type { Message, ToolUseEvent } from '../../features/chat/types';

const { t } = useI18n();

const props = defineProps<{
  member: SpiritMember;
  team: SpiritTeam;
  messages: Message[];
  renderMarkdown: (text: string) => string;
}>();

const emit = defineEmits<{
  'return-to-team': [];
  'return-to-spirit': [];
}>();

const statusLabel = computed(() => spiritMemberStatusToLabel(props.member.status));
const statusConfig = computed(() => STATUS_LABEL_CONFIG[statusLabel.value]);
const teamName = computed(() => props.team.teamName);

const memberMessages = computed<ToolUseEvent[]>(() => {
  return props.messages
    .filter((m) => m.role === 'tool' && m.tool_event)
    .filter((m) => {
      const evt = m.tool_event as ToolUseEvent;
      // Filter to only show this member's tool events
      return evt.agent_key === props.member.agentKey || evt.agent_id === props.member.agentId;
    })
    .map((m) => m.tool_event as ToolUseEvent);
});

const assistantMessages = computed<Message[]>(() => {
  return props.messages.filter((m) => {
    if (m.role !== 'assistant') return false;
    return (
      m.agent_ref?.id === props.member.agentId ||
      m.team_member?.agent_id === props.member.agentId
    );
  });
});

const toolCallCount = computed(() => memberMessages.value.length);

const durationLabel = computed(() => formatDuration(props.team.durationMs));

const tokenLabel = computed(() => formatTokenCount(props.team.tokenIn, props.team.tokenOut));

function isCompleted(event: ToolUseEvent): boolean {
  return event.status === 'completed' || event.status === 'failed';
}
</script>

<style scoped lang="sass">
.member-readonly-panel
  height: 100%

  &__header
    border-bottom: 1px solid var(--glass-border)

  &__info-grid
    display: grid
    grid-template-columns: auto 1fr
    gap: 4px 12px
    align-items: baseline

  &__stats
    flex-wrap: wrap

  &__stat
    display: flex
    align-items: center

  &__stat-icon--accent
    color: var(--color-accent)

  &__stat-icon--warning
    color: var(--color-warning)

  &__stat-icon--tertiary
    color: var(--color-text-tertiary)

  &__messages
    overflow-y: auto
    min-height: 0

  &__assistant
    overflow-y: auto

  &__assistant-item
    padding: var(--space-2) 0
    border-bottom: 1px solid color-mix(in srgb, var(--glass-border) 30%, transparent)

    &:last-child
      border-bottom: none
</style>
