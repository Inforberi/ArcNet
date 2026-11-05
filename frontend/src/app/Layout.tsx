import { useEffect, useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { applyTheme, readTheme } from '../shared/theme';
import { api, setCSRF } from '../shared/api';
import {MonitoringProvider} from '../features/dashboard/Monitoring';
import { Button } from '../shared/ui';

type NavItem = { url: string; label: string; icon: string };

const navigation: NavItem[] = [
  { url: '/', label: 'Обзор', icon: '▦' },
  { url: '/users', label: 'Пользователи', icon: '♙' },
  { url: '/vpn', label: 'Подключения', icon: '⇄' },
  { url: '/nodes', label: 'Серверы', icon: '▤' },
  { url: '/routing', label: 'Маршруты', icon: '⌁' },
  { url: '/security', label: 'Безопасность', icon: '⚙' },
  { url: '/events', label: 'Журнал', icon: '≡' },
  { url: '/backup', label: 'Резервные копии', icon: '⇩' },
];

function MenuLinks({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <>
      {navigation.map(({ url, label, icon }) => (
        <NavLink
          key={url}
          to={url}
          end={url === '/'}
          onClick={onNavigate}
          className={({ isActive }) => (isActive ? 'active' : '')}
        >
          <span className="symbol" aria-hidden="true">{icon}</span>
          <span>{label}</span>
        </NavLink>
      ))}
    </>
  );
}

export default function Layout() {
  const queryClient = useQueryClient();
  const location = useLocation();
  const [theme, setTheme] = useState(readTheme);
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

  const active = navigation.find(({ url }) => url === location.pathname)?.label ?? 'Страница';

  useEffect(() => {
    if (!menuOpen) return;
    const onEscape = (event: KeyboardEvent) => { if (event.key === 'Escape') setMenuOpen(false); };
    window.addEventListener('keydown', onEscape);
    return () => window.removeEventListener('keydown', onEscape);
  }, [menuOpen]);

  function toggleTheme() {
    const next = theme === 'dark' ? 'light' : 'dark';
    applyTheme(next);
    setTheme(next);
  }

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
        <div className="brand"><span className="brand-mark">A</span>ArcVPN</div>
        <button
          type="button"
          className="button mobile-menu-toggle"
          aria-controls="mobile-navigation"
          aria-expanded={menuOpen}
          onClick={() => setMenuOpen(value => !value)}
        >
          {menuOpen ? 'Закрыть' : `Меню · ${active}`}
        </button>
      </div>
      {menuOpen && (
        <nav id="mobile-navigation" className="mobile-links" aria-label="Мобильная навигация">
          <MenuLinks onNavigate={() => setMenuOpen(false)} />
          <button className="button" type="button" onClick={toggleTheme}>
            {theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'}
          </button>
          <button className="button" type="button" onClick={() => void logout()} disabled={signingOut}>
            Выйти
          </button>
        </nav>
      )}
      <div className="layout">
        <aside className="sidebar">
          <div className="brand"><span className="brand-mark">A</span>ArcVPN</div>
          <div className="section-label">УПРАВЛЕНИЕ</div>
          <nav className="nav" aria-label="Основная навигация"><MenuLinks /></nav>
          <div className="side-bottom">Go · SQLite · Xray</div>
          <button className="button ghost" type="button" onClick={toggleTheme}>
            {theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'}
          </button>
          <Button variant="ghost" onClick={() => void logout()} disabled={signingOut}>Выйти</Button>
        </aside>
        <main className="main" id="main-content" tabIndex={-1}>
          {logoutError && <p className="error" role="alert">{logoutError}</p>}
          <MonitoringProvider><Outlet /></MonitoringProvider>
        </main>
      </div>
    </>
  );
}
