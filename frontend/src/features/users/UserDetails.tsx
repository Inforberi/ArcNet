import UserTrafficPanel from './UserTrafficPanel';
import type {TrafficDetail} from './traffic';
import UserEditor from './UserEditor';
import {useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import Dialog,{DialogClose} from '../../shared/Dialog';
import {api, type VPNUser} from '../../shared/api';
import {Badge, Button, Loading, Notice,Properties,SettingsSection} from '../../shared/ui';
import {UsageSummary} from './PackageDialog';
import DevicesDialog from './DevicesDialog';
import {connectionTitle} from '../connections/presentation';
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
 const navigation=<nav className="user-tabs" aria-label="Разделы пользователя">{([{id:'overview',label:'Обзор'},{id:'edit',label:'Настройки'},{id:'devices',label:'Устройства и лимиты'}] as const).map(item=><Button key={item.id} variant={tab===item.id?'primary':'default'} aria-pressed={tab===item.id} disabled={busy} onClick={()=>setTab(item.id)}>{item.label}</Button>)}</nav>;
 if(tab==='edit')return <UserEditor user={user} navigation={navigation} onClose={onClose}/>;
 if(tab==='devices')return <DevicesDialog id={user.id} name={user.name} navigation={navigation} onClose={onClose}/>;
 const reason=account?.block_reason;
 return <Dialog title={`Пользователь · ${user.name}`} onClose={onClose} busy={busy}>
  {navigation}
  <div className="dialog-body">
   <Properties items={[
    {label:'Состояние доступа',value:<Badge variant={user.enabled&&!reason&&user.preferences?.allowed_kinds?.length!==0?'green':'red'}>{!user.enabled?'Отключён':reason?blockLabel[reason]??'Доступ ограничен':user.preferences?.allowed_kinds?.length===0?'Нет разрешённых подключений':'Разрешён в настройках'}</Badge>},
    {label:'Группа',value:user.preferences?.group||'Без группы'},
    {label:'Название подписки',value:user.preferences?.title||`ArcNet · ${user.name}`},
    {label:'Разрешённые подключения',value:!user.preferences?'Настройки не получены':user.preferences.allowed_kinds===null?'Все, включая новые':user.preferences.allowed_kinds.map(connectionTitle).join(', ')||'Ни одного'},
    {label:'Срок действия',value:account?account.expires_at?new Date(account.expires_at*1000).toLocaleString('ru-RU'):'Без срока':'Нет данных'},
    {label:'Создан',value:new Date(user.created_at*1000).toLocaleString('ru-RU')},
    ...(user.preferences?.comment?[{label:'Комментарий',value:user.preferences.comment}]:[])
   ]}/>
   <SettingsSection title="Трафик и активность"><UsageSummary item={account} complete={usage?.complete_coverage??false}/><UserTrafficPanel now={trafficNow} detail={traffic} pending={trafficPending} failed={trafficFailed}/></SettingsSection>
   <Properties items={[{label:'Ранее выданные доступы',value:devices.data?`${devices.data.active} / ${devices.data.limit||'без лимита'}`:devices.isPending?<Loading/>:'Нет данных'}]}/>
   {devices.isError&&<Notice>{devices.error.message}</Notice>}
   <small className="muted">Выданный доступ не подтверждает число физических устройств в сети.</small>
   {error&&<p role="alert" className="error">{error}</p>}
   <div className="flex wrap"><Button disabled={busy} onClick={()=>void change()}>{user.enabled?'Отключить доступ':'Включить доступ'}</Button><Button variant="danger" disabled={busy} onClick={()=>void change(true)}>Удалить пользователя</Button></div>
  </div>
  <footer className="dialog-footer"><DialogClose disabled={busy}>Закрыть</DialogClose><Button disabled={busy} onClick={()=>setTab('edit')}>Настроить пользователя</Button><Button disabled={busy} variant="primary" onClick={()=>{if(user.devices_managed)setTab('devices');else onConnect();}}>{user.devices_managed?'Открыть доступы':'Подключить'}</Button></footer>
 </Dialog>;
}
