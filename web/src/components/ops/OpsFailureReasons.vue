<template>
  <!-- 失败原因：同一错误合成一行；认得出的错误写成人话并给修复去处，涉及的 Agent 可点进它的设置 -->
  <section class="op-panel" :aria-label="title">
    <header class="op-panel__head">
      <h2 class="op-panel__title">
        {{ title }}<span class="op-panel__count op-num">{{ total }}</span>
      </h2>
      <span v-if="hint" class="op-panel__meta">{{ hint }}</span>
    </header>
    <p v-if="error" class="op-note op-note--danger" role="alert">{{ t('ops.common.loadFailed', { msg: error }) }}</p>
    <p v-else-if="!reasons.length && !loading" class="op-empty">{{ t('ops.failures.none') }}</p>
    <ul v-else class="op-list">
      <li v-for="r in reasons.slice(0, limit)" :key="r.key" class="op-row op-fail">
        <span class="op-dot op-dot--alert" aria-hidden="true" />
        <span class="op-row__main">
          <span class="op-row__title">{{ r.cause ? t(`ops.failures.cause.${r.cause}`) : r.message }}</span>
          <span v-if="r.cause" class="op-row__sub" :title="r.message">{{ r.message }}</span>
          <span class="op-fail__agents">
            <template v-if="r.fix === 'agent'">
              <router-link v-for="a in r.agents" :key="a.id" class="op-link" :to="`/agents/${a.id}/settings`">
                {{ t('ops.failures.fixAgent', { name: a.name }) }}
              </router-link>
            </template>
            <router-link v-else-if="r.fix === 'credentials'" class="op-link" to="/credentials">
              {{ t('ops.failures.fixCredentials') }}
            </router-link>
            <span v-if="r.fix !== 'agent' && r.agents.length" class="op-row__sub">
              {{ r.agents.map((a) => a.name).join('、') }}
            </span>
          </span>
        </span>
        <span class="op-tag op-tag--alert op-num">{{ t('ops.failures.times', { n: r.count }) }}</span>
        <span class="op-row__time op-num">{{ relativeTime(t, r.lastAt) }}</span>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { relativeTime } from '../../features/monitor/eventView';
import type { FailureReason } from '../../features/usage/useFailureReasons';

withDefaults(
  defineProps<{
    title: string;
    hint?: string;
    reasons: FailureReason[];
    total: number;
    loading: boolean;
    error: string;
    limit?: number;
  }>(),
  { hint: '', limit: 6 },
);

const { t } = useI18n();
</script>
