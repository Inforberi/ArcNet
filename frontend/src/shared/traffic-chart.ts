export type RateObservation={received_at:number;snapshot:{epoch:string;rx_per_second:number|null;tx_per_second:number|null}};
export const formatBytes = (value: number | null | undefined) => {
 if (value == null || !Number.isFinite(value) || value < 0) return 'Нет данных';
 const units = ['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ']; let index = 0;
 while (value >= 1024 && index < units.length - 1) {value /= 1024; index++;}
 return `${value.toLocaleString('ru', {maximumFractionDigits: index > 1 ? 1 : 0})} ${units[index]}`;
};
export const pointX = (history: RateObservation[], index: number) => {
 const start = history[0]?.received_at ?? 0; const end = history.at(-1)?.received_at ?? start;
 return end > start ? 600 * (history[index].received_at - start) / (end - start) : 0;
};
export const rateText = (value: number | null | undefined) => value == null ? 'Нет данных' : `${formatBytes(value)}/с`;
export const chartPath = (history: RateObservation[], field: 'rx_per_second' | 'tx_per_second', maximum: number) => {
 let drawing = false; let previous = 0;
 return history.map((point, index) => {
  const value = point.snapshot[field];
  if (value == null || value < 0 || !Number.isFinite(value)) {drawing = false; return '';}
  if (previous && (point.received_at - previous > 120 || point.snapshot.epoch !== history[index - 1]?.snapshot.epoch)) drawing = false;
  const command = drawing ? 'L' : 'M'; drawing = true; previous = point.received_at;
  return `${command}${pointX(history, index).toFixed(1)},${(140 - 130 * Math.min(value / Math.max(1, maximum), 1)).toFixed(1)}`;
 }).join(' ');
};
