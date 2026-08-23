<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', !!$event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog kb-writeback-review">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">{{ t('knowledgePage.workbench.commands.review-writeback') }}</div>
      </q-card-section>
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body">
      <div v-if="!homeIsCurrent" class="kb-writeback-review__banner">
        {{ t('knowledgePage.workbench.writebackHomeHint', { name: homeName }) }}
        <q-btn
          flat
          dense
          no-caps
          color="primary"
          class="q-ml-sm"
          :label="t('knowledgePage.workbench.writebackHomeSwitch')"
          @click="$emit('switch-home')"
        />
      </div>
      <div class="kb-writeback-review__toolbar">
        <q-checkbox
          :model-value="allChecked"
          dense
          :label="t('knowledgePage.workbench.pendingSelectAll')"
          @update:model-value="toggleAll"
        />
        <span class="kb-writeback-review__count">{{ selectedIds.length }}/{{ items.length }}</span>
      </div>
      <q-list dense class="kb-writeback-review__list">
        <q-item v-for="it in items" :key="it.fact_id" tag="label" clickable>
          <q-item-section avatar>
            <q-checkbox :model-value="selected.has(it.fact_id)" @update:model-value="(v: boolean) => toggle(it.fact_id, v)" />
          </q-item-section>
          <q-item-section>
            <q-item-label>{{ it.statement }}</q-item-label>
            <q-item-label caption>
              {{ it.kind }} · {{ it.confidence.toFixed(2) }}
              <template v-if="it.agent_id"> · {{ it.agent_id }}</template>
            </q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
        </q-card-section>
      </div>
      <q-card-actions align="right" class="app-glass-dialog__actions">
        <q-btn flat no-caps :label="t('common.cancel')" @click="$emit('update:open', false)" />
        <q-btn
          unelevated
          no-caps
          color="primary"
          :label="t('knowledgePage.workbench.pendingApplySelected')"
          :disable="selectedIds.length === 0"
          :loading="loading"
          @click="$emit('submit', selectedIds)"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PendingWriteBackItem } from '../../features/knowledge/types';

const props = defineProps<{
  open: boolean;
  items: PendingWriteBackItem[];
  homeName: string;
  homeIsCurrent: boolean;
  loading?: boolean;
}>();

defineEmits<{
  'update:open': [open: boolean];
  submit: [factIds: string[]];
  'switch-home': [];
}>();

const { t } = useI18n();
const selected = ref(new Set<string>());

watch(
  () => [props.open, props.items] as const,
  ([open, items]) => {
    if (!open) return;
    selected.value = new Set(items.map((it) => it.fact_id));
  },
  { immediate: true },
);

const selectedIds = computed(() => props.items.map((it) => it.fact_id).filter((id) => selected.value.has(id)));
const allChecked = computed(() => props.items.length > 0 && selectedIds.value.length === props.items.length);

function toggle(id: string, on: boolean) {
  const next = new Set(selected.value);
  if (on) next.add(id);
  else next.delete(id);
  selected.value = next;
}

function toggleAll(on: boolean) {
  selected.value = on ? new Set(props.items.map((it) => it.fact_id)) : new Set();
}
</script>

<style lang="sass" scoped>
.kb-writeback-review
  &__banner
    font-size: 12px
    color: var(--color-text-secondary)
    margin-bottom: 12px

  &__toolbar
    display: flex
    align-items: center
    justify-content: space-between
    margin-bottom: 8px

  &__count
    font-size: 12px
    color: var(--color-text-secondary)

  &__list
    max-height: 360px
    overflow: auto
    margin: 0 -4px
</style>
