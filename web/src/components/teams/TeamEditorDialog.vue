<!--
  Team 域展示组件：仅 props / emits（vue-design.md §0.2）。
  路径约定：vue-design.md §2 → `web/src/components/teams/`。
-->
<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card :class="['team-editor', { 'is-dark': isDark }]">
      <q-card-section class="row items-center justify-between">
        <div>
          <div class="text-h6">{{ editingId ? "编辑 Team" : "新增 Team" }}</div>
          <div class="text-caption text-grey-7">配置成员后，后端会根据 mode 动态选择 sequential / parallel / coordinator / critic_loop 拓扑。</div>
        </div>
        <q-btn flat round icon="close" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section class="q-gutter-md">
        <q-card flat bordered class="template-panel">
          <q-card-section class="row q-col-gutter-md items-center">
            <div class="col-12 col-md-5">
              <div class="text-subtitle2">Team 模板</div>
              <div class="text-caption text-grey-7">选择模板可快速生成成员角色和编排参数。</div>
            </div>
            <q-select class="col-12 col-md-5 team-control" dense outlined emit-value map-options label="选择模板" :options="teamTemplateOptions" @update:model-value="$emit('applyTemplate', $event)" />
            <div class="col-12 col-md-2 row justify-end">
              <q-icon name="auto_awesome" color="primary" size="28px" />
            </div>
          </q-card-section>
        </q-card>

        <div class="row q-col-gutter-md">
          <q-input v-model.trim="form.display_name" class="col-12 col-md-6 team-control" dense outlined label="Team 名称 *" />
          <q-input v-model.trim="form.team_key" class="col-12 col-md-6 team-control" dense outlined label="Team Key *" hint="小写字母、数字、连字符" />
          <q-select v-model="definition.mode" class="col-12 col-md-4 team-control" dense outlined emit-value map-options label="编排模式" :options="modeOptions" />
          <q-select v-model="form.status" class="col-12 col-md-4 team-control" dense outlined emit-value map-options label="状态" :options="statusOptions" />
          <q-input v-model.number="definition.max_concurrency" class="col-12 col-md-4 team-control" dense outlined type="number" min="1" label="并发上限" />
          <q-input v-model="definition.description" class="col-12 team-control" dense outlined autogrow type="textarea" label="Team 说明" />
        </div>

        <q-expansion-item icon="sync_alt" label="A2A 协议">
          <div class="row q-col-gutter-md q-mt-sm">
            <q-toggle v-model="a2aEnabled" class="col-12 col-md-3" color="primary" label="启用 A2A 信封" />
            <q-input v-model="a2aEnvelopeVersion" class="col-12 col-md-3 team-control" dense outlined label="Envelope Version" />
            <q-select v-model="a2aMessageFormat" class="col-12 col-md-3 team-control" dense outlined emit-value map-options label="消息格式" :options="a2aFormatOptions" />
            <q-input v-model.number="a2aMaxPayloadChars" class="col-12 col-md-3 team-control" dense outlined type="number" min="500" label="最大载荷字符" />
            <q-toggle v-model="a2aIncludeTrace" class="col-12" color="primary" label="包含 trace metadata" />
          </div>
        </q-expansion-item>

        <q-separator />
        <div class="row items-center justify-between">
          <div class="text-subtitle2">成员 Agent</div>
          <q-btn flat rounded color="primary" icon="add" label="添加成员" @click="$emit('addMember')" />
        </div>
        <div class="member-editor-list">
          <q-card v-for="(member, index) in definition.members" :key="index" flat bordered class="member-editor">
            <q-card-section class="row q-col-gutter-sm items-center">
              <q-select v-model="member.agent_id" class="col-12 col-md-4 team-control" dense outlined emit-value map-options label="Agent" :options="agentOptions" />
              <q-select v-model="member.role" class="col-12 col-md-2 team-control" dense outlined emit-value map-options label="角色" :options="roleOptions" />
              <q-input v-model="member.name" class="col-12 col-md-3 team-control" dense outlined label="成员名称" />
              <q-input v-model.number="member.sort_order" class="col-6 col-md-1 team-control" dense outlined type="number" label="顺序" />
              <q-toggle v-model="member.enabled" class="col-3 col-md-1" color="primary" />
              <q-btn class="col-3 col-md-1" flat dense round color="negative" icon="delete" @click="$emit('removeMember', index)" />
            </q-card-section>
          </q-card>
        </div>

        <q-expansion-item icon="account_tree" label="图工作流 / JSON">
          <div class="topology-preview q-mb-md">
            <div v-for="node in topologyNodesFromDefinition(definition)" :key="node.label" class="topology-node">
              <q-icon :name="node.icon" />
              <span>{{ node.label }}</span>
            </div>
          </div>
          <div class="graph-preview q-mb-md">
            <div class="graph-canvas">
              <div v-for="edge in graph.edges" :key="edge.id" class="graph-edge">
                <span>{{ nodeLabel(edge.source) }}</span>
                <q-icon name="arrow_forward" />
                <span>{{ nodeLabel(edge.target) }}</span>
                <q-badge v-if="edge.label" rounded color="primary">{{ edge.label }}</q-badge>
              </div>
              <div class="graph-nodes">
                <div v-for="node in graph.nodes" :key="node.id" class="graph-node">
                  <q-icon :name="graphNodeIcon(node.type)" />
                  <div>
                    <div class="text-weight-medium">{{ node.label }}</div>
                    <div v-if="node.role" class="text-caption text-grey-7">{{ node.role }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <pre class="definition-json">{{ definitionJSON }}</pre>
        </q-expansion-item>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat rounded label="取消" @click="$emit('update:modelValue', false)" />
        <q-btn color="primary" rounded unelevated label="保存" :loading="saving" :disable="!canSave" @click="$emit('save')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { TeamDefinition } from "../../features/teams/api";
import { buildGraphFromDefinition, modeOptions, roleOptions, statusOptions, teamTemplateOptions, topologyNodesFromDefinition, type TeamTemplateKey } from "./teamUtils";

const props = defineProps<{
  modelValue: boolean;
  editingId: string;
  form: {
    team_key: string;
    display_name: string;
    status: string;
    adk_app_name: string;
  };
  definition: TeamDefinition;
  definitionJSON: string;
  agentOptions: Array<{ label: string; value: string }>;
  saving: boolean;
  canSave: boolean;
  isDark: boolean;
}>();

defineEmits<{
  "update:modelValue": [value: boolean];
  addMember: [];
  removeMember: [index: number];
  applyTemplate: [template: TeamTemplateKey];
  save: [];
}>();

const graph = computed(() => buildGraphFromDefinition(props.definition));
const a2aFormatOptions = [
  { label: "Markdown + JSON", value: "markdown_json" },
  { label: "Plain", value: "plain" }
];
const a2aEnabled = computed({
  get: () => props.definition.a2a?.enabled ?? true,
  set: (value: boolean) => {
    props.definition.a2a = { ...props.definition.a2a, enabled: value };
  }
});
const a2aEnvelopeVersion = computed({
  get: () => props.definition.a2a?.envelope_version || "a2a.v1",
  set: (value: string) => {
    props.definition.a2a = { ...props.definition.a2a, envelope_version: value };
  }
});
const a2aMessageFormat = computed({
  get: () => props.definition.a2a?.message_format || "markdown_json",
  set: (value: string) => {
    props.definition.a2a = { ...props.definition.a2a, message_format: value };
  }
});
const a2aMaxPayloadChars = computed({
  get: () => props.definition.a2a?.max_payload_chars || 6000,
  set: (value: number) => {
    props.definition.a2a = { ...props.definition.a2a, max_payload_chars: value };
  }
});
const a2aIncludeTrace = computed({
  get: () => props.definition.a2a?.include_trace ?? true,
  set: (value: boolean) => {
    props.definition.a2a = { ...props.definition.a2a, include_trace: value };
  }
});

function nodeLabel(id: string) {
  return graph.value.nodes.find((node) => node.id === id)?.label || id;
}

function graphNodeIcon(type: string) {
  return ({ start: "play_circle", agent: "smart_toy", join: "merge_type", end: "flag" } as Record<string, string>)[type] || "radio_button_unchecked";
}
</script>

<style scoped>
.team-editor {
  width: 920px;
  max-width: 94vw;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 18px 48px rgba(16, 24, 40, 0.06);
  backdrop-filter: blur(16px);
}

.team-control :deep(.q-field__control) {
  border-radius: 16px;
  background: #ffffff;
}

.member-editor-list {
  display: grid;
  gap: 8px;
}

.member-editor {
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 16px;
  background: #fbfcff;
}

.template-panel {
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 18px;
  background: rgba(238, 246, 255, 0.55);
}

.topology-preview {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.topology-node {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 10px;
  border: 1px solid rgba(25, 118, 210, 0.12);
  border-radius: 999px;
  background: #eef6ff;
  color: #155ebc;
  font-size: 12px;
  font-weight: 700;
}

.graph-preview {
  padding: 12px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 18px;
  background: rgba(248, 250, 252, 0.82);
}

.graph-canvas,
.graph-nodes {
  display: grid;
  gap: 10px;
}

.graph-nodes {
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
}

.graph-node,
.graph-edge {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid rgba(25, 118, 210, 0.12);
  border-radius: 14px;
  background: #ffffff;
}

.graph-edge {
  flex-wrap: wrap;
  color: #475467;
  font-size: 12px;
}

.graph-node {
  color: #155ebc;
}

.definition-json {
  white-space: pre-wrap;
  max-height: 240px;
  overflow: auto;
  padding: 12px;
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.06);
}

.team-editor.is-dark,
.team-editor.is-dark .member-editor,
.team-editor.is-dark .template-panel {
  border-color: rgba(148, 163, 184, 0.16);
  background: rgba(17, 24, 39, 0.9);
  box-shadow: 0 14px 38px rgba(0, 0, 0, 0.32);
}

.team-editor.is-dark .team-control :deep(.q-field__control) {
  border-color: rgba(148, 163, 184, 0.14);
  background: rgba(30, 41, 59, 0.76);
}

.team-editor.is-dark .topology-node {
  border-color: rgba(96, 165, 250, 0.22);
  background: rgba(30, 64, 175, 0.24);
  color: #93c5fd;
}

.team-editor.is-dark .graph-preview,
.team-editor.is-dark .graph-node,
.team-editor.is-dark .graph-edge {
  border-color: rgba(148, 163, 184, 0.14);
  background: rgba(30, 41, 59, 0.76);
}

.team-editor.is-dark .graph-node {
  color: #93c5fd;
}

.team-editor.is-dark .graph-edge {
  color: #cbd5e1;
}

.team-editor.is-dark .definition-json {
  background: rgba(15, 23, 42, 0.86);
}
</style>
