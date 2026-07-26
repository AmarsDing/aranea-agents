// Container: approved — 层级全景 Tab 容器；useLayerOverview 拉取数据，子组件纯展示。
<template>
  <div class="column q-gutter-md">
    <q-banner v-if="error" rounded class="bg-negative text-white">
      {{ error }}
      <template #action>
        <q-btn flat color="white" :label="t('memory.error.retry')" @click="reload" />
      </template>
    </q-banner>

    <div v-if="loading && !overview" class="text-center q-pa-xl">
      <q-spinner-dots size="40px" color="primary" />
    </div>

    <template v-else-if="overview">
      <layer-flow-cards :layers="overview.layers" @drill="(layer) => emit('drill-layer', layer)" />

      <div class="row q-col-gutter-md">
        <div class="col-12 col-lg-5">
          <memory-action-items :items="overview.action_items" @navigate="onActionNavigate" />
        </div>
        <div class="col-12 col-lg-7">
          <memory-activity-feed :items="overview.activity_feed" />
        </div>
      </div>
    </template>

    <q-card v-else flat class="memory-card text-center text-grey-7 q-pa-xl">
      {{ t('memory.panorama.selectAgent') }}
    </q-card>
  </div>
</template>

<script setup lang="ts">
import { toRef } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MemoryActionItem } from '../types';
import { useLayerOverview } from './composables/useLayerOverview';
import LayerFlowCards from './LayerFlowCards.vue';
import MemoryActionItems from './MemoryActionItems.vue';
import MemoryActivityFeed from './MemoryActivityFeed.vue';

const props = defineProps<{ agentId: string | null; sessionId: string | null }>();
const emit = defineEmits<{
  (e: 'drill-layer', layer: string): void;
  (e: 'navigate-tab', tab: string): void;
}>();

const { t } = useI18n();
const { overview, loading, error, reload } = useLayerOverview(toRef(props, 'agentId'), toRef(props, 'sessionId'));

function onActionNavigate(item: MemoryActionItem) {
  emit('navigate-tab', item.target_tab);
}
</script>
