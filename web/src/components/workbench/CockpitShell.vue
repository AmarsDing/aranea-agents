<template>
  <div class="wb-scope wb-shell" :class="{ 'wb-scope--retreat': retreat }">
    <slot name="topbar" />
    <div v-if="offline" class="wb-shell__offline">{{ t('workbench.offlineBanner') }}</div>
    <div class="wb-shell__body">
      <div class="wb-shell__side"><slot name="sidebar" /></div>
      <main class="wb-shell__main wb-main"><slot name="main" /></main>
      <div class="wb-shell__you"><slot name="you" /></div>
    </div>
    <slot name="statusbar" />
    <slot name="floats" />
  </div>
</template>

<script setup lang="ts">
/**
 * 作战总台薄壳（结构移植自 KnowledgeWorkbench 四件套先例，KB 组件零 import 防域泄漏）。
 * 五带：TopBar / 主体三栏（花名册 | 工作区 | YOU 金门）/ 状态栏；浮动层走 floats slot。
 */
import { useI18n } from 'vue-i18n';

defineProps<{
  /** 抽屉/浮层时主区退后一步（长桥抽屉先例） */
  retreat: boolean;
  /** WS 断连整屏灰横幅（验收红线 4） */
  offline: boolean;
}>();

const { t } = useI18n();
</script>

<style scoped lang="sass">
.wb-shell
  overflow: hidden
.wb-shell__offline
  flex: none
  padding: 6px 12px
  // 设计 §4.1：顶部灰横条（红脉冲由 TrustIndicator 豁免件承担），与移动端灰条同口径
  background: color-mix(in srgb, var(--color-text-primary) 8%, transparent)
  color: var(--color-text-secondary)
  font-size: var(--wb-caption)
  text-align: center
  letter-spacing: 0.06em
.wb-shell__body
  flex: 1
  display: flex
  min-height: 0
.wb-shell__side
  flex: none
  width: 18%
  min-width: 188px
  max-width: 280px
.wb-shell__main
  flex: 1
  min-width: 0
  min-height: 0
  position: relative
  display: flex
  flex-direction: column
.wb-shell__you
  flex: none
  width: 22%
  min-width: 280px
  max-width: 380px
</style>
