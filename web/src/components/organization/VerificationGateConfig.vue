<template>
  <div class="verification-gate-config">
    <div class="text-subtitle2 q-mb-sm">验证门配置</div>
    <p class="text-body2 text-grey-7 q-mb-md">配置团队跨部门协作时的验证门规则，确保交付物通过审核后才进入下一阶段。</p>

    <q-toggle v-model="enabled" label="启用验证门" />

    <template v-if="enabled">
      <q-input
        v-model="gateName"
        class="q-mt-md"
        outlined
        dense
        label="验证门名称"
      />
      <q-select
        v-model="gateType"
        class="q-mt-sm"
        :options="gateTypeOptions"
        outlined
        dense
        label="验证类型"
      />
      <q-input
        v-model="description"
        class="q-mt-sm"
        outlined
        dense
        type="textarea"
        rows="2"
        label="验证规则描述"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';

const enabled = ref(false);
const gateName = ref('');
const gateType = ref('auto');
const description = ref('');
const gateTypeOptions = [
  { label: '自动验证', value: 'auto' },
  { label: '人工审批', value: 'manual' },
  { label: '混合模式', value: 'hybrid' },
];
</script>
