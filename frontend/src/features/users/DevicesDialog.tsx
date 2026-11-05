import {useEffect,useState} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {api} from '../../shared/api';
import Dialog from '../../shared/Dialog';
import {Advanced,Button,Field,Loading,Notice} from '../../shared/ui';

type Device={id:string;name:string;created_at:number;revoked_at:number};
type Reply={items:Device[];managed:boolean;limit:number;version:number;active:number;supported:boolean;runtime_state:string};
type Links={url:string;links:string[]};

export default function DevicesDialog({id,name,onClose}:{id:number;name:string;onClose:()=>void}) {
 const cache=useQueryClient();
 const query=useQuery({queryKey:['user-devices',id],queryFn:()=>api<Reply>(`/api/users/${id}/devices`)});
 const [deviceName,setDeviceName]=useState('Устройство 1');
 const [limit,setLimit]=useState('0');const [limitDirty,setLimitDirty]=useState(false);const [busy,setBusy]=useState(false);const [error,setError]=useState('');
 const [message,setMessage]=useState('');const [links,setLinks]=useState<Links|null>(null);
 const data=query.data;
 useEffect(()=>{if(data){if(!limitDirty)setLimit(String(data.limit));setLinks(null)}},[data?.version]);
 async function change(method:'POST'|'PUT'|'DELETE',body:object,device?:string) {
  if(!data)return;setBusy(true);setError('');setMessage('');setLinks(null);
  try {
   const updated=await api<Reply>(`/api/users/${id}/devices${device?`/${device}`:''}`,method,{...body,expected_version:data.version});
   cache.setQueryData(['user-devices',id],updated);
   if(method==='PUT'){setLimit(String(updated.limit));setLimitDirty(false)}
   await cache.invalidateQueries({queryKey:['users']});
   await cache.invalidateQueries({queryKey:['user-usage']});
   setMessage(updated.runtime_state==='applied'?'Изменение применено.':'Изменение сохранено. Применение VPN-службой ещё не подтверждено.');
   if(method==='POST'){setDeviceName(`Устройство ${updated.items.length+1}`)}
  }catch(e){setError(e instanceof Error?e.message:'Не удалось сохранить');await query.refetch()}
  finally{setBusy(false)}
 }
 async function showLinks(device:Device) {
  setBusy(true);setError('');setLinks(null);
  try{setLinks(await api<Links>(`/api/users/${id}/devices/${device.id}/links`))}
  catch(e){setError(e instanceof Error?e.message:'Не удалось получить подписку')}
  finally{setBusy(false)}
 }
 return <Dialog title={`Устройства · ${name}`} onClose={onClose} busy={busy}>
  <div className="dialog-body">
   <p className="muted">Каждому устройству — отдельный доступ. Копию одного ключа нельзя отличить от оригинала: лимит считает выданные доступы.</p>
   {error&&<p className="error" role="alert">{error}</p>}{message&&<p className="notice" role="status">{message}</p>}
   {query.isPending?<Loading/>:query.isError?<Notice>{query.error.message}</Notice>:data&&<>
    {!data.supported&&<Notice>Отдельные доступы пока доступны для локального Xray без удалённых нод и других активных движков.</Notice>}
    {data.managed&&!message&&data.runtime_state!=='applied'&&<Notice>Применение доступа VPN-службой пока не подтверждено. При ошибке панель повторит применение; старые соединения могут продолжать работать.</Notice>}
    {data.managed?<p>Активных доступов: {data.active} / {data.limit||'без ограничения'}. Общий ключ отключён.</p>:<Notice>При добавлении первого устройства общий ключ и прежняя подписка пользователя будут отключены. Подключите клиентов по новым индивидуальным подпискам.</Notice>}
    <form onSubmit={e=>{e.preventDefault();if(!data.managed&&!window.confirm('Отключить общий ключ и прежнюю подписку, перейти на отдельные доступы?'))return;void change('POST',{name:deviceName.trim()})}}>
     <Field label="Имя устройства" value={deviceName} onChange={e=>setDeviceName(e.target.value)} required maxLength={60}/>
     <Button type="submit" variant="primary" disabled={busy||!data.supported||!deviceName.trim()||(data.limit>0&&data.active>=data.limit)||data.active>=100}>Добавить устройство</Button>
    </form>
    <div className="protocol-grid">{data.items.map(device=><div className="protocol-card" key={device.id}>
     <strong>{device.name}</strong><p className="muted">{device.revoked_at?'Доступ отозван':`Создан ${new Date(device.created_at*1000).toLocaleDateString('ru-RU')}`}</p>
     {!device.revoked_at&&<div className="flex wrap"><Button disabled={busy} onClick={()=>void showLinks(device)}>Подписка</Button><Button variant="danger" disabled={busy} onClick={()=>{if(window.confirm(`Отозвать доступ «${device.name}»? Его подписка перестанет работать. Завершение VPN-соединений зависит от применения изменения службой.`))void change('DELETE',{},device.id)}}>Отозвать</Button></div>}
    </div>)}</div>
    {links&&<div><Field label="Подписка устройства" value={links.url} readOnly onFocus={e=>e.target.select()}/><p className="muted">Ссылка содержит секрет доступа. Скопируйте её в VPN-клиент.</p>{links.links.map(link=><pre className="code" key={link}>{link}</pre>)}</div>}
    {data.managed&&<Advanced><form onSubmit={e=>{e.preventDefault();const value=Number(limit);if(!Number.isInteger(value)||value<0||value>100){setError('Укажите целое число от 0 до 100');return}if(!window.confirm(`Лимит доступов: ${data.limit||'без ограничения'} → ${value||'без ограничения'}. Уже выданные доступы автоматически не отзываются.`))return;void change('PUT',{limit:value})}}>
     <Field label="Лимит активных доступов" type="number" min={0} max={100} step={1} value={limit} onChange={e=>{setLimit(e.target.value);setLimitDirty(true)}}/><p className="muted">0 — без ограничения тарифа. Технический предел — 100 активных доступов на пользователя.</p>
     <Button type="submit" disabled={busy||!data.supported||Number(limit)===data.limit}>Сохранить лимит</Button>
    </form></Advanced>}
   </>}
  </div>
 </Dialog>
}
