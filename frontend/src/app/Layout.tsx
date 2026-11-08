import {ConfirmationProvider} from '../shared/Confirmation';
import { useEffect, useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import {InterfacePreferences} from '../shared/InterfacePreferences';
import { api, setCSRF } from '../shared/api';
import {MonitoringProvider} from '../features/dashboard/Monitoring';
import { Button } from '../shared/ui';
import {Icon,type IconName} from '../shared/Icon';
import Dialog from '../shared/Dialog';

type NavItem = {url:string;label:string;icon:IconName};
const workspace:NavItem[]=[
 {url:'/',label:'Обзор',icon:'overview'},
 {url:'/vpn',label:'Подключения',icon:'protocols'},
 {url:'/users',label:'Пользователи',icon:'users'},
 {url:'/nodes',label:'Серверы',icon:'servers'},
 {url:'/routing',label:'Маршруты',icon:'routes'},
];
const system:NavItem[]=[
 {url:'/versions',label:'Версии ArcNet',icon:'backup'},
 {url:'/events',label:'Журнал',icon:'logs'},
 {url:'/backup',label:'Резервные копии',icon:'backup'},
 {url:'/security',label:'Настройки',icon:'settings'},
];
function Brand(){return <div className="brand"><span className="brand-mark">A</span><span>ArcNet</span></div>;}
function MenuLinks({items,onNavigate}:{items:NavItem[];onNavigate?:()=>void}){
 return <>{items.map(({url,label,icon})=><NavLink key={url} to={url} end={url==='/'} onClick={onNavigate} className={({isActive})=>isActive?'active':''}><Icon name={icon}/><span>{label}</span></NavLink>)}</>;
}
function SessionActions({busy,onLogout}:{busy:boolean;onLogout:()=>void}){
 return <div className="session-actions"><Button variant="ghost" onClick={onLogout} disabled={busy}><Icon name="logout"/>{busy?'Выходим…':'Выйти'}</Button></div>;
}

export default function Layout() {
 return <InterfacePreferences><ConfirmationProvider><WorkspaceLayout/></ConfirmationProvider></InterfacePreferences>;
}
function WorkspaceLayout() {
  const queryClient = useQueryClient();
  const location = useLocation();
  const [menuOpen, setMenuOpen] = useState(false);
  const [signingOut, setSigningOut] = useState(false);
  const [logoutError, setLogoutError] = useState('');

  useEffect(() => {
    setMenuOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    const onUnauthorized = () => {
      setCSRF('');
      void queryClient.invalidateQueries({ queryKey: ['session'] });
    };
    window.addEventListener('arcvpn:unauthorized', onUnauthorized);
    return () => window.removeEventListener('arcvpn:unauthorized', onUnauthorized);
  }, [queryClient]);

  const active = [...workspace,...system].find(({ url }) => url === location.pathname)?.label ?? 'Страница';

  useEffect(() => {
    if (!menuOpen) return;
    const onEscape = (event: KeyboardEvent) => { if (event.key === 'Escape') setMenuOpen(false); };
    window.addEventListener('keydown', onEscape);
    return () => window.removeEventListener('keydown', onEscape);
  }, [menuOpen]);

  useEffect(()=>{
    const desktop=window.matchMedia('(min-width:681px)');
    const closeOnDesktop=()=>{if(desktop.matches)setMenuOpen(false);};
    desktop.addEventListener('change',closeOnDesktop);
    return ()=>desktop.removeEventListener('change',closeOnDesktop);
  },[]);

  async function logout() {
    setLogoutError('');
    setSigningOut(true);
    try {
      await api('/api/logout', 'POST');
      setCSRF('');
      queryClient.clear();
      window.location.replace('/');
    } catch (error) {
      setLogoutError(error instanceof Error ? error.message : 'Ошибка выхода');
      setSigningOut(false);
    }
  }

  return (
    <>
      <a href="#main-content" className="skip-link">Перейти к содержимому</a>
      <div className="mobile-header">
        <Brand/>
        <button
          type="button"
          className="button mobile-menu-toggle"
          aria-controls="mobile-navigation"
          aria-label={menuOpen?'Закрыть меню':`Открыть меню · ${active}`}
          aria-expanded={menuOpen}
          onClick={() => setMenuOpen(value => !value)}
        >
          <Icon name={menuOpen?'close':'menu'}/><span>{menuOpen?'Закрыть':active}</span>
        </button>
      </div>
      {menuOpen && (
        <Dialog title="Меню" className="navigation-drawer" onClose={()=>setMenuOpen(false)}>
        <nav id="mobile-navigation" className="drawer-navigation" aria-label="Мобильная навигация">
          <div className="nav drawer-workspace"><MenuLinks items={workspace} onNavigate={()=>setMenuOpen(false)}/></div>
          <div className="nav drawer-system"><MenuLinks items={system} onNavigate={()=>setMenuOpen(false)}/></div>
          <SessionActions busy={signingOut} onLogout={()=>void logout()}/>
        </nav>
        </Dialog>
      )}
      <div className="layout">
        <aside className="sidebar">
          <Brand/>
          <nav className="nav" aria-label="Основная навигация"><MenuLinks items={workspace}/></nav>
          <div className="sidebar-footer">
            <nav className="nav" aria-label="Система"><MenuLinks items={system}/></nav>
            <SessionActions busy={signingOut} onLogout={()=>void logout()}/>
          </div>
        </aside>
        <main className="main" id="main-content" tabIndex={-1}>
          {logoutError && <p className="error" role="alert">{logoutError}</p>}
          <MonitoringProvider><Outlet /></MonitoringProvider>
        </main>
      </div>
    </>
  );
}
