import {useState} from 'react';
import {Link} from 'react-router';
import {useMutation,useQuery,useQueryClient} from '@tanstack/react-query';
import {api,ApiError,type VPNConfig,type ProtocolProfile,type ProtocolHealth,type ProtocolKind} from '../shared/api';
import {RealitySetup,ProtocolSetup,ProtocolChoice,protocolCatalog,type RealityInput,type ProtocolInput} from '../features/connections/ConnectionForms';
import {ConnectionTable,connectionItems,connectionLabel,type ConnectionItem} from '../features/connections/ConnectionTable';
import ConnectionEditor,{type ConnectionSettings} from '../features/connections/ConnectionEditor';
import Dialog from '../shared/Dialog';
import {Button,Loading,PageHeading,Panel} from '../shared/ui';

type PendingAction={item:ConnectionItem;remove:boolean};
export default function VPN() {
 const cache=useQueryClient();
 const vpn=useQuery({queryKey:['vpn'],queryFn:()=>api<VPNConfig>('/api/vpn')});
 const profiles=useQuery({queryKey:['protocols'],queryFn:()=>api<{items:ProtocolProfile[]}>('/api/protocols')});
 const health=useQuery({queryKey:['protocol-health'],queryFn:()=>api<{items:ProtocolHealth[]}>('/api/protocols/health')});
 const [creating,setCreating]=useState(false),[kind,setKind]=useState<'vless-reality'|ProtocolKind>('vless-reality');
 const [editing,setEditing]=useState<ConnectionItem>(),[action,setAction]=useState<PendingAction>();
 const [error,setError]=useState(''),[created,setCreated]=useState(false);
 const configured=new Set(profiles.data?.items.map(p=>p.kind)??[]);
 const available=[...(!vpn.data?.configured?[{kind:'vless-reality' as const,name:'VLESS + Reality TCP'}]:[]),...protocolCatalog.filter(p=>!configured.has(p.kind))];
 async function refresh(){await Promise.all(['vpn','protocols','protocol-health','connection-defaults','users','user-devices','user-usage'].map(key=>cache.invalidateQueries({queryKey:[key]})));}
 const create=useMutation({mutationFn:async(input:RealityInput|ProtocolInput)=>{
  if(kind==='vless-reality')await api('/api/vpn','POST',input);
  else await api('/api/protocols','POST',input);
 },onSuccess:async()=>{await refresh();setCreating(false);setCreated(true);}});
 const change=useMutation({mutationFn:({item,settings,remove=false}:{item:ConnectionItem;settings?:ConnectionSettings;remove?:boolean})=>api(`/api/connections/${item.kind}`,remove?'DELETE':'PUT',remove?{revision:item.revision}:{revision:item.revision,...settings}),onSuccess:async()=>{await refresh();setEditing(undefined);setAction(undefined);}});
 async function submit(input:RealityInput|ProtocolInput){setError('');try{await create.mutateAsync(input);}catch(err){setError(err instanceof Error?err.message:'Не удалось создать протокол');}}
 async function save(item:ConnectionItem,settings?:ConnectionSettings,remove=false){setError('');try{await change.mutateAsync({item,settings,remove});}catch(err){if(err instanceof ApiError&&err.status===409)await refresh();setError(err instanceof Error?err.message:'Не удалось применить изменение');}}
 function openCreate(){setKind(available[0]?.kind??'vless-reality');setError('');setCreated(false);setCreating(true);}
 const busy=create.isPending||change.isPending;
 return <>
  <PageHeading title="Подключения" description="Настройте протоколы на сервере. Доступ и подписки выдаются в разделе «Пользователи»." actions={<><Link className="button" to="/users">Пользователи</Link>{available.length>0&&<Button variant="primary" disabled={busy||vpn.isPending||profiles.isPending||vpn.isError||profiles.isError} onClick={openCreate}>Добавить протокол</Button>}</>}/>
  {created&&<p className="connection-success" role="status">Протокол добавлен. <Link to="/users">Выдать доступ пользователю</Link></p>}
  <Panel title="Протоколы сервера">
   {vpn.isPending||profiles.isPending?<Loading/>:vpn.isError||profiles.isError?<div className="panel-body"><p className="error" role="alert">{vpn.error?.message??profiles.error?.message}</p><Button onClick={()=>void refresh()}>Повторить</Button></div>:<ConnectionTable items={connectionItems(vpn.data!,profiles.data!.items)} health={health.data?.items??[]} busy={busy} onEdit={item=>{setError('');setEditing(item);setCreated(false);}} onToggle={item=>{setError('');setAction({item,remove:false});setCreated(false);}} onRemove={item=>{setError('');setAction({item,remove:true});setCreated(false);}}/>}
   {health.isError&&<p className="panel-body muted">Состояние служб недоступно. <Button className="small" onClick={()=>void health.refetch()}>Проверить снова</Button></p>}
  </Panel>
  {creating&&vpn.data&&<Dialog title="Новый протокол" busy={create.isPending} onClose={()=>setCreating(false)}>
   <div className="panel-body"><ProtocolChoice kind={kind} available={available.map(p=>p.kind)} configured={new Set([...configured,...(vpn.data.configured?['vless-reality']:[])])} busy={create.isPending} onChange={value=>{setKind(value);setError('');}}/>
    {error&&<p className="error" role="alert">{error}</p>}
   </div>
   {kind==='vless-reality'?<RealitySetup busy={create.isPending} onSubmit={submit}/>:<ProtocolSetup key={kind} vpn={vpn.data} available={[kind]} busy={create.isPending} onSubmit={submit}/>}
  </Dialog>}
  {editing&&<ConnectionEditor key={editing.kind} item={editing} busy={change.isPending} error={error} onClose={()=>setEditing(undefined)} onSubmit={settings=>save(editing,settings)}/>}
  {action&&<Dialog title={`${action.remove?'Удалить':action.item.enabled?'Отключить':'Включить'} · ${connectionLabel(action.item.kind)}`} busy={change.isPending} onClose={()=>setAction(undefined)}>
   <div className="dialog-body"><p>{action.remove?'Протокол и его ключи будут удалены. При повторном создании потребуется заново импортировать конфигурацию. Пользователи и другие протоколы сохранятся.':action.item.enabled?'Доступ через этот протокол будет отключён. Ключи сохранятся — протокол можно включить снова.':'Протокол будет включён с сохранёнными настройками и ключами.'}</p>
    {error&&<p className="error" role="alert">{error}</p>}
   </div>
   <footer className="dialog-footer"><Button disabled={change.isPending} onClick={()=>setAction(undefined)}>Отмена</Button><Button variant={action.remove?'danger':'primary'} disabled={change.isPending} onClick={()=>void save(action.item,{port:action.item.port,sni:action.item.sni,target:action.item.target,enabled:!action.item.enabled},action.remove)}>{change.isPending?'Применяем…':action.remove?'Удалить':action.item.enabled?'Отключить':'Включить'}</Button></footer>
  </Dialog>}
 </>;
}
