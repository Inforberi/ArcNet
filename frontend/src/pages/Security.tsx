import {useConfirmation} from '../shared/Confirmation';
import {useState, type FormEvent} from 'react';
import {useMutation,useQuery,useQueryClient} from '@tanstack/react-query';
import {api} from '../shared/api';
import AccountSettings from '../features/users/AccountSettings';
import {Link} from 'react-router';
import InterfaceSettings from '../features/settings/InterfaceSettings';
import {Button,Field,Loading,Notice,PageHeading,Panel,SettingsLayout} from '../shared/ui';

type LogSettings={budget_mb:number;used_bytes:number;scope:string;xray_journal_managed:boolean};
type LogTail={text:string;available:boolean};
type JournalSettings={budget_mb:number;scope:string;system_journal_unchanged:boolean};
type FirewallRule={port:number;transport:'tcp'|'udp'};
type FirewallEndpoint=FirewallRule&{kind:string;enabled:boolean};
type FirewallState={available:boolean;writable?:boolean;active?:boolean;rules?:FirewallRule[];endpoints:FirewallEndpoint[];message:string};
type MFAStatus={enabled:boolean};
type MFASetup={secret:string;uri:string;qr_svg_base64:string;expires_in:string};
export default function Security(){
 const confirm=useConfirmation();
 const queryClient=useQueryClient();
 const [section,setSection]=useState<'account'|'server'|'interface'>('account');
 const query=useQuery({queryKey:['log-settings'],queryFn:()=>api<LogSettings>('/api/settings/logs'),enabled:section==='server'});
 const firewall=useQuery({queryKey:['firewall'],queryFn:()=>api<FirewallState>('/api/settings/firewall'),enabled:section==='server'});
 const firewallMutation=useMutation({mutationFn:(value:{port:number;transport:string;allow:boolean})=>api('/api/settings/firewall','POST',value),onSuccess:()=>queryClient.invalidateQueries({queryKey:['firewall']})});
 const mfa=useQuery({queryKey:['mfa-status'],queryFn:()=>api<MFAStatus>('/api/security/mfa'),enabled:section==='account'});
 const journal=useQuery({queryKey:['journal-settings'],queryFn:()=>api<JournalSettings>('/api/settings/journal'),retry:false,enabled:section==='server'});
 const [journalLimit,setJournalLimit]=useState<number|null>(null);
 const updateJournal=useMutation({mutationFn:(value:number)=>api('/api/settings/journal','POST',{budget_mb:value}),onSuccess:()=>queryClient.invalidateQueries({queryKey:['journal-settings']})});
 const[limit,setLimit]=useState<number|null>(null);const [service,setService]=useState('xray');const [serviceTail,setServiceTail]=useState<string|null>(null);
 const[errors,setErrors]=useState<Record<string,string>>({});const error=errors[section]??'';
 function setError(message:string){setErrors(previous=>({...previous,[section]:message}))}
 const[message,setMessage]=useState('');const[tail,setTail]=useState<string|null>(null);
 const[password,setPassword]=useState('');const[code,setCode]=useState('');const[setup,setSetup]=useState<MFASetup|null>(null);
 const[authBusy,setAuthBusy]=useState(false);
 const update=useMutation({mutationFn:(v:number)=>api('/api/settings/logs','POST',{budget_mb:v}),onSuccess:()=>queryClient.invalidateQueries({queryKey:['log-settings']})});
 const clear=useMutation({mutationFn:()=>api('/api/settings/logs/clear','POST',{}),onSuccess:()=>{setTail('Журнал очищен');queryClient.invalidateQueries({queryKey:['log-settings']})}});
 async function showTail(){setError('');try{const result=await api<LogTail>('/api/settings/logs/tail');setTail(result.text||'Журнал пуст')}catch(err){setError(err instanceof Error?err.message:'Ошибка чтения журнала')}}
 async function startMFA(e:FormEvent){e.preventDefault();setAuthBusy(true);setError('');setMessage('');try{setSetup(await api<MFASetup>('/api/security/mfa/setup','POST',{password}));setCode('')}catch(err){setError(err instanceof Error?err.message:'Ошибка настройки 2FA')}finally{setAuthBusy(false)}}
 async function finishMFA(e:FormEvent){e.preventDefault();setAuthBusy(true);setError('');try{await api('/api/security/mfa/confirm','POST',{password,code});setMessage('2FA включена. Сессии завершены. Для нового входа дождись следующего 30-секундного кода.');setPassword('');setCode('');setSetup(null);await queryClient.invalidateQueries({queryKey:['session']});await queryClient.invalidateQueries({queryKey:['mfa-status']})}catch(err){setError(err instanceof Error?err.message:'Неверный код 2FA')}finally{setAuthBusy(false)}}
 async function disableMFA(e:FormEvent){e.preventDefault();if(!(await confirm('Отключить второй фактор и завершить все административные сессии?')))return;setAuthBusy(true);setError('');try{await api('/api/security/mfa/disable','POST',{password,code});setPassword('');setCode('');await queryClient.invalidateQueries({queryKey:['session']})}catch(err){setError(err instanceof Error?err.message:'Не удалось отключить 2FA')}finally{setAuthBusy(false)}}
 return <>
  <PageHeading title="Настройки" description="Аккаунт, интерфейс и обслуживание сервера"/>
  <SettingsLayout navigation={<nav className="settings-navigation" aria-label="Разделы настроек">
   {([{id:'account',label:'Аккаунт'},{id:'interface',label:'Интерфейс'},{id:'server',label:'Сервер'}] as const).map(item=><Button key={item.id} aria-pressed={section===item.id} variant="ghost" onClick={()=>{setSection(item.id);setMessage('');}}>{item.label}</Button>)}
   <Link to="/versions" className="button ghost">Версии ArcNet</Link>
  </nav>}>
  {error&&<p className="error" role="alert">{error}</p>}
  {message&&<Notice>{message}</Notice>}
  <section aria-label="Аккаунт и безопасность" hidden={section!=='account'}>
  <AccountSettings/>
  <Panel title="Состояние панели"><div className="panel-body">
   <div className="kv"><span>Соединение</span><span>{location.protocol==='https:'?'HTTPS':'Локальный HTTP'}</span></div>
   <div className="kv"><span>Сессии</span><span>Защищённые серверные сессии</span></div>
   <div className="kv"><span>Защита входа</span><span>Ограничение попыток</span></div>
   <div className="kv"><span>Двухфакторная авторизация</span><span>{mfa.isPending?'Загрузка…':mfa.isError?'Недоступно':mfa.data.enabled?'Включена':'Выключена'}</span></div>
  </div></Panel>
  <Panel title="Двухфакторная авторизация (TOTP)"><div className="panel-body">
   <p className="muted">Совместима с приложениями, поддерживающими шестизначные TOTP-коды. После включения или отключения все текущие сессии завершаются. Восстановить доступ можно с VPS через <code>arcvpn-core admin-reset</code>.</p>
   {mfa.isPending?<Loading/>:mfa.isError?<Notice>{mfa.error.message}</Notice>:<>
    {!mfa.data.enabled&&!setup&&<form onSubmit={startMFA}>
     <Field label="Текущий пароль администратора" type="password" autoComplete="current-password" value={password} onChange={e=>setPassword(e.target.value)} required/>
     <Button type="submit" variant="primary" disabled={authBusy}>Начать настройку 2FA</Button>
    </form>}
    {!mfa.data.enabled&&setup&&<form onSubmit={finishMFA}>
     <p className="muted">Добавь ArcNet в приложение-аутентификатор. Секрет показан только при настройке, действителен 10 минут. Не передавай QR третьим лицам.</p>
     <img width={200} height={200} alt="QR-код для подключения приложения-аутентификатора" src={`data:image/svg+xml;base64,${setup.qr_svg_base64}`}/>
     <p className="muted">Секрет для ручного ввода:</p><code className="break-all">{setup.secret}</code>
     <Field label="Шестизначный код" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} value={code} onChange={e=>setCode(e.target.value)} required/>
     <div className="flex wrap"><Button variant="primary" type="submit" disabled={authBusy}>Подтвердить и включить</Button><Button type="button" onClick={()=>{setSetup(null);setCode('')}}>Отмена</Button></div>
    </form>}
    {mfa.data.enabled&&<form onSubmit={disableMFA}>
     <Field label="Текущий пароль администратора" type="password" autoComplete="current-password" value={password} onChange={e=>setPassword(e.target.value)} required/>
     <Field label="Код приложения-аутентификатора" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} value={code} onChange={e=>setCode(e.target.value)} required/>
     <Button variant="danger" disabled={authBusy} type="submit">Отключить 2FA</Button>
    </form>}
   </>}
  </div></Panel>
  </section>
  <section aria-label="Настройки сервера" hidden={section!=='server'}>
  <Panel title="Ограничение файлов журнала Go-панели"><div className="panel-body">
   <p className="muted">По умолчанию 500 МБ. Ротация касается только локальных файлов журнала Go-панели, не Xray и не systemd-journald.</p>
   {query.isPending?<Loading/>:query.isError?<Notice>{query.error.message}</Notice>:<>
    <label className="field"><span>Максимальный объём</span><select value={limit??query.data.budget_mb} onChange={e=>setLimit(Number(e.target.value))}>{[50,100,200,500,1024].map(n=><option key={n} value={n}>{n} МБ</option>)}</select></label>
    <p className="muted">Использовано: {(query.data.used_bytes/1048576).toFixed(2)} МБ</p>
    <div className="flex wrap">
     <Button variant="primary" disabled={update.isPending} onClick={async()=>{setError('');try{await update.mutateAsync(limit??query.data.budget_mb)}catch(e){setError(e instanceof Error?e.message:'Ошибка')}}}>Сохранить</Button>
     <Button onClick={()=>void showTail()}>Показать последние строки</Button>
     <Button variant="danger" disabled={clear.isPending} onClick={async()=>{if(!(await confirm('Удалить файлы журналов панели?')))return;setError('');try{await clear.mutateAsync()}catch(e){setError(e instanceof Error?e.message:'Ошибка')}}}>Очистить</Button>
    </div>
   </>}
   {tail!==null&&<pre className="log-view">{tail}</pre>}
  </div></Panel>
  <Panel title="Лимит изолированного systemd-журнала ArcNet"><div className="panel-body">
    <p className="muted">Отдельное пространство systemd-journald для сервисов ArcNet. Журнал ОС и SSH не затрагивается. По умолчанию 200 МБ. Системные изменения доступны только после установки на VPS.</p>
    {journal.isPending?<Loading/>:journal.isError?<Notice>Управление журналом недоступно в локальном режиме без root-обработчика.</Notice>:<>
     <label className="field"><span>Общий лимит журнала служб</span><select value={journalLimit??journal.data.budget_mb} onChange={e=>setJournalLimit(Number(e.target.value))}>
     {[50,100,200,300,500,1024].map(n=><option key={n} value={n}>{n} МБ</option>)}
     </select></label>
     <Button type="button" variant="primary" disabled={updateJournal.isPending} onClick={async()=>{setError('');try{await updateJournal.mutateAsync(journalLimit??journal.data.budget_mb)}catch(e){setError(e instanceof Error?e.message:'Ошибка настройки журнала')}}}>Применить лимит журнала служб</Button>
    </>}
   </div></Panel>
  <Panel title="Журналы служб ArcNet"><div className="panel-body">
    <p className="muted">Показываем последние записи только выбранной службы ArcNet через ограниченный системный обработчик. Это просмотр, а не управление размером journald.</p>
    <label className="field"><span>Служба</span><select value={service} onChange={e=>{setService(e.target.value);setServiceTail(null)}}>
     {['panel','xray','hysteria','wireguard','amneziawg'].map(s=><option key={s} value={s}>{s}</option>)}
    </select></label>
    <Button onClick={async()=>{setError('');try{const v=await api<{text:string}>(`/api/settings/journal/tail?service=${service}`);setServiceTail(v.text)}catch(e){setError(e instanceof Error?e.message:'Журнал недоступен')}}}>Показать журнал</Button>
    {serviceTail!==null&&<pre className="log-view">{serviceTail}</pre>}
   </div></Panel>
  <Panel title="Firewall: порты VPN" actions={<Button disabled={firewall.isFetching} onClick={()=>void firewall.refetch()}>Обновить состояние</Button>}><div className="panel-body">
   <p className="muted">Управляются только правила UFW, созданные ArcNet. Панель не включает UFW и не меняет SSH или сторонние правила. Если UFW выключен, включение выполняется отдельно после проверки доступа по SSH.</p>
   {firewall.isPending?<Loading/>:firewall.isError?<Notice>{firewall.error.message}</Notice>:<>
    <div className="kv"><span>Состояние UFW</span><span>{!firewall.data.available?'Системный обработчик недоступен':firewall.data.active?'Активен':'Выключен'}</span></div>
    <p className="muted">{firewall.data.message}</p>
    {(firewall.data.endpoints??[]).length===0?<Notice>Сначала настрой VPN-протоколы.</Notice>:(firewall.data.endpoints??[]).map((endpoint,index)=>{
      const open=(firewall.data.rules??[]).some(rule=>rule.port===endpoint.port&&rule.transport===endpoint.transport);
      return <div key={`${endpoint.kind}-${index}`} className="kv">
       <span>{endpoint.kind} · {endpoint.port}/{endpoint.transport} · {endpoint.enabled?'Включён':'Отключён'}</span>
       {firewall.data.writable?<Button disabled={!firewall.data.available||!firewall.data.active||firewallMutation.isPending||(!open&&!endpoint.enabled)} variant={open?'danger':'primary'} onClick={async()=>{
        const action=open?'Закрыть':'Открыть';
        if(!(await confirm(`${action} порт ${endpoint.port}/${endpoint.transport} в UFW?`)))return;
        setError('');
        try{await firewallMutation.mutateAsync({port:endpoint.port,transport:endpoint.transport,allow:!open})}
        catch(e){setError(e instanceof Error?e.message:'Ошибка UFW')}
       }}>{open?'Удалить правило ArcNet':'Добавить правило ArcNet'}</Button>:<span className="muted">{!firewall.data.available?'Состояние неизвестно':open?'Правило ArcNet найдено':'Правило ArcNet не найдено'}</span>}
      </div>
    })}
   </>}
  </div></Panel>
  </section>
  <section aria-label="Настройки интерфейса" hidden={section!=='interface'}><InterfaceSettings/></section>
  </SettingsLayout>
 </>;
}
