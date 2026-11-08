import {useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {api,ApiError} from '../../shared/api';
import Dialog,{DialogClose} from '../../shared/Dialog';
import {Button,Loading,Notice,Properties,SettingsSection} from '../../shared/ui';
import {connectionLabel,type ConnectionItem} from './ConnectionTable';

type Plan={revision:string;confirmation:string;affected:number;users:{id:number;name:string}[]};
export default function KeyRotationDialog({item,onClose,onDone}:{item:ConnectionItem;onClose:()=>void;onDone:()=>Promise<void>}){
 const plan=useQuery({queryKey:['key-rotation-plan',item.kind],queryFn:()=>api<Plan>(`/api/connections/${item.kind}/rotation`),retry:false,refetchOnWindowFocus:false,staleTime:0,gcTime:0});
 const [ack,setAck]=useState(false),[busy,setBusy]=useState(false),[error,setError]=useState(''),[saved,setSaved]=useState(false);
 async function rotate(){
  if(!ack||!plan.data||busy||saved)return;setBusy(true);setError('');let committed=false;
  try{await api(`/api/connections/${item.kind}/rotate`,'POST',{revision:plan.data.revision,confirmation:plan.data.confirmation});committed=true;setSaved(true);await onDone();onClose()}
  catch(e){setError(committed?'Ключи сменены. Обновите страницу для получения нового профиля.':e instanceof Error?e.message:'Не удалось сменить ключи');if(e instanceof ApiError&&e.status===409){setAck(false);await plan.refetch()}}
  finally{setBusy(false)}
 }
 return <Dialog title={`Смена ключей · ${connectionLabel(item.kind)}`} onClose={onClose} busy={busy}>
  <div className="dialog-body">
   {plan.isPending?<Loading/>:plan.isError?<><Notice>{plan.error.message}</Notice><Button onClick={()=>void plan.refetch()}>Повторить</Button></>:plan.data&&<>
    <Properties items={[{label:'Пользователей с этим протоколом',value:plan.data.affected},{label:'Что изменится',value:item.kind==='wireguard'?'Ключевая пара сервера WireGuard':'Ключевая пара REALITY и Short ID'},{label:'Что сохранится',value:'Пользователи, пакеты, сроки и адреса подписок'}]}/>
    <SettingsSection title="Кому потребуется обновить подключение" description="Включая отключённых пользователей. Список учитывает назначения этого протокола.">{plan.data.users.length?<ul className="rotation-users">{plan.data.users.map(user=><li key={user.id}>{user.name}<small>ID {user.id}</small></li>)}</ul>:<p className="muted">Пользователей с этим протоколом нет.</p>}{plan.data.affected>plan.data.users.length&&<small className="muted">И ещё {plan.data.affected-plan.data.users.length}.</small>}</SettingsSection>
    <p>Старые профили перестанут подключаться после применения. Потребуется обновить подписку{item.kind==='wireguard'?' или импортировать новый .conf':''}. Активные соединения могут прерваться.</p>
    <label className="connection-choice"><input type="checkbox" disabled={busy||saved} checked={ack} onChange={e=>setAck(e.target.checked)}/><span>Понимаю, что пользователям потребуется обновить подключение</span></label>
   </>}
   {error&&<p className="error" role="alert">{error}</p>}
  </div>
  <footer className="dialog-footer"><DialogClose disabled={busy}>{saved?'Закрыть':'Отмена'}</DialogClose><Button disabled={busy||saved||!ack||!plan.isSuccess} variant="danger" onClick={()=>void rotate()}>{busy?'Меняем ключи…':'Сменить ключи'}</Button></footer>
 </Dialog>;
}
