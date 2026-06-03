<template>
  <div class="provider-wizard-panel">
    <h3 class="provider-step-heading">高可用</h3>
    <p class="provider-step-hint">配置 failover 链路与备用 Provider。</p>
    <ProviderHAConfig
      :model-value="providerHAForm"
      :provider-type-options="haCandidateProviderTypeOptions"
      @update:model-value="$emit('update:haForm', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import type { ProviderHAForm } from '../../features/platform/types';
import ProviderHAConfig from './ProviderHAConfig.vue';

type SelectOption = { label: string; value: string };

defineProps<{
  providerHAForm: ProviderHAForm;
  haCandidateProviderTypeOptions: SelectOption[];
}>();

defineEmits<{
  'update:haForm': [value: ProviderHAForm];
}>();
</script>
