<template>
  <div class="borrow-approval-notification">
    <q-list bordered separator>
      <q-item v-for="req in requests" :key="req.id">
        <q-item-section avatar>
          <q-icon name="swap_horiz" color="warning" />
        </q-item-section>
        <q-item-section>
          <q-item-label>{{ req.memberName }}</q-item-label>
          <q-item-label caption>来自 {{ req.sourceDept }} → 借用至 {{ req.targetTeam }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <div class="row q-gutter-xs">
            <q-btn flat round icon="check" color="positive" size="sm" @click="onApprove(req.id)" />
            <q-btn flat round icon="close" color="negative" size="sm" @click="onReject(req.id)" />
          </div>
        </q-item-section>
      </q-item>
      <q-item v-if="requests.length === 0">
        <q-item-section class="text-grey">暂无待审批的借用请求</q-item-section>
      </q-item>
    </q-list>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';

interface BorrowRequest {
  id: string;
  memberName: string;
  sourceDept: string;
  targetTeam: string;
}

const requests = ref<BorrowRequest[]>([]);

function onApprove(_id: string) {
  // TODO: 调用 API 批准借用
}

function onReject(_id: string) {
  // TODO: 调用 API 拒绝借用
}
</script>
