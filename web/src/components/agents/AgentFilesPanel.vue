// Container: approved — feature-local panel; refineFn injected from Page via props.
<template>
  <q-splitter v-model="splitterModel" :limits="[18, 45]" class="agent-files-splitter fit">
    <template #before>
      <div class="agent-file-side column fit">
        <q-list bordered separator class="agent-file-list col">
          <q-item
            v-for="file in files"
            :key="file.name"
            clickable
            :active="activeFile === file.name"
            active-class="agent-file-item--active"
            class="agent-file-item"
            @click="$emit('update:activeFile', file.name)"
          >
            <q-item-section>
              <q-item-label>{{ file.name }}</q-item-label>
              <q-item-label caption class="agent-file-caption">
                {{ fileTokenLabel(file.name, file.body) }}
                <span v-if="!isInjected(file.name)" class="agent-file-not-injected"
                  >· {{ $t('agentSettings.files.notInjected') }}</span
                >
              </q-item-label>
            </q-item-section>
            <q-item-section v-if="isRemovable(file.name)" side>
              <q-btn
                flat
                round
                dense
                size="sm"
                icon="close"
                class="agent-file-item__remove"
                @click.stop="$emit('remove-file', file.name)"
              >
                <q-tooltip>{{ $t('agentSettings.files.removeTooltip') }}</q-tooltip>
              </q-btn>
            </q-item-section>
          </q-item>
        </q-list>

        <div v-if="availableOptionalFiles?.length" class="agent-file-optional flex-shrink-0">
          <div class="agent-file-optional__title">{{ $t('agentSettings.files.optionalTitle') }}</div>
          <q-btn
            v-for="file in availableOptionalFiles"
            :key="file.name"
            dense
            outline
            rounded
            color="primary"
            icon="add"
            size="sm"
            :label="$t('agentSettings.files.addOptional', { name: file.name })"
            @click="$emit('add-optional-file', file.name)"
          />
        </div>
      </div>
    </template>

    <template #after>
      <div class="agent-file-editor q-pa-md column fit">
        <div class="row items-start justify-between q-gutter-md flex-shrink-0">
          <div>
            <div class="text-h6">{{ activeFile }}</div>
            <div class="text-caption agent-file-caption">{{ activeFileMeta.caption }}</div>
          </div>
          <div class="row q-gutter-sm">
            <q-btn
              outline
              rounded
              icon="refresh"
              :label="$t('agentSettings.files.reload')"
              @click="$emit('confirm-reload')"
            />
            <!-- PGO-3-WEB-03: AIRefineButton for file Tab editor. -->
            <AIRefineButton
              scope="agent.file"
              :file-name="activeFile"
              :resource-id="agentId || undefined"
              :text="bodyModel"
              :refine-fn="props.refineFn"
              outline
              @apply="(v: string) => emit('update-file-body', activeFile, v)"
              @error="(msg: string) => emit('refine-error', msg)"
            />
            <q-btn
              color="primary"
              rounded
              unelevated
              icon="save"
              :label="$t('agentSettings.files.save')"
              :disable="!dirty"
              @click="$emit('save')"
            />
          </div>
        </div>
        <q-input v-model="bodyModel" class="q-mt-md app-markdown-editor" outlined type="textarea" label="Markdown" />
        <div class="agent-file-editor__footer flex-shrink-0">
          {{ $t('agentSettings.files.tokenEstimateFooter', { count: activeTokenCount }) }}
        </div>
      </div>
    </template>
  </q-splitter>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { AgentFile } from './agentUi';
import { isFileInjectedInMode, isRemovableAgentFile, tokenEstimateFor } from './agentUi';
import AIRefineButton from './AIRefineButton.vue';
import type { RefineResponse } from '../../features/agents/aiRefine';

const { t } = useI18n();

const props = defineProps<{
  files: AgentFile[];
  activeFile: string;
  splitter: number;
  dirty: boolean;
  fileTokenByName?: Record<string, number>;
  agentId?: string;
  /** Current system_prompt_mode; drives the "not injected in this mode" marker. */
  systemPromptMode?: string;
  /** Optional files not yet added (e.g. USER_CONTEXT.md). */
  availableOptionalFiles?: AgentFile[];
  refineFn: (params: {
    scope: string;
    fileName?: string;
    resourceId?: string;
    originalText: string;
    userHint: string;
    targetMode: string;
  }) => Promise<RefineResponse>;
}>();

function fileTokenLabel(name: string, body: string) {
  const n = props.fileTokenByName?.[name];
  if (n != null && n > 0) return t('agentSettings.files.tokenEstimateLabel', { n });
  const count = tokenEstimateFor(body);
  return count > 0
    ? t('agentSettings.files.tokenEstimateLabel', { n: count })
    : t('agentSettings.files.tokenEmpty');
}

/** Footer uses the live local estimate so it tracks edits in real time. */
const activeTokenCount = computed(() => tokenEstimateFor(bodyModel.value));

const isInjected = (name: string) => isFileInjectedInMode(name, props.systemPromptMode ?? 'complete');
const isRemovable = (name: string) => isRemovableAgentFile(name);

const emit = defineEmits<{
  'update:activeFile': [value: string];
  'update:splitter': [value: number];
  'update-file-body': [fileName: string, body: string];
  'confirm-reload': [];
  save: [];
  'add-optional-file': [name: string];
  'remove-file': [name: string];
  'refine-error': [message: string];
}>();

const splitterModel = computed({
  get: () => props.splitter,
  set: (value: number) => emit('update:splitter', value),
});

const activeFileMeta = computed(() => props.files.find((file) => file.name === props.activeFile) ?? props.files[0]);

const bodyModel = computed({
  get: () => activeFileMeta.value?.body ?? '',
  set: (value: string) => emit('update-file-body', props.activeFile, value),
});
</script>
