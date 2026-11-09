import TrafficChart from '../../shared/TrafficChart';
import {rateText} from '../../shared/traffic-chart';
import {Loading,Notice} from '../../shared/ui';
import {presenceText,trafficFresh,trafficHistory,type TrafficDetail} from './traffic';

export default function UserTrafficPanel({now,detail,pending,failed}:{now:number;detail?:TrafficDetail;pending:boolean;failed:boolean}){
 if(pending&&!detail)return <Loading/>;
 if(!detail&&!pending)return <Notice>{failed?'Не удалось получить показатели. Повторите открытие карточки.':'Измерения ещё не получены. Трафик и график появятся после первых замеров.'}</Notice>;
 const item=detail?.current,fresh=!failed&&trafficFresh(item,now);
 return <section className="user-traffic-panel" aria-label="Трафик пользователя">
  <div className="section-heading"><h3>Скорость и активность</h3><small className="muted">{item?.at?`Замер ${new Date(item.at*1000).toLocaleTimeString('ru-RU')}`:'Счётчики ещё не получены'}</small></div>
  {failed&&<Notice>Не удалось обновить показатели. История последнего ответа сохранена.</Notice>}
  {detail&&!detail.complete_coverage&&<p className="muted">Показаны только локальные поддерживаемые движки. Полный охват подключений не подтверждён.</p>}
  <div className="user-traffic-current"><div><span>Получает</span><strong>{rateText(fresh?item?.download_bps:null)}</strong></div><div><span>Отправляет</span><strong>{rateText(fresh?item?.upload_bps:null)}</strong></div><div><span>Состояние подключения</span><strong>{presenceText(item,failed,now)}</strong></div></div>
  {item?.presence&&<div className="presence-sources"><h3>Источники наблюдения</h3>{item.presence.sources.map(source=><p key={`${source.node_id??'local'}/${source.source}`}><strong>{source.source==='xray'?'Xray':source.source==='hysteria2'?'Hysteria2':source.source==='wireguard'?'WireGuard':source.source}</strong> · {source.node_id?`Нода ${source.node_id}`:'Локальный сервер'}{source.connection_id?` · ${source.connection_id}`:''}<small className="muted">{source.observed_at?`Замер ${new Date(source.observed_at*1000).toLocaleString('ru-RU')}`:'Подтверждённого замера нет'}{source.last_handshake_at?` · Handshake ${new Date(source.last_handshake_at*1000).toLocaleString('ru-RU')}`:''}</small></p>)}</div>}
  <div className="kv"><span>Последний рост счётчиков</span><span>{item?.last_activity_at?new Date(item.last_activity_at*1000).toLocaleString('ru-RU'):'Не наблюдался'}</span></div>
  <p className="muted text-xs">Скорость — средняя за последний интервал сбора{fresh?` (${item?.window_seconds} с)`:''}. Онлайн подтверждается отдельным источником соединений. Handshake WireGuard и рост трафика показывают недавнюю активность, а не постоянную сессию.</p>
  <h3>График за последние 2 часа</h3>
  <TrafficChart history={trafficHistory(detail?.history??[])} label="История скорости VPN пользователя" emptyText="История накапливается. Для графика нужны хотя бы две точки в разных минутах."/>
  <small className="muted">Один снимок в минуту. История ограничена общим буфером и очищается при перезапуске панели.</small>
 </section>
}
