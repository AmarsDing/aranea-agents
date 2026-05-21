<template>
  <div class="settings-grid">
    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">平台 Skill 挂载策略</span>
        </div>
        <div class="text-caption text-grey-7">
          控制本会话中 ADK 可见的已发布 Skill：Agent 白名单/黑名单、必选标签，以及是否根据用户话术做意图收窄（详见仓库文档「20 skill struct design」十三′）。
        </div>
        <div class="row q-gutter-sm">
          <q-btn outline rounded dense color="primary" label="刷新 Skill 列表" :loading="loadingSkillSlugs" @click="$emit('load-skill-slugs')" />
          <q-btn outline rounded dense color="primary" label="恢复默认" @click="$emit('reset-skill-defaults')" />
        </div>
      </div>
      <q-banner rounded class="q-mb-md settings-info-banner">
        留空「允许的 slug」表示不按 slug 白名单过滤；「意图收窄」开启后，仅对与话术匹配的 taxonomy / 关键词相关的 Skill 并集挂载（可减少无关工具）。运行时只会挂载<strong>已发布且已启用</strong>的平台 Skill；草稿仅便于在此勾选 slug，需先到 Skill 管理发布并启用。
      </q-banner>
      <div class="row q-col-gutter-md">
        <div class="col-12 col-lg-6">
          <q-card flat bordered class="capability-card">
            <q-card-section class="row items-center justify-between">
              <div>
                <div class="text-subtitle2">意图收窄（层 B）</div>
                <div class="text-caption text-grey-7">根据用户消息关键词匹配内置意图路径，缩小 Skill 候选集。</div>
              </div>
              <q-toggle v-model="config.skillRuntime.intent_routing_enabled" color="primary" />
            </q-card-section>
            <q-separator />
            <q-card-section class="app-form-field-grid app-form-field-grid--2col">
              <q-input
                v-model.number="config.skillRuntime.intent_max_paths"
                dense
                outlined
                type="number"
                label="最多意图路径数"
                :min="1"
                :max="32"
              />
              <q-input
                v-model.number="config.skillRuntime.max_skills_in_toolset"
                dense
                outlined
                type="number"
                label="工具集内 Skill 上限"
                :min="1"
                :max="256"
              />
            </q-card-section>
          </q-card>
        </div>
        <div class="col-12 col-lg-6">
          <q-card flat bordered class="capability-card">
            <q-card-section>
              <div class="text-subtitle2 q-mb-xs">slug 与标签（层 A）</div>
              <div class="text-caption text-grey-7 q-mb-sm">
                标签 token 写入 Skill 元数据的标签名（如 file_type:xlsx），多项为「同时满足」。允许与拒绝同一 slug 互斥：在一侧添加会从另一侧去掉同名项；若历史配置两侧重叠，载入/保存时会按运行时规则以<strong>拒绝优先</strong>规整。
              </div>
              <q-select
                v-model="config.skillRuntime.allowed_slugs"
                class="q-mb-sm"
                dense
                outlined
                multiple
                use-chips
                use-input
                new-value-mode="add-unique"
                input-debounce="0"
                :options="skillSlugOptions"
                option-label="label"
                option-value="value"
                emit-value
                map-options
                label="允许的 Skill slug（skill_key）"
                hint="从平台 Skill 勾选或手动输入；留空 = 不启用 slug 白名单。与「拒绝」互斥：此处勾选会从拒绝列表移除同名 slug。"
              />
              <q-select
                v-model="config.skillRuntime.denied_slugs"
                class="q-mb-sm"
                dense
                outlined
                multiple
                use-chips
                use-input
                new-value-mode="add-unique"
                input-debounce="0"
                :options="skillSlugOptions"
                option-label="label"
                option-value="value"
                emit-value
                map-options
                label="拒绝的 Skill slug"
                hint="与「允许」互斥：此处勾选会从允许列表移除同名 slug。"
              />
              <q-select
                v-model="config.skillRuntime.allowed_tags"
                dense
                outlined
                multiple
                use-chips
                use-input
                new-value-mode="add-unique"
                input-debounce="0"
                label="要求的标签（AND）"
                hint="可与用户话术中的 domain:/file_type: 提示合并"
              />
            </q-card-section>
          </q-card>
        </div>
      </div>
      <q-card flat bordered class="capability-card q-mt-md">
        <q-card-section>
          <div class="text-subtitle2 q-mb-xs">代码执行后端</div>
          <div class="text-caption text-grey-7 q-mb-sm">
            Skill 运行时 <code>skill_run</code> 使用的沙箱类型。生产环境建议使用 <strong>docker</strong>；local 无隔离，仅适合开发。
          </div>
          <q-select
            v-model="config.code_executor_type"
            class="app-field-md"
            dense
            outlined
            emit-value
            map-options
            :options="codeExecutorOptions"
            label="执行器类型"
          />
          <q-banner v-if="fallbackHint" rounded dense class="q-mt-sm settings-info-banner">
            {{ fallbackHint }}
          </q-banner>
        </q-card-section>
      </q-card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { CodeExecutorCapability } from "../../features/monitor/api";

const baseExecutorOptions = [
  { label: "Local（子进程，开发用）", value: "local" },
  { label: "Docker（容器隔离，推荐生产）", value: "docker" },
  { label: "E2B（云端沙箱，需 E2B_API_KEY）", value: "e2b" },
  { label: "Container（框架引擎，需 build tag）", value: "container" }
];

const props = defineProps<{
  config: Record<string, unknown> & { code_executor_type?: string };
  skillSlugOptions: { label: string; value: string }[];
  loadingSkillSlugs: boolean;
  codeExecutorCapabilities?: CodeExecutorCapability[];
}>();

defineEmits<{
  "load-skill-slugs": [];
  "reset-skill-defaults": [];
}>();

const capabilityByType = computed(() => {
  const map = new Map<string, CodeExecutorCapability>();
  for (const c of props.codeExecutorCapabilities ?? []) {
    map.set(c.type, c);
  }
  return map;
});

const codeExecutorOptions = computed(() =>
  baseExecutorOptions.map((opt) => {
    const cap = capabilityByType.value.get(opt.value);
    const unavailable = cap && !cap.available;
    return {
      ...opt,
      disable: unavailable,
      label: unavailable && cap?.reason ? `${opt.label}（不可用：${cap.reason}）` : opt.label
    };
  })
);

const fallbackHint = computed(() => {
  const selected = String(props.config.code_executor_type ?? "local");
  const cap = capabilityByType.value.get(selected);
  if (!cap || cap.available) {
    return "";
  }
  const reason = cap.reason ? `（${cap.reason}）` : "";
  return `当前选择「${selected}」在本环境不可用${reason}，运行时将自动回退到 local 执行器。`;
});
</script>

<style scoped>
/* 通用样式由 agent-settings-page.scss 控制；仅保留组件特有 */
.settings-info-banner {
  background: var(--glass-elevated);
  color: var(--color-text-secondary);
}
</style>
