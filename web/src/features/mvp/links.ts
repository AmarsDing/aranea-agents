export function packageFileURL(jobId: string, name: 'manifest.json' | 'body.md') {
  return `/api/v1/mvp/publish/jobs/${encodeURIComponent(jobId)}/files/${name}`;
}
