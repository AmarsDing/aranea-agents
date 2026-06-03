/** Artifact upload limits — keep in sync with internal/biz/artifact/limits.go */
export const ARTIFACT_MAX_BYTES = 10 * 1024 * 1024;
export const ARTIFACT_MAX_LABEL = '10 MB';

/** Returns a user-facing error message when file exceeds the limit, or null if ok. */
export function validateArtifactFileSize(bytes: number): string | null {
  if (bytes > ARTIFACT_MAX_BYTES) {
    return `单个文件最大支持 ${ARTIFACT_MAX_LABEL}，当前文件过大暂不支持上传`;
  }
  return null;
}

export function artifactMaxSizeHint(): string {
  return `单文件最大 ${ARTIFACT_MAX_LABEL}`;
}
