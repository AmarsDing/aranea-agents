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
import { useNodeOutputStore } from '../../../stores/chat/nodeOutputStore';
import ObserveStatusBadge from './ObserveStatusBadge.vue';

const { t } = useI18n();

const props = defineProps<{ node: GraphNode }>();
defineEmits<{
  close: [];
  preview: [art: MediaArtifact];
}>();

const nodeOutputStore = useNodeOutputStore();

const nodeInitial = computed(() => (props.node.Label || '?').charAt(0).toUpperCase());

const mediaOutputs = computed(() => nodeOutputStore.getNodeOutput(props.node.TeamStageID || props.node.ID));
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
