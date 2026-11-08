import {createContext, useContext, useEffect, useState, type ReactNode} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {api} from '../../shared/api';
import {Button, Notice} from '../../shared/ui';

type Settings = {interval_seconds: number; available_seconds: number[]};
type Connection = 'connecting' | 'live' | 'reconnecting' | 'paused';
const MonitoringContext = createContext<{connection: Connection; seconds: number}>({connection: 'connecting', seconds: 5});
const liveKeys = new Set(['overview', 'server-telemetry', 'nodes', 'node-bootstraps', 'protocol-health', 'events', 'users', 'user-usage', 'user-debits', 'user-devices', 'user-traffic']);

// One polling clock for the authenticated layout; only mounted queries refetch.
export function MonitoringProvider({children}: {children: ReactNode}) {
 const cache = useQueryClient();
 const settings = useQuery({queryKey: ['monitoring-settings'], queryFn: () => api<Settings>('/api/settings/monitoring')});
 const seconds = settings.data?.interval_seconds ?? 5;
 const [connection, setConnection] = useState<Connection>('connecting');
 useEffect(() => {
  let stopped=false,running=false,lastHistory=0,timer:ReturnType<typeof setTimeout>|undefined;
  const tick=async()=>{
   clearTimeout(timer);
   if(stopped||running)return;
   if(document.hidden){setConnection('paused');return}
   if(!navigator.onLine){setConnection('reconnecting');return}
   running=true;setConnection('live');
   try{
    await cache.invalidateQueries({predicate:query=>liveKeys.has(String(query.queryKey[0]))},{cancelRefetch:false});
    if(Date.now()-lastHistory>=60000){lastHistory=Date.now();await Promise.all([cache.invalidateQueries({queryKey:['server-history']},{cancelRefetch:false}),cache.invalidateQueries({queryKey:['monitoring-settings']},{cancelRefetch:false})])}
   }finally{running=false;if(!stopped)timer=setTimeout(()=>void tick(),seconds*1000)}
  };
  const resume=()=>void tick();
  void tick();document.addEventListener('visibilitychange',resume);window.addEventListener('online',resume);window.addEventListener('offline',resume);
  return()=>{stopped=true;clearTimeout(timer);document.removeEventListener('visibilitychange',resume);window.removeEventListener('online',resume);window.removeEventListener('offline',resume)};
 },[cache,seconds]);
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
 const label = {connecting: 'Запускаем опрос сервера', live: 'Обновляется автоматически', reconnecting: 'Нет сети · опрос приостановлен', paused: 'Обновления приостановлены в скрытой вкладке'}[connection];
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
