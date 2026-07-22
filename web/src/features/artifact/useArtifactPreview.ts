import { computed, ref, watch } from 'vue';
import type { ArtifactMeta, ArtifactPreview } from './types';
import { useArtifactStore } from '../../stores/artifact';
import { formatBytes } from '../../shared/format';

export function useArtifactPreview(artifactId: () => string, version?: () => number | undefined) {
  const artifactStore = useArtifactStore();
  const preview = ref<ArtifactPreview | null>(null);
  const loading = ref(false);
  const error = ref('');
  // Signed URL (with inline=1) for <audio>/<video> playback. Empty for other
  // preview kinds or when signing fails (header download button still works).
  const inlineMediaSrc = ref('');

  const kindIcon = computed(() => {
    if (!preview.value) return 'insert_drive_file';
    const kind = preview.value.preview_kind;
    if (kind === 'text') return 'code';
    if (kind === 'image') return 'image';
    if (kind === 'pdf') return 'picture_as_pdf';
    if (kind === 'audio') return 'audiotrack';
    if (kind === 'video') return 'movie';
    return 'insert_drive_file';
  });

  const imageSrc = computed(() => {
    if (!preview.value || !preview.value.data_base64) return '';
    return `data:${preview.value.meta.mime_type};base64,${preview.value.data_base64}`;
  });

  const pdfSrc = computed(() => {
    if (!preview.value || !preview.value.data_base64) return '';
    return `data:application/pdf;base64,${preview.value.data_base64}`;
  });

  async function loadPreview() {
    const id = artifactId();
    if (!id) {
      preview.value = null;
      inlineMediaSrc.value = '';
      return;
    }
    loading.value = true;
    error.value = '';
    try {
      preview.value = await artifactStore.loadPreview(id, version?.());
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载预览失败';
      preview.value = null;
    } finally {
      loading.value = false;
    }
    inlineMediaSrc.value = await signInlineMedia(id, preview.value);
  }

  async function signInlineMedia(id: string, p: ArtifactPreview | null): Promise<string> {
    if (!p || (p.preview_kind !== 'audio' && p.preview_kind !== 'video')) return '';
    try {
      const signed = await artifactStore.signDownload(id, version?.());
      if (!signed.url) return '';
      const sep = signed.url.includes('?') ? '&' : '?';
      return artifactStore.artifactDownloadHref(`${signed.url}${sep}inline=1`);
    } catch {
      return '';
    }
  }

  watch(() => [artifactId(), version?.()] as const, loadPreview, { immediate: true });

  return { preview, loading, error, kindIcon, imageSrc, pdfSrc, inlineMediaSrc, formatBytes, loadPreview };
}

export type { ArtifactMeta };
