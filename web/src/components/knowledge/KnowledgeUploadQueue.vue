<template>
  <q-card v-if="tasks.length" flat class="knowledge-upload-queue">
    <div class="row items-center justify-between q-px-md q-pt-sm">
      <div class="text-caption text-grey-7">
        {{ queueHeader }}
      </div>
      <q-btn
        flat
        dense
        no-caps
        size="sm"
        :label="t('knowledgePage.queueClearFinished')"
        :disable="!hasFinished"
        @click="$emit('clear-finished')"
      />
    </div>
    <q-list dense class="q-pb-sm">
      <q-item v-for="task in tasks" :key="task.id" class="knowledge-upload-queue__item">
        <q-item-section avatar class="knowledge-upload-queue__status">
          <q-spinner
            v-if="task.status === 'reading' || task.status === 'uploading' || task.status === 'parsing'"
            color="primary"
            size="18px"
          />
          <q-icon v-else-if="task.status === 'success'" name="check_circle" color="positive" size="20px" />
          <q-icon v-else name="error" color="negative" size="20px" />
        </q-item-section>
        <q-item-section>
          <q-item-label class="ellipsis">
            {{ task.name }}
            <q-chip v-if="task.collection_label" dense size="sm" color="primary" text-color="white" class="q-ml-xs">
              {{ task.collection_label }}
            </q-chip>
          </q-item-label>
          <!-- G-21：四段阶段条（读取→上传→解析→入库），失败段标红 -->
          <q-item-label caption class="kuq-stages">
            <template v-for="(s, i) in stageList(task)" :key="s.key">
              <span v-if="i > 0" class="kuq-stage-sep" aria-hidden="true">›</span>
              <span class="kuq-stage" :class="`kuq-stage--${s.state}`">
                <q-icon v-if="s.state === 'done'" name="check" size="11px" />
                {{ s.label }}
              </span>
            </template>
          </q-item-label>
          <q-item-label
            v-if="task.message || task.status === 'error'"
            caption
            class="ellipsis"
            :class="{ 'text-negative': task.status === 'error' }"
          >
            {{ task.message || statusText(task) }}
          </q-item-label>
        </q-item-section>
        <q-item-section side class="text-caption text-grey-7">
          {{ formatKnowledgeDocSize(task.size) }}
        </q-item-section>
        <q-item-section v-if="task.status === 'error' && task.file" side>
          <q-btn
            flat
            dense
            round
            icon="refresh"
            size="sm"
            :aria-label="t('knowledgePage.queueRetryAria')"
            @click="$emit('retry-task', task.id)"
          />
        </q-item-section>
        <q-item-section v-if="task.status === 'success' || task.status === 'error'" side>
          <q-btn
            flat
            dense
            round
            icon="close"
            size="sm"
            :aria-label="t('knowledgePage.queueRemoveAria')"
            @click="$emit('remove-task', task.id)"
          />
        </q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
// 上传队列（SP2-8）：G-21 上传闭环 —— 四段阶段条 + 错误重传。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatKnowledgeDocSize } from '../../features/knowledge/knowledgeUi';
import type { KnowledgeUploadTask } from '../../features/knowledge/types';

const props = defineProps<{
  tasks: KnowledgeUploadTask[];
}>();

defineEmits<{
  'clear-finished': [];
  'remove-task': [id: string];
  'retry-task': [id: string];
}>();

const { t } = useI18n();

const activeCount = computed(
  () =>
    props.tasks.filter((task) => task.status === 'reading' || task.status === 'uploading' || task.status === 'parsing')
      .length,
);
const hasFinished = computed(() => props.tasks.some((task) => task.status === 'success' || task.status === 'error'));
const queueHeader = computed(() => {
  const base = t('knowledgePage.queueTitle');
  if (!activeCount.value) return base;
  return base + t('knowledgePage.queueActiveSuffix', { active: activeCount.value, total: props.tasks.length });
});

function statusText(task: KnowledgeUploadTask): string {
  if (task.status === 'reading') return t('knowledgePage.queueStatusReading');
  if (task.status === 'uploading') return t('knowledgePage.queueStatusUploading');
  if (task.status === 'parsing') return t('knowledgePage.queueStatusParsing');
  if (task.status === 'success') return t('knowledgePage.queueStatusSuccess');
  return t('knowledgePage.queueStatusFailed');
}

type StageState = 'todo' | 'active' | 'done' | 'failed';
const STAGE_KEYS = ['read', 'upload', 'parse', 'index'] as const;

/** 任务态 → 四段条各段状态：段序 read→upload→parse→index；success 全 done；error 定位 fail_stage。 */
function stageList(task: KnowledgeUploadTask): { key: string; label: string; state: StageState }[] {
  const activeKey =
    task.status === 'reading'
      ? 'read'
      : task.status === 'uploading'
        ? 'upload'
        : task.status === 'parsing'
          ? 'parse'
          : '';
  return STAGE_KEYS.map((key, i) => {
    let state: StageState = 'todo';
    if (task.status === 'success') state = 'done';
    else if (task.status === 'error') {
      const failIdx = STAGE_KEYS.indexOf((task.fail_stage || 'upload') as (typeof STAGE_KEYS)[number]);
      if (i < failIdx) state = 'done';
      else if (i === failIdx) state = 'failed';
      else state = 'todo';
    } else if (key === activeKey) state = 'active';
    else if (STAGE_KEYS.indexOf(key) < STAGE_KEYS.indexOf((activeKey || 'read') as (typeof STAGE_KEYS)[number]))
      state = 'done';
    return {
      key,
      label: t(`knowledgePage.queueStage.${key}`),
      state,
    };
  });
}
</script>

<style scoped>
.knowledge-upload-queue {
  margin-bottom: 12px;
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  background: var(--glass-surface);
}

.knowledge-upload-queue__item {
  min-height: 44px;
}

/* G-21 四段阶段条 */
.kuq-stages {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px;
  margin-top: 2px;
}

.kuq-stage {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 10.5px;
  padding: 0 6px;
  border-radius: 999px;
  border: 1px solid var(--glass-border);
  color: var(--color-text-secondary);
}

.kuq-stage--active {
  color: var(--q-primary);
  border-color: var(--q-primary);
  animation: kuq-stage-pulse 1.4s ease-in-out infinite;
}

.kuq-stage--done {
  color: var(--q-positive);
  border-color: transparent;
  opacity: 0.85;
}

.kuq-stage--failed {
  color: var(--q-negative);
  border-color: var(--q-negative);
}

.kuq-stage-sep {
  font-size: 10px;
  color: var(--color-text-tertiary);
}

@keyframes kuq-stage-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.55;
  }
}
</style>
