<template>
  <section class="settings-section">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">观察记录</span>
        </div>
        <p class="settings-section__hint">Agent 运行时收集的原始行为数据（近 30 天，最新在前）。</p>
      </div>
    </div>
    <q-inner-loading :showing="loading" label="加载观察记录..." />
    <template v-if="!loading && observations.length > 0">
      <q-list separator class="app-glass-list">
        <q-item v-for="item in pagedObservations" :key="item.id" class="app-glass-list__item--md">
          <q-item-section side>
            <q-icon :name="observationKindIcon(item.kind)" :color="observationKindColor(item.kind)" />
          </q-item-section>
          <q-item-section>
            <q-item-label>{{ item.content || observationKindLabel(item.kind) }}</q-item-label>
            <q-item-label caption>
              <span
                v-if="item.session_id"
                class="observation-session-link"
                @click="copySessionId(item.session_id)"
              >
                Session: {{ item.session_id.slice(0, 12) }}...
                <q-tooltip>{{ item.session_id }}（点击复制）</q-tooltip>
              </span>
              <span v-else>Session: —</span>
              · {{ formatDateTime(item.observed_at) }}
            </q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
      <div v-if="pageCount > 1" class="row justify-center q-mt-md">
        <q-pagination
          v-model="page"
          :max="pageCount"
          :max-pages="6"
          boundary-numbers
          direction-links
          color="primary"
          size="sm"
        />
      </div>
    </template>
    <q-banner v-else-if="!loading" rounded class="settings-placeholder-banner"> 暂无观察记录。 </q-banner>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { copyToClipboard, useQuasar } from 'quasar';
import type { LearningObservation } from '../../features/agents/learning.types';
import { formatDateTime } from '../../features/agents/learning.utils';
import { i18n } from '../../i18n';

const t = i18n.global.t;

const PAGE_SIZE = 20;

const props = defineProps<{
  observations: LearningObservation[];
  loading: boolean;
}>();

const $q = useQuasar();
const page = ref(1);

const pageCount = computed(() => Math.ceil(props.observations.length / PAGE_SIZE));
const pagedObservations = computed(() => {
  const start = (page.value - 1) * PAGE_SIZE;
  return props.observations.slice(start, start + PAGE_SIZE);
});

// 数据刷新（如重新运行闭环）后回到第一页，避免停留在失效页码。
watch(() => props.observations, () => {
  page.value = 1;
});

async function copySessionId(sessionId: string) {
  try {
    await copyToClipboard(sessionId);
    $q.notify({ type: 'positive', message: 'Session ID 已复制', timeout: 1200 });
  } catch {
    $q.notify({ type: 'negative', message: '复制失败' });
  }
}

function observationKindIcon(kind: string): string {
  switch (kind) {
    case 'tool_call':
      return 'build';
    case 'feedback':
      return 'chat';
    case 'memory_hit':
      return 'psychology';
    case 'memory_miss':
      return 'psychology';
    default:
      return 'visibility';
  }
}

function observationKindColor(kind: string): string {
  switch (kind) {
    case 'tool_call':
      return 'blue';
    case 'feedback':
      return 'purple';
    case 'memory_hit':
      return 'teal';
    case 'memory_miss':
      return 'grey';
    default:
      return 'grey';
  }
}

function observationKindLabel(kind: string): string {
  switch (kind) {
    case 'tool_call':
      return t('agents.learning_loop.kind_tool_call');
    case 'feedback':
      return t('agents.learning_loop.kind_feedback');
    case 'memory_hit':
      return t('agents.learning_loop.kind_memory_hit');
    case 'memory_miss':
      return t('agents.learning_loop.kind_memory_miss');
    default:
      return kind;
  }
}
</script>

<style scoped>
.observation-session-link {
  cursor: pointer;
}
.observation-session-link:hover {
  text-decoration: underline;
}
</style>
