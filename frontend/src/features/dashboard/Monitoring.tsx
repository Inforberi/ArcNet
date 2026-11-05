import {createContext, useContext, useEffect, useState, type ReactNode} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {api} from '../../shared/api';
import {Button, Notice} from '../../shared/ui';

type Settings = {interval_seconds: number; available_seconds: number[]};
type Connection = 'connecting' | 'live' | 'reconnecting' | 'paused';
const MonitoringContext = createContext<{connection: Connection; seconds: number}>({connection: 'connecting', seconds: 5});
const liveKeys = new Set(['overview', 'server-telemetry', 'nodes', 'node-bootstraps', 'protocol-health', 'events', 'users', 'user-usage', 'user-debits', 'user-devices']);

// One subscription for the authenticated layout; only mounted queries refetch.
// A disconnected stream falls back to bounded polling at the same cadence.
export function MonitoringProvider({children}: {children: ReactNode}) {
 const cache = useQueryClient();
 const settings = useQuery({queryKey: ['monitoring-settings'], queryFn: () => api<Settings>('/api/settings/monitoring')});
 const seconds = settings.data?.interval_seconds ?? 5;
 const [connection, setConnection] = useState<Connection>('connecting');
 useEffect(() => {
  let source: EventSource | undefined;
  let live = false;
  let lastActivity = Date.now();
  let lastHistory = 0;
  const refresh = () => {
   if (document.hidden) return;
   void cache.invalidateQueries({predicate: query => liveKeys.has(String(query.queryKey[0]))}, {cancelRefetch: false});
   if (Date.now() - lastHistory >= 60000) {
    lastHistory = Date.now();
    void cache.invalidateQueries({queryKey: ['server-history']}, {cancelRefetch: false});
   }
  };
  const open = () => {
   source?.close(); source = undefined; live = false;
   if (document.hidden) {setConnection('paused'); return;}
   if (!navigator.onLine) {setConnection('reconnecting'); return;}
   setConnection('connecting');
   if (typeof EventSource === 'undefined') {setConnection('reconnecting'); return;}
   source = new EventSource('/api/monitoring/events');
   source.addEventListener('refresh', event => {
    try {
     const message = JSON.parse((event as MessageEvent<string>).data) as {interval_seconds: number};
     if (![5, 10, 15, 30, 60].includes(message.interval_seconds)) return;
     cache.setQueryData<Settings>(['monitoring-settings'], old => ({available_seconds: old?.available_seconds ?? [5,10,15,30,60], interval_seconds: message.interval_seconds}));
     lastActivity = Date.now();
     live = true; setConnection('live'); refresh();
    } catch {live = false; setConnection('reconnecting');}
   });
   source.addEventListener('keepalive', () => {lastActivity = Date.now();});
   source.addEventListener('unauthorized', () => {
    source?.close(); live = false;
    window.dispatchEvent(new Event('arcvpn:unauthorized'));
   });
   source.onerror = () => {live = false; setConnection('reconnecting'); refresh();};
  };
  const offline = () => {source?.close(); source = undefined; live = false; setConnection('reconnecting');};
  open();
  const timer = window.setInterval(() => {if (!live) refresh();}, seconds * 1000);
  const watchdog = window.setInterval(() => {
   if (live && !document.hidden && Date.now() - lastActivity > Math.max(20000, seconds * 2000)) open();
  }, 1000);
  document.addEventListener('visibilitychange', open);
  window.addEventListener('offline', offline);
  window.addEventListener('online', open);
  return () => {source?.close(); window.clearInterval(timer); window.clearInterval(watchdog); document.removeEventListener('visibilitychange', open); window.removeEventListener('offline', offline); window.removeEventListener('online', open);};
 }, [cache, seconds]);
 return <MonitoringContext.Provider value={{connection, seconds}}>{children}</MonitoringContext.Provider>;
}

export function MonitoringControls() {
 const {connection, seconds} = useContext(MonitoringContext);
 const cache = useQueryClient();
 const [choice, setChoice] = useState<number | null>(null);
 const save = useMutation({
  mutationFn: (interval: number) => api<Settings>('/api/settings/monitoring', 'PUT', {interval_seconds: interval}),
  onSuccess: data => {cache.setQueryData(['monitoring-settings'], data); setChoice(null);},
 });
 const label = {connecting: 'Подключаем обновления', live: 'Обновляется автоматически', reconnecting: 'Восстанавливаем подписку · временно опрашиваем сервер', paused: 'Обновления приостановлены в скрытой вкладке'}[connection];
 return <div className="monitoring-controls">
  <p className="muted" role="status">{label} · каждые {seconds} с</p>
  <details className="profile-advanced"><summary>Расширенные настройки мониторинга</summary><div className="profile-settings">
   <label className="field"><span>Интервал обновления</span><select aria-label="Интервал обновления" value={choice ?? seconds} disabled={save.isPending} onChange={event => setChoice(Number(event.target.value))}>{[5,10,15,30,60].map(n => <option key={n} value={n}>{n} секунд</option>)}</select></label>
   <small>Общий интервал панели и новых агентов. Агент получит изменение при следующем отчёте. История хранит последний снимок каждой минуты; heartbeat и VPN-пробы работают отдельно.</small>
   <Button type="button" disabled={save.isPending || choice === null || choice === seconds} onClick={() => save.mutate(choice ?? seconds)}>Сохранить интервал</Button>
   {save.isError && <Notice>{save.error.message}</Notice>}
  </div></details>
 </div>;
}
