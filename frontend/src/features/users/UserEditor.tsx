import {useState} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {api,type VPNUser,type UserPreferences,type VPNConfig,type ProtocolProfile} from '../../shared/api';
import Dialog from '../../shared/Dialog';
import {quotaBytes, type UserPackage} from './usage';
import {Advanced,Button,Field,Loading,Notice} from '../../shared/ui';

const labels:Record<string,string>={'vless-reality':'VLESS · REALITY TCP','vless-xhttp':'VLESS · REALITY XHTTP',hysteria2:'Hysteria2',wireguard:'WireGuard'};
const resetLabels:Record<string,string>={none:'выключить',daily:'каждый день',weekly:'каждый понедельник',monthly:'первого числа месяца'};
type Reply={name:string;enabled:boolean;preferences:UserPreferences;package:UserPackage};
function localDate(seconds:number){if(!seconds)return '';const d=new Date(seconds*1000);return new Date(d.getTime()-d.getTimezoneOffset()*60000).toISOString().slice(0,16)}
export default function UserEditor({user,onClose,navigation}:{user:VPNUser;onClose:()=>void;navigation?:React.ReactNode}){
 const cache=useQueryClient();
 const q=useQuery({queryKey:['user-editor',user.id],queryFn:()=>api<Reply>(`/api/users/${user.id}/editor`)});
 return q.isPending?<Dialog title="Изменить пользователя" onClose={onClose}><Loading/></Dialog>:q.isError?<Dialog title="Изменить пользователя" onClose={onClose}><div className="dialog-body"><Notice>{q.error.message}</Notice><Button onClick={()=>void q.refetch()}>Повторить</Button></div></Dialog>:<UserForm key={`${user.id}:${q.data.preferences.version}`} initial={q.data} user={user} onClose={onClose} navigation={navigation} onSaved={async()=>{await Promise.all(['users','user-editor','user-preferences','user-usage','user-devices','protocol-health'].map(key=>cache.invalidateQueries({queryKey:[key]})))}}/>;
}
function UserForm({initial,user,onClose,navigation,onSaved}:{initial:Reply;user:VPNUser;onClose:()=>void;navigation?:React.ReactNode;onSaved:()=>Promise<void>}){
 const [name,setName]=useState(initial.name),[p,setP]=useState(initial.preferences),[busy,setBusy]=useState(false),[error,setError]=useState('');
 const [enabled,setEnabled]=useState(initial.enabled),[unlimited,setUnlimited]=useState(initial.package.quota_bytes===0),[quota,setQuota]=useState(initial.package.quota_bytes?String(initial.package.quota_bytes/1e9):'10'),[expires,setExpires]=useState(localDate(initial.package.expires_at)),[reset,setReset]=useState(false),[saved,setSaved]=useState(false);
 const [frequency,setFrequency]=useState(initial.package.reset_schedule?.frequency??'none'),[timezone,setTimezone]=useState(initial.package.reset_schedule?.timezone??Intl.DateTimeFormat().resolvedOptions().timeZone);
 const scheduleChanged=frequency!==(initial.package.reset_schedule?.frequency??'none')||(frequency!=='none'&&timezone!==initial.package.reset_schedule?.timezone);
 const bytes=unlimited?0:quotaBytes(quota),timestamp=expires?Math.floor(new Date(expires).getTime()/1000):0;
 const packageChanged=bytes!==initial.package.quota_bytes||timestamp!==initial.package.expires_at||reset||scheduleChanged;
 const vpn=useQuery({queryKey:['vpn'],queryFn:()=>api<VPNConfig>('/api/vpn')});
 const profiles=useQuery({queryKey:['protocols'],queryFn:()=>api<{items:ProtocolProfile[]}>('/api/protocols')});
 const options=[...(vpn.data?.configured?['vless-reality']:[]),...(profiles.data?.items.map(v=>v.kind).filter(kind=>kind!=='amneziawg')??[])];
 const chosen=p.allowed_kinds??options;
 const changed=!saved&&(enabled!==initial.enabled||packageChanged||name!==initial.name||JSON.stringify(p)!==JSON.stringify(initial.preferences));
 const update=(part:Partial<UserPreferences>)=>setP(v=>({...v,...part}));
 async function save(e:React.FormEvent){
  e.preventDefault();setError('');
  if(bytes===undefined||!Number.isSafeInteger(timestamp)||timestamp<0){setError('Проверьте пакет и срок действия');return}
  if(reset&&!window.confirm('Начать новый период и обнулить расход этого пользователя?'))return;
  setBusy(true);
  try{
   const result=await api<{runtime_state:string}>(`/api/users/${user.id}/editor`,'PUT',{name:name.trim(),preferences:p,expected_version:initial.preferences.version,expected_enabled:initial.enabled,enabled,package:packageChanged?{quota_bytes:bytes,expires_at:timestamp,expected_version:initial.package.version,reset_usage:reset,schedule:scheduleChanged?{frequency,timezone}:null}:null});
   setSaved(true);
   // Keep the committed result visible until the administrator acknowledges pending application.
   if(result.runtime_state==='applied') {await onSaved();onClose()}
   else setError('Все настройки сохранены. Применение к VPN-службам ещё не подтверждено; панель повторит попытку.');
  }catch(e){setError(e instanceof Error?e.message:'Не удалось сохранить пользователя')}finally{setBusy(false)}
 }
 async function finishClose(){if(saved)await onSaved();onClose()}
 async function close(){if(!changed||window.confirm('Закрыть без сохранения?'))await finishClose()}

 return <Dialog title={`Изменить пользователя · ${initial.name}`} onClose={()=>void finishClose()} busy={busy} dirty={changed} className="user-editor">
 {navigation&&<div inert={busy||saved} onClickCapture={e=>{if((e.target as HTMLElement).closest('button[aria-pressed="false"]')&&(busy||(changed&&!window.confirm('Есть несохранённые изменения. Перейти без сохранения?')))){e.preventDefault();e.stopPropagation()}}}>{navigation}</div>}
 <form onSubmit={save}><fieldset disabled={busy||saved} className="user-editor-fields"><div className="dialog-body">
 <label className="connection-choice"><input type="checkbox" checked={enabled} onChange={e=>setEnabled(e.target.checked)}/><span>Доступ пользователя включён</span></label>
 <div className="delivery-modes" role="group" aria-label="Предпочтительный способ выдачи"><Button type="button" aria-pressed={p.access_mode!=='direct'} disabled={busy} onClick={()=>update({access_mode:'subscription'})}>Подписка<span>Несколько профилей в одной ссылке</span></Button><Button type="button" aria-pressed={p.access_mode==='direct'} disabled={busy||!chosen.length} onClick={()=>update({access_mode:'direct',direct_kind:chosen.includes(p.direct_kind)?p.direct_kind:chosen[0]})}>Прямое подключение<span>Выбранный профиль и отдельная ссылка</span></Button></div>
 <p className="muted text-xs">Способ выдачи не отзывает действующие ключи и подписку. Доступ включён только для выбранных ниже протоколов.</p>
 <div className="field-row"><Field label="Имя пользователя" value={name} required maxLength={60} disabled={busy} onChange={e=>setName(e.target.value)}/><Field label="Название подписки" value={p.title} maxLength={25} disabled={busy} onChange={e=>update({title:e.target.value})} description={`В приложении: ${p.title||`ArcNet · ${name}`}`}/></div>
 <section className="user-package-fields" aria-label="Пакет и срок"><h3>Пакет и срок</h3><label className="connection-choice"><input type="checkbox" checked={unlimited} onChange={e=>setUnlimited(e.target.checked)}/><span>Без ограничения трафика</span></label><div className="field-row">{!unlimited&&<Field label="Пакет, ГБ" inputMode="decimal" value={quota} required onChange={e=>setQuota(e.target.value)} description="Загрузка и отправка вместе. 1 ГБ = 1 000 000 000 байт."/>}<Field label="Действует до" type="datetime-local" value={expires} onChange={e=>setExpires(e.target.value)} description="Пусто — без срока. Время вашего браузера."/></div></section>
 <section><div className="section-heading"><h3>Разрешённые подключения</h3><Button type="button" className="small" disabled={busy||vpn.isPending||profiles.isPending} onClick={()=>update({allowed_kinds:null})}>Все, включая новые</Button></div>
 {vpn.isError||profiles.isError?<Notice>Не удалось загрузить подключения. Изменение выбора недоступно.</Notice>:vpn.isPending||profiles.isPending?<Loading/>:<div className="connection-choices">{options.map(kind=><label key={kind} className={`connection-choice ${chosen.includes(kind)?'selected':''}`}><input type="checkbox" checked={chosen.includes(kind)} disabled={busy} onChange={e=>{const next=e.target.checked?[...chosen,kind]:chosen.filter(v=>v!==kind);update({allowed_kinds:next,access_mode:next.length?p.access_mode:'subscription',direct_kind:next.includes(p.direct_kind)?p.direct_kind:next[0]??''})}}/><span>{labels[kind]??kind}</span></label>)}{!options.length&&<p className="muted">Сначала добавьте подключение на сервере.</p>}</div>}
 <p className="muted text-xs">{p.allowed_kinds===null?'Все текущие и будущие протоколы.':'Только выбранные протоколы. Новые автоматически не добавляются.'} Изменение состава может кратко прервать соединения. Для нод выбор пока недоступен.</p></section>
 {p.access_mode==='direct'&&<label className="field"><span>Профиль для прямой ссылки</span><select value={p.direct_kind} disabled={busy} onChange={e=>update({direct_kind:e.target.value})}>{chosen.map(kind=><option key={kind} value={kind}>{labels[kind]??kind}</option>)}</select></label>}
 <Advanced title="Дополнительные данные"><label className="field"><span>Сброс расхода по расписанию</span><select value={frequency} onChange={e=>setFrequency(e.target.value)}><option value="none">Никогда</option><option value="daily">Каждый день</option><option value="weekly">Каждый понедельник</option><option value="monthly">Первого числа месяца</option></select></label>{frequency!=='none'&&<Field label="Часовой пояс расписания" value={timezone} maxLength={64} required onChange={e=>setTimezone(e.target.value)} description="Например, Europe/Moscow или UTC. Сброс в 00:00, срок доступа не продлевается."/>}{initial.package.reset_schedule&&!scheduleChanged&&<small>Следующий сброс: {new Date(initial.package.reset_schedule.next_at*1000).toLocaleString('ru-RU')}. При недоступном учёте сброс откладывается.</small>}<Field label="Группа" value={p.group} maxLength={50} disabled={busy} onChange={e=>update({group:e.target.value})}/><Field label="Комментарий" value={p.comment} maxLength={220} disabled={busy} onChange={e=>update({comment:e.target.value})}/><label className="connection-choice"><input type="checkbox" checked={reset} onChange={e=>setReset(e.target.checked)}/><span>Начать новый период и обнулить расход пакета</span></label><small>Новый период требует свежих счётчиков. Ключи и подписка сохраняются. Ограничения IP/сессий здесь пока недоступны.</small></Advanced>
 {changed&&<section className="notice" aria-label="План изменений"><strong>Будет сохранено</strong>{name!==initial.name&&<div>Имя: {initial.name} → {name}</div>}{enabled!==initial.enabled&&<div>Доступ: {enabled?'включить':'отключить'}</div>}{packageChanged&&<div>Пакет: {unlimited?'без лимита':`${quota} ГБ`}; срок: {expires?new Date(expires).toLocaleString('ru-RU'):'без срока'}{reset?'; расход обнулится':''}{scheduleChanged?`; расписание: ${frequency==='none'?'выключить':`${resetLabels[frequency]}, ${timezone}`}`:''}</div>}{JSON.stringify(p)!==JSON.stringify(initial.preferences)&&<div>Настройки подписки и выбранные подключения обновятся.</div>}</section>}
 {error&&(saved?<Notice>{error}</Notice>:<p className="error" role="alert">{error}</p>)}
 </div></fieldset><footer className="dialog-footer"><Button type="button" disabled={busy} onClick={()=>void close()}>{saved?'Закрыть':'Отмена'}</Button><Button type="submit" variant="primary" disabled={busy||saved||!changed||!name.trim()}>{busy?'Применяем…':'Сохранить изменения'}</Button></footer></form></Dialog>
}
