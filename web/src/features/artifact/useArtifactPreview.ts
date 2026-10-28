import { computed, getCurrentScope, onScopeDispose, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ArtifactMeta, ArtifactPreview } from './types';
import { useArtifactStore } from '../../stores/artifact';
import { formatBytes } from '../../shared/format';

export function useArtifactPreview(
  artifactId: () => string,
  version?: () => number | undefined,
  expectedSessionId?: () => string | undefined,
) {
  const artifactStore = useArtifactStore();
  const { t } = useI18n();
  const preview = ref<ArtifactPreview | null>(null);
  const loading = ref(false);
  const error = ref('');
  // Signed URL (with inline=1) for <audio>/<video> playback. Empty for other
  // preview kinds or when signing fails (header download button still works).
  const inlineMediaSrc = ref('');
  const mediaError = ref(false);
  let generation = 0;

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
    const requestedVersion = version?.();
    const sessionId = expectedSessionId?.();
    const request = ++generation;
    preview.value = null;
    inlineMediaSrc.value = '';
    error.value = '';
    mediaError.value = false;
    loading.value = Boolean(id);
    if (!id) {
      return;
    }
    try {
      const result = await artifactStore.loadPreview(id, requestedVersion);
      if (request !== generation) return;
      if (sessionId && result.meta.session_id !== sessionId) {
        throw new Error(t('taskWorkspace.scopeError'));
      }
      if (sessionId && requestedVersion != null && result.meta.version !== requestedVersion) {
        throw new Error(t('taskWorkspace.versionError'));
      }
      preview.value = result;
      if (result.preview_kind === 'audio' || result.preview_kind === 'video') {
        try {
          const signed = await artifactStore.signDownload(id, requestedVersion);
          if (request !== generation) return;
          if (!signed.url) throw new Error('EMPTY_SIGNED_URL');
          const sep = signed.url.includes('?') ? '&' : '?';
          inlineMediaSrc.value = artifactStore.artifactDownloadHref(`${signed.url}${sep}inline=1`);
        } catch {
          if (request === generation) mediaError.value = true;
        }
      }
    } catch (e) {
      if (request !== generation) return;
      error.value = e instanceof Error ? e.message : t('artifact.preview.loadFailed');
      preview.value = null;
    } finally {
      if (request === generation) loading.value = false;
    }
  }

  watch(() => [artifactId(), version?.(), expectedSessionId?.()] as const, loadPreview, {
    immediate: true,
    flush: 'sync',
  });
  if (getCurrentScope())
    onScopeDispose(() => {
      generation++;
      preview.value = null;
      inlineMediaSrc.value = '';
    });

  return { preview, loading, error, mediaError, kindIcon, imageSrc, pdfSrc, inlineMediaSrc, formatBytes, loadPreview };
}

export type { ArtifactMeta };
