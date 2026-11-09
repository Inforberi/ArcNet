import {useConfirmation} from '../../shared/Confirmation';
import {versionLabel,publishedLabel} from './version-label';
import {useEffect,useRef,useState} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {api,ApiError,uploadSignedUpdate} from '../../shared/api';
import {Button,Loading,Panel,ErrorState} from '../../shared/ui';

type Download={version:string;state:'downloading'|'verified'|'failed';message?:string};
type Status={download?:Download;available:boolean;trusted:boolean;staged:boolean;version?:string;current_version:string;applying?:boolean;result?:string;started_at?:number;message?:string};
type Release={version:string;platform:string;notes:string;size:number;published_at?:string;installable?:boolean;reason?:string;prerelease?:boolean};
type Catalog={items:Release[];history?:Release[];history_partial?:boolean};
type Pending={version:string;until:number;started_at:number;unknown?:boolean};
const pendingKey='arcnet.pending-update';
function readPending():Pending|null{try{const p=JSON.parse(sessionStorage.getItem(pendingKey)??'null') as Pending|null;return p&&typeof p.version==='string'&&p.version.length<65&&Number.isFinite(p.started_at)&&Number.isFinite(p.until)&&p.until>0&&p.until<Date.now()+190000?{...p,unknown:p.unknown||p.until<Date.now()}:null}catch{return null}}
export default function UpdateSettings({active}:{active:boolean}){
 const confirm=useConfirmation(),cache=useQueryClient(),operation=useRef(false);
 const [pending,setPending]=useState<Pending|null>(readPending),[busy,setBusy]=useState(false),[selection,setSelection]=useState(''),[file,setFile]=useState<File|null>(null),[error,setError]=useState(''),[message,setMessage]=useState(''),[historyOpen,setHistoryOpen]=useState(false);
 const state=useQuery({queryKey:['update-state'],queryFn:()=>api<Status>('/api/system/update'),enabled:active||!!pending,retry:false,refetchInterval:q=>pending?.unknown?false:pending||q.state.data?.applying||q.state.data?.download?.state==='downloading'?2000:false,refetchIntervalInBackground:false});
 const releases=useQuery({queryKey:['update-releases'],queryFn:()=>api<Catalog>('/api/system/update/releases'),enabled:active&&!!state.data?.trusted,staleTime:60000,retry:false,refetchOnWindowFocus:false});
 const publicHistory=useQuery({queryKey:['update-history'],queryFn:()=>api<{items:Release[]}>('/api/system/update/history'),enabled:active&&historyOpen,staleTime:60000,retry:false,refetchOnWindowFocus:false});
 const preview=(item:Release)=>item.prerelease??(item.version.split('+')[0].includes('-')&&!/-stage\d+-a\d+$/.test(item.version));
 const items=releases.data?.items??[],recommended=items.find(item=>!preview(item))??items[0];
 const selected=items.find(item=>item.version===selection)??recommended;
 const trusted=!!state.data?.available&&!!state.data.trusted;
 const downloading=state.data?.download?.state==='downloading',disabled=busy||!!pending||!!state.data?.applying||downloading;
 function remember(value:Pending|null){setPending(value);try{if(value)sessionStorage.setItem(pendingKey,JSON.stringify(value));else sessionStorage.removeItem(pendingKey)}catch{/* Server state remains authoritative. */}}
 useEffect(()=>{
  if(!pending){if(state.data?.applying)remember({version:state.data.version??'',until:Date.now()+180000,started_at:state.data.started_at??0});return}
  const s=state.data;
  if(s?.available&&!s.applying&&pending.version&&s.current_version===pending.version&&s.result==='success'){remember(null);setError('');setMessage(`Установлена версия ${versionLabel(s.current_version)}`);void cache.invalidateQueries({queryKey:['update-releases']});return}
  if(s?.available&&!s.applying&&(s.started_at??0)>pending.started_at&&s.result&&s.result!=='success'){remember(null);setError(`Установка завершилась ошибкой. Сейчас работает ${versionLabel(s.current_version)}. Проверьте журнал обновления; восстановление данных не подтверждено.`);return}
  if(pending.unknown)return;
  const timer=setTimeout(()=>remember({...pending,unknown:true}),Math.max(0,pending.until-Date.now()));return ()=>clearTimeout(timer);
 },[pending,state.data,cache]);
 async function perform(work:()=>Promise<void>){if(operation.current)return;operation.current=true;setBusy(true);setError('');setMessage('');try{await work()}catch(e){setError(e instanceof Error?e.message:'Операция не подтверждена');void state.refetch()}finally{operation.current=false;setBusy(false)}}
 async function prepare(manual=false){if(disabled)return;await perform(async()=>{if(manual){if(!file)return;await uploadSignedUpdate(file)}else{if(!selected)return;await api('/api/system/update/download','POST',{version:selected.version})}await state.refetch()})}
 async function install(){
  const version=state.data?.version;if(!version||disabled||operation.current)return;
  operation.current=true;
  const accepted=await confirm(`Установить ${versionLabel(version)}? Панель перезапустится. Сохраните резервную копию. Откат бинарника не заменяет восстановление базы данных.`,{title:'Установить обновление?',action:'Установить'});
  operation.current=false;if(!accepted)return;
  await perform(async()=>{
   remember({version,until:Date.now()+180000,started_at:state.data?.started_at??0});
   try{const reply=await api<{accepted:boolean;version:string;started_at:number}>('/api/system/update/apply','POST',{confirm:'INSTALL_SIGNED_UPDATE',version});remember({version:reply.version,until:Date.now()+180000,started_at:reply.started_at})}
   catch(e){if(e instanceof ApiError&&[400,403,409,422].includes(e.status))remember(null);throw e}
   finally{void state.refetch()}
  });
 }
 const signedHistory=releases.data?.history??items.map(item=>({...item,installable:true}));
 const all=new Map<string,Release>();
 for(const item of publicHistory.data?.items??[])all.set(item.version,{...item,installable:false,reason:'Нет в проверенном каталоге. Установка недоступна.'});
 for(const item of signedHistory)all.set(item.version,{...item,published_at:item.published_at||all.get(item.version)?.published_at});
 if(state.data&&!all.has(state.data.current_version))all.set(state.data.current_version,{version:state.data.current_version,notes:'',size:0,platform:'',installable:false,reason:'Установлена сейчас'});
 return <Panel title="Обновления"><div className="panel-body update-settings">
  <div className="update-current"><div><span className="muted">Установленная версия</span><strong>{state.data?versionLabel(state.data.current_version):'—'}</strong></div><Button pending={state.isFetching||releases.isFetching} onClick={()=>{void state.refetch();if(!pending)void releases.refetch()}}>{pending?'Проверить состояние':'Проверить обновления'}</Button></div>
  {state.isPending&&<Loading/>}
  {state.isError&&(pending?<p role="status" className="muted">Панель временно недоступна. Проверяем возвращение службы.</p>:<ErrorState error={state.error} onRetry={()=>void state.refetch()}/>)}
  {state.data&&!trusted&&<p role="status">{state.data.message??'Доверенный обработчик обновлений недоступен. Проверьте первоначальную установку.'}</p>}
  {releases.isError&&<ErrorState error={releases.error} onRetry={()=>void releases.refetch()}/>}
  {releases.data?.history_partial&&!releases.isError&&<p className="notice">Новый канал версий недоступен. Показан проверенный переходный каталог; актуальность выпуска не подтверждена.</p>}
  {trusted&&!state.data?.staged&&!pending&&!downloading&&(selected?<section className="update-release" aria-label="Доступное обновление"><h3>{selected.version===recommended?.version?'Доступно обновление':'Выбранный выпуск'} · {versionLabel(selected.version)}</h3>{preview(selected)&&<p className="notice">Предварительная версия. Устанавливайте после проверки резервной копии.</p>}<p className="update-notes">{selected.notes||'Описание отсутствует'}</p>{selected.published_at&&<small>{publishedLabel(selected.published_at)}</small>}<Button variant="primary" pending={busy} disabled={disabled} onClick={()=>void prepare()}>Скачать обновление</Button></section>:!releases.isPending&&!releases.isError&&<p role="status">Совместимых обновлений в полученном каталоге нет.</p>)}
  {downloading&&<div className="update-progress" role="status"><p>Скачиваем и проверяем {versionLabel(state.data!.download!.version)}…</p><Button pending={busy} onClick={()=>void perform(async()=>{await api('/api/system/update/download/cancel','POST',{});await state.refetch()})}>Отменить загрузку</Button></div>}
  {state.data?.staged&&!pending&&!downloading&&<div className="update-release"><p role="status">Версия <strong>{state.data.version?versionLabel(state.data.version):'Не определена'}</strong> проверена и готова к установке.</p><div className="flex wrap"><Button variant="primary" disabled={disabled||!state.data.version} onClick={()=>void install()}>Установить обновление</Button><Button disabled={disabled} onClick={()=>void perform(async()=>{await api('/api/system/update/discard','POST',{});setMessage('Подготовленный пакет удалён');await state.refetch()})}>Отменить подготовку</Button></div></div>}
  {pending&&(pending.unknown?<p role="alert" className="error">Результат установки {versionLabel(pending.version)} не подтверждён за 3 минуты. Автоматическое ожидание остановлено. Проверьте состояние и журнал; повторная установка заблокирована.</p>:<p role="status">Устанавливаем {versionLabel(pending.version)}, ожидаем перезапуск и проверяем фактическую версию…</p>)}
  {state.data?.download?.state==='failed'&&<p className="error" role="alert">{state.data.download.message}</p>}
  {error&&<p className="error" role="alert">{error}</p>}{message&&<p className="success" role="status">{message}</p>}
  <details className="profile-advanced" open={historyOpen} onToggle={event=>setHistoryOpen(event.currentTarget.open)}><summary>Другие версии и история</summary><div className="profile-settings">
   {publicHistory.isPending&&historyOpen&&<Loading/>}{publicHistory.isError&&<ErrorState error={publicHistory.error} onRetry={()=>void publicHistory.refetch()}/>}
   <p className="muted">История GitHub не разрешает установку. Возврат на старую версию заблокирован без проверенного восстановления данных.</p>
   {all.size===0?<p>История пока недоступна.</p>:<ul className="update-history">{[...all.values()].map(item=><li key={item.version}><div className="update-history-heading"><strong>{versionLabel(item.version)}</strong><span className="muted">{item.version===state.data?.current_version?'Установлена':item.installable?'Можно обновить':item.reason}</span></div><small className="muted">{publishedLabel(item.published_at)}</small><p className="update-history-summary">{item.notes?item.notes.split('\n').find(line=>line.trim())?.slice(0,140):'Описание отсутствует'}</p>{item.installable&&<Button className="small" disabled={disabled||!!state.data?.staged} onClick={()=>setSelection(item.version)}>Выбрать этот выпуск</Button>}<details className="usage-details"><summary>Описание и технические сведения</summary><p className="update-notes">{item.notes||'Описание отсутствует'}</p><code className="version-build">{item.version}</code></details></li>)}</ul>}
  </div></details>
  <details className="profile-advanced"><summary>Загрузить подписанный ZIP вручную</summary><div className="profile-settings"><label className="field"><span>Пакет обновления</span><input type="file" accept=".zip,application/zip" disabled={disabled} onChange={e=>setFile(e.target.files?.[0]??null)}/></label><Button disabled={disabled||!file||!!state.data?.staged} onClick={()=>void prepare(true)}>Проверить и загрузить файл</Button><small>Только пакет из доверенного каталога. Архив исходников не подходит.</small></div></details>
  {state.data&&<details className="usage-details"><summary>Технические сведения и диагностика</summary><dl className="properties-list"><div><dt>Идентификатор установленного релиза</dt><dd><code>{state.data.current_version}</code></dd></div><div><dt>Последний результат службы</dt><dd>{state.data.result||'Не получен'}</dd></div></dl></details>}
 </div></Panel>;
}
