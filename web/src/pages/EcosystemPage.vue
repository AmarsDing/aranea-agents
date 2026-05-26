<template>
  <q-page class="app-standard-page">
    <q-banner rounded class="bg-info text-white q-mb-md">
      生态市场为<strong>技术预览</strong>：安装与发布流程尚未纳入核心 SLA，数据可能重置。
    </q-banner>
    <div class="row items-center q-mb-md">
      <div class="text-h5 text-weight-bold">生态市场</div>
      <q-space />
      <q-btn color="primary" unelevated rounded icon="add" label="发布" @click="publishOpen = true" />
      <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" class="q-ml-sm" @click="load" />
    </div>

    <q-input v-model="search" dense outlined clearable label="搜索" class="q-mb-md" @update:model-value="load" />

    <div class="row q-col-gutter-md">
      <div v-for="p in products" :key="p.id" class="col-12 col-md-6 col-lg-4">
        <q-card flat bordered>
          <q-card-section>
            <div class="text-subtitle1 text-weight-bold">{{ p.display_name || p.name }}</div>
            <div class="text-caption text-grey-7">{{ p.type }} · v{{ p.version }}</div>
            <div class="text-body2 q-mt-sm">{{ p.description || "暂无描述" }}</div>
            <div class="text-caption q-mt-sm">安装 {{ p.install_count }} 次</div>
          </q-card-section>
          <q-card-actions align="right">
            <q-btn
              v-if="!p.installed"
              flat
              color="primary"
              label="安装"
              :loading="installingId === p.id"
              @click="install(p)"
            />
            <q-chip v-else dense color="positive" text-color="white">已安装</q-chip>
          </q-card-actions>
        </q-card>
      </div>
    </div>

    <q-dialog v-model="publishOpen">
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section class="text-h6">发布产品</q-card-section>
        <q-card-section class="app-dialog-body q-gutter-sm q-pt-none">
          <q-input v-model="draft.name" class="app-field-md" dense outlined label="标识名" />
          <q-input v-model="draft.display_name" class="app-field-md" dense outlined label="显示名" />
          <q-input v-model="draft.description" class="app-field-long" dense outlined autogrow type="textarea" label="描述" />
          <q-select
            v-model="draft.type"
            dense
            outlined
            label="类型"
            :options="['skill_pack', 'agent_template', 'tool_bundle']"
          />
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat no-caps label="取消" v-close-popup />
          <q-btn color="primary" unelevated no-caps label="发布" :loading="publishing" @click="publish" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { useEcosystemPage } from "../features/ecosystem/useEcosystemPage";

const {
  products,
  search,
  loading,
  publishing,
  publishOpen,
  installingId,
  draft,
  load,
  install,
  publish,
} = useEcosystemPage();
</script>
