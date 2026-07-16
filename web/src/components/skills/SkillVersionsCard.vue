<template>
  <q-card flat bordered>
    <q-card-section class="row items-center justify-between">
      <div class="text-subtitle2">版本历史</div>
      <q-btn flat dense round icon="refresh" :loading="loading" @click="$emit('refresh')">
        <q-tooltip>刷新</q-tooltip>
      </q-btn>
    </q-card-section>
    <q-separator />
    <q-card-section v-if="error" class="text-negative">{{ error }}</q-card-section>
    <q-card-section v-else-if="loading && versions.length === 0" class="text-grey-7">加载中…</q-card-section>
    <q-card-section v-else-if="versions.length === 0" class="text-grey-7">暂无版本记录</q-card-section>
    <q-list v-else separator>
      <q-item v-for="v in versions" :key="v.id">
        <q-item-section>
          <q-item-label>{{ v.version || v.id }}</q-item-label>
          <q-item-label caption>
            {{ v.validation_status || '-' }}
            · {{ formatDate(v.published_at || v.created_at) }}
          </q-item-label>
        </q-item-section>
        <q-item-section side>
          <q-btn
            flat
            dense
            no-caps
            color="primary"
            label="回滚"
            :disable="!canRollback || rollingId === v.id"
            :loading="rollingId === v.id"
            @click="$emit('rollback', v)"
          />
        </q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
import type { SkillVersionDetail } from '../../features/skills/types';

defineProps<{
  versions: SkillVersionDetail[];
  loading?: boolean;
  error?: string;
  canRollback?: boolean;
  rollingId?: string;
}>();

defineEmits<{
  refresh: [];
  rollback: [version: SkillVersionDetail];
}>();

function formatDate(value?: string) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}
</script>
