<template>
  <!-- U0-3：OS 文件拖拽全页遮罩。teleport 到 body 脱离 q-page 层叠上下文；
       pointer-events:none 纯视觉——数据路径归页面 window drop 监听（同一入队入口）。 -->
  <teleport to="body">
    <div v-if="active" class="upload-drop-overlay" aria-hidden="true">
      <div class="upload-drop-overlay__panel">
        <q-icon name="cloud_upload" size="48px" color="primary" />
        <div class="upload-drop-overlay__title">{{ t('knowledgePage.dropOverlayTitle') }}</div>
        <div class="upload-drop-overlay__hint">{{ t('knowledgePage.dropOverlayHint') }}</div>
      </div>
    </div>
  </teleport>
</template>

<script setup lang="ts">
// U0-3：dragenter 计数 / dragleave / drop 显示语义（计数器防闪烁，嵌套子元素进出不抖动）。
import { onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { isOsFileDrag } from '../../features/knowledge/dragFiles';

const { t } = useI18n();
const active = ref(false);
let enterCount = 0;

function onDragEnter(e: DragEvent) {
  if (!isOsFileDrag(e.dataTransfer)) return;
  e.preventDefault(); // 允许 drop 语义；数据收集与入库归页面 window drop 监听
  enterCount += 1;
  active.value = true;
}

function onDragLeave(e: DragEvent) {
  if (!isOsFileDrag(e.dataTransfer)) return;
  enterCount = Math.max(0, enterCount - 1);
  // relatedTarget 为空 = 拖出窗口边界，直接复位兜底（部分浏览器跨窗不再补 dragleave）。
  if (enterCount === 0 || !e.relatedTarget) {
    enterCount = 0;
    active.value = false;
  }
}

function onWindowDrop() {
  // 只复位显示；files 抽取/入队/目录跳过提示全部由 KnowledgePage 处理。
  enterCount = 0;
  active.value = false;
}

onMounted(() => {
  window.addEventListener('dragenter', onDragEnter);
  window.addEventListener('dragleave', onDragLeave);
  window.addEventListener('drop', onWindowDrop);
});

onUnmounted(() => {
  window.removeEventListener('dragenter', onDragEnter);
  window.removeEventListener('dragleave', onDragLeave);
  window.removeEventListener('drop', onWindowDrop);
});
</script>

<style scoped>
.upload-drop-overlay {
  position: fixed;
  inset: 0;
  z-index: 9599; /* 高于 Quasar 对话框（~7000），拖文件到本页任意位置都有反馈 */
  pointer-events: none;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--glass-backdrop, rgba(10, 14, 28, 0.45));
}

.upload-drop-overlay__panel {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 36px 56px;
  border: 2px dashed var(--q-primary);
  border-radius: 20px;
  background: var(--glass-surface);
}

.upload-drop-overlay__title {
  font-size: 18px;
  font-weight: 600;
  color: var(--q-primary);
}

.upload-drop-overlay__hint {
  font-size: 12.5px;
  opacity: 0.72;
}
</style>
