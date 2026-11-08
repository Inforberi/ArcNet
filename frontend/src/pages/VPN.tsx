import {useState} from 'react';
import {Link} from 'react-router';
import {useMutation,useQuery,useQueryClient} from '@tanstack/react-query';
import {api,ApiError,type VPNConfig,type VPNUser,type ProtocolProfile,type ProtocolHealth,type ProtocolKind} from '../shared/api';
import {RealitySetup,ProtocolSetup,ProtocolChoice,protocolCatalog,type RealityInput,type ProtocolInput} from '../features/connections/ConnectionForms';
import {ConnectionTable,connectionItems,connectionLabel,connectionKey,type ConnectionItem} from '../features/connections/ConnectionTable';
import ConnectionEditor,{type ConnectionSettings} from '../features/connections/ConnectionEditor';
import KeyRotationDialog from '../features/connections/KeyRotationDialog';
import ConnectionDetails from '../features/connections/ConnectionDetails';
import Dialog,{DialogClose} from '../shared/Dialog';
import {Button,Loading,PageHeading,StatStrip} from '../shared/ui';

import {Icon} from '../shared/Icon';

type PendingAction={item:ConnectionItem;remove:boolean};
export default function VPN() {
 const cache=useQueryClient();
 const vpn=useQuery({queryKey:['vpn'],queryFn:()=>api<VPNConfig>('/api/vpn')});
 const profiles=useQuery({queryKey:['protocols'],queryFn:()=>api<{items:ProtocolProfile[]}>('/api/protocols')});
 const users=useQuery({queryKey:['users'],queryFn:()=>api<{items:VPNUser[]}>('/api/users')});
 const health=useQuery({queryKey:['protocol-health'],queryFn:()=>api<{items:ProtocolHealth[]}>('/api/protocols/health')});
 const [creating,setCreating]=useState(false),[kind,setKind]=useState<'vless-reality'|ProtocolKind>('vless-reality');
 const [editing,setEditing]=useState<ConnectionItem>(),[action,setAction]=useState<PendingAction>();
 const [rotation,setRotation]=useState<ConnectionItem>();
 const [detailKind,setDetailKind]=useState<string>(),[pickTarget,setPickTarget]=useState(false);
 const [error,setError]=useState(''),[created,setCreated]=useState(false);
 const configured=new Set(profiles.data?.items.map(p=>p.kind)??[]);
 const available=[{kind:'vless-reality' as const,name:'VLESS + Reality TCP'},...protocolCatalog.filter(p=>p.kind==='vless-xhttp'||!configured.has(p.kind))];
 async function refresh(){await Promise.all(['vpn','protocols','protocol-health','connection-defaults','users','user-devices','user-usage'].map(key=>cache.invalidateQueries({queryKey:[key]})));}
 const create=useMutation({mutationFn:async(input:RealityInput|ProtocolInput)=>{
  if(kind==='vless-reality'&&!vpn.data?.configured)await api('/api/vpn','POST',input);
  else if(kind==='vless-reality')await api('/api/protocols','POST',{...input,kind});
  else await api('/api/protocols','POST',input);
 },onSuccess:async()=>{await refresh();setCreating(false);setCreated(true);}});
 const change=useMutation({mutationFn:({item,settings,remove=false}:{item:ConnectionItem;settings?:ConnectionSettings;remove?:boolean})=>api(`/api/connections/${connectionKey(item)}`,remove?'DELETE':'PUT',remove?{revision:item.revision}:{revision:item.revision,...settings}),onSuccess:async()=>{await refresh();setEditing(undefined);setAction(undefined);}});
 async function submit(input:RealityInput|ProtocolInput){setError('');try{await create.mutateAsync(input);}catch(err){setError(err instanceof Error?err.message:'Не удалось создать протокол');}}
 async function save(item:ConnectionItem,settings?:ConnectionSettings,remove=false){setError('');try{await change.mutateAsync({item,settings,remove});}catch(err){if(err instanceof ApiError&&err.status===409)await refresh();setError(err instanceof Error?err.message:'Не удалось применить изменение');}}
 function openCreate(){setKind(available[0]?.kind??'vless-reality');setError('');setCreated(false);setCreating(true);}
 const busy=create.isPending||change.isPending;
 const items=vpn.data&&profiles.data?connectionItems(vpn.data,profiles.data.items):[];
 const assignments=users.data?new Map(items.map(item=>[connectionKey(item),users.data.items.filter(user=>user.preferences?.allowed_kinds==null||user.preferences.allowed_kinds.includes(connectionKey(item))).length])):undefined;
 const detail=items.find(item=>connectionKey(item)===detailKind);
 function edit(item:ConnectionItem,pick=false){setError('');setDetailKind(undefined);setPickTarget(pick);setEditing(item);setCreated(false)}
 return <>
  <PageHeading title="Подключения" description="Входящие подключения сервера, назначенные пользователи и параметры защиты." actions={<>{available.length>0&&<Button variant="primary" disabled={busy||vpn.isPending||profiles.isPending||vpn.isError||profiles.isError} onClick={openCreate}><Icon name="plus"/>Добавить подключение</Button>}</>}/>
  {created&&<p className="connection-success" role="status">Протокол добавлен. <Link to="/users">Выдать доступ пользователю</Link></p>}
  <StatStrip label="Сводка подключений" items={[{label:'Подключений',value:items.length},{label:'Включены',value:items.filter(item=>item.enabled).length},{label:'Назначено пользователям',value:users.data?users.data.items.filter(user=>items.some(item=>user.preferences?.allowed_kinds==null||user.preferences.allowed_kinds.includes(connectionKey(item)))).length:'—'},{label:'VPN-проверка',value:'Нет данных',tone:'neutral'}]}/>
  <section className="connections-section"><div className="section-heading"><h2>Входящие подключения</h2><small className="muted">Нажмите на подключение, чтобы открыть подробности</small></div>
   {vpn.isPending||profiles.isPending?<Loading/>:vpn.isError||profiles.isError?<div className="panel-body"><p className="error" role="alert">{vpn.error?.message??profiles.error?.message}</p><Button onClick={()=>void refresh()}>Повторить</Button></div>:<ConnectionTable assignments={users.isError?undefined:assignments} onRefresh={item=>edit(item,true)} items={items} health={health.isError?[]:health.data?.items??[]} busy={busy} onDetails={item=>setDetailKind(connectionKey(item))} onEdit={item=>edit(item)} onToggle={item=>{setError('');setAction({item,remove:false});setCreated(false);}} onRemove={item=>{setError('');setAction({item,remove:true});setCreated(false);}}/>}
   {health.isError&&<p className="panel-body muted">Состояние служб недоступно. <Button className="small" onClick={()=>void health.refetch()}>Проверить снова</Button></p>}
  </section>
  {creating&&vpn.data&&<Dialog title="Добавить подключение" busy={create.isPending} onClose={()=>setCreating(false)}>
   <div className="panel-body"><ProtocolChoice kind={kind} available={available.map(p=>p.kind)} configured={new Set([...configured,...(vpn.data.configured?['vless-reality']:[])])} busy={create.isPending} onChange={value=>{setKind(value);setError('');}}/>
    {error&&<p className="error" role="alert">{error}</p>}
   </div>
   {kind==='vless-reality'?<RealitySetup busy={create.isPending} onSubmit={submit}/>:<ProtocolSetup key={kind} vpn={vpn.data} available={[kind]} busy={create.isPending} onSubmit={submit}/>}
  </Dialog>}
  {detail&&<ConnectionDetails item={detail} state={health.data?.items.find(h=>h.kind===connectionKey(detail))?.state} checking={health.isFetching} checkFailed={health.isError} onCheck={()=>void health.refetch()} onEdit={()=>edit(detail)} onRotate={()=>{setDetailKind(undefined);setRotation(detail)}} onPickTarget={()=>edit(detail,true)} onClose={()=>setDetailKind(undefined)}/>}
  {rotation&&<KeyRotationDialog item={rotation} onClose={()=>setRotation(undefined)} onDone={refresh}/>}
  {editing&&<ConnectionEditor key={connectionKey(editing)} item={editing} initialPickingTarget={pickTarget} busy={change.isPending} error={error} onClose={()=>setEditing(undefined)} onSubmit={settings=>save(editing,settings)}/>}
  {action&&<Dialog title={`${action.remove?'Удалить':action.item.enabled?'Отключить':'Включить'} · ${connectionLabel(action.item.kind)}`} busy={change.isPending} onClose={()=>setAction(undefined)}>
   <div className="dialog-body"><p>{action.remove?'Протокол и его ключи будут удалены. При повторном создании потребуется заново импортировать конфигурацию. Пользователи и другие протоколы сохранятся.':action.item.enabled?'Доступ через этот протокол будет отключён. Ключи сохранятся — протокол можно включить снова.':'Протокол будет включён с сохранёнными настройками и ключами.'}</p>
    {error&&<p className="error" role="alert">{error}</p>}
   </div>
   <footer className="dialog-footer"><DialogClose disabled={change.isPending}>Отмена</DialogClose><Button variant={action.remove?'danger':'primary'} disabled={change.isPending} onClick={()=>void save(action.item,{port:action.item.port,sni:action.item.sni,target:action.item.target,enabled:!action.item.enabled},action.remove)}>{change.isPending?'Применяем…':action.remove?'Удалить':action.item.enabled?'Отключить':'Включить'}</Button></footer>
  </Dialog>}
 </>;
}
