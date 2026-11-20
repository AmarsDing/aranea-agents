<template>
  <form class="org-form" @submit.prevent="emit('save')">
    <dl class="registry-facts">
      <div>
        <dt>{{ t('orgChart.inspector.level') }}</dt>
        <dd>{{ t(`orgChart.level.${level}`) }}</dd>
      </div>
      <div v-if="parent">
        <dt>{{ t('orgChart.inspector.parent') }}</dt>
        <dd>{{ parent.name }}</dd>
      </div>
      <div v-if="nodeKey">
        <dt>{{ t('orgChart.inspector.key') }}</dt>
        <dd class="org-mono">{{ nodeKey }}</dd>
      </div>
    </dl>
    <q-input
      v-model="form.name"
      dense
      outlined
      autofocus
      maxlength="120"
      :label="t('orgChart.inspector.name')"
      :rules="[(v: string) => !!v.trim() || t('orgChart.nameRequired')]"
      lazy-rules
    />
    <div>
      <q-input
        v-model="form.description"
        dense
        outlined
        type="textarea"
        autogrow
        :label="descriptionLabel(levelNum)"
        :placeholder="descriptionPlaceholder(levelNum)"
        input-class="org-form__textarea"
      />
      <div class="org-form__refine">
        <AiRefineButton
          :scope="levelScope(levelNum)"
          :resource-id="nodeId || undefined"
          :text="form.description"
          :refine-fn="refineFn"
          flat
          size="sm"
          :label="t('orgChart.inspector.refine')"
          @apply="(v: string) => (form.description = v)"
          @error="(msg: string) => emit('refine-error', msg)"
        />
      </div>
    </div>
    <div>
      <q-toggle
        v-model="form.enabled"
        color="primary"
        :label="t('orgChart.inspector.enabledLabel')"
        :disable="system"
      />
      <p class="org-note">
        {{ t(system ? 'organizationPage.systemNodeToggleDisabled' : 'orgChart.inspector.enabledHint') }}
      </p>
    </div>
    <div class="org-detail__actions">
      <q-btn
        type="submit"
        unelevated
        no-caps
        color="primary"
        :loading="saving"
        :label="t(nodeId ? 'orgChart.inspector.save' : 'orgChart.inspector.create')"
      />
      <q-btn flat no-caps :label="t('orgChart.cancel')" @click="emit('cancel')" />
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AiRefineButton from '../agents/AIRefineButton.vue';
import {
  descriptionLabel,
  descriptionPlaceholder,
  levelScope,
  parseLevelNumber,
} from '../../features/platform/taxonomyLabels';
import type { RefineRequest, RefineResponse } from '../../features/agents/aiRefine';
import type { OrgNode } from '../../features/organization/orgChart';
import type { OrgNodeForm } from '../../features/organization/useOrgChart';

const props = defineProps<{
  level: string;
  parent: OrgNode | null;
  nodeId: string;
  nodeKey: string;
  system: boolean;
  saving: boolean;
  refineFn: (req: RefineRequest) => Promise<RefineResponse>;
}>();
const form = defineModel<OrgNodeForm>('form', { required: true });
const emit = defineEmits<{ save: []; cancel: []; 'refine-error': [message: string] }>();
const { t } = useI18n();
const levelNum = computed(() => parseLevelNumber(props.level));
</script>
