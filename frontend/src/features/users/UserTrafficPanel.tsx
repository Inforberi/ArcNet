import TrafficChart from '../../shared/TrafficChart';
import {rateText} from '../../shared/traffic-chart';
import {Loading,Notice} from '../../shared/ui';
import {activityText,trafficFresh,trafficHistory,type TrafficDetail} from './traffic';

export default function UserTrafficPanel({now,detail,pending,failed}:{now:number;detail?:TrafficDetail;pending:boolean;failed:boolean}){
 if(pending&&!detail)return <Loading/>;
 if(!detail&&!pending)return <Notice>{failed?'Не удалось получить показатели. Повторите открытие карточки.':'Измерения ещё не получены. Трафик и график появятся после первых замеров.'}</Notice>;
 const item=detail?.current,fresh=!failed&&trafficFresh(item,now);
 return <section className="user-traffic-panel" aria-label="Трафик пользователя">
  <div className="section-heading"><h3>Скорость и активность</h3><small className="muted">{item?.at?`Замер ${new Date(item.at*1000).toLocaleTimeString('ru-RU')}`:'Счётчики ещё не получены'}</small></div>
  {failed&&<Notice>Не удалось обновить показатели. История последнего ответа сохранена.</Notice>}
  {detail&&!detail.complete_coverage&&<p className="muted">Показаны только локальные поддерживаемые движки. Полный охват подключений не подтверждён.</p>}
  <div className="user-traffic-current"><div><span>Получает</span><strong>{rateText(fresh?item?.download_bps:null)}</strong></div><div><span>Отправляет</span><strong>{rateText(fresh?item?.upload_bps:null)}</strong></div><div><span>Активность</span><strong>{activityText(item,failed,now)}</strong></div></div>
  <div className="kv"><span>Последний рост счётчиков</span><span>{item?.last_activity_at?new Date(item.last_activity_at*1000).toLocaleString('ru-RU'):'Не наблюдался'}</span></div>
  <p className="muted text-xs">Скорость — средняя за последний интервал сбора{fresh?` (${item?.window_seconds} с)`:''}. Трафик не подтверждает активную сессию; статус онлайн пока недоступен.</p>
  <h3>График за последние 2 часа</h3>
  <TrafficChart history={trafficHistory(detail?.history??[])} label="История скорости VPN пользователя" emptyText="История накапливается. Для графика нужны хотя бы две точки в разных минутах."/>
  <small className="muted">Один снимок в минуту. История ограничена общим буфером и очищается при перезапуске панели.</small>
 </section>
}
