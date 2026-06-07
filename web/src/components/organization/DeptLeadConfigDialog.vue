<template>
  <q-dialog v-model="modelValue" persistent>
    <q-card class="app-dialog-card" style="min-width: 480px">
      <q-card-section>
        <div class="text-h6">配置部门负责人</div>
      </q-card-section>

      <q-card-section class="q-pt-none">
        <p class="text-body2 text-grey-7">为部门指定负责人 Agent，负责审批借用请求和团队交付物审核。</p>
        <q-select
          v-model="selectedAgentId"
          :options="agentOptions"
          label="负责人 Agent"
          outlined
          dense
          clearable
          :loading="loadingAgents"
        />
        <q-input
          v-model="configJson"
          class="q-mt-md"
          type="textarea"
          label="配置 JSON（可选）"
          outlined
          dense
          rows="3"
        />
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat label="取消" @click="modelValue = false" />
        <q-btn unelevated color="primary" label="保存" :disable="!selectedAgentId" @click="onSave" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue';

const modelValue = defineModel<boolean>({ default: false });

const selectedAgentId = ref<string | null>(null);
const configJson = ref('{}');
const loadingAgents = ref(false);
const agentOptions = ref<{ label: string; value: string }[]>([]);

function onSave() {
  // TODO: 调用 API 保存 deptLeadAgentId 和 deptLeadConfigJson
  modelValue.value = false;
}
</script>
