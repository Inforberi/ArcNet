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
export {formatBytes,rateText,pointX,chartPath} from '../../shared/traffic-chart.ts';
export const formatPercent = (value: number | null | undefined) => value == null || !Number.isFinite(value) ? 'Нет данных' : `${value.toLocaleString('ru', {maximumFractionDigits: 1})}%`;
export const statusText: Record<string, string> = {local: 'На этом сервере', online: 'На связи', offline: 'Нет связи', active: 'Работает', inactive: 'Остановлен', failed: 'Ошибка', activating: 'Запускается', deactivating: 'Останавливается', disabled: 'Выключен', unknown: 'Нет проверки', 'not-configured': 'Не настроен', unverified: 'Не проверен', pass: 'Последняя проба успешна', fail: 'Проба не прошла', stale: 'Нужна новая проверка', 'not-verified': 'Не проверен'};
export const needsAttention = (server: FleetItem) => server.agent === 'offline' || ['failed', 'inactive'].includes(server.xray) || server.vpn === 'fail' || server.stale;
export const ageText = (timestamp: number | null | undefined, now: number) => !timestamp ? 'Не получены' : timestamp > now + 5 ? 'Часы не совпадают' : now - timestamp < 60 ? `${Math.max(0, now - timestamp)} с назад` : `${Math.floor((now - timestamp) / 60)} мин назад`;
