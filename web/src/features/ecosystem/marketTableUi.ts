// 商城 Registry 表格列定义：买家工作台 / 创作者中心的表格统一从这里取列（红线：禁止写在 Page script 内）。
// label 走 i18n（新文件硬编码中文会被 check-i18n 判违规），故以 t 工厂函数导出。
import type { QTableColumn } from 'quasar';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../ui/registryTableColumns';
import type { MyInstall, MyOrder, StudioAsset } from './types';

type Translate = (key: string) => string;

/** 买家工作台：已安装资产列 */
export function buildInstallColumns(t: Translate): QTableColumn<MyInstall>[] {
  return [
    registryCol<MyInstall>('name', t('shopPage.colAsset'), 'name', 'left', REGISTRY_COL_W.nameWide),
    registryCol<MyInstall>('version', t('shopPage.colVersion'), 'version', 'left', REGISTRY_COL_W.status),
    registryCol<MyInstall>('installedAt', t('shopPage.colInstalledAt'), 'installedAt', 'left', REGISTRY_COL_W.name),
    registryCol<MyInstall>('health7d', t('shopPage.colHealth'), 'health7d', 'left', REGISTRY_COL_W.actions),
    registryCol<MyInstall>('status', t('shopPage.colStatus'), 'status', 'left', REGISTRY_COL_W.status),
    registryColActions<MyInstall>(REGISTRY_COL_W.actionsWide, '', 'assetId'),
  ];
}

/** 买家工作台：订单列 */
export function buildOrderColumns(t: Translate): QTableColumn<MyOrder>[] {
  return [
    registryCol<MyOrder>('id', t('shopPage.colOrderId'), 'id', 'left', REGISTRY_COL_W.name),
    registryCol<MyOrder>('name', t('shopPage.colAsset'), 'name', 'left', REGISTRY_COL_W.nameWide),
    registryCol<MyOrder>('amountCents', t('shopPage.colAmount'), 'amountCents', 'left', REGISTRY_COL_W.metric),
    registryCol<MyOrder>('status', t('shopPage.colStatus'), 'status', 'left', REGISTRY_COL_W.status),
    registryCol<MyOrder>('createdAt', t('shopPage.colOrderDate'), 'createdAt', 'left', REGISTRY_COL_W.name),
    registryColActions<MyOrder>(REGISTRY_COL_W.actions, '', 'id'),
  ];
}

/** 创作者中心：我的资产列 */
export function buildStudioAssetColumns(t: Translate): QTableColumn<StudioAsset>[] {
  return [
    registryCol<StudioAsset>('name', t('shopPage.colAsset'), 'name', 'left', REGISTRY_COL_W.nameWide),
    registryCol<StudioAsset>(
      'reviewStatus',
      t('shopPage.colReviewStatus'),
      'reviewStatus',
      'left',
      REGISTRY_COL_W.status,
    ),
    registryCol<StudioAsset>('installs', t('shopPage.colInstalls'), 'installs', 'right', REGISTRY_COL_W.metric),
    registryCol<StudioAsset>('rating', t('shopPage.colRating'), 'rating', 'left', REGISTRY_COL_W.actions),
    registryCol<StudioAsset>('revenueCents', t('shopPage.colRevenue'), 'revenueCents', 'right', REGISTRY_COL_W.metric),
    registryCol<StudioAsset>('updatedAt', t('shopPage.colUpdatedAt'), 'updatedAt', 'left', REGISTRY_COL_W.name),
    registryColActions<StudioAsset>(REGISTRY_COL_W.actionsWide, '', 'id'),
  ];
}
