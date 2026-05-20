<template>
  <div>
    <q-card flat bordered class="q-mb-md">
      <q-card-section class="row q-col-gutter-md">
        <q-input v-model="workspace" class="col-12 col-md-5" dense outlined clearable label="Workspace" />
        <q-input v-model="capability" class="col-12 col-md-5" dense outlined clearable label="Capability" />
        <div class="col-12 col-md-2 flex items-center">
          <q-btn color="primary" unelevated icon="search" label="发现" :loading="loading" @click="$emit('discover')" />
        </div>
      </q-card-section>
    </q-card>
    <q-table flat :rows="agents" :columns="columns" row-key="agent_id" :loading="loading" :pagination="{ rowsPerPage: 10 }">
      <template #body-cell-enabled="props">
        <q-td :props="props">
          <q-chip dense :color="props.row.enabled ? 'positive' : 'grey'" text-color="white" size="sm">
            {{ props.row.enabled ? "启用" : "禁用" }}
          </q-chip>
        </q-td>
      </template>
      <template #body-cell-capabilities="props">
        <q-td :props="props">
          <q-chip v-for="c in props.row.capabilities" :key="c.name" dense outline size="sm">{{ c.name }}</q-chip>
        </q-td>
      </template>
    </q-table>
  </div>
</template>

<script setup lang="ts">
import type { A2AAgentCard } from "../../features/a2a/types";

defineProps<{
  agents: A2AAgentCard[];
  loading: boolean;
  columns: { name: string; label: string; field: string; align: "left" | "right" | "center" }[];
}>();

const workspace = defineModel<string>("workspace", { default: "" });
const capability = defineModel<string>("capability", { default: "" });

defineEmits<{ discover: [] }>();
</script>
