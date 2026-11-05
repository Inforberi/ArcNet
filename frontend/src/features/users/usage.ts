export type UserPackage = {
 user_id: number; quota_bytes: number; upload_bytes: number; download_bytes: number;
 expires_at: number; period: number; period_started_at: number; version: number; observed_at: number; block_reason: string; incomplete: boolean; needs_review: boolean;
};
export type UsageReply = {items: UserPackage[]; sampled_at: number; source_fresh: boolean; source_last_success: number; complete_coverage: boolean; runtime_state: string};
export type Debit = {at: number; period: number; upload_bytes: number; download_bytes: number};
export const usageGB = (bytes: number) => `${(bytes / 1e9).toLocaleString('ru', {maximumFractionDigits: 3})} ГБ`;
export function quotaBytes(value: string): number | undefined {
 if (!/^\d+(?:[.,]\d{1,3})?$/.test(value.trim())) return;
 const bytes = Math.round(Number(value.replace(',', '.')) * 1e9);
 return Number.isSafeInteger(bytes) && bytes > 0 && bytes <= 9e15 ? bytes : undefined;
}
export const blockLabel: Record<string, string> = {quota_exhausted: 'Пакет исчерпан', expired: 'Срок истёк', accounting_unavailable: 'Нет свежего учёта — доступ ограничен', accounting_gap: 'Разрыв учёта после перезапуска — нужен новый период'};
