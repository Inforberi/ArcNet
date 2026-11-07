import {useState, type FormEvent} from 'react';
import {useMutation,useQuery,useQueryClient} from '@tanstack/react-query';
import {api,uploadSignedUpdate} from '../shared/api';
import AccountSettings from '../features/users/AccountSettings';
import {Button,Field,Loading,Notice,PageHeading,Panel} from '../shared/ui';

type LogSettings={budget_mb:number;used_bytes:number;scope:string;xray_journal_managed:boolean};
type LogTail={text:string;available:boolean};
type JournalSettings={budget_mb:number;scope:string;system_journal_unchanged:boolean};
type FirewallRule={port:number;transport:'tcp'|'udp'};
type FirewallEndpoint=FirewallRule&{kind:string;enabled:boolean};
type FirewallState={available:boolean;active?:boolean;rules?:FirewallRule[];endpoints:FirewallEndpoint[];message:string};
type UpdateState={available:boolean;trusted:boolean;staged:boolean;version?:string;message?:string};
type MFAStatus={enabled:boolean};
type MFASetup={secret:string;uri:string;qr_svg_base64:string;expires_in:string};
export default function Security(){
 const queryClient=useQueryClient();
 const query=useQuery({queryKey:['log-settings'],queryFn:()=>api<LogSettings>('/api/settings/logs')});
 const updateState=useQuery({queryKey:['update-state'],queryFn:()=>api<UpdateState>('/api/system/update')});
 const [signedFile,setSignedFile]=useState<File|null>(null);
 const [uploading,setUploading]=useState(false);
 const [applying,setApplying]=useState(false);
 const firewall=useQuery({queryKey:['firewall'],queryFn:()=>api<FirewallState>('/api/settings/firewall')});
 const firewallMutation=useMutation({mutationFn:(value:{port:number;transport:string;allow:boolean})=>api('/api/settings/firewall','POST',value),onSuccess:()=>queryClient.invalidateQueries({queryKey:['firewall']})});
 const mfa=useQuery({queryKey:['mfa-status'],queryFn:()=>api<MFAStatus>('/api/security/mfa')});
 const journal=useQuery({queryKey:['journal-settings'],queryFn:()=>api<JournalSettings>('/api/settings/journal'),retry:false});
 const [journalLimit,setJournalLimit]=useState<number|null>(null);
 const updateJournal=useMutation({mutationFn:(value:number)=>api('/api/settings/journal','POST',{budget_mb:value}),onSuccess:()=>queryClient.invalidateQueries({queryKey:['journal-settings']})});
 const[limit,setLimit]=useState<number|null>(null);const [service,setService]=useState('xray');const [serviceTail,setServiceTail]=useState<string|null>(null);
 const[error,setError]=useState('');const[message,setMessage]=useState('');const[tail,setTail]=useState<string|null>(null);
 const[password,setPassword]=useState('');const[code,setCode]=useState('');const[setup,setSetup]=useState<MFASetup|null>(null);
 const[authBusy,setAuthBusy]=useState(false);
 const update=useMutation({mutationFn:(v:number)=>api('/api/settings/logs','POST',{budget_mb:v}),onSuccess:()=>queryClient.invalidateQueries({queryKey:['log-settings']})});
 const clear=useMutation({mutationFn:()=>api('/api/settings/logs/clear','POST',{}),onSuccess:()=>{setTail('Журнал очищен');queryClient.invalidateQueries({queryKey:['log-settings']})}});
 async function showTail(){setError('');try{const result=await api<LogTail>('/api/settings/logs/tail');setTail(result.text||'Журнал пуст')}catch(err){setError(err instanceof Error?err.message:'Ошибка чтения журнала')}}
 async function startMFA(e:FormEvent){e.preventDefault();setAuthBusy(true);setError('');setMessage('');try{setSetup(await api<MFASetup>('/api/security/mfa/setup','POST',{password}));setCode('')}catch(err){setError(err instanceof Error?err.message:'Ошибка настройки 2FA')}finally{setAuthBusy(false)}}
 async function finishMFA(e:FormEvent){e.preventDefault();setAuthBusy(true);setError('');try{await api('/api/security/mfa/confirm','POST',{password,code});setMessage('2FA включена. Сессии завершены. Для нового входа дождись следующего 30-секундного кода.');setPassword('');setCode('');setSetup(null);await queryClient.invalidateQueries({queryKey:['session']});await queryClient.invalidateQueries({queryKey:['mfa-status']})}catch(err){setError(err instanceof Error?err.message:'Неверный код 2FA')}finally{setAuthBusy(false)}}
 async function disableMFA(e:FormEvent){e.preventDefault();if(!window.confirm('Отключить второй фактор и завершить все административные сессии?'))return;setAuthBusy(true);setError('');try{await api('/api/security/mfa/disable','POST',{password,code});setPassword('');setCode('');await queryClient.invalidateQueries({queryKey:['session']})}catch(err){setError(err instanceof Error?err.message:'Не удалось отключить 2FA')}finally{setAuthBusy(false)}}
 return <>
  <PageHeading title="Безопасность" description="Авторизация администратора, HTTPS и журналы панели"/>
  <AccountSettings/>
  {error&&<p className="error" role="alert">{error}</p>}
  {message&&<Notice>{message}</Notice>}
  <Panel title="Состояние панели"><div className="panel-body">
   <div className="kv"><span>Соединение</span><span>{location.protocol==='https:'?'HTTPS':'Локальный HTTP'}</span></div>
   <div className="kv"><span>Сессии</span><span>HttpOnly, SameSite=Strict, CSRF</span></div>
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
     <p className="muted">Добавь ArcVPN в приложение-аутентификатор. Секрет показан только при настройке, действителен 10 минут. Не передавай QR третьим лицам.</p>
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
  <Panel title="Ограничение файлов журнала Go-панели"><div className="panel-body">
   <p className="muted">По умолчанию 500 МБ. Ротация касается только локальных файлов журнала Go-панели, не Xray и не systemd-journald.</p>
   {query.isPending?<Loading/>:query.isError?<Notice>{query.error.message}</Notice>:<>
    <label className="field"><span>Максимальный объём</span><select value={limit??query.data.budget_mb} onChange={e=>setLimit(Number(e.target.value))}>{[50,100,200,500,1024].map(n=><option key={n} value={n}>{n} МБ</option>)}</select></label>
    <p className="muted">Использовано: {(query.data.used_bytes/1048576).toFixed(2)} МБ</p>
    <div className="flex wrap">
     <Button variant="primary" disabled={update.isPending} onClick={async()=>{setError('');try{await update.mutateAsync(limit??query.data.budget_mb)}catch(e){setError(e instanceof Error?e.message:'Ошибка')}}}>Сохранить</Button>
     <Button onClick={()=>void showTail()}>Показать последние строки</Button>
     <Button variant="danger" disabled={clear.isPending} onClick={async()=>{if(!window.confirm('Удалить файлы журналов панели?'))return;setError('');try{await clear.mutateAsync()}catch(e){setError(e instanceof Error?e.message:'Ошибка')}}}>Очистить</Button>
    </div>
   </>}
   {tail!==null&&<pre className="log-view">{tail}</pre>}
  </div></Panel>
  <Panel title="Лимит изолированного systemd-журнала ArcVPN"><div className="panel-body">
    <p className="muted">Отдельное пространство systemd-journald для сервисов ArcVPN. Журнал ОС и SSH не затрагивается. По умолчанию 200 МБ. Системные изменения доступны только после установки на VPS.</p>
    {journal.isPending?<Loading/>:journal.isError?<Notice>Управление журналом недоступно в локальном режиме без root-обработчика.</Notice>:<>
     <label className="field"><span>Общий лимит журнала служб</span><select value={journalLimit??journal.data.budget_mb} onChange={e=>setJournalLimit(Number(e.target.value))}>
     {[50,100,200,300,500,1024].map(n=><option key={n} value={n}>{n} МБ</option>)}
     </select></label>
     <Button type="button" variant="primary" disabled={updateJournal.isPending} onClick={async()=>{setError('');try{await updateJournal.mutateAsync(journalLimit??journal.data.budget_mb)}catch(e){setError(e instanceof Error?e.message:'Ошибка настройки журнала')}}}>Применить лимит журнала служб</Button>
    </>}
   </div></Panel>
  <Panel title="Журналы служб ArcVPN"><div className="panel-body">
    <p className="muted">Показываем последние записи только выбранной службы ArcVPN через ограниченный системный обработчик. Это просмотр, а не управление размером journald.</p>
    <label className="field"><span>Служба</span><select value={service} onChange={e=>{setService(e.target.value);setServiceTail(null)}}>
     {['panel','xray','hysteria','wireguard','amneziawg'].map(s=><option key={s} value={s}>{s}</option>)}
    </select></label>
    <Button onClick={async()=>{setError('');try{const v=await api<{text:string}>(`/api/settings/journal/tail?service=${service}`);setServiceTail(v.text)}catch(e){setError(e instanceof Error?e.message:'Журнал недоступен')}}}>Показать журнал</Button>
    {serviceTail!==null&&<pre className="log-view">{serviceTail}</pre>}
   </div></Panel>
  <Panel title="Firewall: порты VPN"><div className="panel-body">
   <p className="muted">Управляются только правила UFW, созданные ArcVPN. Панель не включает UFW и не меняет SSH или сторонние правила. Если UFW выключен, включение выполняется отдельно после проверки доступа по SSH.</p>
   {firewall.isPending?<Loading/>:firewall.isError?<Notice>{firewall.error.message}</Notice>:<>
    <div className="kv"><span>Состояние UFW</span><span>{!firewall.data.available?'Системный обработчик недоступен':firewall.data.active?'Активен':'Выключен'}</span></div>
    <p className="muted">{firewall.data.message}</p>
    {(firewall.data.endpoints??[]).length===0?<Notice>Сначала настрой VPN-протоколы.</Notice>:(firewall.data.endpoints??[]).map((endpoint,index)=>{
      const open=(firewall.data.rules??[]).some(rule=>rule.port===endpoint.port&&rule.transport===endpoint.transport);
      return <div key={`${endpoint.kind}-${index}`} className="kv">
       <span>{endpoint.kind} · {endpoint.port}/{endpoint.transport} · {endpoint.enabled?'Включён':'Отключён'}</span>
       <Button disabled={!firewall.data.available||!firewall.data.active||firewallMutation.isPending||(!open&&!endpoint.enabled)} variant={open?'danger':'primary'} onClick={async()=>{
        const action=open?'Закрыть':'Открыть';
        if(!window.confirm(`${action} порт ${endpoint.port}/${endpoint.transport} в UFW?`))return;
        setError('');
        try{await firewallMutation.mutateAsync({port:endpoint.port,transport:endpoint.transport,allow:!open})}
        catch(e){setError(e instanceof Error?e.message:'Ошибка UFW')}
       }}>{open?'Закрыть порт':'Открыть порт'}</Button>
      </div>
    })}
   </>}
  </div></Panel>
  <Panel title="Обновление ArcVPN"><div className="panel-body">
   <p className="muted">Устанавливаются только архивы с проверенной подписью Ed25519. Один архив может содержать панель и совместимые VPN-движки. В случае ошибки перезапуска предыдущие бинарники восстанавливаются; реальная проверка восстановления служб требуется на VPS.</p>
   {updateState.isPending?<Loading/>:updateState.isError?<Notice>{updateState.error.message}</Notice>:<>
    <div className="kv"><span>Системная служба</span><span>{updateState.data.available?'Доступна':'Недоступна локально'}</span></div>
    <div className="kv"><span>Доверенный ключ релизов</span><span>{updateState.data.trusted?'Настроен':'Не настроен'}</span></div>
    <div className="kv"><span>Проверенный архив</span><span>{updateState.data.version||'Нет'}</span></div>
    <label className="field"><span>Подписанный архив обновления (.zip)</span><input type="file" accept=".zip,application/zip" onChange={e=>setSignedFile(e.target.files?.[0]??null)}/></label>
    <div className="flex wrap">
     <Button variant="primary" disabled={!updateState.data.available||!updateState.data.trusted||!signedFile||uploading||applying} onClick={async()=>{
       if(!signedFile)return;setError('');setUploading(true);
       try{const result=await uploadSignedUpdate(signedFile);setMessage(`Архив версии ${result.version} проверен и готов к применению`);await queryClient.invalidateQueries({queryKey:['update-state']})}
       catch(e){setError(e instanceof Error?e.message:'Ошибка загрузки обновления')}
       finally{setUploading(false)}
     }}>Проверить и загрузить</Button>
     <Button disabled={!updateState.data.available||!updateState.data.staged||!updateState.data.version||applying} onClick={async()=>{
       if(!window.confirm('Обновить ArcVPN и кратковременно перезапустить панель? При ошибке запуска предыдущая версия будет восстановлена.'))return;
       setApplying(true);setError('');
       try{await api('/api/system/update/apply','POST',{confirm:'INSTALL_SIGNED_UPDATE'});setMessage('Запущено обновление. Страница может временно стать недоступной.');}
       catch(e){setError(e instanceof Error?e.message:'Обновление не запущено')}
       finally{setApplying(false)}
     }}>Установить проверенную версию</Button>
     <Button variant="danger" disabled={!updateState.data.available||!updateState.data.staged||uploading||applying} onClick={async()=>{
       if(!window.confirm('Удалить подготовленный архив обновления, не устанавливая его?'))return;
       setError('');
       try{await api('/api/system/update/discard','POST',{});setSignedFile(null);setMessage('Подготовленный архив удалён');await queryClient.invalidateQueries({queryKey:['update-state']})}
       catch(e){setError(e instanceof Error?e.message:'Не удалось удалить архив')}
     }}>Отменить подготовленное обновление</Button>
    </div>
   </>}
  </div></Panel>
  <Notice>Системные операции требуют установки на Linux с systemd. До проверки в реальном окружении не используйте эту сборку на действующем VPN-сервере.</Notice>
 </>;
}
