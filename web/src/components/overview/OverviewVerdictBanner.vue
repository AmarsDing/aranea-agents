<template>
  <!--
    基座总览顶部一条：左边四个信号（告警 / 失败运行 / 调用异常 / 积压与审批），右边筛选与刷新。
    2026-11-19 去掉了大标题结论、说明句、「实时」和更新时间（用户：页顶这类状态行浪费空间）；
    整体状态只留顶缘一道色线（绿 / 琥珀 / 红）。
  -->
  <div class="ov-verdict" :class="`ov-verdict--${verdict}`">
    <div class="ov-verdict__chips">
      <span
        v-for="chip in chips"
        :key="chip.key"
        class="ov-verdict__chip"
        :class="`ov-verdict__chip--${chip.severity}`"
      >
        <q-icon :name="chip.icon" size="13px" />
        {{ chip.label }}
      </span>
    </div>
    <div class="ov-verdict__side">
      <slot name="filters" />
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :loading="loading"
        :title="t('overviewPage.v2.refresh')"
        :aria-label="t('overviewPage.v2.refresh')"
        @click="$emit('refresh')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { OvVerdict, OvVerdictChip } from './ovTypes';

withDefaults(
  defineProps<{
    verdict: OvVerdict;
    chips: OvVerdictChip[];
    loading?: boolean;
  }>(),
  { loading: false },
);

defineEmits<{ refresh: [] }>();

const { t } = useI18n();
</script>
