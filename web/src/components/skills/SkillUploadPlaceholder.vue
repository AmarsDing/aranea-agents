<template>
  <q-card flat bordered class="skill-upload-card">
    <q-card-section class="row items-center q-col-gutter-md">
      <div class="col">
        <div class="text-subtitle1 text-weight-medium">上传 Skill 与 AI 炼化</div>
        <div class="text-body2 text-grey-7 q-mt-xs">
          上传 zip 后会严格校验 SKILL.md，先检查名称重复，再通过已配置模型输出相似度指标并按冲突组炼化。
        </div>
      </div>
      <div class="col-auto row q-gutter-sm">
        <q-btn color="primary" rounded unelevated icon="upload_file" label="上传 Skill" @click="dialogOpen = true" />
      </div>
    </q-card-section>
  </q-card>

  <q-dialog v-model="dialogOpen" persistent>
    <q-card class="skill-import-dialog">
      <q-card-section class="row items-start justify-between q-gutter-md">
        <div>
          <div class="text-h6">上传 Skill</div>
          <div class="text-body2 text-grey-7">只接受 `.zip`，通过检查后无冲突 Skill 可直接导入；冲突组可在组内炼化。</div>
        </div>
        <q-btn flat round dense icon="close" :disable="busy" v-close-popup />
      </q-card-section>

      <q-separator />

      <q-card-section class="q-gutter-md">
        <q-file v-model="file" outlined dense accept=".zip" label="选择 Skill zip" :disable="busy">
          <template #prepend><q-icon name="upload_file" /></template>
        </q-file>
        <q-linear-progress v-if="busy" indeterminate rounded color="primary" />
        <q-banner v-if="error" rounded class="bg-negative text-white">{{ error }}</q-banner>

        <div v-if="job">
          <div class="row items-center q-gutter-sm q-mb-sm">
            <q-badge rounded :color="statusColor(job.validation_status)">{{ job.validation_status }}</q-badge>
            <span class="text-caption text-grey-7">存储根目录：{{ job.storage_root }}</span>
          </div>
          <q-banner v-if="job.validation_status === 'block'" rounded class="bg-negative text-white q-mb-md">
            {{ job.message || allBlockMessages }}
          </q-banner>

          <q-list bordered separator class="rounded-borders">
            <q-item v-for="candidate in job.candidates" :key="candidate.candidate_id">
              <q-item-section avatar>
                <q-icon :name="candidateIcon(candidate.validation_status)" :color="candidateColor(candidate.validation_status)" />
              </q-item-section>
              <q-item-section>
                <q-item-label>{{ candidate.name }}</q-item-label>
                <q-item-label caption>{{ candidate.description }}</q-item-label>
                <q-item-label caption>{{ candidate.slug }} -> {{ candidate.target_dir }}</q-item-label>
                <q-item-label v-if="candidate.blocks.length" caption class="text-negative">
                  {{ candidate.blocks.map((item) => item.message).join("；") }}
                </q-item-label>
                <div v-if="candidateRequiresRiskApproval(candidate)" class="q-mt-sm row q-gutter-sm">
                  <q-btn
                    dense
                    flat
                    rounded
                    color="negative"
                    icon="block"
                    label="不同意，设为上传失败"
                    :disable="busy"
                    @click="rejectRisk(candidate.candidate_id)"
                  />
                  <q-btn
                    dense
                    outline
                    rounded
                    color="warning"
                    icon="verified_user"
                    label="同意放行上传整个 Skill"
                    :disable="busy"
                    @click="approveRisk(candidate.candidate_id)"
                  />
                  <q-chip v-if="approvedRiskyCandidateIds.includes(candidate.candidate_id)" dense color="warning" text-color="white">已同意放行</q-chip>
                  <q-chip v-if="rejectedRiskyCandidateIds.includes(candidate.candidate_id)" dense color="negative" text-color="white">已设为失败</q-chip>
                </div>
                <q-item-label v-if="candidate.warnings.length" caption class="text-warning">
                  {{ candidate.warnings.map((item) => item.message).join("；") }}
                </q-item-label>
              </q-item-section>
            </q-item>
          </q-list>

          <div v-if="job.conflict_groups.length" class="q-mt-md q-gutter-md">
            <q-card v-for="group in job.conflict_groups" :key="group.group_id" flat bordered class="conflict-card skill-ux-subcard">
              <q-card-section>
                <div class="row items-start justify-between q-gutter-md">
                  <div>
                    <div class="text-subtitle1 text-weight-medium">冲突组：{{ percent(group.highest_similarity_score) }} 相似</div>
                    <div class="text-body2 text-grey-7">{{ group.reason }}</div>
                  </div>
                  <q-btn color="primary" rounded unelevated icon="auto_fix_high" label="炼化" :loading="refiningGroupId === group.group_id" :disable="!group.can_refine || busy" @click="refineGroup(group.group_id)" />
                </div>
                <div class="metrics-grid q-mt-md">
                  <div v-for="metric in metricItems(group.metrics)" :key="metric.label" class="metric-pill">
                    <span>{{ metric.label }}</span>
                    <strong>{{ metric.value }}</strong>
                  </div>
                </div>
                <div class="text-caption text-grey-7 q-mt-sm">证据：{{ group.evidence.join("；") }}</div>
                <div class="q-mt-sm">
                  <q-chip v-for="skill in group.existing_skills" :key="skill.id" dense outline color="primary">
                    已有：{{ skill.name }} {{ skill.version }}
                  </q-chip>
                </div>
              </q-card-section>
            </q-card>
          </div>

          <q-card v-if="refineResult" flat bordered class="skill-ux-subcard q-mt-md">
            <q-card-section>
              <div class="text-subtitle1 text-weight-medium">炼化预览：{{ refineResult.merged_name }}</div>
              <div class="text-body2 text-grey-7 q-mt-xs">{{ refineResult.merged_description }}</div>
              <q-input v-model="refineResult.merged_body" type="textarea" autogrow outlined class="q-mt-md" />
            </q-card-section>
          </q-card>
        </div>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat rounded label="关闭" :disable="busy" v-close-popup />
        <q-btn outline rounded color="primary" label="开始上传检查" :disable="!file || busy" :loading="uploading" @click="startUpload" />
        <q-btn color="primary" rounded unelevated label="应用导入" :disable="!canApply || busy" :loading="applying" @click="applyImportResult" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useQuasar } from "quasar";
import type {
  SkillConflictGroup,
  SkillImportApplyResult,
  SkillImportDecision,
  SkillImportJob,
  SkillRefineResult,
  SkillSimilarityMetrics
} from "../../features/skills/types";

const props = defineProps<{
  /** 遗留 `/api/v1/skills/import*`；由 Page 绑定 `features/skills/api` */
  uploadSkillZip: (file: File) => Promise<{ job_id: string }>;
  getSkillImportJob: (jobId: string) => Promise<SkillImportJob>;
  refineSkillConflictGroup: (
    jobId: string,
    groupId: string,
    payload: { provider?: string; model?: string; instructions?: string }
  ) => Promise<SkillRefineResult>;
  applySkillImport: (jobId: string, decisions: SkillImportDecision[]) => Promise<SkillImportApplyResult>;
}>();

const emit = defineEmits<{
  completed: [];
}>();

const $q = useQuasar();
const dialogOpen = ref(false);
const file = ref<File | null>(null);
const job = ref<SkillImportJob | null>(null);
const error = ref("");
const uploading = ref(false);
const applying = ref(false);
const refiningGroupId = ref("");
const refineResult = ref<SkillRefineResult | null>(null);
const approvedRiskyCandidateIds = ref<string[]>([]);
const rejectedRiskyCandidateIds = ref<string[]>([]);

const busy = computed(() => uploading.value || applying.value || refiningGroupId.value !== "");
const canApply = computed(() => {
  if (!job.value) return false;
  const hasPass = job.value.candidates.some((candidate) => candidate.validation_status === "pass");
  return hasPass || !!refineResult.value || approvedRiskyCandidateIds.value.length > 0 || rejectedRiskyCandidateIds.value.length > 0;
});
const allBlockMessages = computed(() => {
  if (!job.value) return "上传检查被阻塞";
  const messages = job.value.candidates.flatMap((candidate) => candidate.blocks.map((item) => item.message)).filter(Boolean);
  return messages.length ? Array.from(new Set(messages)).join("；") : "上传检查被阻塞";
});

async function startUpload() {
  if (!file.value) return;
  error.value = "";
  uploading.value = true;
  refineResult.value = null;
  try {
    const created = await props.uploadSkillZip(file.value);
    job.value = await props.getSkillImportJob(created.job_id);
    approvedRiskyCandidateIds.value = [];
    rejectedRiskyCandidateIds.value = [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : "上传检查失败";
  } finally {
    uploading.value = false;
  }
}

async function refineGroup(groupId: string) {
  if (!job.value) return;
  error.value = "";
  refiningGroupId.value = groupId;
  try {
    refineResult.value = await props.refineSkillConflictGroup(job.value.job_id, groupId, {});
    $q.notify({ type: "positive", message: "炼化预览已生成" });
  } catch (err) {
    error.value = err instanceof Error ? err.message : "炼化失败";
  } finally {
    refiningGroupId.value = "";
  }
}

async function applyImportResult() {
  if (!job.value) return;
  const decisions: SkillImportDecision[] = job.value.candidates
    .filter((candidate) => candidate.validation_status === "pass")
    .map((candidate) => ({ candidate_id: candidate.candidate_id, action: "import_passed" }));
  decisions.push(...approvedRiskyCandidateIds.value.map((candidateId) => ({ candidate_id: candidateId, action: "approve_risky_import" as const })));
  decisions.push(...rejectedRiskyCandidateIds.value.map((candidateId) => ({ candidate_id: candidateId, action: "reject_risky_upload" as const })));
  if (refineResult.value) {
    decisions.push({
      group_id: firstRefinedGroup(job.value.conflict_groups, refineResult.value.source_candidate_ids),
      action: "merge_group_with_ai",
      merged_name: refineResult.value.merged_name,
      merged_description: refineResult.value.merged_description,
      merged_body: refineResult.value.merged_body,
      merged_tags: refineResult.value.merged_tags
    });
  }
  applying.value = true;
  error.value = "";
  try {
    await props.applySkillImport(job.value.job_id, decisions);
    $q.notify({ type: "positive", message: "Skill 导入完成" });
    dialogOpen.value = false;
    emit("completed");
  } catch (err) {
    error.value = err instanceof Error ? err.message : "导入失败";
  } finally {
    applying.value = false;
  }
}

function statusColor(status: string) {
  return status === "pass" ? "positive" : status === "warn" ? "warning" : "negative";
}

function candidateIcon(status: string) {
  return status === "pass" ? "check_circle" : status === "warn" ? "merge_type" : "error";
}

function candidateColor(status: string) {
  return status === "pass" ? "positive" : status === "warn" ? "warning" : "negative";
}

function candidateRequiresRiskApproval(candidate: SkillImportJob["candidates"][number]) {
  return candidate.validation_status === "block" && candidate.blocks.length > 0 && candidate.blocks.every((item) => item.type === "high_risk_file");
}

function approveRisk(candidateId: string) {
  approvedRiskyCandidateIds.value = Array.from(new Set([...approvedRiskyCandidateIds.value, candidateId]));
  rejectedRiskyCandidateIds.value = rejectedRiskyCandidateIds.value.filter((id) => id !== candidateId);
}

function rejectRisk(candidateId: string) {
  rejectedRiskyCandidateIds.value = Array.from(new Set([...rejectedRiskyCandidateIds.value, candidateId]));
  approvedRiskyCandidateIds.value = approvedRiskyCandidateIds.value.filter((id) => id !== candidateId);
}

function percent(value: number) {
  return `${Math.round(value * 100)}%`;
}

function metricItems(metrics: SkillSimilarityMetrics) {
  return [
    { label: "总相似", value: percent(metrics.similarity_score) },
    { label: "名称", value: percent(metrics.name_similarity) },
    { label: "简介", value: percent(metrics.description_similarity) },
    { label: "正文", value: percent(metrics.body_similarity) },
    { label: "触发", value: percent(metrics.trigger_similarity) },
    { label: "工具", value: percent(metrics.tool_similarity) },
    { label: "风险", value: metrics.conflict_risk },
    { label: "置信", value: percent(metrics.confidence) }
  ];
}

function firstRefinedGroup(groups: SkillConflictGroup[], candidateIds: string[]) {
  return groups.find((group) => group.candidate_ids.some((id) => candidateIds.includes(id)))?.group_id ?? groups[0]?.group_id ?? "";
}
</script>

<style scoped lang="sass">
// 卡片玻璃与 token 见 app-global.sass（.skill-upload-card / .metric-pill）；此处仅布局与对话框尺寸（UX.md §5.2a）
.skill-import-dialog
  width: min(920px, 94vw)
  max-height: 92vh
  border-radius: 24px !important

.conflict-card
  border-radius: 18px !important

.metrics-grid
  display: grid
  grid-template-columns: repeat(auto-fit, minmax(108px, 1fr))
  gap: 8px

.metric-pill span
  display: block
  color: var(--color-text-secondary)
  font-size: 11px

.metric-pill strong
  font-size: 14px
</style>
