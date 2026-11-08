import UserTrafficPanel from './UserTrafficPanel';
import type {TrafficDetail} from './traffic';
import UserEditor from './UserEditor';
import {useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import Dialog from '../../shared/Dialog';
import {api, type VPNUser} from '../../shared/api';
import {Badge, Button, Loading, Notice} from '../../shared/ui';
import {UsageSummary} from './PackageDialog';
import DevicesDialog from './DevicesDialog';
import {blockLabel, type UsageReply, type UserPackage} from './usage';

export default function UserDetails({trafficNow,traffic,trafficPending,trafficFailed,user,usage,account,onClose,onConnect,onToggle,onDelete}:{trafficNow:number;traffic?:TrafficDetail;trafficPending:boolean;trafficFailed:boolean;user:VPNUser;usage?:UsageReply;account?:UserPackage;onClose:()=>void;onConnect:()=>void;onToggle:()=>Promise<unknown>;onDelete:()=>Promise<unknown>}) {
 const [tab,setTab]=useState<'edit'|'overview'|'devices'>('overview');
 const [busy,setBusy]=useState(false),[error,setError]=useState('');
 async function change(remove=false){
  if(remove&&!window.confirm(`Удалить пользователя «${user.name}» и отозвать его доступ?`))return;
  if(!remove&&user.enabled&&!window.confirm(`Отключить доступ пользователя «${user.name}»?`))return;
  setBusy(true);setError('');
  try{if(remove){await onDelete();onClose();}else await onToggle();}
  catch(err){setError(err instanceof Error?err.message:'Не удалось изменить пользователя');}
  finally{setBusy(false);}
 }
 const devices=useQuery({queryKey:['user-devices',user.id],queryFn:()=>api<{active:number;limit:number;managed:boolean}>(`/api/users/${user.id}/devices`)});
 const navigation=<nav className="user-tabs" aria-label="Разделы пользователя">{([{id:'edit',label:'Настройки'},{id:'overview',label:'Основное'},{id:'devices',label:'Устройства и лимиты'}] as const).map(item=><Button key={item.id} variant={tab===item.id?'primary':'default'} aria-pressed={tab===item.id} disabled={busy} onClick={()=>setTab(item.id)}>{item.label}</Button>)}</nav>;
 if(tab==='edit')return <UserEditor user={user} navigation={navigation} onClose={onClose}/>;
 if(tab==='devices')return <DevicesDialog id={user.id} name={user.name} navigation={navigation} onClose={onClose}/>;
 const reason=account?.block_reason;
 return <Dialog title={`Пользователь · ${user.name}`} onClose={onClose} busy={busy}>
  {navigation}
  <div className="dialog-body">
   <div className="kv"><span>Состояние доступа</span><Badge variant={user.enabled&&!reason?'green':'red'}>{!user.enabled?'Отключён':reason?blockLabel[reason]??'Доступ ограничен':'Включён'}</Badge></div>
   <div className="kv"><span>Трафик</span><UsageSummary item={account} complete={usage?.complete_coverage??false}/></div>
   <div className="kv"><span>Срок действия</span><span>{account?account.expires_at?new Date(account.expires_at*1000).toLocaleString('ru-RU'):'Без срока':'Нет данных'}</span></div>
   <div className="kv"><span>Ранее выданные доступы</span><span>{devices.data?`${devices.data.active} / ${devices.data.limit||'без лимита'}`:devices.isPending?<Loading/>:'Нет данных'}</span></div>
   {devices.isError&&<Notice>{devices.error.message}</Notice>}
   <small className="muted">Лимит устройств ограничивает выданные доступы. Он не подтверждает число физических устройств в сети.</small>
   <UserTrafficPanel now={trafficNow} detail={traffic} pending={trafficPending} failed={trafficFailed}/>
   <div className="kv"><span>Создан</span><span>{new Date(user.created_at*1000).toLocaleString('ru-RU')}</span></div>
   {error&&<p role="alert" className="error">{error}</p>}
   <div className="flex wrap"><Button disabled={busy} onClick={()=>void change()}>{user.enabled?'Отключить доступ':'Включить доступ'}</Button><Button variant="danger" disabled={busy} onClick={()=>void change(true)}>Удалить пользователя</Button></div>
  </div>
  <footer className="dialog-footer"><Button disabled={busy} onClick={onClose}>Закрыть</Button><Button disabled={busy} variant="primary" onClick={()=>{if(user.devices_managed)setTab('devices');else onConnect();}}>{user.devices_managed?'Открыть доступы':'Подключить'}</Button></footer>
 </Dialog>;
}
