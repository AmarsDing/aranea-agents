<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="text-h6">启动评估</q-card-section>
      <q-card-section class="app-dialog-body q-gutter-md q-pt-none">
        <q-select
          :model-value="agentId"
          class="app-field-md"
          dense
          outlined
          emit-value
          map-options
          label="Agent"
          :options="agentOptions"
          @update:model-value="$emit('update:agentId', String($event ?? ''))"
        />
        <q-input
          :model-value="metrics"
          class="app-field-long"
          dense
          outlined
          label="指标（逗号分隔，留空=全部）"
          @update:model-value="$emit('update:metrics', String($event ?? ''))"
        />
        <q-input
          :model-value="numRuns"
          class="app-field-sm"
          dense
          outlined
          type="number"
          min="1"
          max="20"
          label="MultiRun 次数"
          hint="AgentEvaluator 重复运行次数（1–20，与后端 MaxNumRuns 一致）"
          @update:model-value="$emit('update:numRuns', Math.min(Math.max(Number($event) || 1, 1), 20))"
        />
        <q-input
          :model-value="model"
          class="app-field-md"
          dense
          outlined
          :label="$t('evaluationPage.experimentModel')"
          :hint="$t('evaluationPage.experimentModelHint')"
          @update:model-value="$emit('update:model', String($event ?? ''))"
        />
        <q-select
          :model-value="extraModels"
          class="app-field-md"
          dense
          outlined
          multiple
          use-input
          use-chips
          hide-dropdown-icon
          new-value-mode="add-unique"
          :options="[]"
          :label="$t('evaluationPage.experimentModels')"
          :hint="$t('evaluationPage.experimentModelsHint')"
          @update:model-value="$emit('update:extraModels', (($event as string[]) ?? []).map(String))"
        />
        <q-input
          :model-value="prompt"
          class="app-field-long"
          dense
          outlined
          type="textarea"
          autogrow
          :label="$t('evaluationPage.experimentPrompt')"
          :hint="$t('evaluationPage.experimentPromptHint')"
          @update:model-value="$emit('update:prompt', String($event ?? ''))"
        />
        <q-select
          :model-value="extraAgentIds"
          class="app-field-md"
          dense
          outlined
          multiple
          emit-value
          map-options
          use-chips
          :label="$t('evaluationPage.experimentAgents')"
          :hint="$t('evaluationPage.experimentAgentsHint')"
          :options="agentOptions"
          @update:model-value="$emit('update:extraAgentIds', (($event as string[]) ?? []).map(String))"
        />
        <div v-if="versionLabel" class="text-caption text-grey-7">
          {{ $t('evaluationPage.versionPinned', { version: versionLabel }) }}
        </div>
        <q-toggle
          :model-value="userSimulation"
          :label="$t('evaluationPage.userSimulationLabel')"
          @update:model-value="$emit('update:userSimulation', Boolean($event))"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps label="取消" @click="$emit('update:open', false)" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          label="运行"
          :loading="loading"
          :disable="!agentId"
          @click="$emit('submit')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
defineProps<{
  open: boolean;
  agentId: string;
  metrics: string;
  numRuns: number;
  userSimulation: boolean;
  extraAgentIds: string[];
  model: string;
  extraModels: string[];
  prompt: string;
  versionLabel: string;
  loading: boolean;
  agentOptions: { label: string; value: string }[];
}>();
defineEmits<{
  'update:open': [value: boolean];
  'update:agentId': [value: string];
  'update:metrics': [value: string];
  'update:numRuns': [value: number];
  'update:userSimulation': [value: boolean];
  'update:extraAgentIds': [value: string[]];
  'update:model': [value: string];
  'update:extraModels': [value: string[]];
  'update:prompt': [value: string];
  submit: [];
}>();
</script>
