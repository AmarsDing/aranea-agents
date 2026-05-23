<template>
  <div class="graph-template-picker">
    <div class="graph-template-picker__title">从模板创建</div>
    <q-btn
      flat
      dense
      color="primary"
      icon="dashboard_customize"
      label="选择模板"
      class="full-width q-mt-xs"
      :loading="loading"
      @click="dialogOpen = true"
    />

    <q-dialog v-model="dialogOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
        <q-card-section class="app-glass-dialog__head">
          <div class="app-glass-dialog__title">Graph 模板</div>
          <div class="app-glass-dialog__subtitle">选择内置设计模式快速起步</div>
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-dialog-body app-glass-dialog__body">
            <q-spinner v-if="loading" color="primary" size="32px" class="q-ma-md" />
            <div v-else-if="!templates.length" class="text-caption app-text-secondary">暂无可用模板。</div>
            <q-list v-else bordered separator class="rounded-borders">
              <q-item
                v-for="template in templates"
                :key="template.id"
                clickable
                @click="selectTemplate(template.id)"
              >
                <q-item-section>
                  <q-item-label>{{ template.name }}</q-item-label>
                  <q-item-label caption>{{ template.description }}</q-item-label>
                  <q-item-label caption>{{ template.category }} · {{ template.nodes.length }} 节点</q-item-label>
                </q-item-section>
                <q-item-section side>
                  <q-icon name="chevron_right" />
                </q-item-section>
              </q-item>
            </q-list>
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn flat rounded label="关闭" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import type { GraphTemplateInfo } from "../../features/graph/types";

const props = defineProps<{
  templates: GraphTemplateInfo[];
  loading: boolean;
}>();

const emit = defineEmits<{
  createFromTemplate: [templateId: string];
  requestTemplates: [];
}>();

const dialogOpen = ref(false);

watch(dialogOpen, (open) => {
  if (open && !props.templates.length) {
    emit("requestTemplates");
  }
});

function selectTemplate(templateId: string) {
  emit("createFromTemplate", templateId);
  dialogOpen.value = false;
}
</script>

<style scoped>
.graph-template-picker__title {
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-text-secondary, var(--color-text-secondary));
}
</style>
