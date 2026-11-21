<template>
  <!-- 任务页的两种视图：看板（按阶段，管计划和分工）/ 列表（按最近动静，看每件事交给了谁、做到哪）。同一批任务 -->
  <nav class="tk-views" :aria-label="t('taskBoard.view.label')">
    <router-link
      v-for="v in VIEWS"
      :key="v.to"
      class="tk-views__item"
      :class="{ 'tk-views__item--on': route.path === v.to }"
      :to="v.to"
      :aria-current="route.path === v.to ? 'page' : undefined"
    >
      <q-icon :name="v.icon" size="16px" aria-hidden="true" />{{ t(v.labelKey) }}
    </router-link>
  </nav>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { useRoute } from 'vue-router';

const VIEWS = [
  { to: '/tasks', icon: 'view_kanban', labelKey: 'taskBoard.view.board' },
  { to: '/tasks/list', icon: 'view_list', labelKey: 'taskBoard.view.list' },
] as const;

const { t } = useI18n();
const route = useRoute();
</script>
