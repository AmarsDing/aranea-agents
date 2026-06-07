<template>
  <q-drawer :model-value="modelValue" side="right" :width="420" bordered @update:model-value="emit('update:modelValue', $event)">
    <div class="org-drawer">
      <div class="org-drawer__header">
        <h3>{{ company?.name }}</h3>
        <q-btn flat round icon="close" @click="emit('update:modelValue', false)" />
      </div>

      <p class="org-drawer__desc">{{ company?.description }}</p>

      <q-separator />

      <div class="org-drawer__section">
        <h4>部门与岗位</h4>
        <div v-if="detailLoading" class="text-grey q-pa-md">加载中...</div>
        <div v-else-if="!detail?.departments?.length" class="text-grey q-pa-md">该公司暂无部门数据</div>
        <template v-else>
          <q-expansion-item
            v-for="dept in detail.departments"
            :key="dept.key"
            :label="dept.name"
            :caption="`${(detail.positionsByDept[dept.key] ?? []).length} 个岗位`"
          >
            <q-list dense>
              <q-item v-for="pos in detail.positionsByDept[dept.key] ?? []" :key="pos.key">
                <q-item-section>{{ pos.name }}</q-item-section>
              </q-item>
            </q-list>
          </q-expansion-item>
        </template>
      </div>

      <div class="org-drawer__actions">
        <q-btn flat label="查看 Prompt 模板" @click="emit('viewPrompts', company!)" />
        <q-btn unelevated color="primary" label="安装公司" @click="emit('install', company!)" />
      </div>
    </div>
  </q-drawer>
</template>

<script setup lang="ts">
import type { Company, Department, Position } from '../../features/organization/types';
import type { OrgDetail } from '../../features/organization/useOrgMarket';

const props = defineProps<{
  modelValue: boolean;
  company: Company | null;
  detail: OrgDetail;
  detailLoading: boolean;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  install: [company: Company];
  viewPrompts: [company: Company];
}>();
</script>

<style lang="sass" scoped>
.org-drawer
  padding: 16px

.org-drawer__header
  display: flex
  align-items: center
  justify-content: space-between

  h3
    margin: 0
    font-size: 18px
    font-weight: 600

.org-drawer__desc
  font-size: 13px
  color: var(--color-text-secondary, #6B5B4D)
  margin: 12px 0

.org-drawer__section
  h4
    margin: 16px 0 8px
    font-size: 14px
    font-weight: 600

.org-drawer__actions
  display: flex
  gap: 8px
  margin-top: 24px
</style>
