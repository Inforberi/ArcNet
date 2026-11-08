import {Field} from '../../shared/ui';
import {resetWeekdays} from './usage';

export type PackageScheduleDraft={frequency:string;timezone:string;weekday:number;day:number};
export default function PackageScheduleFields({value,onChange,nextAt}:{value:PackageScheduleDraft;onChange:(patch:Partial<PackageScheduleDraft>)=>void;nextAt?:number}){
 return <div className="user-reset-settings">
  <div className="field-row">
   <label className="field"><span>Сброс расхода</span><select value={value.frequency} onChange={e=>onChange({frequency:e.target.value})}><option value="none">Никогда</option><option value="daily">Каждый день</option><option value="weekly">Каждую неделю</option><option value="monthly">Каждый месяц</option></select></label>
   {value.frequency==='weekly'&&<label className="field"><span>День недели</span><select value={value.weekday} onChange={e=>onChange({weekday:Number(e.target.value)})}>{[1,2,3,4,5,6,0].map(day=><option key={day} value={day}>{resetWeekdays[day]}</option>)}</select></label>}
   {value.frequency==='monthly'&&<Field label="Число месяца" type="number" min={1} max={31} step={1} required value={value.day} onChange={e=>onChange({day:Number(e.target.value)})} description="Если такого числа нет, сброс будет в последний день месяца."/>}
  </div>
  {value.frequency!=='none'&&<Field label="Часовой пояс расписания" value={value.timezone} maxLength={64} required onChange={e=>onChange({timezone:e.target.value})} description="Например, Europe/Moscow или UTC. Сброс в 00:00 этого пояса, срок доступа не продлевается."/>}
  {value.frequency!=='none'&&<small className="muted">{nextAt?`Следующий сброс: ${new Date(nextAt*1000).toLocaleString('ru-RU')} (время вашего браузера).`:'Следующая дата будет рассчитана сервером при сохранении.'} При недоступном учёте сброс откладывается.</small>}
 </div>;
}
