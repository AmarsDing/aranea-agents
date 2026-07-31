<template>
  <div class="org-bundle-tree">
    <div class="row q-gutter-sm q-mb-md">
      <q-chip dense icon="corporate_fare" color="primary" text-color="white">
        {{ t('shopPage.orgTotals.departments', { count: bundle.totals.departments }) }}
      </q-chip>
      <q-chip dense icon="badge" color="teal" text-color="white">
        {{ t('shopPage.orgTotals.positions', { count: bundle.totals.positions }) }}
      </q-chip>
      <q-chip dense icon="smart_toy" color="deep-purple" text-color="white">
        {{ t('shopPage.orgTotals.agents', { count: bundle.totals.agents }) }}
      </q-chip>
    </div>

    <q-expansion-item
      v-for="dept in bundle.departments"
      :key="dept.key"
      :label="dept.name"
      icon="corporate_fare"
      default-opened
      header-class="org-bundle-tree__dept"
      class="org-bundle-tree__group"
    >
      <div class="org-bundle-tree__positions">
        <div v-for="pos in dept.positions" :key="pos.key" class="org-bundle-tree__position">
          <div class="row items-center q-gutter-xs q-mb-xs">
            <q-icon name="badge" size="15px" color="teal" />
            <span class="text-weight-medium">{{ pos.name }}</span>
          </div>
          <div class="org-bundle-tree__agents">
            <q-chip
              v-for="agent in pos.agents"
              :key="agent"
              dense
              size="sm"
              icon="smart_toy"
              class="org-bundle-tree__agent"
            >
              {{ agent }}
            </q-chip>
          </div>
        </div>
      </div>
    </q-expansion-item>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { OrgBundlePreview } from '../../features/ecosystem/types';

defineProps<{
  bundle: OrgBundlePreview;
}>();

const { t } = useI18n();
</script>

<style scoped>
.org-bundle-tree__group {
  border: 1px solid var(--glass-border);
  border-radius: 10px;
  margin-bottom: 8px;
  overflow: hidden;
}
.org-bundle-tree__dept {
  font-weight: 600;
}
.org-bundle-tree__positions {
  padding: 4px 16px 12px 44px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.org-bundle-tree__agents {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding-left: 22px;
}
.org-bundle-tree__agent {
  background: var(--interaction-surface-hover);
  color: var(--color-text-primary);
}
body.body--dark .org-bundle-tree__agent {
  background: rgba(255, 255, 255, 0.08);
}
</style>
