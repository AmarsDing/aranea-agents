<template>
  <q-badge :color="badgeColor" :label="badgeLabel" :outline="status === 'none'" />
</template>

<script setup lang="ts">
import { computed } from 'vue';

type BorrowStatus = 'none' | 'pending' | 'approved' | 'rejected';

const props = defineProps<{ status: BorrowStatus }>();

const badgeColor = computed(() => {
  switch (props.status) {
    case 'pending': return 'warning';
    case 'approved': return 'positive';
    case 'rejected': return 'negative';
    default: return 'grey';
  }
});

const badgeLabel = computed(() => {
  switch (props.status) {
    case 'pending': return '待审批';
    case 'approved': return '已批准';
    case 'rejected': return '已拒绝';
    default: return '未借用';
  }
});
</script>
