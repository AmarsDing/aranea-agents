<template>
  <div class="permission-list">
    <div v-for="(p, i) in permissions" :key="i" class="permission-list__row row items-start no-wrap q-gutter-sm">
      <q-icon :name="kindIcon(p.kind)" size="16px" :class="`permission-list__icon--${p.risk}`" class="q-mt-xs" />
      <div class="col">
        <div class="row items-center q-gutter-xs">
          <span class="text-weight-medium permission-list__kind">{{ permKindLabel(p.kind) }}</span>
          <q-badge :class="`permission-list__risk--${p.risk}`" :label="t(`shopPage.risk.${p.risk}`)" />
        </div>
        <div class="text-caption permission-list__value ellipsis">{{ p.value }}</div>
        <div v-if="p.note" class="text-caption text-grey-7">{{ p.note }}</div>
      </div>
    </div>
    <div v-if="permissions.length === 0" class="text-caption text-grey-7 q-pa-sm">
      {{ t('shopPage.permNone') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { MarketPermission } from '../../features/ecosystem/types';

defineProps<{
  permissions: MarketPermission[];
}>();

const { t, te } = useI18n();

function permKindLabel(kind: string): string {
  const key = `shopPage.permKind.${kind}`;
  return te(key) ? t(key) : kind;
}

const KIND_ICONS: Record<string, string> = {
  model: 'model_training',
  tool: 'handyman',
  credential: 'key',
  network: 'public',
  fs: 'folder',
  command: 'terminal',
  mcp: 'extension',
};

function kindIcon(kind: string): string {
  return KIND_ICONS[kind] ?? 'shield';
}
</script>

<style scoped>
.permission-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.permission-list__kind {
  font-size: 13px;
}
.permission-list__value {
  color: var(--color-text-secondary);
  font-family: monospace;
}
.permission-list__icon--low {
  color: var(--color-success);
}
.permission-list__icon--medium {
  color: var(--color-warning);
}
.permission-list__icon--high {
  color: var(--color-danger);
}
.permission-list__risk--low {
  background: rgba(76, 175, 124, 0.15);
  color: var(--color-success);
}
.permission-list__risk--medium {
  background: rgba(240, 155, 84, 0.15);
  color: var(--color-warning);
}
.permission-list__risk--high {
  background: rgba(229, 92, 92, 0.15);
  color: var(--color-danger);
}
</style>
