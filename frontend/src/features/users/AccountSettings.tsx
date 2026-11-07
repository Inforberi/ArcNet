import {useState,type FormEvent} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {api,setCSRF,type Session} from '../../shared/api';
import {Button,Field,Panel} from '../../shared/ui';

export default function AccountSettings(){
 const cache=useQueryClient();
 const session=useQuery({queryKey:['session'],queryFn:()=>api<Session>('/api/session')});
 const [manualName,setName]=useState<string>();
 const [current,setCurrent]=useState(''),[password,setPassword]=useState(''),[confirm,setConfirm]=useState('');
 const [busy,setBusy]=useState(false),[error,setError]=useState('');
 async function submit(event:FormEvent){
  event.preventDefault();setError('');
  if(password!==confirm){setError('Пароли не совпадают');return;}
  setBusy(true);
  try{await api('/api/security/account','PUT',{username:manualName??session.data?.username,current_password:current,password});setCurrent('');setPassword('');setConfirm('');setCSRF('');await cache.invalidateQueries({queryKey:['session']});}
  catch(err){setError(err instanceof Error?err.message:'Не удалось изменить доступ');}
  finally{setBusy(false);}
 }
 return <Panel title="Логин и пароль"><form className="panel-body" onSubmit={submit}>
  <Field label="Логин" autoComplete="username" required minLength={3} maxLength={64} pattern="[a-zA-Z0-9][a-zA-Z0-9_.-]{2,63}" value={manualName??session.data?.username??''} onChange={e=>setName(e.target.value)}/>
  <Field label="Текущий пароль" type="password" autoComplete="current-password" required maxLength={256} value={current} onChange={e=>setCurrent(e.target.value)}/>
  <Field label="Новый пароль" type="password" autoComplete="new-password" required minLength={16} maxLength={256} value={password} onChange={e=>setPassword(e.target.value)}/>
  <Field label="Повторите новый пароль" type="password" autoComplete="new-password" required value={confirm} onChange={e=>setConfirm(e.target.value)}/>
  <p className="muted">После изменения войдите заново. Настройки 2FA и VPN сохранятся.</p>
  {error&&<p className="error" role="alert">{error}</p>}
  <Button type="submit" variant="primary" disabled={busy||session.isPending}>{busy?'Сохраняем…':'Сохранить и войти заново'}</Button>
 </form></Panel>;
}
