// REV-D: mirrors backend extractSupportedMimes — keep in sync with
// internal/service/knowledge.go:extractSupportedMimes.
export const EXTRACT_SUPPORTED_MIMES = new Set([
  'text/plain',
  'text/markdown',
  'text/csv',
  'text/html',
  'text/xml',
  'application/json',
  'application/xml',
  'application/pdf',
  'application/msword',
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  'image/png',
  'image/jpeg',
  'image/webp',
]);

export function isExtractSupported(mimeType: string): boolean {
  const m = mimeType.trim().toLowerCase();
  return EXTRACT_SUPPORTED_MIMES.has(m) || m.startsWith('text/');
}

export function inferUploadMime(file: File): string {
  if (file.type && file.type.trim()) return file.type;
  const lower = file.name.toLowerCase();
  if (lower.endsWith('.md')) return 'text/markdown';
  if (lower.endsWith('.txt') || lower.endsWith('.log')) return 'text/plain';
  if (lower.endsWith('.json')) return 'application/json';
  if (lower.endsWith('.csv')) return 'text/csv';
  if (lower.endsWith('.html') || lower.endsWith('.htm')) return 'text/html';
  if (lower.endsWith('.xml')) return 'application/xml';
  if (lower.endsWith('.yaml') || lower.endsWith('.yml')) return 'text/yaml';
  if (lower.endsWith('.toml')) return 'text/toml';
  if (lower.endsWith('.pdf')) return 'application/pdf';
  if (lower.endsWith('.docx')) return 'application/vnd.openxmlformats-officedocument.wordprocessingml.document';
  if (lower.endsWith('.doc')) return 'application/msword';
  if (lower.endsWith('.pptx')) return 'application/vnd.openxmlformats-officedocument.presentationml.presentation';
  if (lower.endsWith('.xlsx')) return 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';
  if (lower.endsWith('.png')) return 'image/png';
  if (lower.endsWith('.jpg') || lower.endsWith('.jpeg')) return 'image/jpeg';
  if (lower.endsWith('.webp')) return 'image/webp';
  return 'application/octet-stream';
}

export function bytesToBase64(bytes: Uint8Array): string {
  const CHUNK = 0x8000;
  let binary = '';
  for (let i = 0; i < bytes.length; i += CHUNK) {
    const slice = bytes.subarray(i, Math.min(i + CHUNK, bytes.length));
    binary += String.fromCharCode.apply(null, slice as unknown as number[]);
  }
  return btoa(binary);
}

export function utf8ToBase64(str: string): string {
  return bytesToBase64(new TextEncoder().encode(str));
}

export function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const buf = reader.result as ArrayBuffer | null;
      if (!buf) {
        reject(new Error('read failed'));
        return;
      }
      resolve(bytesToBase64(new Uint8Array(buf)));
    };
    reader.onerror = () => reject(new Error('read failed'));
    reader.readAsArrayBuffer(file);
  });
}
