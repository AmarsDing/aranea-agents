<template>
  <div class="observe-node-detail">
    <header class="observe-node-detail__header">
      <span class="observe-node-detail__avatar">{{ nodeInitial }}</span>
      <div class="observe-node-detail__info">
        <h3 class="observe-node-detail__name">{{ node.Label }}</h3>
        <ObserveStatusBadge :status="node.Status" />
      </div>
      <q-btn flat round dense icon="close" size="sm" @click="$emit('close')" />
    </header>

    <!-- Description -->
    <section v-if="description" class="observe-node-detail__section">
      <h4 class="observe-node-detail__section-title">{{ t('observe.description') }}</h4>
      <p class="observe-node-detail__description">{{ description }}</p>
    </section>

    <!-- Error panel (failed only) -->
    <section v-if="error" class="observe-node-detail__section">
      <h4 class="observe-node-detail__section-title">{{ t('observe.errorDetail') }}</h4>
      <div class="observe-node-detail__error">
        <q-icon name="error" size="14px" />
        <span>{{ error }}</span>
      </div>
    </section>

    <!-- Members -->
    <section v-if="members.length" class="observe-node-detail__section">
      <h4 class="observe-node-detail__section-title">{{ t('observe.members') }}</h4>
      <div class="observe-node-detail__member-list">
        <div v-for="m in members" :key="m.agentKey" class="observe-node-detail__member">
          <span class="observe-node-detail__member-avatar">
            <img v-if="m.avatarUrl" :src="m.avatarUrl" :alt="m.agentName" />
            <template v-else>{{ memberInitial(m.agentName) }}</template>
            <span :class="['observe-node-detail__member-dot', `observe-node-detail__member-dot--${m.status}`]" />
          </span>
          <div class="observe-node-detail__member-info">
            <span class="observe-node-detail__member-name">{{ m.agentName }}</span>
            <span v-if="m.currentAction" class="observe-node-detail__member-action">{{ m.currentAction }}</span>
          </div>
          <span :class="['observe-node-detail__member-status', `observe-node-detail__member-status--${m.status}`]">
            {{ memberStatusLabel(m.status) }}
          </span>
        </div>
      </div>
    </section>

    <!-- Text output (completed only) -->
    <section v-if="textOutput" class="observe-node-detail__section">
      <h4 class="observe-node-detail__section-title">{{ t('observe.textOutput') }}</h4>
      <div class="observe-node-detail__text-output">
        <q-icon name="description" size="14px" />
        <span>{{ textOutput }}</span>
      </div>
    </section>

    <!-- Media outputs -->
    <section v-if="mediaOutputs.length" class="observe-node-detail__section">
      <h4 class="observe-node-detail__section-title">{{ t('observe.mediaOutputs') }}</h4>
      <div class="observe-node-detail__media-grid">
        <div
          v-for="art in mediaOutputs"
          :key="art.artifact_id"
          class="observe-node-detail__media-item"
          @click="$emit('preview', art)"
        >
          <video
            v-if="art.mime_type.startsWith('video/')"
            :src="art.url"
            :poster="art.thumbnail"
            muted
            preload="metadata"
            class="observe-node-detail__media"
          />
          <img v-else :src="art.url" loading="lazy" class="observe-node-detail__media" />
        </div>
      </div>
    </section>

    <!-- Node info -->
    <section class="observe-node-detail__section">
      <h4 class="observe-node-detail__section-title">{{ t('observe.nodeInfo') }}</h4>
      <div class="observe-node-detail__meta">
        <div v-if="durationLabel" class="observe-node-detail__meta-row">
          <span class="observe-node-detail__meta-label">{{ t('observe.duration') }}:</span>
          <span>{{ durationLabel }}</span>
        </div>
        <div class="observe-node-detail__meta-row">
          <span class="observe-node-detail__meta-label">DAG Node:</span>
          <code>{{ node.DagNodeID }}</code>
        </div>
        <div class="observe-node-detail__meta-row">
          <span class="observe-node-detail__meta-label">Team Stage:</span>
          <code>{{ node.TeamStageID || '-' }}</code>
        </div>
        <div v-if="node.DependsOn?.length" class="observe-node-detail__meta-row">
          <span class="observe-node-detail__meta-label">Dependencies:</span>
          <span>{{ node.DependsOn.join(', ') }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphNode } from '../../../features/chat/v2Types';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';
import { useObserveNodeEnrichment } from '../../../features/chat/composables/useObserveGraph';
import { formatDuration } from '../../../features/spirit/spiritUi';
import ObserveStatusBadge from './ObserveStatusBadge.vue';

const { t } = useI18n();

const props = defineProps<{ node: GraphNode }>();
defineEmits<{
  close: [];
  preview: [art: MediaArtifact];
}>();

const { extractMembers, extractDuration, extractError, extractDescription, extractTextOutput, extractMediaOutputs } =
  useObserveNodeEnrichment();

const nodeInitial = computed(() => (props.node.Label || '?').charAt(0).toUpperCase());

const mediaOutputs = computed(() => extractMediaOutputs(props.node));

const members = computed(() => extractMembers(props.node));
const durationLabel = computed(() => formatDuration(extractDuration(props.node)));
const error = computed(() => extractError(props.node));
const description = computed(() => extractDescription(props.node));
const textOutput = computed(() => extractTextOutput(props.node));

function memberInitial(name: string): string {
  return (name || '?').charAt(0).toUpperCase();
}

const MEMBER_STATUS_KEY_MAP: Record<string, string> = {
  pending: 'observe.statusPending',
  running: 'observe.statusRunning',
  completed: 'observe.statusCompleted',
  failed: 'observe.statusFailed',
  skipped: 'observe.statusSkipped',
};

function memberStatusLabel(status: string): string {
  const key = MEMBER_STATUS_KEY_MAP[status];
  return key ? t(key) : status;
}
</script>

<style scoped lang="sass">
.observe-node-detail
  position: absolute
  top: 0
  right: 0
  width: 280px
  height: 100%
  background: var(--color-surface-soft)
  border-left: 1px solid var(--color-border-soft)
  overflow-y: auto
  padding: 12px
  z-index: 10

.observe-node-detail__header
  display: flex
  align-items: flex-start
  gap: 8px
  margin-bottom: 12px

.observe-node-detail__avatar
  width: 32px
  height: 32px
  border-radius: 50%
  background: var(--color-accent)
  color: white
  display: flex
  align-items: center
  justify-content: center
  font-size: 14px
  font-weight: 600
  flex-shrink: 0

.observe-node-detail__info
  flex: 1

.observe-node-detail__name
  font-size: 14px
  font-weight: 600
  margin: 0 0 4px

.observe-node-detail__section
  margin-bottom: 16px

.observe-node-detail__section-title
  font-size: 12px
  font-weight: 600
  color: var(--color-text-secondary)
  margin: 0 0 8px

.observe-node-detail__error
  display: flex
  align-items: flex-start
  gap: 6px
  padding: 8px
  border-radius: 6px
  background: color-mix(in srgb, var(--color-danger) 8%, transparent)
  color: var(--color-danger)
  font-size: 12px
  word-break: break-word

.observe-node-detail__description
  font-size: 12px
  color: var(--color-text-secondary)
  margin: 0
  line-height: 1.5

.observe-node-detail__text-output
  display: flex
  align-items: flex-start
  gap: 6px
  padding: 8px
  border-radius: 6px
  background: color-mix(in srgb, var(--color-success) 8%, transparent)
  color: var(--color-text-secondary)
  font-size: 12px
  word-break: break-word

.observe-node-detail__member-list
  display: flex
  flex-direction: column
  gap: 8px

.observe-node-detail__member
  display: flex
  align-items: center
  gap: 8px

.observe-node-detail__member-avatar
  position: relative
  width: 28px
  height: 28px
  border-radius: 50%
  background: var(--q-primary)
  color: var(--color-on-accent)
  display: flex
  align-items: center
  justify-content: center
  font-size: 12px
  font-weight: 600
  flex-shrink: 0

  img
    width: 100%
    height: 100%
    border-radius: 50%
    object-fit: cover

.observe-node-detail__member-dot
  position: absolute
  right: -1px
  bottom: -1px
  width: 9px
  height: 9px
  border-radius: 50%
  border: 1.5px solid var(--color-surface-soft)

  &--completed
    background: var(--color-success)

  &--running
    background: var(--color-warning)

  &--failed
    background: var(--color-danger)

  &--pending,
  &--skipped
    background: var(--color-text-tertiary)

.observe-node-detail__member-info
  flex: 1
  min-width: 0
  display: flex
  flex-direction: column

.observe-node-detail__member-name
  font-size: 12px
  font-weight: 500
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.observe-node-detail__member-action
  font-size: 11px
  color: var(--color-text-tertiary)
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.observe-node-detail__member-status
  font-size: 10px
  flex-shrink: 0

  &--completed
    color: var(--color-success)

  &--running
    color: var(--color-warning)

  &--failed
    color: var(--color-danger)

  &--pending,
  &--skipped
    color: var(--color-text-tertiary)

.observe-node-detail__media-grid
  display: grid
  grid-template-columns: repeat(2, 1fr)
  gap: 6px

.observe-node-detail__media-item
  aspect-ratio: 1
  border-radius: 6px
  overflow: hidden
  cursor: pointer
  border: 1px solid var(--color-border-soft)

  &:hover
    border-color: var(--color-accent)

.observe-node-detail__media
  width: 100%
  height: 100%
  object-fit: cover

.observe-node-detail__meta
  font-size: 12px

.observe-node-detail__meta-row
  display: flex
  gap: 6px
  margin-bottom: 4px

.observe-node-detail__meta-label
  color: var(--color-text-tertiary)
  flex-shrink: 0
</style>
