<!--
  Cron 表单：计划类型与参数（纯展示子块，v-model:form）。
-->
<template>
  <div class="app-grid-span-full">
    <div class="section-label q-mb-sm">计划类型</div>
    <q-btn-toggle
      v-model="form.schedule_type"
      spread
      no-caps
      unelevated
      toggle-color="primary"
      class="cron-btn-toggle"
      :options="cronScheduleToggleOptions"
    />
  </div>

  <q-input
    v-if="form.schedule_type === 'interval'"
    v-model.number="form.interval_minutes"
    class="cron-field"
    dense
    outlined
    type="number"
    min="1"
    suffix="分钟"
    label="每隔 *"
    :rules="[cronPositiveMinutesRule]"
  />
  <q-input
    v-if="form.schedule_type === 'cron'"
    v-model="form.cron_expression"
    class="cron-field app-grid-span-full app-field-long"
    dense
    outlined
    label="Cron 表达式 *"
    placeholder="0 * * * *"
    :hint="cronHint"
    :rules="[cronExpressionRule]"
  />
  <template v-if="form.schedule_type === 'once'">
    <q-input
      v-model="form.run_at_date"
      class="cron-field"
      dense
      outlined
      mask="####-##-##"
      label="执行日期 *"
      placeholder="2026-04-22"
    >
      <template #append>
        <q-icon name="event" class="cursor-pointer">
          <q-popup-proxy cover transition-show="scale" transition-hide="scale">
            <q-date v-model="form.run_at_date" mask="YYYY-MM-DD" />
          </q-popup-proxy>
        </q-icon>
      </template>
    </q-input>
    <q-input
      v-model="form.run_at_time"
      class="cron-field"
      dense
      outlined
      mask="##:##"
      label="执行时间 *"
      placeholder="09:00"
    >
      <template #append>
        <q-icon name="access_time" class="cursor-pointer">
          <q-popup-proxy cover transition-show="scale" transition-hide="scale">
            <q-time v-model="form.run_at_time" format24h />
          </q-popup-proxy>
        </q-icon>
      </template>
    </q-input>
  </template>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { CronTaskFormValue } from '../../features/cron/types';
import {
  cronExpressionRule,
  cronPositiveMinutesRule,
  cronScheduleToggleOptions,
  describeCronExpression,
} from './cronTaskUtils';

const form = defineModel<CronTaskFormValue>('form', { required: true });

// 边输边翻译：看得懂就直接告诉用户「工作日 08:30」，看不懂再给字段顺序提示
const cronHint = computed(() => {
  const text = describeCronExpression(form.value.cron_expression);
  return text ? `= ${text}（按所选时区）` : '5 个字段：分 时 日 月 周，例如 30 8 * * 1-5 = 工作日 08:30';
});
</script>
