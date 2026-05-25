<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">Ralph Loop</div>
        <div class="text-caption text-grey-7">
          外层迭代验证：Agent 反复执行直到输出满足完成承诺，且/或验证命令退出码为 0。需配置最大迭代次数与停止条件之一。
        </div>
      </div>
      <q-toggle v-model="form.enabled" color="primary" label="启用" />
    </div>

    <q-banner v-if="!form.enabled" rounded dense class="settings-info-banner">
      关闭时 Runner 不包装 Ralph Loop；与「规划模式」独立。
    </q-banner>

    <div v-else class="app-form-field-grid q-mt-sm">
      <q-input
        v-model.number="form.max_iterations"
        dense
        outlined
        type="number"
        min="0"
        label="最大迭代次数"
        hint="0 表示仅依赖完成承诺/验证命令，不额外限制轮数上限（框架默认）"
      />
      <q-input
        v-model="form.completion_promise"
        class="app-field-long"
        dense
        outlined
        label="完成承诺 (completion_promise)"
        hint="Agent 输出包含该文本（默认包裹在 &lt;promise&gt; 标签内）时视为完成"
      />
      <q-input
        v-model="form.verify_command"
        class="app-field-long"
        dense
        outlined
        label="验证命令 (verify_command)"
        hint="每轮结束后执行的 shell 命令；退出码 0 才视为通过"
      />
      <q-input
        v-model="form.verify_work_dir"
        dense
        outlined
        label="验证命令工作目录"
        placeholder="留空使用进程 cwd"
      />
      <q-input
        v-model.number="form.verify_timeout_seconds"
        dense
        outlined
        type="number"
        min="0"
        suffix="秒"
        label="验证超时"
      />
      <q-input
        v-model="form.promise_tag_open"
        dense
        outlined
        label="承诺开始标签"
        placeholder="&lt;promise&gt;"
      />
      <q-input
        v-model="form.promise_tag_close"
        dense
        outlined
        label="承诺结束标签"
        placeholder="&lt;/promise&gt;"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import type { RalphLoopFormState } from "../../features/agents/ralphLoopConfig";

const form = defineModel<RalphLoopFormState>("form", { required: true });
</script>
