<template>
  <AppDialog
    :model-value="open"
    :title="result ? t('knowledgePage.promoteResultTitle') : t('knowledgePage.promoteTitle')"
    :subtitle="docName"
    size="sm"
    :loading="loading"
    @update:model-value="$emit('update:open', $event)"
  >
    <!-- 阶段一：目标库选择 -->
    <template v-if="!result">
      <q-select
            :model-value="targetId"
            dense
            outlined
            emit-value
            map-options
            options-dense
            :options="options"
            :label="t('knowledgePage.promoteTargetLabel')"
            @update:model-value="$emit('update:targetId', String($event ?? ''))"
          >
            <template #no-option>
              <q-item>
                <q-item-section class="text-grey-7">{{ t('knowledgePage.promoteEmpty') }}</q-item-section>
              </q-item>
            </template>
          </q-select>
      <div class="text-caption text-grey-6 q-mt-sm">{{ t('knowledgePage.promoteHint') }}</div>
    </template>

    <!-- 阶段二：结果反馈（新建块数 + 级联提示清单） -->
    <template v-else>
      <q-banner dense rounded class="bg-positive text-white q-mb-sm">
            {{ t('knowledgePage.promoteResultCreated', { count: result.created_blocks.length }) }}
          </q-banner>
      <template v-if="result.cascade_candidates.length">
            <q-banner dense rounded class="app-banner-warning q-mb-xs">
              {{ t('knowledgePage.promoteCascadeTitle', { count: result.cascade_candidates.length }) }}
            </q-banner>
            <q-list dense separator class="knowledge-promote-dialog__cascade">
              <q-item v-for="(c, i) in result.cascade_candidates" :key="i">
                <q-item-section avatar>
                  <q-icon name="link_off" size="16px" color="warning" />
                </q-item-section>
                <q-item-section>
                  <q-item-label lines="1">{{ c.raw_target }}</q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
            <div class="text-caption text-grey-6 q-mt-xs">{{ t('knowledgePage.promoteCascadeHint') }}</div>
          </template>
    </template>

    <template #actions>
      <template v-if="!result">
        <q-btn flat no-caps :label="t('knowledgePage.promoteCancel')" @click="$emit('update:open', false)" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="t('knowledgePage.promoteSubmit')"
          :disable="!targetId"
          :loading="loading"
          @click="$emit('submit')"
        />
      </template>
      <q-btn
        v-else
        color="primary"
        unelevated
        no-caps
        :label="t('knowledgePage.promoteClose')"
        @click="$emit('update:open', false)"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../../components/common/AppDialog.vue';
import type { PromoteResult } from '../../features/knowledge/types';

// SP1-G/I-3 晋升对话框：阶段一选目标团队库；阶段二展示新建块谱系数 + 级联提示
// （未一并晋升的私有引用在团队侧落 dangling，raw_target 保留复活线索）。
defineProps<{
  open: boolean;
  targetId: string;
  docName: string;
  options: Array<{ label: string; value: string }>;
  loading: boolean;
  /** 非空 = 结果反馈阶段。 */
  result: PromoteResult | null;
}>();

defineEmits<{
  'update:open': [value: boolean];
  'update:targetId': [value: string];
  submit: [];
}>();

const { t } = useI18n();
</script>

<style lang="scss" scoped>
.knowledge-promote-dialog {
  &__cascade {
    max-height: 200px;
    overflow-y: auto;
    border: 1px solid var(--color-border-soft);
    border-radius: 8px;
  }
}
</style>
