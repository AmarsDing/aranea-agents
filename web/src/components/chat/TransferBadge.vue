<template>
  <q-chip v-if="transfer" dense color="accent" text-color="white" size="sm" icon="swap_horiz">
    {{ transfer.from_agent }} → {{ transfer.to_agent }}
  </q-chip>
</template>

<script setup lang="ts">
defineProps<{
  transfer?: { from_agent: string; to_agent: string } | null;
}>();
</script>
