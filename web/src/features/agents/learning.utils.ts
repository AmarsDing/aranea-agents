export function formatDate(iso: string): string {
  if (!iso) return '';
  try {
    return new Date(iso).toLocaleDateString();
  } catch {
    return iso;
  }
}
