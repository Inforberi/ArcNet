import {useConfirmation} from '../../shared/Confirmation';
import {useEffect,useRef,useState} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {api,ApiError,type VPNUser,type UserPreferences,type VPNConfig,type ProtocolProfile} from '../../shared/api';
import {revealInvalidField} from '../../shared/EditorSections';
import {quotaBytes,resetScheduleLabel, type UserPackage} from './usage';
import PackageScheduleFields,{type PackageScheduleDraft} from './PackageScheduleFields';
import ConnectionChoiceList,{connectionChoiceLabels} from './ConnectionChoiceList';
import {Advanced,Button,Field,Loading,Notice,SettingsSection,SwitchButton} from '../../shared/ui';

type EditorSections={section:string;onSectionChange:(section:string)=>void;activity?:React.ReactNode};
const baseLabels:Record<string,string>={'vless-reality':'VLESS · REALITY TCP','vless-xhttp':'VLESS · REALITY XHTTP',hysteria2:'Hysteria2',wireguard:'WireGuard'};
type Reply={name:string;enabled:boolean;preferences:UserPreferences;package:UserPackage};
function localDate(seconds:number){if(!seconds)return '';const d=new Date(seconds*1000);return new Date(d.getTime()-d.getTimezoneOffset()*60000).toISOString().slice(0,16)}
export default function UserEditor({user,onClose,navigation,onStateChange,...sections}:{user:VPNUser;onClose:()=>void;navigation?:React.ReactNode;onStateChange?:(state:{busy:boolean;dirty:boolean})=>void}&EditorSections){
 const cache=useQueryClient();
 const q=useQuery({queryKey:['user-editor',user.id],queryFn:()=>api<Reply>(`/api/users/${user.id}/editor`)});
 return q.data?<UserForm {...sections} key={user.id} initial={q.data} onCheck={()=>void q.refetch()} checking={q.isFetching} onStateChange={onStateChange} user={user} onClose={onClose} navigation={navigation} onSaved={async()=>{await Promise.all(['users','user-editor','user-preferences','user-usage','user-devices','protocol-health'].map(key=>cache.invalidateQueries({queryKey:[key]},{throwOnError:true})))}}/>:q.isError?<div className="dialog-body"><p className="error" role="alert">{q.error.message}</p><Button onClick={()=>void q.refetch()}>Повторить</Button></div>:<Loading/>;
}
function UserForm({initial:loaded,user,onClose,navigation,onSaved,onStateChange,onCheck,checking,section,onSectionChange:setSection,activity}:{initial:Reply;onCheck:()=>void;checking:boolean;user:VPNUser;onClose:()=>void;navigation?:React.ReactNode;onSaved:()=>Promise<void>;onStateChange?:(state:{busy:boolean;dirty:boolean})=>void}&EditorSections){
 const [initial]=useState(loaded);
 const [blocked,setBlocked]=useState(false);
 const submitting=useRef(false);
 const stale=loaded.preferences.version!==initial.preferences.version||loaded.package.version!==initial.package.version||loaded.enabled!==initial.enabled;
 const confirm=useConfirmation();
 const [name,setName]=useState(initial.name),[p,setP]=useState(initial.preferences),[busy,setBusy]=useState(false),[error,setError]=useState('');
 const [enabled,setEnabled]=useState(initial.enabled),[unlimited,setUnlimited]=useState(initial.package.quota_bytes===0),[quota,setQuota]=useState(initial.package.quota_bytes?String(initial.package.quota_bytes/1e9):'10'),[expires,setExpires]=useState(localDate(initial.package.expires_at)),[reset,setReset]=useState(false),[saved,setSaved]=useState(false);
 const [schedule,setSchedule]=useState<PackageScheduleDraft>({frequency:initial.package.reset_schedule?.frequency??'none',timezone:initial.package.reset_schedule?.timezone??Intl.DateTimeFormat().resolvedOptions().timeZone,weekday:initial.package.reset_schedule?.weekday??1,day:initial.package.reset_schedule?.day||1});
 const {frequency,timezone,weekday,day}=schedule;
 const scheduleChanged=frequency!==(initial.package.reset_schedule?.frequency??'none')||(frequency!=='none'&&timezone!==initial.package.reset_schedule?.timezone)||(frequency==='weekly'&&weekday!==(initial.package.reset_schedule?.weekday??1))||(frequency==='monthly'&&day!==(initial.package.reset_schedule?.day||1));
 const bytes=unlimited?0:quotaBytes(quota),timestamp=expires?Math.floor(new Date(expires).getTime()/1000):0;
 const packageChanged=bytes!==initial.package.quota_bytes||timestamp!==initial.package.expires_at||reset||scheduleChanged;
 const vpn=useQuery({queryKey:['vpn'],queryFn:()=>api<VPNConfig>('/api/vpn')});
 const profiles=useQuery({queryKey:['protocols'],queryFn:()=>api<{items:ProtocolProfile[]}>('/api/protocols')});
 const labels={...baseLabels,...connectionChoiceLabels(profiles.data?.items)};
 const options=[...(vpn.data?.configured?['vless-reality']:[]),...(profiles.data?.items.map(v=>v.id||v.kind).filter(kind=>kind!=='amneziawg')??[])];
 const chosen=p.allowed_kinds??options;
 const changed=!saved&&(enabled!==initial.enabled||packageChanged||name!==initial.name||JSON.stringify(p)!==JSON.stringify(initial.preferences));
 useEffect(()=>{onStateChange?.({busy,dirty:changed})},[busy,changed,onStateChange]);
 const update=(part:Partial<UserPreferences>)=>setP(v=>({...v,...part}));
 function selectConnections(next:string[]){update({allowed_kinds:next,access_mode:next.length?p.access_mode:'subscription',direct_kind:next.includes(p.direct_kind)?p.direct_kind:next[0]??''})}
 async function save(e:React.FormEvent){
  e.preventDefault();if(submitting.current||busy||blocked||stale)return;setError('');
  if(bytes===undefined||!Number.isSafeInteger(timestamp)||timestamp<0){setError('Проверьте пакет и срок действия');return}
  submitting.current=true;
  if(reset&&!(await confirm('Начать новый период и обнулить расход этого пользователя?',{title:'Начать новый период?',action:'Обнулить расход'}))){submitting.current=false;return;}
  setBusy(true);
  try{
   const result=await api<{runtime_state:string}>(`/api/users/${user.id}/editor`,'PUT',{name:name.trim(),preferences:p,expected_version:initial.preferences.version,expected_enabled:initial.enabled,enabled,package:packageChanged?{quota_bytes:bytes,expires_at:timestamp,expected_version:initial.package.version,reset_usage:reset,schedule:scheduleChanged?{frequency,timezone,...(frequency==='weekly'?{weekday}:{}),...(frequency==='monthly'?{day}:{})}:null}:null});
   setSaved(true);
   // Keep the committed result visible until the administrator acknowledges pending application.
   if(result.runtime_state==='applied') {try{await onSaved();onClose()}catch{setError('Изменение применено, список не обновлён. Повторите загрузку данных.')}}
   else setError('Все настройки сохранены. Применение к VPN-службам ещё не подтверждено; панель повторит попытку.');
  }catch(e){setError(e instanceof Error?e.message:'Не удалось сохранить пользователя');if(e instanceof ApiError&&(e.status===409||e.outcomeUnknown))setBlocked(true)}finally{setBusy(false);submitting.current=false}
 }


 return <>
 {navigation}
 <form className="sectioned-user-form" onSubmit={save} onInvalidCapture={event=>{revealInvalidField(event,setSection);setError('Проверьте обязательные поля в открытом разделе.')}}>
 <fieldset disabled={busy||saved} className="user-editor-fields"><div className="dialog-body">
 <div data-editor-section="profile" hidden={section!=='profile'}><SettingsSection title="Основное" description="Имя пользователя и разрешение доступа."><div className="field-row"><Field label="Имя пользователя" value={name} required maxLength={60} disabled={busy} onChange={e=>setName(e.target.value)}/><div className="toggle-row"><div><strong>Доступ</strong><small>Разрешить подключения пользователя</small></div><SwitchButton checked={enabled} disabled={busy} label="Доступ пользователя" onClick={()=>setEnabled(value=>!value)}/></div></div></SettingsSection></div>
 <div data-editor-section="package" hidden={section!=='package'}><SettingsSection title="Пакет и срок" description="Ограничения применяются к суммарному трафику пользователя."><div className="toggle-row"><div><strong>Без лимита трафика</strong><small>Отключите, чтобы указать объём пакета</small></div><SwitchButton checked={unlimited} disabled={busy} label="Без лимита трафика" onClick={()=>setUnlimited(value=>!value)}/></div><div className="field-row">{!unlimited&&<Field label="Пакет, ГБ" inputMode="decimal" value={quota} required onChange={e=>setQuota(e.target.value)} description="Загрузка и отправка вместе. 1 ГБ = 1 000 000 000 байт."/>}<Field label="Действует до" type="datetime-local" value={expires} onChange={e=>setExpires(e.target.value)} description="Пусто — без срока. Время вашего браузера."/></div><Advanced title="Расписание сброса трафика"><PackageScheduleFields value={schedule} onChange={patch=>setSchedule(s=>({...s,...patch}))} nextAt={!scheduleChanged?initial.package.reset_schedule?.next_at:undefined}/></Advanced><Advanced title="Начать новый период"><label className="connection-choice"><input type="checkbox" checked={reset} onChange={e=>setReset(e.target.checked)}/><span>Начать новый период и обнулить расход пакета</span></label><small>Новый период требует свежих счётчиков. Ключи и подписка сохраняются. Ограничения IP/сессий здесь пока недоступны.</small></Advanced></SettingsSection></div>
 <div data-editor-section="connections" hidden={section!=='connections'}><SettingsSection title="Выдача доступа" description="Выберите способ подключения и протоколы, доступные этому пользователю.">
 <div className="segmented-control delivery-modes" role="group" aria-label="Предпочтительный способ выдачи"><Button type="button" aria-pressed={p.access_mode!=='direct'} disabled={busy} onClick={()=>update({access_mode:'subscription'})}>Подписка</Button><Button type="button" aria-pressed={p.access_mode==='direct'} disabled={busy||!chosen.length} onClick={()=>update({access_mode:'direct',direct_kind:chosen.includes(p.direct_kind)?p.direct_kind:chosen[0]})}>Прямое подключение</Button></div>
 <p className="muted text-xs">Способ выдачи не отзывает действующие ключи и подписку. Доступ включён только для выбранных ниже протоколов.</p>
 <section><div className="section-heading"><h3>Разрешённые подключения</h3><Button type="button" className="small" disabled={busy||vpn.isPending||profiles.isPending||vpn.isError||profiles.isError} onClick={()=>update({allowed_kinds:null})}>Все, включая новые</Button></div>
 {vpn.isError||profiles.isError?<Notice>Не удалось загрузить подключения. Изменение выбора недоступно.</Notice>:vpn.isPending||profiles.isPending?<Loading/>:<ConnectionChoiceList labels={labels} options={options} value={p.allowed_kinds} disabled={busy} onChange={selectConnections}/>}
 <p className="muted text-xs">{p.allowed_kinds===null?'Все текущие и будущие протоколы.':'Только выбранные протоколы. Новые автоматически не добавляются.'} Изменение состава может кратко прервать соединения. Для нод выбор пока недоступен.</p></section>
 {p.allowed_kinds?.length===0&&<Notice>Ни одного подключения не выбрано. После сохранения и применения к службам доступ ко всем протоколам будет отозван.</Notice>}
 {p.access_mode==='direct'&&<label className="field"><span>Профиль для прямой ссылки</span><select value={p.direct_kind} disabled={busy} onChange={e=>update({direct_kind:e.target.value})}>{chosen.map(kind=><option key={kind} value={kind}>{labels[kind]??kind}</option>)}</select></label>}
 </SettingsSection></div>

 <div data-editor-section="profile" hidden={section!=='profile'}><SettingsSection title="Подписка и заметки"><div className="field-row"><Field label="Название подписки" value={p.title} maxLength={25} disabled={busy} onChange={e=>update({title:e.target.value})} description={`В приложении: ${p.title||`ArcNet · ${name}`}`}/><Field label="Группа" value={p.group} maxLength={50} disabled={busy} onChange={e=>update({group:e.target.value})}/></div><Field label="Комментарий" value={p.comment} maxLength={220} disabled={busy} onChange={e=>update({comment:e.target.value})}/></SettingsSection></div>
 <div data-editor-section="activity" hidden={section!=='activity'}>{activity}</div>
 {changed&&<details className="profile-advanced edit-change-summary"><summary>Посмотреть изменения</summary><section className="notice" aria-label="План изменений"><strong>Будет сохранено</strong>{name!==initial.name&&<div>Имя: {initial.name} → {name}</div>}{enabled!==initial.enabled&&<div>Доступ: {enabled?'включить':'отключить'}</div>}{packageChanged&&<div>Пакет: {unlimited?'без лимита':`${quota} ГБ`}; срок: {expires?new Date(expires).toLocaleString('ru-RU'):'без срока'}{reset?'; расход обнулится':''}{scheduleChanged?`; расписание: ${frequency==='none'?'выключить':`${resetScheduleLabel(schedule)}, ${timezone}`}`:''}</div>}{JSON.stringify(p)!==JSON.stringify(initial.preferences)&&<div>Настройки подписки и выбранные подключения обновятся.</div>}</section></details>}
 </div></fieldset>
 {(error||stale||blocked)&&<div className="editor-operation-error">{error&&(saved?<p role="status">{error}</p>:<p className="error" role="alert">{error}</p>)}{!saved&&(stale||blocked)&&<p className="muted">Сверьте данные перед повтором: закройте редактор и откройте актуальную версию. Ваш черновик сохранён в этом окне. <Button type="button" className="small" pending={checking} onClick={onCheck}>Сверить с сервером</Button></p>}</div>}
 <footer className="dialog-footer"><Button type="button" disabled={busy} onClick={async()=>{if(saved){try{await onSaved();onClose()}catch{setError('Изменение сохранено, список не обновлён. Повторите загрузку данных.')}}else if(!changed||await confirm('Закрыть без сохранения изменений?',{title:'Несохранённые изменения',action:'Закрыть без сохранения'}))onClose()}}>{saved?'Закрыть':'Отмена'}</Button><Button type="submit" variant="primary" pending={busy} disabled={saved||blocked||stale||!changed||!name.trim()}>Сохранить изменения</Button></footer></form></>
}
