<template>
  <q-card flat bordered class="industry-card cursor-pointer" @click="$emit('select', industry)">
    <q-card-section horizontal>
      <q-card-section class="col-auto flex flex-center q-pa-md">
        <span class="text-h3">{{ industry.icon }}</span>
      </q-card-section>
      <q-card-section>
        <div class="text-h6">{{ industry.name }}</div>
        <div class="text-caption text-grey">{{ industry.description }}</div>
      </q-card-section>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { Industry } from '../../features/industries/types';

defineProps<{ industry: Industry }>();
defineEmits<{ select: [industry: Industry] }>();
</script>
