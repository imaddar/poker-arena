export function formatArchiveTableId(id: string): string {
  const digits = id.replace(/\D/g, '');
  if (!digits) {
    return 'TX_000';
  }

  return `TX_${digits.padStart(3, '0')}`;
}
