<template>
  <div class="app-form-wide a2a-invoke-form">
    <!-- Header -->
    <div class="a2a-form-header">
      <q-icon name="send" size="sm" color="primary" />
      <span class="a2a-form-header__title">Invoke 测试</span>
    </div>

    <div class="a2a-form-body">
      <!-- Group: 目标 -->
      <div class="a2a-form-group">
        <div class="a2a-form-group__title">目标</div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">Callee Agent *</div>
          <div class="a2a-form-row__control">
            <q-select
              v-if="agentOptions.length"
              v-model="calleeAgentId"
              dense
              outlined
              emit-value
              map-options
              use-input
              input-debounce="0"
              clearable
              :options="filteredAgentOptions"
              class="app-glass-control"
              @filter="onFilterAgent"
              @new-value="onNewAgentValue"
            >
              <template #no-option>
                <q-item dense>
                  <q-item-section class="text-grey-6">输入 Agent ID 后回车确认</q-item-section>
                </q-item>
              </template>
            </q-select>
            <q-input v-else v-model="calleeAgentId" dense outlined class="app-glass-control" />
          </div>
        </div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">Capability *</div>
          <div class="a2a-form-row__control">
            <q-select
              v-if="capabilityOptions.length"
              v-model="capability"
              dense
              outlined
              emit-value
              map-options
              use-input
              input-debounce="0"
              clearable
              :options="filteredCapabilityOptions"
              class="app-glass-control"
              @filter="onFilterCapability"
              @new-value="onNewCapabilityValue"
            >
              <template #no-option>
                <q-item dense>
                  <q-item-section class="text-grey-6">输入 Capability 后回车确认</q-item-section>
                </q-item>
              </template>
            </q-select>
            <q-input v-else v-model="capability" dense outlined class="app-glass-control" />
          </div>
        </div>
      </div>

      <!-- Group: 参数 -->
      <div class="a2a-form-group">
        <div class="a2a-form-group__title">参数</div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">Workspace</div>
          <div class="a2a-form-row__control">
            <q-input v-model="workspace" dense outlined class="app-glass-control" />
          </div>
          <div class="a2a-form-row__hint">跨工作区 Invoke 时须与 X-Workspace-ID 及被调 Agent Card 一致</div>
        </div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">Timeout</div>
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
          <div class="a2a-form-row__label">Payload</div>
          <div class="a2a-form-row__control">
            <q-input v-model="payloadJson" dense outlined type="textarea" rows="5" class="app-glass-control" />
          </div>
          <div class="a2a-form-row__hint">例如 {"message":"你好"}</div>
        </div>
      </div>

      <!-- Actions -->
      <div class="a2a-form-actions">
        <q-btn
          color="primary"
          unelevated
          icon="send"
          label="Invoke"
          no-caps
          :loading="loading"
          @click="$emit('invoke')"
        />
      </div>

      <!-- Result -->
      <q-card v-if="result" flat bordered class="a2a-invoke-result app-glass-side-panel">
        <div class="a2a-invoke-result__meta">
          <span class="a2a-invoke-result__id">invoke_id: {{ result.invoke_id }}</span>
          <span class="a2a-invoke-result__status">status: {{ result.status }} · {{ result.duration_ms }}ms</span>
        </div>
        <JsonCodeViewer :text="result.result_json || result.error_message" :show-toolbar="false" scroll-height="240px" />
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import JsonCodeViewer from '../common/JsonCodeViewer.vue';
import type { A2AAgentCard, A2AInvokeResult } from '../../features/a2a/types';

const props = defineProps<{
  loading: boolean;
  result: A2AInvokeResult | null;
  discoveredAgents?: A2AAgentCard[];
}>();

const calleeAgentId = defineModel<string>('calleeAgentId', { default: '' });
const capability = defineModel<string>('capability', { default: '' });
const payloadJson = defineModel<string>('payloadJson', { default: '{}' });
const timeoutSeconds = defineModel<number>('timeoutSeconds', { default: 30 });
const workspace = defineModel<string>('workspace', { default: '' });

defineEmits<{ invoke: [] }>();

watch(calleeAgentId, (v) => {
  if (v == null) calleeAgentId.value = '';
});
watch(capability, (v) => {
  if (v == null) capability.value = '';
});

const agentOptions = computed(() =>
  (props.discoveredAgents ?? [])
    .filter((a) => a.enabled)
    .map((a) => ({
      label: `${a.display_name || a.agent_id} (${a.agent_id})`,
      value: a.agent_id,
    })),
);

const capabilityOptions = computed(() => {
  const agent = (props.discoveredAgents ?? []).find((a) => a.agent_id === calleeAgentId.value);
  if (!agent) return [];
  return agent.capabilities.map((c) => ({ label: c.name, value: c.name }));
});

// Filtered options for q-select with use-input
const filteredAgentOptions = ref(agentOptions.value);
const filteredCapabilityOptions = ref(capabilityOptions.value);

function onFilterAgent(val: string, update: (fn: () => void) => void) {
  update(() => {
    const needle = val.toLowerCase();
    filteredAgentOptions.value = agentOptions.value.filter(
      (o) => o.label.toLowerCase().includes(needle) || o.value.toLowerCase().includes(needle),
    );
  });
}

function onNewAgentValue(val: string, done: (item?: string, mode?: 'add' | 'toggle' | 'add-unique') => void) {
  done(val, 'add-unique');
}

function onFilterCapability(val: string, update: (fn: () => void) => void) {
  update(() => {
    const needle = val.toLowerCase();
    filteredCapabilityOptions.value = capabilityOptions.value.filter(
      (o) => o.label.toLowerCase().includes(needle) || o.value.toLowerCase().includes(needle),
    );
  });
}

function onNewCapabilityValue(val: string, done: (item?: string, mode?: 'add' | 'toggle' | 'add-unique') => void) {
  done(val, 'add-unique');
}

// Sync filtered options when source options change
watch(agentOptions, (opts) => {
  filteredAgentOptions.value = opts;
});
watch(capabilityOptions, (opts) => {
  filteredCapabilityOptions.value = opts;
});
</script>
