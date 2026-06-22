<template>
  <div class="provider-wizard-panel">
    <h3 class="provider-step-heading">高级选项</h3>
    <p class="provider-step-hint">Token tailoring、缓存优化与速率限制。</p>
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-toggle v-model="providerForm.enable_token_tailoring" label="Token Tailoring" />
      <q-toggle
        v-if="providerForm.provider_type === 'openai'"
        v-model="providerForm.optimize_for_cache"
        label="Prompt Cache 优化"
      />
      <q-toggle
        v-if="providerForm.provider_type === 'openai' && providerForm.variant === 'deepseek'"
        v-model="providerForm.reasoning_backfill"
        label="Reasoning 回填"
      />
      <q-toggle
        v-if="['openai', 'anthropic'].includes(providerForm.provider_type)"
        v-model="providerForm.show_tool_call_delta"
        label="Tool Call Delta"
      />
      <q-input
        v-model.number="providerForm.rate_limit_rpm"
        dense
        outlined
        type="number"
        min="0"
        label="速率限制 (RPM)"
      />
      <q-input
        v-if="providerForm.provider_type === 'ollama'"
        v-model.number="providerForm.keep_alive_minutes"
        dense
        outlined
        type="number"
        min="0"
        label="Keep Alive (分钟)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ProviderForm } from '../../features/platform/types';

const providerForm = defineModel<ProviderForm>('providerForm', { required: true });
</script>
