<template>
  <!-- 任务详情 › 资料：你给的（资料袋）+ 交出来的（按执行先后：会话产物 / 编码工具的摘要）。设计：07-会话中心.md §12.5 -->
  <div class="cc-files">
    <div class="cc-files__in">
      <p class="cc-files__sect">{{ t('cc.files.gave') }}</p>
      <ul v-if="inputs.length" class="cc-mats">
        <li v-for="a in inputs" :key="a.id" class="cc-mat cc-mat--row">
          <span class="cc-mat__ic">{{ fileExt(a.name) }}</span>
          <span class="cc-mat__n" :title="a.name">{{ a.name }}</span>
          <span class="cc-mat__s cc-num">{{ fmtSize(a.size) }}</span>
          <span class="cc-mat__acts">
            <q-btn flat dense no-caps :label="t('cc.files.open')" @click="store.openArtifactPreview(a.id)" />
            <q-btn flat dense no-caps :label="t('cc.files.download')" @click="download(a.id)" />
          </span>
        </li>
      </ul>
      <p v-else-if="!loading" class="cc-files__note">
        {{ t(task.recordId ? 'cc.files.noInputs' : 'cc.files.noRecord') }}
      </p>

      <p class="cc-files__sect cc-files__sect--gap">{{ t('cc.files.made') }}</p>
      <template v-for="g in groups" :key="g.exec.id">
        <div class="cc-og">
          <div class="cc-og__h">
            <CcGlyph :executor="g.exec.executor" :name="g.exec.executorName" size="xs" />
            <b>{{ g.exec.executorName }}</b
            ><span>{{ g.exec.what }} · {{ t(`cc.status.${g.exec.status}`) }}</span>
          </div>
          <p v-if="g.say" class="cc-og__say" :class="{ 'cc-og__say--fail': g.exec.status === 'failed' }">
            {{ g.say }}
          </p>
          <ul v-if="g.files.length" class="cc-mats">
            <li v-for="a in g.files" :key="a.id" class="cc-mat cc-mat--row">
              <span class="cc-mat__ic">{{ fileExt(a.name) }}</span>
              <span class="cc-mat__n" :title="a.name">{{ a.name }}</span>
              <span class="cc-mat__s cc-num">{{ fmtSize(a.size) }}</span>
              <span class="cc-mat__acts">
                <q-btn flat dense no-caps :label="t('cc.files.open')" @click="store.openArtifactPreview(a.id)" />
                <q-btn flat dense no-caps :label="t('cc.files.download')" @click="download(a.id)" />
              </span>
            </li>
          </ul>
          <p v-if="g.exec.kind !== 'session'" class="cc-files__note cc-files__note--sm">
            {{ t(g.exec.kind === 'bridge' ? 'cc.files.bridgeChanges' : 'cc.files.terminalChanges') }}
          </p>
        </div>
      </template>
      <p v-if="!groups.length && !loading" class="cc-files__note">{{ t('cc.files.noOutputs') }}</p>
      <p v-if="failed" class="cc-files__note" role="status">{{ t('cc.files.partial') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import CcGlyph from '../../../components/conversations/CcGlyph.vue';
import type { ArtifactMeta } from '../../artifact/types';
import { useArtifactStore } from '../../../stores/artifact';
import { fileExt, fmtSize } from '../handoff';
import { flat, type ConvTask } from '../model';
import type { ExecOutputs } from '../useTaskFiles';

const props = defineProps<{
  task: ConvTask;
  inputs: ArtifactMeta[];
  outputs: ExecOutputs[];
  loading: boolean;
  failed: boolean;
}>();
const { t } = useI18n();
const store = useArtifactStore();

// 每次执行一组：会话的产物；编码工具 / 终端只有一句结论（改动在项目目录里，P2 再列文件）
const groups = computed(() =>
  flat(props.task)
    .map((exec) => ({
      exec,
      files: props.outputs.find((o) => o.exec.id === exec.id)?.files ?? [],
      say: exec.kind === 'session' ? '' : exec.detail,
    }))
    .filter((g) => g.files.length || g.say || g.exec.kind !== 'session'),
);

async function download(id: string) {
  try {
    await store.download({ id });
  } catch {
    Notify.create({ type: 'negative', message: t('uiSystem.operations.downloadFailed') });
  }
}
</script>
