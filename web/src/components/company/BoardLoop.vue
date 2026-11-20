<template>
  <!-- 闭环四格：等你拍板 / 在做 / 卡住 / 待验收。非 0 才上色，颜色旁边一律有文字。 -->
  <dl class="kb-loop" :class="{ 'kb-loop--sm': small }">
    <div v-for="key in KEYS" :key="key" class="kb-loop__cell" :class="{ [`kb-loop__cell--${key}`]: loop[key] > 0 }">
      <dt>{{ t(`company.loop.${key}`) }}</dt>
      <dd class="kb-mono">{{ loop[key] }}</dd>
    </div>
  </dl>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { Loop } from '../../features/company/tree';

const KEYS = ['waiting', 'running', 'stuck', 'review'] as const;

defineProps<{ loop: Loop; small?: boolean }>();
const { t } = useI18n();
</script>
