import { useEffect, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type VPNUser, type Subscription, type ConnectionProfile } from '../shared/api';
import PackageDialog, {UsageSummary} from '../features/users/PackageDialog';
import DevicesDialog from '../features/users/DevicesDialog';
import {blockLabel, type UsageReply} from '../features/users/usage';
import { Badge, Button, Field, Loading, PageHeading, Panel, Notice, Empty } from '../shared/ui';

type QRFormat = 'subscription' | 'vless';

export default function Users() {
  const cache = useQueryClient();
  const users = useQuery({ queryKey: ['users'], queryFn: () => api<{ items: VPNUser[] }>('/api/users') });
  const usage = useQuery({queryKey:['user-usage'],queryFn:()=>api<UsageReply>('/api/users/usage')});
  const [packageUser,setPackageUser] = useState<VPNUser | null>(null);
  const [deviceUser,setDeviceUser] = useState<VPNUser | null>(null);
  const packages = new Map(usage.data?.items.map(item=>[item.user_id,item]));
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [subscription, setSubscription] = useState<Subscription | null>(null);
  const [subscriptionUser, setSubscriptionUser] = useState<VPNUser | null>(null);
  const [qrFormat, setQRFormat] = useState<QRFormat>('subscription');
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
      setQRFormat('subscription');
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

  async function toggleUser(u: VPNUser) {
    setError('');
    try { await toggle.mutateAsync(u); }
    catch (err) { setError(explain(err)); }
  }

  async function deleteUser(u: VPNUser) {
    if (!window.confirm(`Безвозвратно удалить пользователя «${u.name}» и отозвать его доступ?`)) return;
    setError('');
    try {
      await remove.mutateAsync(u);
      if (subscriptionUser?.id === u.id) { setSubscription(null); setSubscriptionUser(null); }
      setMessage('Пользователь удалён');
    } catch (err) { setError(explain(err)); }
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
      {usage.data && !usage.data.source_fresh && <p className="muted">Нет свежего источника учёта трафика. Известный расход сохранён; новые байты не подставляются нулями.</p>}
      {usage.data && ['apply_failed','source_unavailable','unverified'].includes(usage.data.runtime_state) && <Notice>Исполнение ограничений: {usage.data.runtime_state === 'apply_failed' ? 'изменение доступа ожидает повторного применения' : usage.data.runtime_state === 'source_unavailable' ? 'источник учёта недоступен' : 'работа VPN-службы не подтверждена'}.</Notice>}
      {users.isPending ? <Loading /> : users.isError ? <Notice>{explain(users.error)}</Notice> : (
        <Panel title={`Пользователи · ${users.data.items.length}`}>
          <div className="table-scroll">
            {users.data.items.length === 0 ? <Empty>Сначала создай VPN-подключение, затем добавь пользователя.</Empty> : (
              <table>
                <thead><tr><th scope="col">Клиент</th><th scope="col">Состояние</th><th scope="col">Расход / пакет</th><th scope="col">Дата создания</th><th scope="col">Действия</th></tr></thead>
                <tbody>
                  {users.data.items.map(u => (
                    <tr key={u.id}>
                      <td><strong>{u.name}</strong><div className="muted">ID {u.id}</div></td>
                      <td><Badge variant={u.enabled && !packages.get(u.id)?.block_reason ? 'green' : 'red'}>{!u.enabled ? 'Отключён' : packages.get(u.id)?.block_reason ? blockLabel[packages.get(u.id)!.block_reason] ?? 'Доступ ограничен' : 'Включён'}</Badge></td>
                      <td><UsageSummary item={packages.get(u.id)} complete={usage.data?.complete_coverage ?? false}/></td>
                      <td>{new Date(u.created_at * 1000).toLocaleDateString('ru-RU')}</td>
                      <td><div className="flex wrap">
                        <Button className="small" onClick={()=>{setSubscription(null);setSubscriptionUser(null);setDeviceUser(u)}}>Устройства</Button>
                        <Button className="small" disabled={!packages.has(u.id)} onClick={() => setPackageUser(u)}>Пакет / расход</Button>
                        <Button className="small" disabled={u.devices_managed} title={u.devices_managed?'Подписки доступны в разделе «Устройства»':undefined} onClick={() => void showSubscription(u)}>{u.devices_managed?'Общий ключ отключён':'Подписка / QR'}</Button>
                        <Button className="small" disabled={toggle.isPending || remove.isPending} onClick={() => void toggleUser(u)}>
                          {u.enabled ? 'Отключить' : 'Включить'}
                        </Button>
                        <Button className="small" variant="danger" disabled={remove.isPending || toggle.isPending} onClick={() => void deleteUser(u)}>Удалить</Button>
                      </div></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </Panel>
      )}

      {packageUser && usage.data && packages.get(packageUser.id) && <PackageDialog key={packageUser.id} name={packageUser.name} initial={packages.get(packageUser.id)!} status={usage.data} onClose={()=>setPackageUser(null)}/>}
      {deviceUser && <DevicesDialog id={deviceUser.id} name={deviceUser.name} onClose={()=>setDeviceUser(null)}/>}

      {subscription && subscriptionUser && (
        <Panel title={`Подключение · ${subscriptionUser.name}`}>
          <div className="panel-body">
            <p className="muted">Ссылки содержат секретные данные. Не пересылай их посторонним.</p>
            <div className="field"><label>HTTPS-ссылка подписки</label><pre className="code">{subscription.url}</pre></div>
            <div className="field"><label>Прямая VLESS-ссылка</label><pre className="code">{subscription.link}</pre></div>
            <div className="flex wrap">
              <Button onClick={() => void copy(subscription.url)}>Копировать подписку</Button>
              <Button onClick={() => void copy(subscription.link)}>Копировать VLESS</Button>
              <a className="button inline-action" href={`/api/users/${subscriptionUser.id}/export?format=raw`} download>Скачать VLESS</a>
              <a className="button inline-action" href={`/api/users/${subscriptionUser.id}/export?format=base64`} download>Скачать Base64</a>
              <a className="button inline-action" href={`/api/users/${subscriptionUser.id}/export?format=url`} download>Скачать URL</a>
            </div>
            <div className="flex wrap qr-actions" role="group" aria-label="Вид QR-кода">
              <Button onClick={() => setQRFormat('subscription')} variant={qrFormat === 'subscription' ? 'primary' : 'default'}>QR подписки</Button>
              <Button onClick={() => setQRFormat('vless')} variant={qrFormat === 'vless' ? 'primary' : 'default'}>QR VLESS</Button>
            </div>
            <img
              key={`${subscriptionUser.id}-${qrFormat}`}
              className="qr-image"
              src={`/api/users/${subscriptionUser.id}/qr?format=${qrFormat}`}
              alt={`QR-код ${qrFormat === 'subscription' ? 'подписки' : 'VLESS-подключения'}`}
            />
            <div className="protocol-grid" aria-label="Отдельные VPN-подключения">
              {connections.map(connection => <div className="protocol-card" key={connection.kind}>
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
        </Panel>
      )}
    </>
  );
}
