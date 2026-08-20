import { defineStore } from 'pinia';
import {
  getAvatarThumbnailDataUrl,
  listAvatarAssets,
  refreshChannelPlatformIcons,
  uploadAvatarAsset,
} from '../../features/avatar/api';
import type { AvatarAsset } from '../../features/avatar/types';
import { isAvatarAssetRef } from '../../features/avatar/iconModel';
import { onSessionMutation } from '../sessionMutationBus';

/** 头像域：目录缓存 + 缩略图缓存 + 上传；对外业务均由 actions 经 api 层访问后端 */
export const useAvatarCatalogStore = defineStore('avatarCatalog', {
  state: () => ({
    agentsCatalog: [] as AvatarAsset[],
    agentsCatalogLoaded: false,
    pickerSystem: [] as AvatarAsset[],
    pickerMine: [] as AvatarAsset[],
    pickerLoaded: false,
    /** assetId → data URL，空字符串表示已请求过但无图 */
    thumbnailById: {} as Record<string, string>,
  }),
  actions: {
    async ensureAgentsCatalog() {
      if (this.agentsCatalogLoaded) return;
      this.agentsCatalog = await listAvatarAssets();
      this.agentsCatalogLoaded = true;
    },
    async ensurePickerAssets(force = false) {
      if (this.pickerLoaded && !force) return;
      const [system, mine] = await Promise.all([listAvatarAssets('system'), listAvatarAssets('mine')]);
      this.pickerSystem = system;
      this.pickerMine = mine;
      this.pickerLoaded = true;
    },
    /** 丢弃缩略图缓存项（用于路由切换后强制重拉，或清理错误写入的空串缓存） */
    forgetThumbnail(rawId: string) {
      const trimmed = String(rawId || '').trim();
      if (!trimmed) return;
      if (!Object.prototype.hasOwnProperty.call(this.thumbnailById, trimmed)) return;
      const next = { ...this.thumbnailById };
      delete next[trimmed];
      this.thumbnailById = next;
    },
    /** 拉取并写入缩略图缓存（幂等：已有 key 则跳过，含空串表示已请求过） */
    async ensureThumbnail(rawId: string) {
      const trimmed = String(rawId || '').trim();
      if (!trimmed || /^(https?:|data:|blob:)/i.test(trimmed)) return;
      if (!isAvatarAssetRef(trimmed)) return;
      if (Object.prototype.hasOwnProperty.call(this.thumbnailById, trimmed)) return;
      const dataUrl = await getAvatarThumbnailDataUrl(trimmed);
      this.thumbnailById = { ...this.thumbnailById, [trimmed]: dataUrl };
    },
    mergeUploaded(asset: AvatarAsset) {
      this.pickerMine = [asset, ...this.pickerMine.filter((a) => a.id !== asset.id)];
      if (!this.agentsCatalog.some((a) => a.id === asset.id)) {
        this.agentsCatalog = [asset, ...this.agentsCatalog];
      }
    },
    async uploadAvatarFromFile(file: File): Promise<AvatarAsset> {
      const asset = await uploadAvatarAsset(file);
      this.mergeUploaded(asset);
      this.forgetThumbnail(asset.id);
      await this.ensureThumbnail(asset.id);
      return asset;
    },
    invalidateAll() {
      this.agentsCatalogLoaded = false;
      this.pickerLoaded = false;
      this.agentsCatalog = [];
      this.pickerSystem = [];
      this.pickerMine = [];
      this.thumbnailById = {};
    },
    /** 从 Iconify API 重新获取渠道平台图标，刷新后清除缓存 */
    async refreshChannelIcons(): Promise<{ updated: number; failed: number }> {
      const result = await refreshChannelPlatformIcons();
      this.invalidateAll();
      return result;
    },
  },
});

onSessionMutation((mutation) => {
  if (mutation.type === 'agents_dependencies_loaded') {
    const store = useAvatarCatalogStore();
    store.ensureAgentsCatalog();
  }
});
