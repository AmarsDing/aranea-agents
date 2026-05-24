<template>
  <section class="team-orchestrate-runtime">
    <div class="text-subtitle2 q-mb-sm">运行时与容错</div>
    <div class="text-caption app-text-secondary q-mb-md">
      OrchestrationSpec v2：Graph 为默认执行引擎；失败策略在保存时写入 definition_json。
    </div>

    <q-banner v-if="nativeLocked" dense rounded class="bg-orange-1 text-dark q-mb-sm">
      Native 执行引擎仅平台管理员可选。
    </q-banner>

    <q-list dense bordered separator class="rounded-borders q-mb-md">
      <q-item>
        <q-item-section>
          <q-item-label caption>执行引擎</q-item-label>
          <q-item-label>{{ runtimeLabel }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <q-badge :color="runtimeBadgeColor" rounded>{{ runtimeEngineValue }}</q-badge>
        </q-item-section>
      </q-item>
      <q-item>
        <q-item-section>
          <q-item-label caption>失败策略</q-item-label>
          <q-item-label class="text-body2">{{ failureSummary }}</q-item-label>
        </q-item-section>
      </q-item>
      <q-item v-if="timeoutSec">
        <q-item-section>
          <q-item-label caption>运行超时</q-item-label>
          <q-item-label>{{ timeoutSec }}s</q-item-label>
        </q-item-section>
      </q-item>
    </q-list>

    <div v-if="!readOnly" class="app-form-field-grid app-form-field-grid--wide q-gutter-sm">
      <q-select
        v-model="localRuntime"
        dense
        outlined
        emit-value
        map-options
        label="runtime_engine"
        :options="filteredRuntimeOptions"
        @update:model-value="emitRuntime"
      />
      <q-select
        v-model="localFailureDefault"
        dense
        outlined
        emit-value
        map-options
        clearable
        label="failure_policy.default"
        :options="failureDefaultOptions"
        @update:model-value="emitFailure"
      />
      <q-input
        v-model.number="localRetryMax"
        dense
        outlined
        type="number"
        min="0"
        max="10"
        label="retry.max_attempts"
        @update:model-value="emitFailure"
      />
    </div>

    <q-banner v-if="readOnly" dense rounded class="bg-orange-1 text-dark q-mt-sm">
      运行中只读；停止 Run 后可编辑 runtime / failure_policy。
    </q-banner>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { TeamDefinition } from "../../features/teams/types";
import {
  failureDefaultOptions,
  failurePolicySummary,
  runtimeEngineLabel,
  runtimeEngineOptions,
} from "./teamUtils";

const props = withDefaults(
  defineProps<{
    definition: TeamDefinition | null;
    readOnly: boolean;
    isPlatformAdmin?: boolean;
  }>(),
  { isPlatformAdmin: false },
);

const emit = defineEmits<{
  patch: [patch: Partial<TeamDefinition>];
}>();

const runtimeEngineValue = computed(() =>
  String(props.definition?.runtime_engine || "graph").toLowerCase() === "native" ? "native" : "graph",
);
const runtimeLabel = computed(() => runtimeEngineLabel(props.definition?.runtime_engine));
const runtimeBadgeColor = computed(() => (runtimeEngineValue.value === "graph" ? "primary" : "grey-7"));
const failureSummary = computed(() => (props.definition ? failurePolicySummary(props.definition) : "—"));
const timeoutSec = computed(() => props.definition?.timeout_seconds ?? 0);

const filteredRuntimeOptions = computed(() =>
  props.isPlatformAdmin ? runtimeEngineOptions : runtimeEngineOptions.filter((o) => o.value !== "native"),
);

const nativeLocked = computed(
  () => !props.isPlatformAdmin && runtimeEngineValue.value === "native",
);

const localRuntime = ref(runtimeEngineValue.value);
const localFailureDefault = ref(props.definition?.failure_policy?.default ?? "retry_then_block");
const localRetryMax = ref(props.definition?.failure_policy?.retry?.max_attempts ?? 3);

watch(
  () => props.definition,
  (def) => {
    if (!def) return;
    localRuntime.value = runtimeEngineValue.value;
    localFailureDefault.value = def.failure_policy?.default ?? "retry_then_block";
    localRetryMax.value = def.failure_policy?.retry?.max_attempts ?? 3;
  },
  { deep: true },
);

function emitRuntime(value: "native" | "graph") {
  if (value === "native" && !isPlatformAdmin.value) {
    emit("patch", { runtime_engine: "graph", team_graph_runtime: true });
    localRuntime.value = "graph";
    return;
  }
  emit("patch", {
    runtime_engine: value,
    team_graph_runtime: value === "graph",
  });
}

function emitFailure() {
  const prev = props.definition?.failure_policy ?? {};
  emit("patch", {
    failure_policy: {
      ...prev,
      default: localFailureDefault.value || "retry_then_block",
      retry: { ...(prev.retry ?? {}), max_attempts: Math.max(0, Math.floor(Number(localRetryMax.value) || 0)) },
    },
  });
}
</script>

<style scoped>
.team-orchestrate-runtime {
  margin-bottom: 16px;
}
</style>
