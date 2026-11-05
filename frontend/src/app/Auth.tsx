import {useState,type FormEvent} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Outlet} from 'react-router';
import {api,ApiError,setCSRF,type Session} from '../shared/api';
import {Button,Field,Loading} from '../shared/ui';

export function AuthGate(){
  const session=useQuery({queryKey:['session'],queryFn:async()=>{const s=await api<Session>('/api/session');setCSRF(s.csrf);return s},retry:false,staleTime:300_000,refetchInterval:300_000,refetchOnWindowFocus:true});
  if(session.isPending)return <Loading/>;
  if(session.isError){
    if(session.error instanceof ApiError && session.error.status===401)return <Login/>;
    return <main className="auth-container"><section className="auth-card"><h1>Панель недоступна</h1><p role="alert">{session.error instanceof Error ? session.error.message : 'Не удалось проверить сессию'}</p><Button type="button" onClick={()=>void session.refetch()}>Повторить</Button></section></main>;
  }
  return <Outlet/>;
}
function Login(){
  const client=useQueryClient();const [username,setUsername]=useState('admin');const [password,setPassword]=useState('');const [error,setError]=useState('');const [busy,setBusy]=useState(false);const [code,setCode]=useState('');
  async function submit(event:FormEvent<HTMLFormElement>){event.preventDefault();setError('');setBusy(true);try{const reply=await api<{ok:boolean;csrf:string}>('/api/login','POST',{username,password,code});setCSRF(reply.csrf);client.setQueryData(['session'],{authenticated:true,username,csrf:reply.csrf} satisfies Session)}catch(err){setError(err instanceof Error?err.message:'Ошибка входа')}finally{setBusy(false)}}
  return <main className="auth-container"><section className="auth-card"><span className="brand-mark">A</span><h1>ArcVPN</h1><p>Панель управления VPN</p><form onSubmit={submit}><Field label="Логин" autoComplete="username" value={username} onChange={event=>setUsername(event.target.value)} required/><Field label="Пароль" type="password" autoComplete="current-password" value={password} onChange={event=>setPassword(event.target.value)} required/><Field label="Код 2FA (если включён)" inputMode="numeric" autoComplete="one-time-code" maxLength={6} pattern="[0-9]{6}" value={code} onChange={event=>setCode(event.target.value)} />{error&&<p className="error" role="alert">{error}</p>}<Button type="submit" variant="primary" disabled={busy}>{busy?'Проверка…':'Войти'}</Button></form></section></main>;
}
