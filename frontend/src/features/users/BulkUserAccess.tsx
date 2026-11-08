import {useState} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {api,type VPNUser} from '../../shared/api';
import Dialog from '../../shared/Dialog';
import {Button,Notice} from '../../shared/ui';

export default function BulkUserAccess({users,onDone,onCancel,onEdit}:{users:VPNUser[];onDone:()=>void;onCancel:()=>void;onEdit:()=>void}) {
 const cache=useQueryClient();
 const [action,setAction]=useState<boolean|null>(null),[busy,setBusy]=useState(false),[error,setError]=useState(''),[saved,setSaved]=useState(false);
 async function apply(){
  if(action===null)return;setBusy(true);setError('');
  try{
   const result=await api<{runtime_state:string}>('/api/users/bulk-access','POST',{enabled:action,items:users.map(u=>({id:u.id,expected_version:u.preferences?.version??0,expected_enabled:u.enabled}))});
   setSaved(true);
   await Promise.all(['users','user-usage','user-editor','user-preferences'].map(key=>cache.invalidateQueries({queryKey:[key]})));
   if(result.runtime_state==='applied'){setAction(null);onDone()}
   else setError('Доступ сохранён для всех выбранных пользователей. Применение к VPN-службам ещё не подтверждено; панель повторит попытку.');
  }catch(e){setError(e instanceof Error?e.message:'Не удалось изменить доступ')}finally{setBusy(false)}
 }
 return <>
  <div className="users-bulk-bar" role="region" aria-label="Массовые действия"><strong>Выбрано: {users.length}</strong><Button disabled={users.length>100} onClick={onEdit}>Изменить параметры</Button><Button disabled={!users.length||users.length>100} onClick={()=>{setSaved(false);setError('');setAction(true)}}>Включить доступ</Button><Button disabled={!users.length||users.length>100} onClick={()=>{setSaved(false);setError('');setAction(false)}}>Отключить доступ</Button><Button variant="ghost" onClick={onCancel}>Снять выбор</Button>{users.length>100&&<Notice>За один раз можно изменить до 100 пользователей.</Notice>}</div>
  {action!==null&&<Dialog title={action?'Включить доступ':'Отключить доступ'} busy={busy} onClose={()=>{setAction(null);if(saved)onDone()}}><div className="dialog-body"><p>Пользователей: <strong>{users.length}</strong>. {action?'Пакеты, сроки и разрешённые подключения продолжат действовать.':'Активные соединения выбранных пользователей будут прерваны после применения.'}</p><p className="muted">{users.slice(0,10).map(u=>u.name).join(', ')}{users.length>10?'…':''}</p>{error&&<Notice>{error}</Notice>}</div><footer className="dialog-footer"><Button disabled={busy} onClick={()=>{setAction(null);if(saved)onDone()}}>{saved?'Закрыть':'Отмена'}</Button><Button variant={action?'primary':'danger'} disabled={busy||saved} onClick={()=>void apply()}>{busy?'Применяем…':'Подтвердить'}</Button></footer></Dialog>}
 </>;
}
