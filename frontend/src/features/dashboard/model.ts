export type ResourceSnapshot = {
 sampled_at: number; epoch: string; cpus: number; cpu_percent: number | null;
 memory_total: number | null; memory_available: number | null; swap_total: number | null; swap_free: number | null;
 disk_total: number | null; disk_free: number | null; load_1: number | null; uptime_seconds: number | null;
 rx_bytes: number | null; tx_bytes: number | null; rx_per_second: number | null; tx_per_second: number | null;
 tcp_sockets: number | null; udp_sockets: number | null;
};
export type Observed = {received_at: number; snapshot: ResourceSnapshot};
export type FleetItem = {
 id: string; name: string; address: string; agent: string; xray: string; vpn: string; last_seen: number;
 stale: boolean; legacy: boolean; cpu_percent: number | null; memory_percent: number | null;
};
export type ServerDetail = {telemetry_fresh_for: number; sampled_at: number; server: Omit<FleetItem, 'cpu_percent' | 'memory_percent'> & {version: string; vpn_checked_at: number; route: string; telemetry: Observed | null}; history: Observed[]; history_available: boolean; vpn_traffic_verified: boolean};
export type FleetOverview = {users: number; enabled_users: number; nodes: number; servers: FleetItem[]; sampled_at: number};
export const percentUsed = (total: number | null | undefined, free: number | null | undefined) => total != null && total > 0 && free != null && free >= 0 && free <= total ? 100 * (total - free) / total : null;
export const formatBytes = (value: number | null | undefined) => {
 if (value == null || !Number.isFinite(value) || value < 0) return 'Нет данных';
 const units = ['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ']; let index = 0;
 while (value >= 1024 && index < units.length - 1) {value /= 1024; index++;}
 return `${value.toLocaleString('ru', {maximumFractionDigits: index > 1 ? 1 : 0})} ${units[index]}`;
};
export const formatPercent = (value: number | null | undefined) => value == null || !Number.isFinite(value) ? 'Нет данных' : `${value.toLocaleString('ru', {maximumFractionDigits: 1})}%`;
export const statusText: Record<string, string> = {local: 'На этом сервере', online: 'На связи', offline: 'Нет связи', active: 'Работает', inactive: 'Остановлен', failed: 'Ошибка', activating: 'Запускается', deactivating: 'Останавливается', disabled: 'Выключен', unknown: 'Нет проверки', 'not-configured': 'Не настроен', unverified: 'Не проверен', pass: 'Последняя проба успешна', fail: 'Проба не прошла', stale: 'Нужна новая проверка', 'not-verified': 'Не проверен'};
export const needsAttention = (server: FleetItem) => server.agent === 'offline' || ['failed', 'inactive'].includes(server.xray) || server.vpn === 'fail' || server.stale;
export const ageText = (timestamp: number | null | undefined, now: number) => !timestamp ? 'Не получены' : timestamp > now + 5 ? 'Часы не совпадают' : now - timestamp < 60 ? `${Math.max(0, now - timestamp)} с назад` : `${Math.floor((now - timestamp) / 60)} мин назад`;
export const pointX = (history: Observed[], index: number) => {
 const start = history[0]?.received_at ?? 0; const end = history.at(-1)?.received_at ?? start;
 return end > start ? 600 * (history[index].received_at - start) / (end - start) : 0;
};
export const rateText = (value: number | null | undefined) => value == null ? 'Нет данных' : `${formatBytes(value)}/с`;
export const chartPath = (history: Observed[], field: 'rx_per_second' | 'tx_per_second', maximum: number) => {
 let drawing = false; let previous = 0;
 return history.map((point, index) => {
  const value = point.snapshot[field];
  if (value == null || value < 0 || !Number.isFinite(value)) {drawing = false; return '';}
  if (previous && (point.received_at - previous > 120 || point.snapshot.epoch !== history[index - 1]?.snapshot.epoch)) drawing = false;
  const command = drawing ? 'L' : 'M'; drawing = true; previous = point.received_at;
  return `${command}${pointX(history, index).toFixed(1)},${(140 - 130 * Math.min(value / Math.max(1, maximum), 1)).toFixed(1)}`;
 }).join(' ');
};
