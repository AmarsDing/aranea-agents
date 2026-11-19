<template>
  <q-banner v-if="error" rounded class="app-page-error-banner" role="alert">
    A2A 运行配置加载失败：{{ error }}
    <template #action>
      <q-btn flat :label="t('shell.common.retry')" @click="emit('retry')" />
    </template>
  </q-banner>
  <q-banner v-else-if="config" rounded class="app-info-banner">
    <div class="text-subtitle2">A2A 公开 Endpoint</div>
    <div class="text-body2">{{ config.public_base_url || '—' }}</div>
    <div class="text-caption app-text-secondary">
      来源：{{ sourceLabel }} · 可在
      <router-link to="/settings">系统设置</router-link>
      配置 A2A 公开前缀（保存后立即生效）
    </div>
  </q-banner>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { A2ARuntimeConfig } from '../../features/a2a/types';

const props = defineProps<{
  config: A2ARuntimeConfig | null;
  /** 配置探测失败信息；非空时显示可重试错误，而不是静默隐藏横幅。 */
  error?: string;
}>();

const emit = defineEmits<{ retry: [] }>();
const { t } = useI18n();

const sourceLabel = computed(() => {
  const s = props.config?.public_base_url_source;
  if (!s) return '未配置';
  if (s === 'env') return '环境变量';
  if (s === 'db') return '系统设置';
  if (s === 'config') return '配置文件';
  if (s === 'derived') return '推导（开发默认）';
  return s;
});
</script>
