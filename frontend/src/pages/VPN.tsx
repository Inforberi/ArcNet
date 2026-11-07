import {useState} from 'react';
import {Link} from 'react-router';
import {useMutation,useQuery,useQueryClient} from '@tanstack/react-query';
import {api,type VPNConfig,type ProtocolProfile,type ProtocolHealth,type ProtocolKind} from '../shared/api';
import {RealitySetup,ProtocolSetup,protocolCatalog,type RealityInput,type ProtocolInput} from '../features/connections/ConnectionForms';
import {ConnectionTable} from '../features/connections/ConnectionTable';
import Dialog from '../shared/Dialog';
import {Button,Loading,PageHeading,Panel} from '../shared/ui';

export default function VPN() {
  const cache=useQueryClient();
  const vpn=useQuery({queryKey:['vpn'],queryFn:()=>api<VPNConfig>('/api/vpn')});
  const profiles=useQuery({queryKey:['protocols'],queryFn:()=>api<{items:ProtocolProfile[]}>('/api/protocols')});
  const health=useQuery({queryKey:['protocol-health'],queryFn:()=>api<{items:ProtocolHealth[]}>('/api/protocols/health')});
  const [creating,setCreating]=useState(false),[kind,setKind]=useState<'vless-reality'|ProtocolKind>('vless-reality');
  const [error,setError]=useState(''),[created,setCreated]=useState(false);
  const configured=new Set(profiles.data?.items.map(p=>p.kind)??[]);
  const available=[...(!vpn.data?.configured?[{kind:'vless-reality' as const,name:'VLESS + Reality TCP'}]:[]),...protocolCatalog.filter(p=>!configured.has(p.kind))];
  async function refresh(){await Promise.all(['vpn','protocols','protocol-health','connection-defaults','users'].map(key=>cache.invalidateQueries({queryKey:[key]})));}
  const create=useMutation({mutationFn:async(input:RealityInput|ProtocolInput)=>{
    if(kind==='vless-reality')await api('/api/vpn','POST',input);
    else await api('/api/protocols','POST',input);
  },onSuccess:async()=>{await refresh();setCreating(false);setCreated(true);}});
  const change=useMutation({mutationFn:({profile,remove}:{profile:ProtocolProfile;remove:boolean})=>remove?api(`/api/protocols/${profile.kind}`,'DELETE'):api(`/api/protocols/${profile.kind}`,'PATCH',{enabled:!profile.enabled}),onSuccess:refresh});
  async function submit(input:RealityInput|ProtocolInput){setError('');try{await create.mutateAsync(input);}catch(err){setError(err instanceof Error?err.message:'Не удалось создать подключение');}}
  async function update(profile:ProtocolProfile,remove=false){
    if(remove&&!window.confirm('Удалить отключённое подключение и его ключи?'))return;
    setError('');setCreated(false);try{await change.mutateAsync({profile,remove});}catch(err){setError(err instanceof Error?err.message:'Не удалось применить изменение');}
  }
  function openCreate(){setKind(available[0]?.kind??'vless-reality');setError('');setCreated(false);setCreating(true);}
  return <>
    <PageHeading title="Подключения" description="Создайте подключение и выдайте пользователю QR или подписку" actions={available.length>0&&<Button variant="primary" disabled={vpn.isPending||profiles.isPending||vpn.isError||profiles.isError} onClick={openCreate}>Добавить подключение</Button>}/>
    {!creating&&error&&<p className="error" role="alert">{error}</p>}
    {created&&<p className="connection-success" role="status">Подключение добавлено. <Link to="/users">Добавить пользователя и получить доступ →</Link></p>}
    <Panel title="Подключения сервера">
      {vpn.isPending||profiles.isPending?<Loading/>:vpn.isError||profiles.isError?<div className="panel-body"><p className="error" role="alert">{vpn.error?.message??profiles.error?.message}</p><Button onClick={()=>void refresh()}>Повторить</Button></div>:<ConnectionTable vpn={vpn.data!} profiles={profiles.data!.items} health={health.data?.items??[]} busy={change.isPending} onToggle={p=>void update(p)} onRemove={p=>void update(p,true)}/>}
      {health.isError&&<p className="panel-body muted">Состояние служб недоступно. <Button className="small" onClick={()=>void health.refetch()}>Проверить снова</Button></p>}
    </Panel>
    {creating&&vpn.data&&<Dialog title="Новое подключение" busy={create.isPending} onClose={()=>setCreating(false)}>
      <div className="panel-body"><label className="field"><span>Протокол</span><select aria-label="Протокол" disabled={create.isPending} value={kind} onChange={e=>{setKind(e.target.value as typeof kind);setError('');}}>{available.map(p=><option key={p.kind} value={p.kind}>{p.name}</option>)}</select></label>
        {error&&<p className="error" role="alert">{error}</p>}
      </div>
      {kind==='vless-reality'?<RealitySetup busy={create.isPending} onSubmit={submit}/>:<ProtocolSetup key={kind} vpn={vpn.data} available={[kind]} busy={create.isPending} onSubmit={submit}/>}
    </Dialog>}
  </>;
}
