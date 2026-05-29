import { ref } from "vue";
import type { ArtifactMeta } from "./types";
import { useArtifactStore } from "../../stores/artifact";
import { formatBytes } from "../../shared/format";

export function useArtifactList() {
  const artifactStore = useArtifactStore();
  const previewOpen = ref(false);
  const previewMeta = ref<ArtifactMeta | null>(null);
  const previewArtifactId = ref("");

  function mimeIcon(mime: string): string {
    if (!mime) return "insert_drive_file";
    const m = mime.toLowerCase();
    if (m.startsWith("image/")) return "image";
    if (m === "application/pdf") return "picture_as_pdf";
    if (m.startsWith("text/") || m.includes("json") || m.includes("xml") || m.includes("javascript") || m.includes("yaml")) return "code";
    if (m.startsWith("video/")) return "videocam";
    if (m.startsWith("audio/")) return "audiotrack";
    if (m.includes("zip") || m.includes("tar") || m.includes("gzip") || m.includes("compressed")) return "folder_zip";
    return "insert_drive_file";
  }

  function openPreview(item: ArtifactMeta, onOpen?: (id: string) => void) {
    previewMeta.value = item;
    previewArtifactId.value = item.id;
    previewOpen.value = true;
    onOpen?.(item.id);
  }

  async function downloadItem(item: ArtifactMeta) {
    try {
      const signed = await artifactStore.signDownload(item.id, item.version);
      window.open(artifactStore.artifactDownloadHref(signed.url), "_blank", "noopener,noreferrer");
    } catch {
      // silent — user can retry
    }
  }

  return {
    previewOpen,
    previewMeta,
    previewArtifactId,
    mimeIcon,
    formatBytes,
    openPreview,
    downloadItem
  };
}
