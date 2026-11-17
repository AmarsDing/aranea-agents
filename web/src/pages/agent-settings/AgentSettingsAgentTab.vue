<template>
  <div class="settings-grid settings-grid--wide">
    <agent-settings-prompt-section
      :system-prompt-mode="form.system_prompt_mode"
      :prompt-modes="promptModes"
      @update:system-prompt-mode="form.system_prompt_mode = $event"
    />

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">{{ $t('domains.agentSettings.personality') }}</span>
          </div>
          <p class="settings-section__hint">{{ $t('domains.agentSettings.personalityHint') }}</p>
        </div>
        <div class="settings-section__actions">
          <q-toggle v-model="form.is_default" dense :label="$t('domains.agentSettings.defaultAgent')" />
        </div>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model="form.display_name" dense outlined :label="$t('domains.agentSettings.displayName')" />
        <q-input v-model="form.agent_key" dense outlined readonly :label="$t('domains.agentSettings.agentKey')">
          <template #append><q-btn flat round dense icon="content_copy" @click="$emit('copy-key')" /></template>
        </q-input>
        <q-select
          v-model="form.status"
          dense
          outlined
          emit-value
          map-options
          :label="$t('domains.agentSettings.status')"
          :options="statusOptions"
        />
        <taxonomy-picker
          :model-value="form.taxonomy_position_id || null"
          :tree="taxonomyTree"
          :label="$t('agentsPage.taxonomy.labelOptional')"
          :placeholder="$t('domains.agentSettings.taxonomyPlaceholder')"
          @update:model-value="form.taxonomy_position_id = $event ?? ''"
        />
        <div class="app-grid-span-full">
          <div class="desc-field-head">
            <span class="desc-field-head__label">{{ $t('domains.agentSettings.summaryLabel') }}</span>
            <!-- PGO-3-WEB-03: AI Refine button for agent description -->
            <AiRefineButton
              scope="agent.description"
              :resource-id="agentId"
              :text="form.agent_description ?? ''"
              :refine-fn="refinePromptField"
              flat
              dense
              size="sm"
              :label="$t('domains.agentSettings.refineDescription')"
              @apply="
                (v: string) => {
                  form.agent_description = v;
                }
              "
              @error="(msg: string) => emit('refine-error', msg)"
            />
          </div>
          <q-input
            v-model="form.agent_description"
            class="app-field-long"
            outlined
            autogrow
            type="textarea"
            :placeholder="$t('domains.agentSettings.descriptionPlaceholder')"
          />
        </div>
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">{{ $t('domains.agentSettings.model') }}</span>
          </div>
          <p class="settings-section__hint">{{ $t('domains.agentSettings.modelSectionHint') }}</p>
        </div>
      </div>
      <div class="model-field-row">
        <q-select
          v-model="selectedProviderModelId"
          dense
          outlined
          emit-value
          map-options
          use-input
          clearable
          input-debounce="0"
          :label="$t('domains.agentSettings.model')"
          :hint="$t('domains.agentSettings.modelHint')"
          :options="filteredProviderModelOptions"
          :loading="loadingProviderModels || checkingAgentModel"
          @filter="(val, update) => $emit('filter-provider-models', val, update)"
          @popup-show="$emit('reset-provider-model-filter')"
        >
          <template #option="scope">
            <q-item v-bind="scope.itemProps">
              <q-item-section>
                <q-item-label>{{ scope.opt.label }}</q-item-label>
                <q-item-label caption>{{ scope.opt.caption }}</q-item-label>
              </q-item-section>
            </q-item>
          </template>
        </q-select>
        <div v-if="form.provider && form.model" class="model-field-row__badge">
          <q-chip
            v-if="checkingAgentModel"
            dense
            square
            color="blue-grey-2"
            text-color="blue-grey-9"
            icon="hourglass_empty"
            :label="$t('domains.agentSettings.modelChecking')"
          />
          <q-chip
            v-else-if="agentModelCheckOk === true"
            dense
            square
            color="positive"
            text-color="white"
            icon="check_circle"
            :label="$t('domains.agentSettings.modelAvailable')"
          />
          <q-chip
            v-else-if="agentModelCheckOk === false"
            dense
            square
            color="negative"
            text-color="white"
            icon="error"
            :label="agentModelCheckMessage || $t('domains.agentSettings.modelUnavailable')"
          />
        </div>
      </div>
      <q-banner v-if="orphanProviderModel" rounded class="q-mt-sm settings-warning-banner" dense>
        {{ $t('domains.agentSettings.orphanProviderPrefix') }}
        <code>{{ form.provider }} / {{ form.model }}</code
        >{{ $t('domains.agentSettings.orphanProviderSuffix') }}
        <template #action>
          <q-btn
            flat
            no-caps
            color="primary"
            :label="$t('domains.agentSettings.openModelManager')"
            @click="$emit('open-provider-manager')"
          />
        </template>
      </q-banner>
      <q-banner v-else-if="disabledCatalogMatch" rounded class="q-mt-sm settings-warning-banner" dense>
        {{ $t('domains.agentSettings.disabledModelBanner') }}
        <template #action>
          <q-btn
            flat
            no-caps
            color="primary"
            :label="$t('domains.agentSettings.openModelManager')"
            @click="$emit('open-provider-manager')"
          />
        </template>
      </q-banner>
      <p class="settings-section__hint q-mt-sm">
        {{ $t('domains.agentSettings.costCapPrefix') }}
        <q-btn
          flat
          dense
          size="sm"
          color="primary"
          no-caps
          :label="$t('domains.agentSettings.costCapTab')"
          @click="$emit('open-permissions-tab')"
        />
        {{ $t('domains.agentSettings.costCapSuffix') }}
      </p>
    </section>

    <agent-planner-section v-model:form="plannerForm" :model-provider="form.provider" />
    <agent-ralph-loop-section v-model:form="ralphLoopForm" />

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">{{ $t('domains.agentSettings.collaboration') }}</span>
          </div>
          <p class="settings-section__hint">{{ $t('domains.agentSettings.collaborationHint') }}</p>
        </div>
      </div>

      <div class="settings-subsection-grid">
        <div class="settings-subsection">
          <div class="settings-subsection__head row items-center justify-between">
            <div>
              <div class="settings-subsection__title">{{ $t('domains.agentSettings.subagents') }}</div>
              <p class="settings-subsection__hint">{{ $t('domains.agentSettings.subagentsHint') }}</p>
            </div>
            <q-toggle v-model="config.subagents.enabled" />
          </div>
          <div v-if="config.subagents.enabled" class="app-form-field-grid app-form-field-grid--2col">
            <q-input
              v-model.number="config.subagents.max_concurrency"
              dense
              outlined
              type="number"
              :label="$t('domains.agentSettings.maxConcurrency')"
            />
            <q-input
              v-model.number="config.subagents.max_generation_depth"
              dense
              outlined
              type="number"
              :label="$t('domains.agentSettings.maxGenerationDepth')"
            />
            <q-input
              v-model.number="config.subagents.max_children_per_agent"
              dense
              outlined
              type="number"
              :label="$t('domains.agentSettings.maxChildrenPerAgent')"
            />
            <q-input
              v-model.number="config.subagents.archive_after_minutes"
              dense
              outlined
              type="number"
              :label="$t('domains.agentSettings.archiveAfterMinutes')"
            />
            <q-input
              v-model.number="config.subagents.max_retries"
              dense
              outlined
              type="number"
              :label="$t('domains.agentSettings.maxRetries')"
            />
            <q-input
              v-model.number="config.subagents.stored_result_runes"
              dense
              outlined
              type="number"
              min="100"
              :label="$t('domains.agentSettings.storedResultRunes')"
              :hint="$t('domains.agentSettings.storedResultRunesHint')"
            />
            <q-input
              v-model.number="config.subagents.stored_summary_runes"
              dense
              outlined
              type="number"
              min="50"
              :label="$t('domains.agentSettings.storedSummaryRunes')"
              :hint="$t('domains.agentSettings.storedSummaryRunesHint')"
            />
            <q-input
              v-model="config.subagents.model_override"
              dense
              outlined
              :label="$t('domains.agentSettings.modelOverride')"
              :placeholder="$t('domains.agentSettings.modelOverridePlaceholder')"
            />
          </div>
        </div>

        <div class="settings-subsection">
          <div class="settings-subsection__head row items-center justify-between">
            <div>
              <div class="settings-subsection__title">{{ $t('domains.agentSettings.intentPass') }}</div>
              <p class="settings-subsection__hint">
                {{ $t('domains.agentSettings.intentPassHintPrefix') }}
                <code>ARANEA_INTENT_PASS=0</code>
                {{ $t('domains.agentSettings.intentPassHintSuffix') }}
              </p>
            </div>
            <q-toggle v-model="config.intent_pass.enabled" />
          </div>
        </div>

        <div v-if="form.agent_key === SPIRIT_AGENT_KEY" class="settings-subsection settings-subsection--span">
          <div class="settings-subsection__head">
            <div>
              <div class="settings-subsection__title">{{ $t('domains.agentSettings.spiritConfig') }}</div>
              <p class="settings-subsection__hint">{{ $t('domains.agentSettings.spiritConfigHint') }}</p>
            </div>
          </div>
          <div class="app-form-field-grid app-form-field-grid--3col">
            <q-input
              v-model.number="config.spirit.max_concurrent_teams"
              dense
              outlined
              type="number"
              min="1"
              :label="$t('domains.agentSettings.maxConcurrentTeams')"
              :hint="$t('domains.agentSettings.maxConcurrentTeamsHint')"
            />
            <q-input
              v-model.number="config.spirit.max_team_concurrency"
              dense
              outlined
              type="number"
              min="1"
              :label="$t('domains.agentSettings.maxTeamConcurrency')"
              :hint="$t('domains.agentSettings.maxTeamConcurrencyHint')"
            />
            <q-input
              v-model.number="config.spirit.team_timeout_seconds"
              dense
              outlined
              type="number"
              min="60"
              :label="$t('domains.agentSettings.teamTimeout')"
              :hint="$t('domains.agentSettings.teamTimeoutHint')"
            />
            <q-input
              v-model.number="config.spirit.auto_archive_seconds"
              dense
              outlined
              type="number"
              min="0"
              :label="$t('domains.agentSettings.autoArchive')"
              :hint="$t('domains.agentSettings.autoArchiveHint')"
            />
            <q-input
              v-model.number="config.spirit.max_session_depth"
              dense
              outlined
              type="number"
              min="1"
              :label="$t('domains.agentSettings.maxSessionDepth')"
              :hint="$t('domains.agentSettings.maxSessionDepthHint')"
            />
            <q-input
              v-model.number="config.spirit.verification_truncate_chars"
              dense
              outlined
              type="number"
              min="500"
              :label="$t('domains.agentSettings.verificationTruncate')"
              :hint="$t('domains.agentSettings.verificationTruncateHint')"
            />
            <q-input
              v-model.number="config.spirit.timeout_handler_db_timeout_seconds"
              dense
              outlined
              type="number"
              min="5"
              :label="$t('domains.agentSettings.timeoutHandlerDbTimeout')"
              :hint="$t('domains.agentSettings.timeoutHandlerDbTimeoutHint')"
            />
          </div>
        </div>
      </div>

      <p class="settings-section__hint q-mt-md">
        {{ $t('domains.agentSettings.memoryHintPrefix') }}
        <q-btn
          flat
          dense
          size="sm"
          color="primary"
          no-caps
          :label="$t('domains.agentSettings.memoryTab')"
          @click="$emit('open-memory-tab')"
        />
        {{ $t('agentSettings.memoryTabGotoSuffix') }}
      </p>
    </section>

    <agent-channel-refs-section :agent-id="agentId" :agent-key="form.agent_key" />
  </div>
</template>

<script setup lang="ts">
const form = defineModel<Agent>('form', { required: true });
const config = defineModel<AgentRuntimeConfigForm>('config', { required: true });
import type { Agent } from '../../features/agents/types';
import { SPIRIT_AGENT_KEY } from '../../features/spirit/types';
import type { AgentRuntimeConfigForm } from '../../features/agents/agentRuntimeConfig';
import AgentChannelRefsSection from './AgentChannelRefsSection.vue';
import AgentSettingsPromptSection from './AgentSettingsPromptSection.vue';
import TaxonomyPicker from '../../components/agents/TaxonomyPicker.vue';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import AgentPlannerSection from '../../components/agents/AgentPlannerSection.vue';
import AgentRalphLoopSection from '../../components/agents/AgentRalphLoopSection.vue';
import AiRefineButton from '../../components/agents/AIRefineButton.vue';
import { refinePromptField } from '../../features/agents/aiRefine';
import type { PlannerFormState } from '../../features/agents/plannerConfig';
import type { RalphLoopFormState } from '../../features/agents/ralphLoopConfig';

const plannerForm = defineModel<PlannerFormState>('plannerForm', { required: true });
const ralphLoopForm = defineModel<RalphLoopFormState>('ralphLoopForm', { required: true });
const selectedProviderModelId = defineModel<string>('selectedProviderModelId', { default: '' });

withDefaults(
  defineProps<{
    agentId?: string;
    taxonomyTree?: PlatformResourceTreeNode[];
    promptModes?: { value: string; label: string; caption: string; tokens: string }[];
    statusOptions?: { label: string; value: string }[];
    filteredProviderModelOptions?: { label: string; value: string; caption?: string }[];
    loadingProviderModels?: boolean;
    orphanProviderModel?: boolean;
    disabledCatalogMatch?: boolean;
    checkingAgentModel?: boolean;
    agentModelCheckOk?: boolean | null;
    agentModelCheckMessage?: string;
  }>(),
  {
    agentId: '',
    taxonomyTree: () => [],
    promptModes: () => [],
    statusOptions: () => [],
    filteredProviderModelOptions: () => [],
    loadingProviderModels: false,
    orphanProviderModel: false,
    disabledCatalogMatch: false,
    checkingAgentModel: false,
    agentModelCheckOk: null,
    agentModelCheckMessage: '',
  },
);

const emit = defineEmits<{
  'copy-key': [];
  'open-permissions-tab': [];
  'open-memory-tab': [];
  'open-provider-manager': [];
  'filter-provider-models': [val: string, update: (fn: () => void) => void];
  'reset-provider-model-filter': [];
  'refine-error': [message: string];
}>();
</script>
