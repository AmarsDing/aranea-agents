<template>
  <div class="app-form-wide a2a-invoke-form">
    <div class="a2a-form-header">
      <q-icon name="send" size="sm" color="primary" />
      <span class="a2a-form-header__title">{{ t('a2a.federation.invokeTitle') }}</span>
    </div>

    <div class="a2a-form-body">
      <div class="a2a-form-group">
        <div class="a2a-form-group__title">
          {{ t('a2a.federation.invokeOrg') }} / {{ t('a2a.federation.invokeAgent') }}
        </div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">{{ t('a2a.federation.invokeOrg') }} *</div>
          <div class="a2a-form-row__control">
            <q-select
              v-model="orgId"
              dense
              outlined
              emit-value
              map-options
              :options="orgOptions"
              class="app-glass-control"
            />
          </div>
        </div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">{{ t('a2a.federation.invokeAgent') }} *</div>
          <div class="a2a-form-row__control">
            <q-select
              v-model="agentId"
              dense
              outlined
              emit-value
              map-options
              :options="agentOptions"
              class="app-glass-control"
            />
          </div>
        </div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">{{ t('a2a.federation.invokeCapability') }} *</div>
          <div class="a2a-form-row__control">
            <q-select
              v-model="capability"
              dense
              outlined
              emit-value
              map-options
              :options="capabilityOptions"
              class="app-glass-control"
            />
          </div>
        </div>
      </div>

      <div class="a2a-form-group">
        <div class="a2a-form-group__title">{{ t('a2a.federation.invokePayload') }}</div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">{{ t('a2a.federation.invokeTimeout') }}</div>
          <div class="a2a-form-row__control">
            <q-input
              v-model.number="timeoutSeconds"
              dense
              outlined
              type="number"
              class="app-glass-control app-field-sm"
            />
          </div>
        </div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">{{ t('a2a.federation.invokePayload') }}</div>
          <div class="a2a-form-row__control">
            <q-input v-model="payloadJson" dense outlined type="textarea" rows="5" class="app-glass-control" />
          </div>
          <div class="a2a-form-row__hint">{{ t('a2a.federation.invokePayloadHint') }}</div>
        </div>
      </div>

      <div class="a2a-form-actions">
        <q-btn
          color="primary"
          unelevated
          icon="send"
          no-caps
          :label="t('a2a.federation.invokeSubmit')"
          :loading="loading"
          @click="emit('invoke')"
        />
      </div>

      <q-card v-if="result" flat bordered class="a2a-invoke-result app-glass-side-panel">
        <div class="a2a-invoke-result__meta">
          <span class="a2a-invoke-result__id">{{ t('a2a.federation.invokeAuditId') }}: {{ result.audit_id }}</span>
          <span class="a2a-invoke-result__status">status: {{ result.status }} · {{ result.latency_ms }}ms</span>
        </div>
        <pre class="a2a-result">{{ result.result_json || result.error_message }}</pre>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { FederatedInvokeResult, FederationAgentEntry } from '../../../features/a2a/federationTypes';

const props = defineProps<{
  entries: FederationAgentEntry[];
  loading: boolean;
  result: FederatedInvokeResult | null;
}>();

const emit = defineEmits<{ invoke: [] }>();

const orgId = defineModel<string>('orgId', { default: '' });
const agentId = defineModel<string>('agentId', { default: '' });
const capability = defineModel<string>('capability', { default: '' });
const payloadJson = defineModel<string>('payloadJson', { default: '{}' });
const timeoutSeconds = defineModel<number>('timeoutSeconds', { default: 30 });

const { t } = useI18n();

const orgOptions = computed(() => {
  const seen = new Map<string, string>();
  for (const e of props.entries) {
    if (!seen.has(e.org.id)) seen.set(e.org.id, e.org.name || e.org.domain);
  }
  return [...seen.entries()].map(([value, label]) => ({ label, value }));
});

const agentOptions = computed(() =>
  props.entries
    .filter((e) => e.org.id === orgId.value)
    .map((e) => ({
      label: `${e.card.display_name || e.card.agent_id} (${e.remote_agent.id})`,
      value: e.remote_agent.id,
    })),
);

const capabilityOptions = computed(() => {
  const entry = props.entries.find((e) => e.org.id === orgId.value && e.remote_agent.id === agentId.value);
  return (entry?.card.capabilities ?? []).map((c) => ({ label: c.name, value: c.name }));
});

watch(orgId, () => {
  agentId.value = '';
  capability.value = '';
});
watch(agentId, () => {
  capability.value = '';
});
</script>
