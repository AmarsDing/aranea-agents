import { ref } from 'vue';
import { useQuasar } from 'quasar';
import type { ArtifactMeta } from './types';
import { useArtifactStore } from '../../stores/artifact';
import { formatBytes } from '../../shared/format';

export function useArtifactList() {
  const $q = useQuasar();
  const artifactStore = useArtifactStore();
  const previewOpen = ref(false);
  const previewMeta = ref<ArtifactMeta | null>(null);
  const previewArtifactId = ref('');

  function mimeIcon(mime: string): string {
    if (!mime) return 'insert_drive_file';
    const m = mime.toLowerCase();
    if (m.startsWith('image/')) return 'image';
    if (m === 'application/pdf') return 'picture_as_pdf';
    if (
      m.startsWith('text/') ||
      m.includes('json') ||
      m.includes('xml') ||
      m.includes('javascript') ||
      m.includes('yaml')
    )
      return 'code';
    if (m.startsWith('video/')) return 'videocam';
    if (m.startsWith('audio/')) return 'audiotrack';
    if (m.includes('zip') || m.includes('tar') || m.includes('gzip') || m.includes('compressed')) return 'folder_zip';
    return 'insert_drive_file';
  }

  function openPreview(item: ArtifactMeta) {
    previewMeta.value = item;
    previewArtifactId.value = item.id;
    previewOpen.value = true;
  }

  async function downloadItem(item: ArtifactMeta) {
    try {
      const signed = await artifactStore.signDownload(item.id, item.version);
      window.open(artifactStore.artifactDownloadHref(signed.url), '_blank', 'noopener,noreferrer');
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '获取下载链接失败' });
    }
  }

  async function deleteItem(item: ArtifactMeta): Promise<boolean> {
    try {
      await artifactStore.remove(item.id);
      $q.notify({ type: 'positive', message: '已删除' });
      return true;
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '删除失败' });
      return false;
    }
  }

  return {
    previewOpen,
    previewMeta,
    previewArtifactId,
    mimeIcon,
    formatBytes,
    openPreview,
    downloadItem,
    deleteItem,
  };
}
