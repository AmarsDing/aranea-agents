<template>
  <div class="q-gutter-md">
    <q-select
      :model-value="modelValue.haMode"
      class="app-field-md"
      :options="haModeOptions"
      label="高可用模式"
      emit-value
      map-options
      outlined
      dense
      @update:model-value="updateField('haMode', String($event ?? ''))"
    />
    <template v-if="modelValue.haMode">
      <q-card
        v-for="(candidate, idx) in modelValue.haCandidates"
        :key="idx"
        flat
        bordered
        class="capability-card q-pa-sm"
      >
        <div class="app-form-field-grid app-form-field-grid--wide items-end">
          <q-input
            :model-value="candidate.name"
            label="模型名"
            dense
            outlined
            @update:model-value="updateCandidate(idx, 'name', String($event ?? ''))"
          />
          <q-select
            :model-value="candidate.providerType"
            :options="providerTypeOptions"
            label="Provider"
            emit-value
            map-options
            dense
            outlined
            @update:model-value="updateCandidate(idx, 'providerType', String($event ?? ''))"
          />
          <q-input
            :model-value="candidate.baseUrl"
            label="Base URL"
            dense
            outlined
            @update:model-value="updateCandidate(idx, 'baseUrl', String($event ?? ''))"
          />
          <q-input
            :model-value="candidate.apiKey"
            label="API Key"
            type="password"
            :placeholder="candidate.apiKey ? undefined : '留空不修改'"
            dense
            outlined
            @update:model-value="updateCandidate(idx, 'apiKey', String($event ?? ''))"
          />
          <div class="app-actions-bar app-actions-bar--start">
            <q-btn flat round dense icon="close" color="negative" @click="removeCandidate(idx)" />
          </div>
        </div>
      </q-card>
      <div class="app-actions-bar app-actions-bar--start">
        <q-btn flat no-caps label="+ 添加候选模型" color="primary" icon="add" @click="addCandidate" />
      </div>
      <q-input
        v-if="modelValue.haMode === 'hedge'"
        :model-value="modelValue.haHedgeDelayMs"
        class="app-field-sm"
        type="number"
        min="0"
        label="Hedge 延迟"
        suffix="ms"
        dense
        outlined
        @update:model-value="updateField('haHedgeDelayMs', Number($event ?? 0))"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
export type HACandidateForm = {
  name: string;
  providerType: string;
  baseUrl: string;
  apiKey: string;
};

export type ProviderHAForm = {
  haMode: "" | "failover" | "hedge";
  haCandidates: HACandidateForm[];
  haHedgeDelayMs: number;
};

const props = defineProps<{
  modelValue: ProviderHAForm;
  providerTypeOptions: { label: string; value: string }[];
}>();

const emit = defineEmits<{
  "update:modelValue": [value: ProviderHAForm];
}>();

const haModeOptions = [
  { label: "无", value: "" },
  { label: "Failover", value: "failover" },
  { label: "Hedge", value: "hedge" }
];

function updateField<K extends keyof ProviderHAForm>(key: K, value: ProviderHAForm[K]) {
  emit("update:modelValue", { ...props.modelValue, [key]: value });
}

function updateCandidate(idx: number, key: keyof HACandidateForm, value: string) {
  const next = props.modelValue.haCandidates.map((item, i) => (i === idx ? { ...item, [key]: value } : item));
  updateField("haCandidates", next);
}

function addCandidate() {
  updateField("haCandidates", [
    ...props.modelValue.haCandidates,
    { name: "", providerType: "openai", baseUrl: "", apiKey: "" }
  ]);
}

function removeCandidate(idx: number) {
  updateField(
    "haCandidates",
    props.modelValue.haCandidates.filter((_, i) => i !== idx)
  );
}
</script>
