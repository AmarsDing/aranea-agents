<template>
  <section class="today-tasks">
    <header class="today-panel__head">
      <h2 class="today-panel__title">
        {{ t('today.tasks.title') }}
        <span class="today-panel__count">{{ tasks.length }}</span>
      </h2>
    </header>

    <form class="today-tasks__add" @submit.prevent="submit">
      <q-input
        v-model="draft"
        dense
        outlined
        maxlength="200"
        :placeholder="t('today.tasks.addPlaceholder')"
        :aria-label="t('today.tasks.addLabel')"
        :disable="adding"
      />
      <q-btn type="submit" flat no-caps :label="t('today.tasks.add')" :loading="adding" :disable="!draft.trim()" />
    </form>
    <p v-if="error" class="today-note today-note--danger" role="alert">
      {{ t('today.tasks.saveFailed', { message: error }) }}
    </p>

    <p v-if="tasksStatus === 'error'" class="today-note today-note--danger" role="alert">
      {{ t('today.tasks.failed') }}
    </p>
    <p v-else-if="tasksStatus === 'ok' && tasks.length === 0" class="today-empty">{{ t('today.tasks.empty') }}</p>
    <ul v-else class="today-tasks__list">
      <li v-for="task in tasks" :key="task.id" class="today-tasks__row">
        <q-checkbox
          :model-value="false"
          dense
          :disable="taskBusy.has(task.id)"
          :aria-label="t('today.tasks.complete', { title: task.title })"
          @update:model-value="emit('complete', task)"
        />
        <span class="today-tasks__title">{{ task.title }}</span>
        <router-link v-if="task.source === 'mail'" class="today-tag" to="/mail">
          {{ t('today.tasks.fromMail') }}
        </router-link>
      </li>
    </ul>
    <p v-if="lastDone" class="today-tasks__undo" role="status" aria-live="polite">
      {{ t('today.tasks.completed', { title: lastDone.title }) }}
      <button type="button" class="today-link" @click="emit('undo', lastDone)">{{ t('today.tasks.undo') }}</button>
    </p>

    <header class="today-panel__head today-panel__head--sub">
      <h2 class="today-panel__title">
        {{ t('today.delegated.title') }}
        <span class="today-panel__count">{{ delegated.length }}</span>
      </h2>
      <router-link class="today-link" to="/coding-tasks">{{ t('today.delegated.viewAll') }}</router-link>
    </header>
    <p v-if="delegatedStatus === 'error'" class="today-note today-note--danger" role="alert">
      {{ t('today.delegated.failed') }}
    </p>
    <p v-else-if="delegatedStatus === 'ok' && delegated.length === 0" class="today-empty">
      {{ t('today.delegated.empty') }}
    </p>
    <ul v-else class="today-tasks__list">
      <li v-for="row in visibleDelegated" :key="row.id">
        <router-link class="today-delegated__row" to="/coding-tasks">
          <span class="today-delegated__title">{{ row.title }}</span>
          <span v-if="row.agentKey" class="today-mono today-delegated__agent">{{ row.agentKey }}</span>
          <span class="today-state" :class="`today-state--${row.state}`">
            {{ t(`today.delegated.state.${row.state}`) }}
          </span>
        </router-link>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { OfficeTaskRow } from '../../features/office/types';
import type { DelegatedRow } from '../../features/today/logic';
import type { BlockStatus } from '../../features/today/useTodaySide';

const MAX_DELEGATED_ROWS = 8;

const props = defineProps<{
  tasks: OfficeTaskRow[];
  tasksStatus: BlockStatus;
  taskBusy: Set<string>;
  /** 最近一次勾选完成的任务（提供撤销）；没有时为 null */
  lastDone: OfficeTaskRow | null;
  delegated: DelegatedRow[];
  delegatedStatus: BlockStatus;
  /** 新建任务：返回错误文案，成功返回空串 */
  add: (title: string) => Promise<string>;
}>();

const emit = defineEmits<{
  complete: [task: OfficeTaskRow];
  undo: [task: OfficeTaskRow];
}>();

const { t } = useI18n();

const draft = ref('');
const adding = ref(false);
const error = ref('');

const visibleDelegated = computed(() => props.delegated.slice(0, MAX_DELEGATED_ROWS));

async function submit() {
  const title = draft.value.trim();
  if (!title || adding.value) return;
  adding.value = true;
  error.value = await props.add(title);
  adding.value = false;
  if (!error.value) draft.value = '';
}
</script>
