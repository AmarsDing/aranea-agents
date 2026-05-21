<template>
  <q-chip v-if="transfer" dense color="purple" text-color="white" size="sm" icon="swap_horiz">
    {{ transfer.from_agent }} → {{ transfer.to_agent }}
  </q-chip>
</template>

<script setup lang="ts">
import type { EnvelopeTransfer } from "../../features/chat/envelope";

defineProps<{
  transfer?: EnvelopeTransfer | null;
}>();
</script>
