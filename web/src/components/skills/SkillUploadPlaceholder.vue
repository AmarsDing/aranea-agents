<template>
  <AppDialog
    v-model="dialogOpen"
    :title="t('domains.skillUpload.title')"
    :subtitle="t('domains.skillUpload.subtitle')"
    persistent
    :loading="busy"
  >
    <div class="q-gutter-md">
      <q-file v-model="file" outlined dense accept=".zip" :label="t('domains.skillUpload.pickZip')" :disable="busy">
        <template #prepend><q-icon name="upload_file" /></template>
      </q-file>
      <q-linear-progress v-if="busy" indeterminate rounded color="primary" />
      <q-banner v-if="error" rounded class="bg-negative text-white">{{ error }}</q-banner>

      <div v-if="job">
        <div class="row items-center q-gutter-sm q-mb-sm">
          <AppStatusChip :status="job.validation_status" />
          <span class="text-caption text-grey-7">{{
            t('domains.skillUpload.storageRoot', { path: job.storage_root })
          }}</span>
        </div>
        <q-banner v-if="job.validation_status === 'block'" rounded class="bg-negative text-white q-mb-md">
          {{ job.message || allBlockMessages }}
        </q-banner>

        <q-list bordered separator class="rounded-borders">
          <q-item v-for="candidate in job.candidates" :key="candidate.candidate_id">
            <q-item-section avatar>
              <q-icon
                :name="candidateIcon(candidate.validation_status)"
                :color="candidateColor(candidate.validation_status)"
              />
            </q-item-section>
            <q-item-section>
              <q-item-label>{{ candidate.name }}</q-item-label>
              <q-item-label caption>{{ candidate.description }}</q-item-label>
              <q-item-label caption>{{ candidate.slug }} -> {{ candidate.target_dir }}</q-item-label>
              <q-item-label v-if="candidate.blocks.length" caption class="text-negative">
                {{ candidate.blocks.map((item) => item.message).join(t('domains.skillUpload.listSeparator')) }}
              </q-item-label>
              <div v-if="candidateRequiresRiskApproval(candidate)" class="q-mt-sm row q-gutter-sm">
                <q-btn
                  dense
                  flat
                  rounded
                  color="negative"
                  icon="block"
                  :label="t('domains.skillUpload.rejectRisk')"
                  :disable="busy"
                  @click="rejectRisk(candidate.candidate_id)"
                />
                <q-btn
                  dense
                  outline
                  rounded
                  color="warning"
                  icon="verified_user"
                  :label="t('domains.skillUpload.approveRisk')"
                  :disable="busy"
                  @click="approveRisk(candidate.candidate_id)"
                />
                <q-chip
                  v-if="approvedRiskyCandidateIds.includes(candidate.candidate_id)"
                  dense
                  color="warning"
                  text-color="white"
                  >{{ t('domains.skillUpload.approved') }}</q-chip
                >
                <q-chip
                  v-if="rejectedRiskyCandidateIds.includes(candidate.candidate_id)"
                  dense
                  color="negative"
                  text-color="white"
                  >{{ t('domains.skillUpload.rejected') }}</q-chip
                >
              </div>
              <q-item-label v-if="candidate.warnings.length" caption class="text-warning">
                {{ candidate.warnings.map((item) => item.message).join(t('domains.skillUpload.listSeparator')) }}
              </q-item-label>
            </q-item-section>
          </q-item>
        </q-list>

        <div v-if="job.conflict_groups.length" class="q-mt-md q-gutter-md">
          <q-card
            v-for="group in job.conflict_groups"
            :key="group.group_id"
            flat
            bordered
            class="conflict-card skill-ux-subcard"
          >
            <q-card-section>
              <div class="row items-start justify-between q-gutter-md">
                <div>
                  <div class="text-subtitle1 text-weight-medium">
                    {{ t('domains.skillUpload.conflictGroup', { percent: percent(group.highest_similarity_score) }) }}
                  </div>
                  <div class="text-body2 text-grey-7">{{ group.reason }}</div>
                </div>
                <div class="column q-gutter-sm items-end">
                  <q-btn
                    color="primary"
                    rounded
                    unelevated
                    no-caps
                    icon="auto_fix_high"
                    :label="t('domains.skillUpload.refine')"
                    :loading="refiningGroupId === group.group_id"
                    :disable="!group.can_refine || busy"
                    @click="refineGroup(group.group_id)"
                  />
                  <q-btn
                    v-if="isKeepSeparateRecommended(group)"
                    outline
                    rounded
                    no-caps
                    color="positive"
                    icon="call_split"
                    :label="
                      isKeptSeparate(group) ? t('skillImport.keepSeparateSelected') : t('skillImport.keepSeparate')
                    "
                    :disable="busy || isKeptSeparate(group)"
                    @click="keepSeparate(group)"
                  />
                </div>
              </div>
              <div class="metrics-grid q-mt-md">
                <div v-for="metric in metricItems(group.metrics)" :key="metric.label" class="metric-pill">
                  <span>{{ metric.label }}</span>
                  <strong>{{ metric.value }}</strong>
                </div>
              </div>
              <div class="text-caption text-grey-7 q-mt-sm">
                {{
                  t('domains.skillUpload.evidence', {
                    text: group.evidence.join(t('domains.skillUpload.listSeparator')),
                  })
                }}
              </div>
              <div class="q-mt-sm">
                <q-chip v-for="skill in group.existing_skills" :key="skill.id" dense outline color="primary">
                  {{ t('domains.skillUpload.existing', { name: skill.name, version: skill.version }) }}
                </q-chip>
              </div>
            </q-card-section>
          </q-card>
        </div>

        <q-card v-if="refineResult" flat bordered class="skill-ux-subcard q-mt-md">
          <q-card-section>
            <div class="text-subtitle1 text-weight-medium">
              {{ t('domains.skillUpload.refinePreview', { name: refineResult.merged_name }) }}
            </div>
            <div class="text-body2 text-grey-7 q-mt-xs">{{ refineResult.merged_description }}</div>
            <q-input v-model="refineResult.merged_body" type="textarea" autogrow outlined class="q-mt-md" />
          </q-card-section>
        </q-card>
      </div>
    </div>
    <template #actions>
      <q-btn
        flat
        rounded
        no-caps
        :label="t('domains.skillUpload.close')"
        :disable="busy"
        @click="dialogOpen = false"
      />
      <q-btn
        outline
        rounded
        no-caps
        color="primary"
        :label="t('domains.skillUpload.startCheck')"
        :disable="!file || busy"
        :loading="uploading"
        @click="startUpload"
      />
      <q-btn
        color="primary"
        rounded
        unelevated
        no-caps
        :label="t('domains.skillUpload.apply')"
        :disable="!canApply || busy"
        :loading="applying"
        @click="applyImportResult"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type {
  SkillConflictGroup,
  SkillImportApplyResult,
  SkillImportDecision,
  SkillImportJob,
  SkillRefineResult,
  SkillSimilarityMetrics,
} from '../../features/skills/types';
import AppStatusChip from '../common/AppStatusChip.vue';
import AppDialog from '../common/AppDialog.vue';
import { skillCandidateIcon as candidateIcon, skillCandidateStatusColor as candidateColor } from './skillTableUi';
import { buildImportDecisions } from '../../features/skills/importDecisions';

const { t } = useI18n();

const props = defineProps<{
  /** `kratosApi` **`/v1/skills/import*`**（**`cmd/admin`**）；由 Page 绑定 `features/skills/api` */
  uploadSkillZip: (file: File) => Promise<{ job_id: string }>;
  getSkillImportJob: (jobId: string) => Promise<SkillImportJob>;
  refineSkillConflictGroup: (
    jobId: string,
    groupId: string,
    payload: { provider?: string; model?: string; instructions?: string },
  ) => Promise<SkillRefineResult>;
  applySkillImport: (jobId: string, decisions: SkillImportDecision[]) => Promise<SkillImportApplyResult>;
  /** 由 composable 注入通知函数，展示层不直接 useQuasar */
  notify: (opts: { type: string; message: string }) => void;
}>();

const emit = defineEmits<{
  completed: [];
}>();
const dialogOpen = ref(false);
const file = ref<File | null>(null);
const job = ref<SkillImportJob | null>(null);
const error = ref('');
const uploading = ref(false);
const applying = ref(false);
const refiningGroupId = ref('');
const refineResult = ref<SkillRefineResult | null>(null);
const approvedRiskyCandidateIds = ref<string[]>([]);
const rejectedRiskyCandidateIds = ref<string[]>([]);
const keptSeparateCandidateIds = ref<string[]>([]);

const busy = computed(() => uploading.value || applying.value || refiningGroupId.value !== '');
const canApply = computed(() => {
  if (!job.value) return false;
  const hasPass = job.value.candidates.some((candidate) => candidate.validation_status === 'pass');
  const hasPositiveAction =
    hasPass ||
    !!refineResult.value ||
    approvedRiskyCandidateIds.value.length > 0 ||
    keptSeparateCandidateIds.value.length > 0;
  // Pure-reject (no actual import) should not enable "apply".
  return hasPositiveAction;
});
const allBlockMessages = computed(() => {
  const blocked = t('domains.skillUpload.blocked');
  if (!job.value) return blocked;
  const messages = job.value.candidates
    .flatMap((candidate) => candidate.blocks.map((item) => item.message))
    .filter(Boolean);
  return messages.length ? Array.from(new Set(messages)).join(t('domains.skillUpload.listSeparator')) : blocked;
});

function openDialog() {
  dialogOpen.value = true;
}

function resetState() {
  file.value = null;
  job.value = null;
  error.value = '';
  uploading.value = false;
  applying.value = false;
  refiningGroupId.value = '';
  refineResult.value = null;
  approvedRiskyCandidateIds.value = [];
  rejectedRiskyCandidateIds.value = [];
  keptSeparateCandidateIds.value = [];
}

watch(dialogOpen, (val) => {
  if (!val) {
    resetState();
  }
});

defineExpose({ openDialog });

async function startUpload() {
  if (!file.value) return;
  error.value = '';
  uploading.value = true;
  refineResult.value = null;
  try {
    const created = await props.uploadSkillZip(file.value);
    job.value = await props.getSkillImportJob(created.job_id);
    approvedRiskyCandidateIds.value = [];
    rejectedRiskyCandidateIds.value = [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : t('domains.skillUpload.uploadFailed');
  } finally {
    uploading.value = false;
  }
}

async function refineGroup(groupId: string) {
  if (!job.value) return;
  error.value = '';
  refiningGroupId.value = groupId;
  try {
    refineResult.value = await props.refineSkillConflictGroup(job.value.job_id, groupId, {});
    props.notify({ type: 'positive', message: t('domains.skillUpload.refineDone') });
  } catch (err) {
    error.value = err instanceof Error ? err.message : t('domains.skillUpload.refineFailed');
  } finally {
    refiningGroupId.value = '';
  }
}

async function applyImportResult() {
  if (!job.value) return;
  const decisions = buildImportDecisions(
    job.value,
    refineResult.value,
    approvedRiskyCandidateIds.value,
    rejectedRiskyCandidateIds.value,
    keptSeparateCandidateIds.value,
  );
  applying.value = true;
  error.value = '';
  try {
    await props.applySkillImport(job.value.job_id, decisions);
    props.notify({ type: 'positive', message: t('domains.skillUpload.importDone') });
    dialogOpen.value = false;
    emit('completed');
  } catch (err) {
    error.value = err instanceof Error ? err.message : t('domains.skillUpload.importFailed');
  } finally {
    applying.value = false;
  }
}

function candidateRequiresRiskApproval(candidate: SkillImportJob['candidates'][number]) {
  return (
    candidate.validation_status === 'block' &&
    candidate.blocks.length > 0 &&
    candidate.blocks.every((item) => item.type === 'high_risk_file')
  );
}

function approveRisk(candidateId: string) {
  approvedRiskyCandidateIds.value = Array.from(new Set([...approvedRiskyCandidateIds.value, candidateId]));
  rejectedRiskyCandidateIds.value = rejectedRiskyCandidateIds.value.filter((id) => id !== candidateId);
}

function rejectRisk(candidateId: string) {
  rejectedRiskyCandidateIds.value = Array.from(new Set([...rejectedRiskyCandidateIds.value, candidateId]));
  approvedRiskyCandidateIds.value = approvedRiskyCandidateIds.value.filter((id) => id !== candidateId);
}

function isKeepSeparateRecommended(group: SkillConflictGroup) {
  return group.metrics.recommendation === 'keep_separate';
}

function isKeptSeparate(group: SkillConflictGroup) {
  return group.candidate_ids.some((id) => keptSeparateCandidateIds.value.includes(id));
}

function keepSeparate(group: SkillConflictGroup) {
  keptSeparateCandidateIds.value = Array.from(new Set([...keptSeparateCandidateIds.value, ...group.candidate_ids]));
}

function percent(value: number) {
  return `${Math.round(value * 100)}%`;
}

function metricItems(metrics: SkillSimilarityMetrics) {
  return [
    { label: t('domains.skillUpload.metricSimilarity'), value: percent(metrics.similarity_score) },
    { label: t('domains.skillUpload.metricName'), value: percent(metrics.name_similarity) },
    { label: t('domains.skillUpload.metricDescription'), value: percent(metrics.description_similarity) },
    { label: t('domains.skillUpload.metricBody'), value: percent(metrics.body_similarity) },
    { label: t('domains.skillUpload.metricTrigger'), value: percent(metrics.trigger_similarity) },
    { label: t('domains.skillUpload.metricTool'), value: percent(metrics.tool_similarity) },
    { label: t('domains.skillUpload.metricRisk'), value: metrics.conflict_risk },
    { label: t('domains.skillUpload.metricConfidence'), value: percent(metrics.confidence) },
  ];
}
</script>
