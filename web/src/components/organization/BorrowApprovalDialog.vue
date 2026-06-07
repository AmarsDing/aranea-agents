<template>
  <q-dialog v-model="modelValue" persistent>
    <q-card class="app-dialog-card" style="min-width: 420px">
      <q-card-section>
        <div class="text-h6">借用审批</div>
      </q-card-section>

      <q-card-section class="q-pt-none">
        <div class="q-mb-md">
          <div class="text-body2"><strong>成员：</strong>{{ request?.memberName }}</div>
          <div class="text-body2"><strong>来源部门：</strong>{{ request?.sourceDept }}</div>
          <div class="text-body2"><strong>目标团队：</strong>{{ request?.targetTeam }}</div>
          <div class="text-body2"><strong>借用原因：</strong>{{ request?.reason || '未填写' }}</div>
        </div>

        <q-input
          v-model="comment"
          type="textarea"
          outlined
          dense
          rows="2"
          label="审批意见（可选）"
        />
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat label="拒绝" color="negative" @click="onReject" />
        <q-btn unelevated color="primary" label="批准" @click="onApprove" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue';

interface BorrowRequest {
  id: string;
  memberName: string;
  sourceDept: string;
  targetTeam: string;
  reason?: string;
}

const modelValue = defineModel<boolean>({ default: false });
const props = defineProps<{ request: BorrowRequest | null }>();
const emit = defineEmits<{
  approve: [requestId: string];
  reject: [requestId: string];
}>();

const comment = ref('');

function onApprove() {
  if (props.request) {
    emit('approve', props.request.id);
  }
  modelValue.value = false;
}

function onReject() {
  if (props.request) {
    emit('reject', props.request.id);
  }
  modelValue.value = false;
}
</script>
