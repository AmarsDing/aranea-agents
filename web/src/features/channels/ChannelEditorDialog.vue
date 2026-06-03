// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="app-dialog-card app-dialog-card--900 app-glass-dialog channel-editor-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between no-wrap">
        <div class="col min-width-0">
          <div class="app-glass-dialog__title">
            {{ row ? t('channelEditor.editTitle') : t('channelEditor.createTitle') }}
          </div>
        </div>
        <q-btn
          flat
          dense
          round
          icon="close"
          :aria-label="t('channelEditor.close')"
          @click="$emit('update:modelValue', false)"
        />
      </q-card-section>
      <q-separator />

      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body">
          <section v-if="!row" class="app-dialog-section channel-editor-platform-pick">
            <h3 class="channel-editor-platform-pick__title">{{ t('channelEditor.pickPlatformTitle') }}</h3>
            <p class="channel-editor-platform-pick__hint">{{ t('channelEditor.pickPlatformHint') }}</p>
            <div class="channel-catalog-shell">
              <ChannelCatalogPicker v-model="selectedType" :catalog="catalog" />
            </div>
          </section>

          <div v-if="selectedCatalog" ref="configPanelRef" class="app-musebot-config-panel channel-editor-config-panel">
            <header class="channel-editor-config-intro">
              <div class="channel-editor-config-intro__head row items-center no-wrap q-gutter-sm">
                <channel-platform-avatar
                  :type="selectedType"
                  :label="form.name || selectedCatalog.label"
                  :metadata="iconPreviewMetadata"
                  size="36px"
                />
                <div class="col min-width-0">
                  <h2 class="channel-editor-config-intro__title">
                    {{ form.name || selectedCatalog.label }}
                  </h2>
                  <p class="channel-editor-config-intro__meta">
                    {{ selectedCatalog.label }} · {{ selectedCatalog.type }} · {{ selectedCatalog.group }}
                  </p>
                </div>
              </div>
              <p v-if="catalogDescription" class="channel-editor-config-intro__desc">
                {{ catalogDescription }}
              </p>
              <p class="channel-editor-config-intro__note">{{ t('channelEditor.credentialsNote') }}</p>
            </header>

            <section
              v-for="section in platformSections"
              :id="sectionDomId(section.id)"
              :key="section.id"
              class="app-musebot-section"
            >
              <h3 class="app-musebot-section__title">{{ sectionTitle(section) }}</h3>
              <p v-if="sectionHint(section)" class="app-musebot-section__hint">{{ sectionHint(section) }}</p>

              <channel-config-row
                v-if="section.id === 'long_task'"
                :label="t('channelEditor.longTaskPresetLabel')"
                field-key="long_task_preset"
                :help="longTaskPresetHelp"
              >
                <q-select
                  :model-value="selectedLongTaskPreset"
                  dense
                  outlined
                  emit-value
                  map-options
                  clearable
                  :options="longTaskPresetOptions"
                  :placeholder="t('channelEditor.longTaskPresetPlaceholder')"
                  @update:model-value="applyLongTaskPreset(String($event ?? ''))"
                />
              </channel-config-row>

              <div class="app-musebot-section__rows">
                <ChannelRoutingFields
                  v-if="section.id === 'routing'"
                  v-model:target-type="routingTargetType"
                  v-model:agent-id="defaultAgentId"
                  v-model:team-id="defaultTeamId"
                  v-model:dm-scope="dmScope"
                  v-model:routing-rules="routingRules"
                  :agents="routingAgents"
                  :teams="routingTeams"
                  :loading="routingOptionsLoading"
                  :routing-agent-provider="selectedRoutingAgent?.provider"
                  :routing-agent-model="selectedRoutingAgent?.model"
                  :routing-agent-model-checking="routingAgentModelChecking"
                  :routing-agent-model-ok="routingAgentModelOk"
                  :routing-agent-model-message="routingAgentModelMessage"
                />

                <template v-for="field in visibleSectionFields(section)">
                  <div v-if="field.bind.source === 'icon'" :key="field.museKey" class="app-musebot-row">
                    <label class="app-musebot-row__label">{{ field.museKey }}</label>
                    <div class="app-musebot-row__control">
                      <div class="channel-icon-pick" @click="iconPickerOpen = true">
                        <channel-platform-avatar
                          :type="selectedType"
                          :label="form.name || selectedCatalog?.label || selectedType"
                          :metadata="iconPreviewMetadata"
                          size="48px"
                        />
                        <span class="channel-icon-pick__hint">{{ t('channelEditor.changeIcon') }}</span>
                      </div>
                      <q-btn
                        v-if="iconAssetId"
                        flat
                        dense
                        no-caps
                        :label="t('channelEditor.resetPlatformIcon')"
                        class="q-mt-sm"
                        @click.stop="iconAssetId = ''"
                      />
                      <q-btn
                        flat
                        dense
                        no-caps
                        icon="refresh"
                        :label="t('channelEditor.refreshPlatformIcons')"
                        :loading="refreshingIcons"
                        class="q-mt-sm"
                        @click.stop="onRefreshPlatformIcons"
                      />
                      <agent-avatar-picker v-model="iconAssetId" v-model:open="iconPickerOpen" scope="channel" />
                    </div>
                    <span class="app-musebot-row__status">{{ fieldStatusLabel(fieldStatus(field)) }}</span>
                  </div>

                  <channel-config-row
                    v-else-if="field.bind.source === 'webhook' && field.bind.key === 'preview'"
                    :key="field.museKey"
                    :label="fieldLabel(field)"
                    :field-key="field.museKey"
                    :help="fieldHelp(field)"
                    :status="fieldStatusLabel(fieldStatus(field))"
                  >
                    <div class="app-musebot-row__control--readonly row items-center no-wrap q-gutter-xs">
                      <q-input
                        :model-value="webhookPreview"
                        dense
                        outlined
                        readonly
                        class="col"
                        :placeholder="t('channelEditor.webhookPreviewPlaceholder')"
                      />
                      <q-btn
                        flat
                        dense
                        round
                        icon="content_copy"
                        :aria-label="t('channelEditor.copyWebhook')"
                        :disable="!webhookPreview"
                        @click="copyWebhookPreview"
                      />
                    </div>
                  </channel-config-row>

                  <channel-config-row
                    v-else
                    :key="field.museKey"
                    :label="fieldLabel(field)"
                    :field-key="field.museKey"
                    :help="fieldHelp(field)"
                    :status="fieldStatusLabel(fieldStatus(field))"
                  >
                    <q-toggle
                      v-if="fieldKind(field) === 'toggle'"
                      :model-value="readFieldBool(field)"
                      @update:model-value="writeFieldBool(field, $event)"
                    />
                    <q-select
                      v-else-if="fieldKind(field) === 'select'"
                      :model-value="readField(field)"
                      dense
                      outlined
                      emit-value
                      map-options
                      :options="selectOptions(field)"
                      @update:model-value="writeField(field, String($event ?? ''))"
                    />
                    <q-input
                      v-else-if="fieldKind(field) === 'textarea'"
                      :model-value="readField(field)"
                      dense
                      outlined
                      autogrow
                      type="textarea"
                      :placeholder="fieldPlaceholder(field)"
                      @update:model-value="writeField(field, String($event ?? ''))"
                    />
                    <q-input
                      v-else
                      :model-value="readField(field)"
                      dense
                      outlined
                      :type="fieldKind(field) === 'password' && !showSecrets ? 'password' : 'text'"
                      :placeholder="fieldPlaceholder(field)"
                      @update:model-value="writeField(field, String($event ?? ''))"
                    />
                  </channel-config-row>
                </template>

                <channel-config-row
                  v-if="section.id === 'base' && credentialKeys.length"
                  :label="t('channelEditor.showSecretsLabel')"
                  field-key="show_secrets"
                >
                  <q-toggle v-model="showSecrets" :label="t('channelEditor.showSecrets')" />
                </channel-config-row>
              </div>

              <q-banner
                v-if="section.id === 'connection' && webhookIsLocalhost"
                dense
                rounded
                class="webhook-local-banner q-mt-md"
              >
                <template #avatar>
                  <q-icon name="info" color="warning" />
                </template>
                <div class="webhook-local-banner__body">
                  <div>{{ t('channelEditor.webhookLocalTitle') }}</div>
                  <ol class="webhook-local-steps">
                    <li>{{ t('channelEditor.webhookLocalStep1') }}</li>
                    <li>{{ t('channelEditor.webhookLocalStep2') }}</li>
                    <li>{{ t('channelEditor.webhookLocalStep3') }}</li>
                  </ol>
                </div>
              </q-banner>
            </section>

            <q-expansion-item
              id="channel-section-advanced"
              icon="data_object"
              :label="t('channelEditor.advancedTitle')"
              class="channel-editor-expansion"
              header-class="text-weight-medium"
            >
              <p class="channel-editor-platform-pick__hint q-px-md q-pt-sm q-mb-none">
                {{ t('channelEditor.advancedHint') }}
              </p>
              <div class="app-form-field-grid q-pt-sm q-px-md q-pb-md">
                <q-input
                  v-model="configExtraText"
                  class="app-grid-span-full"
                  dense
                  outlined
                  autogrow
                  type="textarea"
                  :label="t('channelEditor.configExtraLabel')"
                  :error="Boolean(configError)"
                  :error-message="configError"
                />
                <q-input
                  v-model="metadataExtraText"
                  class="app-grid-span-full"
                  dense
                  outlined
                  autogrow
                  type="textarea"
                  :label="t('channelEditor.metadataExtraLabel')"
                  :error="Boolean(metadataError)"
                  :error-message="metadataError"
                />
              </div>
            </q-expansion-item>

            <section v-if="row?.id" class="app-musebot-section q-mt-md">
              <ChannelTurnJobsPanel :channel-id="row.id" />
            </section>
          </div>
        </q-card-section>
      </div>

      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn flat no-caps :label="t('channelEditor.cancel')" @click="$emit('update:modelValue', false)" />
        <q-space />
        <q-btn
          flat
          no-caps
          class="channel-dialog-test"
          icon="science"
          :label="t('channelEditor.saveAndTest')"
          :loading="testing"
          :disable="!canSave || saving"
          @click="saveAndTest"
        />
        <q-btn
          unelevated
          no-caps
          class="channel-dialog-save"
          :label="t('channelEditor.save')"
          :loading="saving"
          :disable="!canSave"
          @click="save"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { toRef, computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import ChannelCatalogPicker from './ChannelCatalogPicker.vue';
import ChannelConfigRow from '../../components/channels/ChannelConfigRow.vue';
import ChannelRoutingFields from '../../components/channels/ChannelRoutingFields.vue';
import ChannelPlatformAvatar from '../../components/channels/ChannelPlatformAvatar.vue';
import AgentAvatarPicker from '../../components/avatar/AgentAvatarPicker.vue';
import ChannelTurnJobsPanel from './ChannelTurnJobsPanel.vue';
import { useChannelEditorForm } from './useChannelEditorForm';
import { useChannelEditorLabels } from './useChannelEditorLabels';
import { useAvatarCatalogStore } from '../../stores/avatar';
import type { ChannelCatalogItem, ChannelCredential, ChannelRow } from './types';

const props = defineProps<{
  modelValue: boolean;
  catalog: ChannelCatalogItem[];
  row: ChannelRow | null;
  credentials: ChannelCredential[];
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  saved: [row: ChannelRow];
  tested: [];
}>();

const { t } = useI18n();

const {
  saving,
  testing,
  selectedType,
  showSecrets,
  iconAssetId,
  iconPickerOpen,
  configExtraText,
  metadataExtraText,
  form,
  selectedCatalog,
  platformSections,
  credentialKeys,
  configError,
  metadataError,
  canSave,
  webhookPreview,
  webhookIsLocalhost,
  iconPreviewMetadata,
  routingTargetType,
  defaultAgentId,
  defaultTeamId,
  dmScope,
  routingRules,
  routingAgents,
  routingTeams,
  routingOptionsLoading,
  selectedRoutingAgent,
  routingAgentModelChecking,
  routingAgentModelOk,
  routingAgentModelMessage,
  selectedLongTaskPreset,
  longTaskPresetOptions,
  applyLongTaskPreset,
  visibleSectionFields,
  fieldKind,
  readField,
  writeField,
  readFieldBool,
  writeFieldBool,
  fieldStatus,
  save,
  saveAndTest,
  copyWebhookPreview,
} = useChannelEditorForm(props, toRef(props, 'modelValue'), emit);

const {
  catalogDescription,
  sectionTitle,
  sectionHint,
  fieldLabel,
  fieldHelp,
  fieldStatusLabel,
  fieldPlaceholder,
  selectOptions,
} = useChannelEditorLabels(selectedCatalog);

const longTaskPresetHelp = computed(() => ({
  description: t('channelEditor.longTaskPresetHelp'),
  example: t('channelEditor.longTaskPresetExample'),
}));

function sectionDomId(id: string) {
  return `channel-section-${id}`;
}

const refreshingIcons = ref(false);
const avatarStore = useAvatarCatalogStore();
const $q = useQuasar();

async function onRefreshPlatformIcons() {
  refreshingIcons.value = true;
  try {
    const result = await avatarStore.refreshChannelIcons();
    $q.notify({
      type: result.failed > 0 ? 'warning' : 'positive',
      message: t('channelEditor.refreshIconsResult', { updated: result.updated, failed: result.failed }),
      position: 'top',
    });
  } catch {
    $q.notify({ type: 'negative', message: t('channelEditor.refreshIconsFailed'), position: 'top' });
  } finally {
    refreshingIcons.value = false;
  }
}
</script>
