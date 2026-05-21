<template>
  <div class="q-gutter-md">
    <q-select
      :model-value="modelValue.haMode"
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
        class="q-pa-sm"
      >
        <div class="row q-col-gutter-sm items-center">
          <q-input
            :model-value="candidate.name"
            class="col-12 col-md-3"
            label="模型名"
            dense
            outlined
            @update:model-value="updateCandidate(idx, 'name', String($event ?? ''))"
          />
          <q-select
            :model-value="candidate.providerType"
            class="col-12 col-md-3"
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
            class="col-12 col-md-3"
            label="Base URL"
            dense
            outlined
            @update:model-value="updateCandidate(idx, 'baseUrl', String($event ?? ''))"
          />
          <q-input
            :model-value="candidate.apiKey"
            class="col-12 col-md-2"
            label="API Key"
            type="password"
            :placeholder="candidate.apiKey ? undefined : '留空不修改'"
            dense
            outlined
            @update:model-value="updateCandidate(idx, 'apiKey', String($event ?? ''))"
          />
          <q-btn flat round dense icon="close" color="negative" @click="removeCandidate(idx)" />
        </div>
      </q-card>
      <q-btn flat label="+ 添加候选模型" color="primary" icon="add" @click="addCandidate" />
      <q-input
        v-if="modelValue.haMode === 'hedge'"
        :model-value="modelValue.haHedgeDelayMs"
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
