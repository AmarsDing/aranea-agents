<template>
  <q-page class="app-page-cream q-pa-lg">
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
      <q-card style="min-width: 360px">
        <q-card-section class="text-h6">发布产品</q-card-section>
        <q-card-section class="q-gutter-sm">
          <q-input v-model="draft.name" dense outlined label="标识名" />
          <q-input v-model="draft.display_name" dense outlined label="显示名" />
          <q-input v-model="draft.type" dense outlined label="类型" hint="skill_pack / agent_template" />
          <q-input v-model="draft.description" dense outlined type="textarea" label="描述" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="取消" v-close-popup />
          <q-btn color="primary" unelevated label="发布" :loading="publishing" @click="publish" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { useQuasar } from "quasar";
import { installEcosystemProduct, listEcosystemProducts, publishEcosystemProduct, type EcosystemProduct } from "../features/ecosystem/api";

const $q = useQuasar();
const products = ref<EcosystemProduct[]>([]);
const search = ref("");
const loading = ref(false);
const publishing = ref(false);
const publishOpen = ref(false);
const installingId = ref("");
const draft = reactive({ name: "", display_name: "", description: "", type: "skill_pack" });

async function load() {
  loading.value = true;
  try {
    products.value = await listEcosystemProducts(search.value.trim());
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载失败" });
  } finally {
    loading.value = false;
  }
}

async function install(p: EcosystemProduct) {
  installingId.value = p.id;
  try {
    await installEcosystemProduct(p.id);
    $q.notify({ type: "positive", message: "安装成功" });
    await load();
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : "安装失败" });
  } finally {
    installingId.value = "";
  }
}

async function publish() {
  publishing.value = true;
  try {
    await publishEcosystemProduct({ ...draft });
    publishOpen.value = false;
    $q.notify({ type: "positive", message: "已发布" });
    await load();
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : "发布失败" });
  } finally {
    publishing.value = false;
  }
}

onMounted(() => void load());
</script>
