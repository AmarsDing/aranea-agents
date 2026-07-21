<template>
  <div class="graph-template-picker">
    <div class="graph-template-picker__title">{{ t('graphs.templatePickerTitle') }}</div>
    <q-btn
      flat
      dense
      color="primary"
      icon="dashboard_customize"
      :label="t('graphs.templatePickerButton')"
      class="full-width q-mt-xs"
      :loading="loading"
      @click="dialogOpen = true"
    />

    <q-dialog v-model="dialogOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
        <q-card-section class="app-glass-dialog__head">
          <div class="app-glass-dialog__title">{{ t('graphs.templatePickerDialogTitle') }}</div>
          <div class="app-glass-dialog__subtitle">{{ t('graphs.templatePickerDialogSubtitle') }}</div>
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-dialog-body app-glass-dialog__body">
            <q-spinner v-if="loading" color="primary" size="32px" class="q-ma-md" />
            <div v-else-if="!templates.length" class="text-caption app-text-secondary">
              {{ t('graphs.templatePickerEmpty') }}
            </div>
            <q-list v-else bordered separator class="rounded-borders">
              <q-item v-for="template in templates" :key="template.id" clickable @click="selectTemplate(template.id)">
                <q-item-section>
                  <q-item-label>{{ template.name }}</q-item-label>
                  <q-item-label caption>{{ template.description }}</q-item-label>
                  <q-item-label caption>
                    {{ template.category }} ·
                    {{ t('graphs.templatePickerNodesCount', { count: template.nodes.length }) }}
                  </q-item-label>
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
          <q-btn v-close-popup flat rounded :label="t('graphs.templatePickerClose')" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphTemplateInfo } from '../../features/graph/types';

const { t } = useI18n();
void t;

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
    emit('requestTemplates');
  }
});

function selectTemplate(templateId: string) {
  emit('createFromTemplate', templateId);
  dialogOpen.value = false;
}
</script>
