import {useEffect,useState} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {api,ApiError,uploadSignedUpdate} from '../../shared/api';
import {Button,Loading,Panel} from '../../shared/ui';

type Download={version:string;state:'downloading'|'verified'|'failed';message?:string};
type Status={download?:Download;available:boolean;trusted:boolean;staged:boolean;version?:string;current_version:string;applying?:boolean;result?:string;started_at?:number;message?:string};
type Release={version:string;platform:string;notes:string;size:number};
type Pending={version:string;until:number;started_at:number};
const pendingKey='arcnet.pending-update';
function readPending():Pending|null{try{const p=JSON.parse(sessionStorage.getItem(pendingKey)??'null') as Pending|null;return p&&typeof p.version==='string'&&p.version.length<65&&typeof p.started_at==='number'&&p.until>Date.now()&&p.until<Date.now()+190000?p:null;}catch{return null;}}

export default function UpdateSettings({active}:{active:boolean}){
 const cache=useQueryClient();
 const [pending,setPending]=useState<Pending|null>(readPending),[busy,setBusy]=useState(false),[selection,setSelection]=useState(''),[file,setFile]=useState<File|null>(null),[error,setError]=useState(''),[message,setMessage]=useState('');
 const state=useQuery({queryKey:['update-state'],queryFn:()=>api<Status>('/api/system/update'),enabled:active||!!pending,retry:false,refetchInterval:q=>pending||q.state.data?.applying||q.state.data?.download?.state==='downloading'?2000:false,refetchIntervalInBackground:false});
 const releases=useQuery({queryKey:['update-releases'],queryFn:()=>api<{items:Release[]}>('/api/system/update/releases'),enabled:active&&!!state.data?.available&&!!state.data.trusted,staleTime:60000,retry:false,refetchOnWindowFocus:false});
 const items=releases.data?.items??[],selected=items.find(item=>item.version===selection)??items[0];
 function remember(value:Pending|null){setPending(value);try{if(value)sessionStorage.setItem(pendingKey,JSON.stringify(value));else sessionStorage.removeItem(pendingKey);}catch{/* Storage is optional; server state remains authoritative. */}}
 useEffect(()=>{
  if(!pending)return;
  const s=state.data;
  if(s?.available&&!s.applying&&s.current_version===pending.version&&s.result==='success'){remember(null);setMessage(`Установлена версия ${s.current_version}`);void cache.invalidateQueries({queryKey:['update-releases']});return;}
  if(s?.available&&!s.applying&&(s.started_at??0)>pending.started_at&&s.result&&s.result!=='success'){remember(null);setError(`Обновление завершилось ошибкой. Сейчас работает версия ${s.current_version}. Проверьте журнал обновления.`);return;}
  const timer=window.setInterval(()=>{if(Date.now()>pending.until){remember(null);setError('Не удалось подтвердить завершение обновления за отведённое время. Проверьте состояние панели и службы.');}},1000);
  return ()=>window.clearInterval(timer);
 },[pending,state.data,cache]);
 const downloading=state.data?.download?.state==='downloading';
 const disabled=busy||!!pending||!!state.data?.applying||downloading;
 async function prepare(manual=false){
  setBusy(true);setError('');setMessage('');
  try{if(manual)await uploadSignedUpdate(file!);else await api('/api/system/update/download','POST',{version:selected!.version});await state.refetch();}
  catch(e){setError(e instanceof Error?e.message:'Не удалось подготовить обновление');await state.refetch();}
  finally{setBusy(false);}
 }
 async function install(){
  const version=state.data?.version;if(!version)return;
  if(!window.confirm(`Установить ${version}? Панель кратковременно перезапустится. При ошибке запуска служба попытается восстановить предыдущие бинарники.`))return;
  setError('');setMessage('');setBusy(true);
  remember({version,until:Date.now()+180000,started_at:state.data?.started_at??0});
  try{const reply=await api<{accepted:boolean;version:string;started_at:number}>('/api/system/update/apply','POST',{confirm:'INSTALL_SIGNED_UPDATE',version});remember({version:reply.version,until:Date.now()+180000,started_at:reply.started_at});}
  catch(e){if(e instanceof ApiError&&[400,403,409,422].includes(e.status))remember(null);setError(e instanceof Error?e.message:'Ответ запуска не получен. Проверяем состояние сервера.');}
  finally{setBusy(false);void state.refetch();}
 }
 return <Panel title="Обновления ArcNet"><div className="panel-body grid gap-4">
  {state.isPending?<Loading/>:state.data?<>
   <div className="kv"><span>Установленная версия</span><strong className="min-w-0 break-all text-right font-mono text-sm">{state.data.current_version}</strong></div>
   {!state.data.available||!state.data.trusted?<p role="status" className="muted">{state.data.message??'Доверенный ключ обновлений не настроен. Нужна проверенная первоначальная установка.'}</p>:<>
    {!state.data.staged&&<>
     <div className="flex flex-wrap items-center justify-between gap-3"><strong>Доступные версии</strong><Button disabled={disabled||releases.isFetching} onClick={()=>{void state.refetch();void releases.refetch();}}>Проверить обновления</Button></div>
     {releases.isPending?<Loading/>:releases.isError?<p role="alert" className="error">{releases.error.message}</p>:selected?<>
      <label className="field"><span>Версия</span><select disabled={disabled} value={selected.version} onChange={e=>setSelection(e.target.value)}>{items.map(item=><option key={item.version} value={item.version}>{item.version}</option>)}</select></label>
      <p className="muted whitespace-pre-wrap break-words">{selected.notes}</p><small className="muted">{selected.platform} · {selected.size<1048576?`${Math.ceil(selected.size/1024)} КиБ`:`${(selected.size/1048576).toFixed(1)} МБ`} · Подписанный пакет панели и интерфейса</small>
      <Button variant="primary" disabled={disabled} onClick={()=>void prepare()}>{busy?'Загружаем и проверяем…':'Загрузить выбранную версию'}</Button>
     </>:<p className="muted">Новых совместимых версий нет.</p>}
    </>}
    {state.data.staged&&<div className="grid gap-3"><p role="status">Подготовлена версия <strong>{state.data.version??'Проверка не пройдена'}</strong></p><div className="flex flex-wrap gap-2"><Button variant="primary" disabled={disabled||!state.data.version} onClick={()=>void install()}>Установить проверенную версию</Button><Button disabled={disabled} onClick={async()=>{if(!window.confirm('Отменить подготовленное обновление?'))return;setBusy(true);setError('');try{await api('/api/system/update/discard','POST',{});setMessage('Подготовка отменена');await state.refetch();}catch(e){setError(e instanceof Error?e.message:'Не удалось отменить');}finally{setBusy(false);}}}>Отменить подготовку</Button></div></div>}
    <details className="profile-advanced"><summary>Загрузить ZIP вручную</summary><div className="profile-settings"><label className="field"><span>Подписанный архив обновления</span><input type="file" accept=".zip,application/zip" disabled={disabled} onChange={e=>setFile(e.target.files?.[0]??null)}/></label><Button disabled={disabled||!file||state.data.staged} onClick={()=>void prepare(true)}>Проверить и загрузить файл</Button></div></details>
   </>}
  </>:null}
  {downloading&&<div className="flex flex-wrap items-center gap-3" role="status"><span>Сервер загружает и проверяет {state.data?.download?.version}…</span><Button disabled={busy} onClick={async()=>{setBusy(true);try{await api('/api/system/update/download/cancel','POST',{});await state.refetch();}catch(e){setError(e instanceof Error?e.message:'Не удалось отменить загрузку');}finally{setBusy(false);}}}>Отменить загрузку</Button></div>}
  {state.data?.download?.state==='failed'&&<p className="error" role="alert">{state.data.download.message}</p>}
  {(pending||state.data?.applying)&&<p role="status">Устанавливаем обновление и проверяем возвращение панели. Это может занять некоторое время.</p>}
  {state.isError&&<p className="muted" role="status">{pending?'Панель перезапускается. Повторяем проверку состояния.':state.error.message}</p>}
  {error&&<p className="error" role="alert">{error}</p>}{message&&<p className="success" role="status">{message}</p>}
  <small className="muted">Сервер сам скачивает пакет из проверенного каталога и повторно проверяет подпись перед установкой. Установка запускается только с вашим подтверждением.</small>
 </div></Panel>;
}
