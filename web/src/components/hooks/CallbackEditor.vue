<template>
  <div class="callback-editor">
    <section class="callback-editor__section">
      <div class="callback-editor__section-head">
        <span class="callback-editor__section-title">{{ t('hooksPage.callbackEditor.sectionSummary') }}</span>
        <span class="hook-tag hook-tag--point">{{ callbackPointLabel(localRule.callback_point, t) }}</span>
        <span :class="actionTagClass(localRule.action.type)">{{ actionTypeLabel(localRule.action.type, t) }}</span>
      </div>
      <div class="app-form-field-grid app-form-field-grid--wide">
        <q-select
          v-model="localRule.callback_point"
          dense
          outlined
          emit-value
          map-options
          :label="t('hooksPage.callbackEditor.fieldCallbackPoint')"
          :options="pointOptions"
          @update:model-value="emitChange"
        />
        <q-select
          v-model="localRule.action.type"
          dense
          outlined
          emit-value
          map-options
          :label="t('hooksPage.callbackEditor.fieldAction')"
          :options="actionOptions"
          @update:model-value="emitChange"
        />
        <q-input
          v-model.number="sortOrder"
          dense
          outlined
          type="number"
          :label="t('hooksPage.callbackEditor.fieldSortOrder')"
          :hint="t('hooksPage.callbackEditor.sortOrderHint')"
          @update:model-value="emitMeta"
        />
      </div>
    </section>

    <q-expansion-item
      dense-toggle
      default-opened
      icon="filter_alt"
      :label="t('hooksPage.callbackEditor.expansionCondition')"
      header-class="callback-editor__expansion-head"
    >
      <div class="callback-editor__panel app-form-field-grid">
        <q-input
          v-model="localRule.condition.agent_id"
          dense
          outlined
          :clearable="!lockAgentId"
          :label="t('hooksPage.callbackEditor.fieldAgentId')"
          :hint="
            lockAgentId ? t('hooksPage.callbackEditor.agentIdLockedHint') : t('hooksPage.callbackEditor.agentIdHint')
          "
          :disable="lockAgentId"
          @update:model-value="emitChange"
        />
        <q-select
          v-if="useToolSelect"
          v-model="localRule.condition.tool_name"
          dense
          outlined
          clearable
          use-input
          emit-value
          map-options
          :label="t('hooksPage.callbackEditor.fieldToolName')"
          :hint="t('hooksPage.callbackEditor.toolNameHint')"
          :disable="!toolPoint"
          :options="filteredToolOptions"
          :loading="loadingToolOptions"
          @filter="onToolFilter"
          @update:model-value="emitChange"
        >
          <template #option="scope">
            <q-item v-bind="scope.itemProps">
              <q-item-section>
                <q-item-label>{{ scope.opt.label }}</q-item-label>
              </q-item-section>
              <q-item-section v-if="enabledToolSet.has(scope.opt.value)" side>
                <q-badge outline color="positive" :label="t('hooksPage.callbackEditor.toolEnabledBadge')" />
              </q-item-section>
            </q-item>
          </template>
        </q-select>
        <q-input
          v-else
          v-model="localRule.condition.tool_name"
          dense
          outlined
          clearable
          :label="t('hooksPage.callbackEditor.fieldToolName')"
          :hint="t('hooksPage.callbackEditor.toolNameHint')"
          :disable="!toolPoint"
          @update:model-value="emitChange"
        />
        <q-select
          v-model="localRule.condition.event_type"
          dense
          outlined
          clearable
          emit-value
          map-options
          :label="t('hooksPage.callbackEditor.fieldEventType')"
          :hint="t('hooksPage.callbackEditor.eventTypeHint')"
          :disable="!onEventPoint"
          :options="eventTypeOptions"
          @update:model-value="emitChange"
        />
      </div>
    </q-expansion-item>

    <q-expansion-item
      dense-toggle
      default-opened
      icon="bolt"
      :label="t('hooksPage.callbackEditor.expansionAction')"
      header-class="callback-editor__expansion-head"
    >
      <div class="callback-editor__panel q-gutter-md">
        <template v-if="showNotifyFields">
          <q-input
            v-model="localRule.action.webhook_url"
            class="app-grid-span-full"
            dense
            outlined
            :label="t('hooksPage.callbackEditor.fieldWebhookUrl')"
            :error="webhookUrlError !== ''"
            :error-message="webhookUrlError"
            @update:model-value="onWebhookUrlChange"
          />
          <q-input
            v-model="localRule.action.webhook_secret"
            class="app-grid-span-full"
            dense
            outlined
            :type="showSecret ? 'text' : 'password'"
            :label="t('hooksPage.callbackEditor.fieldWebhookSecret')"
            :hint="t('hooksPage.callbackEditor.webhookSecretHint')"
            @update:model-value="emitChange"
          >
            <template #append>
              <q-icon
                :name="showSecret ? 'visibility_off' : 'visibility'"
                class="cursor-pointer"
                @click="showSecret = !showSecret"
              />
            </template>
          </q-input>
          <div class="app-form-field-grid app-form-field-grid--2col app-grid-span-full">
            <q-input
              v-model.number="localRule.action.notify_max_retries"
              dense
              outlined
              type="number"
              min="1"
              max="10"
              :label="t('hooksPage.callbackEditor.fieldMaxRetries')"
              :hint="t('hooksPage.callbackEditor.maxRetriesHint')"
              @update:model-value="emitChange"
            />
            <q-input
              v-model.number="localRule.action.notify_timeout_sec"
              dense
              outlined
              type="number"
              min="1"
              max="60"
              :label="t('hooksPage.callbackEditor.fieldTimeoutSec')"
              :hint="t('hooksPage.callbackEditor.timeoutSecHint')"
              @update:model-value="emitChange"
            />
          </div>
        </template>

        <q-select
          v-if="showLogFields"
          v-model="localRule.action.log_level"
          dense
          outlined
          emit-value
          map-options
          :label="t('hooksPage.callbackEditor.fieldLogLevel')"
          :options="logLevelOptions"
          @update:model-value="emitChange"
        />

        <q-input
          v-if="showMessageField"
          v-model="localRule.action.message"
          class="app-grid-span-full"
          dense
          outlined
          type="textarea"
          autogrow
          :label="t('hooksPage.callbackEditor.fieldMessage')"
          @update:model-value="emitChange"
        />

        <template v-if="showModifyFields">
          <p class="callback-editor__hint app-grid-span-full">{{ t('hooksPage.callbackEditor.modifyPatchHint') }}</p>
          <q-input
            v-model="modifyPatchText"
            class="app-grid-span-full"
            dense
            outlined
            type="textarea"
            rows="8"
            :label="t('hooksPage.callbackEditor.modifyPatchLabel')"
            :error="Boolean(modifyPatchError)"
            :error-message="modifyPatchError"
            @update:model-value="onModifyPatchInput"
          />
        </template>
      </div>
    </q-expansion-item>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { actionTagClass, actionTypeLabel } from './callbackEditorUi';
import { callbackPointLabel } from '../../features/callback/constants';
import { useCallbackEditor } from './useCallbackEditor';
import type { HookRuleConfig } from '../../features/hooks/types';

const { t } = useI18n();

const logLevelOptions = computed(() => [
  { label: t('hooksPage.logLevels.debug'), value: 'debug' },
  { label: t('hooksPage.logLevels.info'), value: 'info' },
  { label: t('hooksPage.logLevels.warn'), value: 'warn' },
  { label: t('hooksPage.logLevels.error'), value: 'error' },
]);

const props = defineProps<{
  modelValue: HookRuleConfig;
  sortOrder?: number;
  agentId?: string;
  agentKey?: string;
  lockAgentId?: boolean;
  toolOptions?: { label: string; value: string }[];
  loadingToolOptions?: boolean;
  /** 当前 Agent 生效工具 key（用于 Tool 下拉「已启用」标注；不传则不标注）。 */
  enabledToolKeys?: string[];
}>();

const emit = defineEmits<{
  'update:modelValue': [HookRuleConfig];
  'update:sortOrder': [number];
  'update:valid': [boolean];
}>();

const enabledToolSet = computed(() => new Set(props.enabledToolKeys ?? []));

const {
  localRule,
  sortOrder,
  modifyPatchText,
  modifyPatchError,
  pointOptions,
  actionOptions,
  toolPoint,
  onEventPoint,
  showNotifyFields,
  showLogFields,
  showModifyFields,
  showMessageField,
  showSecret,
  webhookUrlError,
  useToolSelect,
  filteredToolOptions,
  eventTypeOptions,
  emitChange,
  emitMeta,
  onModifyPatchInput,
  onWebhookUrlChange,
  onToolFilter,
} = useCallbackEditor(props, emit);
</script>
