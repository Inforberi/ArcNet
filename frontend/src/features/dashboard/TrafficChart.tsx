import {useState} from 'react';
import {chartPath, formatBytes, pointX, rateText, type Observed} from './model';

export default function TrafficChart({history}: {history: Observed[]}) {
 const [hovered, setHovered] = useState<number | null>(null);
 const points = history.filter(point => point.snapshot.rx_per_second != null || point.snapshot.tx_per_second != null);
 if (points.length < 2) return <div className="empty">История накапливается. Для графика нужны хотя бы два замера в разных минутах.</div>;
 const maximum = Math.max(1, ...points.flatMap(point => [point.snapshot.rx_per_second ?? 0, point.snapshot.tx_per_second ?? 0]));
 const index = hovered == null ? history.length - 1 : Math.min(hovered, history.length - 1);
 const point = history[index];
 return <div className="traffic-chart">
  <div className="flex wrap traffic-legend"><span>↓ Получено: {rateText(point.snapshot.rx_per_second)}</span><span>↑ Отправлено: {rateText(point.snapshot.tx_per_second)}</span><time>{new Date(point.received_at * 1000).toLocaleTimeString('ru')}</time></div>
  <svg viewBox="0 0 600 150" role="img" tabIndex={0} aria-label="История скорости сети ОС. Стрелками влево и вправо можно выбрать замер." onPointerMove={event => {const box = event.currentTarget.getBoundingClientRect(); setHovered(Math.max(0, Math.min(history.length - 1, history.reduce((best, _, index) => Math.abs(pointX(history, index) - 600 * (event.clientX - box.left) / box.width) < Math.abs(pointX(history, best) - 600 * (event.clientX - box.left) / box.width) ? index : best, 0))));}} onPointerLeave={() => setHovered(null)} onKeyDown={event => {if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {event.preventDefault(); setHovered(Math.max(0, Math.min(history.length - 1, index + (event.key === 'ArrowLeft' ? -1 : 1))));}}}>
   <path d="M0,140 L600,140 M0,75 L600,75 M0,10 L600,10" className="chart-grid"/>
   <path d={chartPath(history, 'rx_per_second', maximum)} className="chart-received"/>
   <path d={chartPath(history, 'tx_per_second', maximum)} className="chart-sent"/>
   {hovered != null && <line x1={pointX(history, index)} x2={pointX(history, index)} y1={0} y2={150} className="chart-cursor"/>}
  </svg>
  <div className="traffic-axis"><span>{new Date(history[0].received_at * 1000).toLocaleTimeString('ru', {hour: '2-digit', minute: '2-digit'})}</span><small>Шкала до {formatBytes(maximum)}/с</small><span>{new Date(history.at(-1)!.received_at * 1000).toLocaleTimeString('ru', {hour: '2-digit', minute: '2-digit'})}</span></div>
 </div>;
}
