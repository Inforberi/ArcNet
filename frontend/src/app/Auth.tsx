import {useState, type FormEvent, type ReactNode} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Outlet} from 'react-router';
import {api, ApiError, setCSRF, type Session} from '../shared/api';
import {Button, Field, Loading} from '../shared/ui';

export function AuthGate() {
  const session = useQuery({queryKey:['session'],queryFn:async()=>{const s=await api<Session>('/api/session');setCSRF(s.csrf);return s;},retry:false,staleTime:300_000,refetchInterval:300_000,refetchOnWindowFocus:true});
  if(session.isPending) return <Loading/>;
  if(session.isError) {
    if(session.error instanceof ApiError && session.error.status===401) return <FirstAccess/>;
    return <AuthFrame title="Панель недоступна"><p role="alert">{session.error.message}</p><Button onClick={()=>void session.refetch()}>Повторить</Button></AuthFrame>;
  }
  return <Outlet/>;
}
function AuthFrame({title,children}:{title:string;children:ReactNode}) {
  return <main className="auth-container"><section className="auth-card"><span className="brand-mark">A</span><p className="muted">ArcNet · управление VPN</p><h1>{title}</h1>{children}</section></main>;
}
function FirstAccess() {
  const setup=useQuery({queryKey:['admin-setup'],queryFn:()=>api<{required:boolean;available:boolean}>('/api/setup'),retry:false});
  if(setup.isPending) return <Loading/>;
  if(setup.isError) return <AuthFrame title="Не удалось проверить первый запуск"><p role="alert">{setup.error.message}</p><Button onClick={()=>void setup.refetch()}>Повторить</Button></AuthFrame>;
  return setup.data.required ? <RegisterAdmin available={setup.data.available}/> : <Login/>;
}
function RegisterAdmin({available}:{available:boolean}) {
  const cache=useQueryClient();
  const [username,setUsername]=useState(''),[password,setPassword]=useState(''),[confirm,setConfirm]=useState(''),[code,setCode]=useState('');
  const [error,setError]=useState(''),[busy,setBusy]=useState(false);
  async function submit(event:FormEvent) {
    event.preventDefault();setError('');
    if(password!==confirm){setError('Пароли не совпадают');return;}
    setBusy(true);
    try {await api('/api/setup','POST',{username,password,code});setPassword('');setConfirm('');setCode('');await cache.invalidateQueries({queryKey:['admin-setup']});}
    catch(err){setError(err instanceof Error?err.message:'Не удалось создать администратора');}
    finally{setBusy(false);}
  }
  return <AuthFrame title="Создайте администратора">
    <p className="muted">Выберите свой логин и пароль. Владение сервером подтверждается одноразовым кодом из терминала.</p>
    {!available&&<p role="status">Код истёк. Получите новый на сервере: <code>sudo arcvpn setup</code>.</p>}
    <form onSubmit={submit}>
      <Field label="Одноразовый код из терминала" autoComplete="off" required minLength={24} maxLength={128} value={code} onChange={e=>setCode(e.target.value.trim())}/>
      <Field label="Ваш логин" autoComplete="username" required minLength={3} maxLength={64} pattern="[a-zA-Z0-9][a-zA-Z0-9_.-]{2,63}" description="3–64 символа: латиница, цифры, точка, дефис" value={username} onChange={e=>setUsername(e.target.value)}/>
      <Field label="Ваш пароль" type="password" autoComplete="new-password" required minLength={16} maxLength={256} description="Не менее 16 символов" value={password} onChange={e=>setPassword(e.target.value)}/>
      <Field label="Повторите пароль" type="password" autoComplete="new-password" required value={confirm} onChange={e=>setConfirm(e.target.value)}/>
      {error&&<p className="error" role="alert">{error}</p>}
      <Button type="submit" variant="primary" disabled={busy}>{busy?'Создаём…':'Создать администратора'}</Button>
    </form>
  </AuthFrame>;
}
function Login() {
  const cache=useQueryClient();
  const [username,setUsername]=useState(''),[password,setPassword]=useState(''),[code,setCode]=useState('');
  const [secondFactor,setSecondFactor]=useState(false),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  async function submit(event:FormEvent) {
    event.preventDefault();setError('');setBusy(true);
    try {
      const reply=await api<{ok:boolean;csrf?:string;mfa_required?:boolean}>('/api/login','POST',{username,password,code});
      if(reply.mfa_required){setSecondFactor(true);return;}
      if(!reply.ok||!reply.csrf)throw new Error('Не удалось завершить вход');
      setPassword('');setCode('');setCSRF(reply.csrf);
      cache.setQueryData(['session'],{authenticated:true,username,csrf:reply.csrf} satisfies Session);
    }catch(err){setError(err instanceof Error?err.message:'Ошибка входа');}
    finally{setBusy(false);}
  }
  return <AuthFrame title={secondFactor?'Подтвердите вход':'Войти в панель'}><form onSubmit={submit}>
    {secondFactor?<><p className="muted">Введите код из приложения-аутентификатора.</p><Field key="otp" label="Код подтверждения" autoFocus required inputMode="numeric" autoComplete="one-time-code" maxLength={6} pattern="[0-9]{6}" value={code} onChange={e=>setCode(e.target.value)}/></>:<>
      <Field label="Логин" autoComplete="username" required value={username} onChange={e=>setUsername(e.target.value)}/>
      <Field label="Пароль" type="password" autoComplete="current-password" required value={password} onChange={e=>setPassword(e.target.value)}/>
    </>}
    {error&&<p className="error" role="alert">{error}</p>}
    <Button type="submit" variant="primary" disabled={busy}>{busy?'Проверка…':secondFactor?'Подтвердить':'Войти'}</Button>
    {secondFactor&&<Button type="button" variant="ghost" disabled={busy} onClick={()=>{setSecondFactor(false);setPassword('');setCode('');setError('');}}>Назад</Button>}
  </form></AuthFrame>;
}
