import { useEffect, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type VPNUser, type Subscription, type ConnectionProfile } from '../shared/api';
import {UsageSummary} from '../features/users/PackageDialog';
import DevicesDialog from '../features/users/DevicesDialog';
import DirectAccess from '../features/users/DirectAccess';
import UserDetails from '../features/users/UserDetails';
import Dialog from '../shared/Dialog';
import {blockLabel, type UsageReply} from '../features/users/usage';
import { Badge, Button, Field, Loading, PageHeading, Panel, Notice, Empty } from '../shared/ui';



export default function Users() {
  const cache = useQueryClient();
  const users = useQuery({ queryKey: ['users'], queryFn: () => api<{ items: VPNUser[] }>('/api/users') });
  const usage = useQuery({queryKey:['user-usage'],queryFn:()=>api<UsageReply>('/api/users/usage')});
  const [deviceUser,setDeviceUser] = useState<VPNUser | null>(null);
  const [detailID,setDetailID] = useState<number|null>(null);
  const [search,setSearch] = useState('');
  const [filter,setFilter] = useState('all');
  const packages = new Map(usage.data?.items.map(item=>[item.user_id,item]));
  const visibleUsers=(users.data?.items??[]).filter(user=>(`${user.name} ${user.id}`.toLocaleLowerCase('ru').includes(search.trim().toLocaleLowerCase('ru')))&& (filter==='all'||filter==='enabled'&&user.enabled&&!packages.get(user.id)?.block_reason||filter==='disabled'&&!user.enabled||filter==='blocked'&&!!packages.get(user.id)?.block_reason));
  const detailUser=users.data?.items.find(user=>user.id===detailID);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [subscription, setSubscription] = useState<Subscription | null>(null);
  const [subscriptionUser, setSubscriptionUser] = useState<VPNUser | null>(null);
  const [connections, setConnections] = useState<ConnectionProfile[]>([]);
  const [selectedQR, setSelectedQR] = useState<string | null>(null);

  useEffect(() => {
    if (subscriptionUser && (packages.get(subscriptionUser.id)?.block_reason || users.data?.items.find(item=>item.id===subscriptionUser.id)?.enabled === false)) {
      setSubscription(null);setSubscriptionUser(null);setConnections([]);setSelectedQR(null);
    }
  }, [usage.data, users.data, subscriptionUser]);

  const create = useMutation({
    mutationFn: () => api<VPNUser>('/api/users', 'POST', { name: name.trim() }),
    onSuccess: async () => {
      setName('');
      setCreating(false);
      setMessage('Пользователь создан');
      await cache.invalidateQueries({ queryKey: ['users'] });
    },
  });
  const toggle = useMutation({
    mutationFn: (u: VPNUser) => api(`/api/users/${u.id}`, 'PATCH', { enabled: !u.enabled }),
    onSuccess: async () => { await cache.invalidateQueries({ queryKey: ['users'] }); },
  });
  const remove = useMutation({
    mutationFn: (u: VPNUser) => api(`/api/users/${u.id}`, 'DELETE'),
    onSuccess: async () => { await cache.invalidateQueries({ queryKey: ['users'] }); },
  });

  function explain(e: unknown): string { return e instanceof Error ? e.message : 'Неизвестная ошибка'; }

  async function showSubscription(user: VPNUser) {
    setError('');
    setMessage('');
    setSubscription(null);
    try {
      const [result, linked] = await Promise.all([
        api<Subscription>(`/api/users/${user.id}/subscription`),
        api<{items: ConnectionProfile[]}>(`/api/users/${user.id}/connections`),
      ]);
      setConnections(linked.items);
      setSelectedQR(null);
      setSubscription(result);
      setSubscriptionUser(user);
    } catch (err) {
      setError(explain(err));
    }
  }

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError('');
    setMessage('');
    if (!name.trim()) { setError('Укажи имя пользователя'); return; }
    try { await create.mutateAsync(); }
    catch (err) { setError(explain(err)); }
  }

  async function copy(value: string) {
    setError('');
    try { await navigator.clipboard.writeText(value); setMessage('Скопировано в буфер обмена'); }
    catch { setError('Не удалось скопировать. Выдели и скопируй ссылку вручную.'); }
  }

  return (
    <>
      <PageHeading
        title="Пользователи"
        description="Доступ, расход пакета трафика и подписки"
        actions={<Button variant="primary" onClick={() => setCreating(true)}>Добавить пользователя</Button>}
      />
      {error && <p className="error" role="alert">{error}</p>}
      {message && <p className="notice" role="status">{message}</p>}

      {creating && (
        <Panel title="Новый пользователь">
          <form className="panel-body" onSubmit={e => void submit(e)}>
            <Field
              label="Имя пользователя" value={name} maxLength={60} required
              onChange={e => setName(e.target.value)}
              autoComplete="off"
            />
            <div className="form-actions">
              <Button type="button" onClick={() => { setCreating(false); setName(''); }}>Отмена</Button>
              <Button type="submit" variant="primary" disabled={create.isPending}>Создать</Button>
            </div>
          </form>
        </Panel>
      )}

      {usage.isError && <Notice>{usage.error.message}</Notice>}
      {usage.data?.items.some(p=>p.quota_bytes>0||p.expires_at>0)&&!usage.data.source_fresh&&<p className="muted">Учёт пакетов временно недоступен. Сохранённый расход показан в таблице.</p>}
      {usage.data?.items.some(p=>p.quota_bytes>0||p.expires_at>0)&&['apply_failed','source_unavailable','unverified'].includes(usage.data.runtime_state)&&<Notice>Изменение ограничений ещё не подтверждено VPN-службой. Проверьте состояние сервера.</Notice>}
      {users.isPending ? <Loading /> : users.isError ? <Notice>{explain(users.error)}</Notice> : (
        <Panel title={`Пользователи · ${users.data.items.length}`}>
          <div className="users-toolbar"><Field label="Поиск пользователя" type="search" placeholder="Имя или ID" value={search} onChange={event=>setSearch(event.target.value)}/><label className="field"><span>Состояние</span><select value={filter} onChange={event=>setFilter(event.target.value)}><option value="all">Все пользователи</option><option value="enabled">Доступ включён</option><option value="disabled">Отключены</option><option value="blocked">Ограничены пакетом</option></select></label></div>
          <div className="table-scroll">
            {users.data.items.length === 0 ? <Empty>Добавьте пользователя, затем нажмите «Подключить», чтобы получить QR и подписку.</Empty> : (
              <table>
                <thead><tr><th scope="col">Пользователь</th><th scope="col">Состояние</th><th scope="col">Расход / пакет</th><th scope="col">Срок действия</th><th scope="col">Действия</th></tr></thead>
                <tbody>
                  {!visibleUsers.length&&<tr><td colSpan={5}><Empty>Ничего не найдено. Измените поиск или фильтр.</Empty></td></tr>}
                  {visibleUsers.map(u => (
                    <tr key={u.id}>
                      <td><Button className="user-name" variant="ghost" onClick={()=>setDetailID(u.id)}>{u.name}</Button><div className="muted">ID {u.id}</div></td>
                      <td><Badge variant={u.enabled && !packages.get(u.id)?.block_reason ? 'green' : 'red'}>{!u.enabled ? 'Отключён' : packages.get(u.id)?.block_reason ? blockLabel[packages.get(u.id)!.block_reason] ?? 'Доступ ограничен' : 'Включён'}</Badge></td>
                      <td><UsageSummary item={packages.get(u.id)} complete={usage.data?.complete_coverage ?? false}/></td>
                      <td>{packages.has(u.id)?packages.get(u.id)!.expires_at?new Date(packages.get(u.id)!.expires_at*1000).toLocaleDateString('ru-RU'):'Без срока':'Нет данных'}</td>
                      <td><div className="flex wrap">
                        <Button className="small" onClick={()=>setDetailID(u.id)}>Управление</Button>
                        <Button className="small" onClick={()=>{if(u.devices_managed){setSubscription(null);setSubscriptionUser(null);setDeviceUser(u);}else void showSubscription(u);}}>Подключить</Button>
                      </div></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </Panel>
      )}

      {deviceUser && <DevicesDialog id={deviceUser.id} name={deviceUser.name} onClose={()=>setDeviceUser(null)}/>}
      {detailUser&&<UserDetails key={detailUser.id} user={detailUser} usage={usage.data} account={packages.get(detailUser.id)} onClose={()=>setDetailID(null)} onConnect={()=>{setDetailID(null);void showSubscription(detailUser);}} onToggle={()=>toggle.mutateAsync(detailUser)} onDelete={()=>remove.mutateAsync(detailUser)}/>}

      {subscription && subscriptionUser && (
        <Dialog title={`Подключить · ${subscriptionUser.name}`} onClose={()=>{setSubscription(null);setSubscriptionUser(null);setConnections([]);setSelectedQR(null);}}>
          <div className="panel-body">
            <DirectAccess links={subscription.links} ids={subscription.link_ids} subscription={{title:subscription.title,url:subscription.url,qrURL:`/api/users/${subscriptionUser.id}/qr?format=subscription`}} qrURL={selection => `/api/users/${subscriptionUser.id}/qr?format=link&selection=${encodeURIComponent(selection)}`} onCopy={copy} onError={() => setError('QR недоступен: обновите подключения или скопируйте ссылку вручную.')}/>
            <details className="profile-advanced"><summary>Дополнительные форматы</summary>
              <div className="profile-settings">
                <div className="flex wrap"><a className="button inline-action" href={`/api/users/${subscriptionUser.id}/export?format=raw`} download>Скачать ссылки</a><a className="button inline-action" href={`/api/users/${subscriptionUser.id}/export?format=base64`} download>Скачать Base64</a><a className="button inline-action" href={`/api/users/${subscriptionUser.id}/export?format=url`} download>Скачать URL</a></div>
              </div>
            </details>
            <div className="protocol-grid" aria-label="Отдельные VPN-подключения">
              {connections.filter(connection => connection.format === 'conf').map(connection => <div className="protocol-card" key={connection.kind}>
                <strong>{connection.label}</strong>
                <p className="muted">{connection.format === 'uri' ? 'Прямая ссылка на подключение' : 'Конфигурационный файл для совместимого клиента'}</p>
                {connection.link ? <pre className="code">{connection.link}</pre> : null}
                <div className="flex wrap">
                  {connection.link && <Button className="small" onClick={() => void copy(connection.link!)}>Копировать</Button>}
                  {connection.configuration && <Button className="small" onClick={() => void copy(connection.configuration!)}>Скопировать конфиг</Button>}
                  {connection.kind !== 'vless-reality' && <a className="button inline-action small" download href={`/api/users/${subscriptionUser.id}/connections/${connection.kind}/export`}>Скачать</a>}
                  {connection.kind === 'vless-reality'
                    ? <Button className="small" onClick={() => setSelectedQR('vless')}>QR</Button>
                    : <Button className="small" onClick={() => setSelectedQR(connection.kind)}>QR</Button>}
                </div>
              </div>)}
            </div>
            {selectedQR && <div className="qr-preview">
              <p className="muted">QR: {selectedQR}</p>
              <img
                className="qr-image"
                src={selectedQR === 'vless'
                  ? `/api/users/${subscriptionUser.id}/qr?format=vless`
                  : `/api/users/${subscriptionUser.id}/connections/${selectedQR}/export?format=qr`}
                alt={`QR-конфигурация ${selectedQR}`}
              />
              <Button onClick={() => setSelectedQR(null)}>Закрыть QR</Button>
            </div>}
            <div className="form-actions">
              <Button onClick={() => { setSubscription(null); setSubscriptionUser(null); setConnections([]); setSelectedQR(null); }}>Закрыть</Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  );
}
