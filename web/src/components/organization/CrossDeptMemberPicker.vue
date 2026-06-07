<template>
  <div class="cross-dept-member-picker">
    <div class="text-subtitle2 q-mb-sm">跨部门成员借用</div>
    <p class="text-body2 text-grey-7 q-mb-md">从其他部门选择成员临时加入当前团队。</p>

    <q-select
      v-model="selectedDepartment"
      :options="departmentOptions"
      label="选择来源部门"
      outlined
      dense
      clearable
      class="q-mb-sm"
    />

    <q-select
      v-model="selectedMembers"
      :options="memberOptions"
      label="选择成员"
      outlined
      dense
      multiple
      :disable="!selectedDepartment"
      use-chips
    />

    <q-btn
      unelevated
      color="primary"
      label="发起借用请求"
      class="q-mt-md"
      :disable="!selectedDepartment || selectedMembers.length === 0"
      @click="onBorrow"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';

const selectedDepartment = ref<string | null>(null);
const selectedMembers = ref<string[]>([]);
const departmentOptions = ref<{ label: string; value: string }[]>([]);
const memberOptions = ref<{ label: string; value: string }[]>([]);

function onBorrow() {
  // TODO: 调用 API 发起借用请求
}
</script>
