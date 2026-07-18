<template>
  <div class="observation-panel">
    <!-- Toolbar -->
    <div class="observation-panel__toolbar">
      <q-btn flat dense icon="refresh" :loading="loading" @click="refresh">
        <q-tooltip>{{ t('observe.refresh') }}</q-tooltip>
      </q-btn>
      <q-space />
      <q-badge v-if="liveConnected" rounded color="positive" :label="t('observe.live')" />
    </div>

    <!-- Canvas area -->
    <div class="observation-panel__canvas">
      <ObservationCanvas
        v-if="graphStage"
        :graph-stage="graphStage"
        :spirit-session-id="spiritSessionId"
        :is-dark="isDark"
        @select-node="onSelectNode"
        @preview="onPreview"
      />
      <div v-else class="observation-panel__empty">
        <q-icon name="visibility_off" size="48px" color="grey-5" />
        <p class="text-grey-6">{{ t('observe.noActiveGraph') }}</p>
      </div>
    </div>

    <!-- Node detail sidebar -->
    <ObserveNodeDetail v-if="selectedNode" :node="selectedNode" @close="selectedNode = null" @preview="onPreview" />

    <!-- Fullscreen media preview -->
    <MediaLightbox v-if="previewArtifact" :artifact="previewArtifact" @close="previewArtifact = null" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import ObservationCanvas from './ObservationCanvas.vue';
import ObserveNodeDetail from './ObserveNodeDetail.vue';
import MediaLightbox from './MediaLightbox.vue';
import { useObserveGraph } from '../../../features/chat/composables/useObserveGraph';
import type { GraphNode } from '../../../features/chat/v2Types';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';

const { t } = useI18n();

const props = defineProps<{
  sessionId: string;
  spiritSessionId: string;
  isDark: boolean;
  wsConnected?: boolean;
}>();

const spiritSessionIdRef = computed(() => props.spiritSessionId);
const { graphStage } = useObserveGraph(spiritSessionIdRef);

const selectedNode = ref<GraphNode | null>(null);
const previewArtifact = ref<MediaArtifact | null>(null);
const loading = ref(false);
const liveConnected = computed(() => props.wsConnected ?? false);

function onSelectNode(node: GraphNode) {
  selectedNode.value = node;
}

function onPreview(art: MediaArtifact) {
  previewArtifact.value = art;
}

function refresh() {
  loading.value = true;
  // activityV2Store auto-refreshes via WebSocket; this is a visual ack only.
  setTimeout(() => {
    loading.value = false;
  }, 500);
}
</script>

<style scoped lang="sass">
.observation-panel
  display: flex
  flex-direction: column
  width: 100%
  height: 100%
  min-height: 0
  position: relative

.observation-panel__toolbar
  display: flex
  align-items: center
  padding: 4px 8px
  border-bottom: 1px solid var(--color-border-soft)
  flex-shrink: 0

.observation-panel__canvas
  flex: 1
  min-height: 0
  position: relative

.observation-panel__empty
  display: flex
  flex-direction: column
  align-items: center
  justify-content: center
  height: 100%
  gap: 8px
</style>
