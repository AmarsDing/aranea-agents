<!--
  Cron 表单字段区：q-form + 校验暴露（aranea-frontend-guide SKILL §1 红线 #1，无 API）。
  皮肤：UX token，见父级 CronTaskFormDialog 与同文件 scoped :deep。
-->
<template>
  <q-form
    ref="formRef"
    class="app-form-field-grid app-form-field-grid--2col cron-form-fields-root"
    @submit.prevent="$emit('submit')"
  >
    <q-input
      v-model="form.name"
      class="cron-field"
      dense
      outlined
      label="标识 *"
      placeholder="my-daily-task"
      :rules="[cronSlugRule]"
    />
    <q-input v-model="form.display_name" class="cron-field" dense outlined label="展示名称" />
    <q-input
      v-model="form.description"
      class="cron-field app-grid-span-full"
      dense
      outlined
      autogrow
      type="textarea"
      label="描述"
    />

    <CronTaskFormTargetFields v-model:form="form" :agents="agents" :teams="teams" />
    <CronTaskFormScheduleFields v-model:form="form" />

    <q-input v-model="form.timezone" class="cron-field" dense outlined label="时区" placeholder="Asia/Shanghai" />
    <q-input
      v-model.number="form.retry_max_attempts"
      class="cron-field"
      dense
      outlined
      type="number"
      min="0"
      max="4"
      label="失败重试次数"
      hint="0=不重试，默认 3 次（30s/2m/10m 退避，不含首次执行）"
    />
    <q-toggle v-model="form.enabled" color="primary" label="启用任务" />
    <q-input
      v-model="form.message"
      class="cron-field app-grid-span-full app-field-long"
      dense
      outlined
      autogrow
      type="textarea"
      label="消息 *"
      placeholder="Agent 应该做什么?"
      :rules="[cronMessageRule]"
    />

    <div v-if="serverError" class="app-grid-span-full text-negative">{{ serverError }}</div>
  </q-form>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import type { QForm } from 'quasar';
import type { Agent } from '../../features/agents/types';
import type { CronTaskFormValue } from '../../features/cron/types';
import type { Team } from '../../features/teams/types';
import CronTaskFormScheduleFields from './CronTaskFormScheduleFields.vue';
import CronTaskFormTargetFields from './CronTaskFormTargetFields.vue';
import { cronMessageRule, cronSlugRule } from './cronTaskUtils';

defineProps<{
  agents: Agent[];
  teams: Team[];
  serverError?: string;
}>();

defineEmits<{
  submit: [];
}>();

const form = defineModel<CronTaskFormValue>('form', { required: true });

const formRef = ref<QForm>();

defineExpose({
  validate: () => formRef.value?.validate(),
});
</script>
