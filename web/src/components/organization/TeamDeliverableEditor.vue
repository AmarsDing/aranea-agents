<template>
  <div class="team-deliverable-editor">
    <div class="text-subtitle2 q-mb-sm">团队交付物</div>
    <p class="text-body2 text-grey-7 q-mb-md">定义团队需要完成的交付物清单，用于验证门检查。</p>

    <q-list bordered separator>
      <q-item v-for="(item, idx) in deliverables" :key="idx">
        <q-item-section>
          <q-input v-model="deliverables[idx]" dense outlined placeholder="交付物描述" />
        </q-item-section>
        <q-item-section side>
          <q-btn flat round icon="delete" size="sm" color="negative" @click="deliverables.splice(idx, 1)" />
        </q-item-section>
      </q-item>
      <q-item v-if="deliverables.length === 0">
        <q-item-section class="text-grey">暂无交付物</q-item-section>
      </q-item>
    </q-list>

    <q-btn flat icon="add" label="添加交付物" class="q-mt-sm" @click="deliverables.push('')" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';

const deliverables = ref<string[]>([]);
</script>
