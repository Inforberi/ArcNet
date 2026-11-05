import {useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {Link} from 'react-router';
import {api} from '../shared/api';
import {Badge, Button, Field, Loading, Notice, PageHeading, Panel} from '../shared/ui';
import {ageText, formatBytes, formatPercent, needsAttention, percentUsed, rateText, statusText, type FleetOverview, type ServerDetail} from '../features/dashboard/model';
import TrafficChart from '../features/dashboard/TrafficChart';

function Metric({name, value, note, percent}: {name: string; value: string; note: string; percent: number | null}) {
 return <div className="metric"><div className="label">{name}</div><div className="value">{value}</div><div className="description">{note}</div>{percent != null && <div className="bar"><span style={{width: `${Math.max(0, Math.min(100, percent))}%`}}/></div>}</div>;
}
const health = (value: string) => <Badge variant={['online', 'active', 'pass', 'local'].includes(value) ? 'green' : ['offline', 'failed', 'fail'].includes(value) ? 'red' : 'blue'}>{statusText[value] ?? 'Неизвестно'}</Badge>;

export default function Overview() {
 const [selected, setSelected] = useState('local');
 const [filter, setFilter] = useState('');
 const [attention, setAttention] = useState(false);
 const [visibleCount, setVisibleCount] = useState(20);
 const overview = useQuery({queryKey: ['overview'], queryFn: () => api<FleetOverview>('/api/overview'), refetchInterval: 30000, refetchIntervalInBackground: false});
 const details = useQuery({queryKey: ['server-telemetry', selected], queryFn: () => api<ServerDetail>(`/api/servers/${encodeURIComponent(selected)}/telemetry?history=0`), refetchInterval: 30000, refetchIntervalInBackground: false});
 const history = useQuery({queryKey: ['server-history', selected], queryFn: () => api<ServerDetail>(`/api/servers/${encodeURIComponent(selected)}/telemetry?history=1`), refetchInterval: 60000, refetchIntervalInBackground: false});
 const data = details.data; const server = data?.server; const snapshot = server?.telemetry?.snapshot;
 const now = Math.floor(Math.max(overview.data ? overview.data.sampled_at + (Date.now() - overview.dataUpdatedAt) / 1000 : 0, details.data ? details.data.sampled_at + (Date.now() - details.dataUpdatedAt) / 1000 : 0)) || Math.floor(Date.now() / 1000);
 const fresh = !!server && !details.isError && !server.stale && !!server.telemetry && now - server.telemetry.received_at <= (data?.telemetry_fresh_for ?? 180);
 const memory = fresh ? percentUsed(snapshot?.memory_total, snapshot?.memory_available) : null;
 const disk = fresh ? percentUsed(snapshot?.disk_total, snapshot?.disk_free) : null;
 const swap = fresh ? percentUsed(snapshot?.swap_total, snapshot?.swap_free) : null;
 const fleet = (overview.data?.servers ?? []).map(item => {
  const expired = item.id !== 'local' && item.last_seen > 0 && now - item.last_seen > 75;
  return expired || overview.isError ? {...item, stale: true, agent: expired ? 'offline' : 'unknown', xray: 'unknown', vpn: item.vpn === 'pass' ? 'stale' : item.vpn, cpu_percent: null, memory_percent: null} : item;
 });
 const shown = fleet.filter(item => (!attention || needsAttention(item)) && `${item.name} ${item.address}`.toLocaleLowerCase('ru').includes(filter.toLocaleLowerCase('ru')));
 return <>
  <PageHeading title="Обзор" description="Ресурсы серверов и отдельные статусы агента, Xray и VPN" actions={<Button onClick={() => {void overview.refetch(); void details.refetch(); void history.refetch();}} disabled={overview.isFetching || details.isFetching}>Обновить</Button>}/>
  <div className="dashboard-toolbar"><label className="field"><span>Сервер на дашборде</span><select aria-label="Сервер на дашборде" value={selected} onChange={event => setSelected(event.target.value)}><option value="local">Сервер панели</option>{fleet.filter(item => item.id !== 'local').map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label><p className="muted">{overview.data ? `Серверов: ${overview.data.nodes} · пользователей включено: ${overview.data.enabled_users} из ${overview.data.users}` : 'Получаем состояние инфраструктуры'}</p></div>
  {overview.isError && <Notice>{overview.error.message}</Notice>}
  {details.isError && <Notice>{details.error.message} <Button onClick={() => setSelected('local')}>Открыть сервер панели</Button></Notice>}
  {details.isPending ? <Loading/> : data && server && <>
   <div className="dashboard-status-row"><strong>{server.name}</strong>{server.address && <span>{server.address}</span>}<span className="muted">Метрики: {ageText(server.telemetry?.received_at, now)}{!fresh ? ' · нет свежих данных' : ''}</span></div>
   {server.legacy && <p className="inline-status">Агент передаёт базовые показатели. Для CPU, диска и сети нужен агент с расширенной телеметрией.</p>}
   <section className="metrics" aria-label="Ресурсы выбранного сервера">
    <Metric name="CPU" value={formatPercent(fresh ? snapshot?.cpu_percent : null)} note={snapshot?.cpu_percent == null ? 'Процент CPU требует двух замеров' : snapshot?.cpus ? `${snapshot.cpus} CPU · load 1m: ${fresh && snapshot.load_1 != null ? snapshot.load_1.toFixed(2) : 'нет данных'}` : 'Первый процент появится после двух замеров'} percent={fresh ? snapshot?.cpu_percent ?? null : null}/>
    <Metric name="Оперативная память" value={formatPercent(memory)} note={fresh && snapshot?.memory_total != null ? `Доступно ${formatBytes(snapshot.memory_available)} из ${formatBytes(snapshot.memory_total)}` : 'Нет свежих показателей'} percent={memory}/>
    <Metric name="Swap" value={fresh && snapshot?.swap_total === 0 ? 'Не используется' : formatPercent(swap)} note={fresh && snapshot?.swap_total != null ? `${formatBytes(snapshot.swap_free)} свободно из ${formatBytes(snapshot.swap_total)}` : 'Нет свежих показателей'} percent={swap}/>
    <Metric name="Диск" value={formatPercent(disk)} note={fresh && snapshot?.disk_total != null ? `Доступно ${formatBytes(snapshot.disk_free)} из ${formatBytes(snapshot.disk_total)}` : 'Файловая система данных сервера'} percent={disk}/>
   </section>
   <div className="grid2"><Panel title="Доступность выбранного сервера"><div className="panel-body dashboard-health">
    <div><span>{server.id === 'local' ? 'Панель' : 'Связь с агентом'}</span>{health(server.agent)}</div>
    <div><span>Служба Xray</span>{health(server.xray)}</div>
    <div><span>Внешняя VPN-проба</span>{health(server.vpn)}</div>
    <div><span>Маршрут / egress</span>{health(server.route)}</div>
    {server.vpn_checked_at > 0 && <small className="muted">VPN-проба: {ageText(server.vpn_checked_at, now)}. Это последняя проверка, а не гарантия текущей доступности.</small>}
    <small className="muted">Связь с агентом и работающая служба не подтверждают доступность VPN из сети пользователя.</small>
   </div></Panel><Panel title="Сеть и система"><div className="panel-body dashboard-health">
    <div><span>Получено / отправлено</span><strong>{fresh ? `${rateText(snapshot?.rx_per_second)} ↓ · ${rateText(snapshot?.tx_per_second)} ↑` : 'Нет свежих показателей'}</strong></div>
    <div><span>TCP / UDP сокеты ОС</span><strong>{fresh ? `${snapshot?.tcp_sockets ?? '—'} / ${snapshot?.udp_sockets ?? '—'}` : '—'}</strong></div>
    <div><span>Аптайм ОС</span><strong>{fresh && snapshot?.uptime_seconds != null ? `${Math.floor(snapshot.uptime_seconds / 86400)} д ${Math.floor(snapshot.uptime_seconds % 86400 / 3600)} ч` : 'Нет данных'}</strong></div>
    {server.version && <div><span>Версия агента</span><strong>{server.version}</strong></div>}
    <small className="muted">Сеть ОС на интерфейсах маршрута по умолчанию. Это не трафик отдельных VPN-пользователей.</small>
   </div></Panel></div>
   <Panel title="История сети за последние 2 часа"><div className="panel-body">{(history.isError || history.data?.history_available === false) && <p className="error" role="alert">История временно недоступна. Текущие показатели показаны отдельно.</p>}<TrafficChart history={history.data?.history ?? []}/></div></Panel>
  </>}
  <Panel title="Подключённые серверы" actions={<Link to="/nodes">Управление серверами →</Link>}><div className="panel-body">
   <div className="dashboard-fleet-filter"><Field label="Найти сервер" value={filter} onChange={event => setFilter(event.target.value)} placeholder="Название или IP"/><Button aria-pressed={attention} onClick={() => setAttention(value => !value)}>Требуют внимания ({fleet.filter(needsAttention).length})</Button></div>
   {overview.isPending ? <Loading/> : shown.length === 0 ? <p className="muted">Серверов с такими условиями нет.</p> : <div className="dashboard-fleet">{shown.slice(0, visibleCount).map(item => <article key={item.id} className={`fleet-server ${selected === item.id ? 'selected' : ''}`}><div><Button variant="ghost" aria-pressed={selected === item.id} onClick={() => setSelected(item.id)}>{item.name}</Button><small>{item.address || (item.id === 'local' ? 'Контроллер' : 'Адрес не назначен')}</small></div><div><small>Агент / панель</small>{health(item.agent)}</div><div><small>Xray</small>{health(item.xray)}</div><div><small>Последняя VPN-проба</small>{health(item.vpn)}</div><div><small>CPU / RAM</small><span>{formatPercent(item.cpu_percent)} / {formatPercent(item.memory_percent)}</span><small>{item.stale ? 'Данные устарели' : ageText(item.last_seen, now)}</small></div></article>)}</div>}
   {shown.length > visibleCount && <Button onClick={() => setVisibleCount(value => value + 20)}>Показать ещё · отображается {visibleCount} из {shown.length}</Button>}
  </div></Panel>
 </>;
}
