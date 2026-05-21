<!--
  Cron 表单：计划类型与参数（纯展示子块，v-model:form）。
-->
<template>
  <div class="app-grid-span-full">
    <div class="section-label q-mb-sm">计划类型</div>
    <q-btn-toggle v-model="form.schedule_type" spread no-caps unelevated toggle-color="primary" class="cron-btn-toggle" :options="cronScheduleToggleOptions" />
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
    hint="标准 5 字段 cron: 分 时 日 月 周"
    :rules="[cronExpressionRule]"
  />
  <template v-if="form.schedule_type === 'once'">
    <q-input v-model="form.run_at_date" class="cron-field" dense outlined mask="####-##-##" label="执行日期 *" placeholder="2026-04-22">
      <template #append>
        <q-icon name="event" class="cursor-pointer">
          <q-popup-proxy cover transition-show="scale" transition-hide="scale">
            <q-date v-model="form.run_at_date" mask="YYYY-MM-DD" />
          </q-popup-proxy>
        </q-icon>
      </template>
    </q-input>
    <q-input v-model="form.run_at_time" class="cron-field" dense outlined mask="##:##" label="执行时间 *" placeholder="09:00">
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
import type { CronTaskFormValue } from "../../features/cron/types";
import {
  cronExpressionRule,
  cronPositiveMinutesRule,
  cronScheduleToggleOptions
} from "./cronTaskUtils";

const form = defineModel<CronTaskFormValue>("form", { required: true });
</script>
