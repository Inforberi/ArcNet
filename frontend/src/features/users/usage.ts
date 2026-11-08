import type {VPNUser} from '../../shared/api';
export type UserPackage = {
 reset_schedule?:{frequency:string;timezone:string;next_at:number;weekday?:number;day?:number};
 user_id: number; quota_bytes: number; upload_bytes: number; download_bytes: number;
 expires_at: number; period: number; period_started_at: number; version: number; observed_at: number; block_reason: string; incomplete: boolean; needs_review: boolean;
};
export type UsageReply = {items: UserPackage[]; sampled_at: number; source_fresh: boolean; source_last_success: number; complete_coverage: boolean; runtime_state: string};
export type Debit = {at: number; period: number; upload_bytes: number; download_bytes: number};
export const resetWeekdays=['Воскресенье','Понедельник','Вторник','Среда','Четверг','Пятница','Суббота'];
export function resetScheduleLabel(schedule?:{frequency:string;weekday?:number;day?:number}){
 if(!schedule||schedule.frequency==='none')return 'Без автосброса';
 if(schedule.frequency==='daily')return 'Каждый день';
 if(schedule.frequency==='weekly')return `Каждую неделю · ${resetWeekdays[schedule.weekday??1]??'день не получен'}`;
 if(schedule.frequency==='monthly')return `Каждый месяц · ${schedule.day||1}-го числа`;
 return 'Расписание не получено';
}
export const usageGB = (bytes: number) => `${(bytes / 1e9).toLocaleString('ru', {maximumFractionDigits: 3})} ГБ`;
export function quotaBytes(value: string): number | undefined {
 if (!/^\d+(?:[.,]\d{1,3})?$/.test(value.trim())) return;
 const bytes = Math.round(Number(value.replace(',', '.')) * 1e9);
 return Number.isSafeInteger(bytes) && bytes > 0 && bytes <= 9e15 ? bytes : undefined;
}
export const blockLabel: Record<string, string> = {quota_exhausted: 'Пакет исчерпан', expired: 'Срок истёк', accounting_unavailable: 'Нет свежего учёта — доступ ограничен', accounting_gap: 'Разрыв учёта после перезапуска — нужен новый период'};

export function usageAmount(bytes:number){
 const units=['Б','КБ','МБ','ГБ','ТБ'];let index=0;while(bytes>=1000&&index<units.length-1){bytes/=1000;index++}
 return `${bytes.toLocaleString('ru-RU',{maximumFractionDigits:index?1:0})} ${units[index]}`;
}
export function userAccess(user:VPNUser,pack?:UserPackage):{label:string;tone:'green'|'red'|'neutral';allowed:boolean;description:string}{
 if(!user.enabled)return {label:'Отключён',tone:'neutral',allowed:false,description:'Доступ отключён администратором'};
 if(user.preferences?.allowed_kinds?.length===0)return {label:'Нет подключений',tone:'neutral',allowed:false,description:'Пользователю не разрешено ни одного подключения'};
 if(!pack)return {label:'Нет данных',tone:'neutral',allowed:false,description:'Состояние пакета ещё не получено'};
 if(pack.block_reason){const short:Record<string,string>={quota_exhausted:'Пакет исчерпан',expired:'Срок истёк',accounting_unavailable:'Учёт недоступен',accounting_gap:'Нужна проверка'};return {label:short[pack.block_reason]??'Ограничен',tone:'red',allowed:false,description:blockLabel[pack.block_reason]??'Доступ ограничен пакетом'}};
 return {label:'Разрешён',tone:'green',allowed:true,description:'Настройки разрешают доступ. Подключение пользователя не проверено'};
}
