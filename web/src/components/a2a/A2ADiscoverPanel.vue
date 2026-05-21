<template>
  <div>
    <q-card flat class="app-registry-panel">
      <q-card-section class="app-form-field-grid app-registry-toolbar items-end">
        <q-input v-model="workspace" class="app-field-md" dense outlined clearable label="Workspace" />
        <q-input v-model="capability" class="app-field-md" dense outlined clearable label="Capability" />
        <div class="app-actions-bar app-actions-bar--start">
          <q-btn color="primary" unelevated no-caps rounded icon="search" label="发现" :loading="loading" @click="$emit('discover')" />
        </div>
      </q-card-section>
    </q-card>
    <div class="app-registry-table-shell">
      <q-table
        flat
        dense
        class="app-registry-table"
        :rows="agents"
        :columns="columns"
        row-key="agent_id"
        :loading="loading"
        :pagination="{ rowsPerPage: 10 }"
      >
        <template #body-cell-agent_id="props">
          <q-td :props="props">
            <div class="app-registry-cell-primary">{{ props.row.agent_id }}</div>
          </q-td>
        </template>
        <template #body-cell-enabled="props">
          <q-td :props="props">
            <q-chip dense :color="props.row.enabled ? 'positive' : 'grey'" text-color="white" size="sm">
              {{ props.row.enabled ? "启用" : "禁用" }}
            </q-chip>
          </q-td>
        </template>
        <template #body-cell-capabilities="props">
          <q-td :props="props">
            <div class="app-registry-chip-wrap">
              <q-chip v-for="c in props.row.capabilities" :key="c.name" dense outline size="sm">{{ c.name }}</q-chip>
            </div>
          </q-td>
        </template>
      </q-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { A2AAgentCard } from "../../features/a2a/types";

defineProps<{
  agents: A2AAgentCard[];
  loading: boolean;
  columns: { name: string; label: string; field: string; align: "left" | "right" | "center"; style?: string }[];
}>();

const workspace = defineModel<string>("workspace", { default: "" });
const capability = defineModel<string>("capability", { default: "" });

defineEmits<{ discover: [] }>();
</script>
