import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type VPNUser, type Subscription, type ConnectionProfile } from '../shared/api';
import { Badge, Button, Field, Loading, PageHeading, Panel, Notice, Empty } from '../shared/ui';

type QRFormat = 'subscription' | 'vless';

export default function Users() {
  const cache = useQueryClient();
  const users = useQuery({ queryKey: ['users'], queryFn: () => api<{ items: VPNUser[] }>('/api/users') });
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [subscription, setSubscription] = useState<Subscription | null>(null);
  const [subscriptionUser, setSubscriptionUser] = useState<VPNUser | null>(null);
  const [qrFormat, setQRFormat] = useState<QRFormat>('subscription');
  const [connections, setConnections] = useState<ConnectionProfile[]>([]);
  const [selectedQR, setSelectedQR] = useState<string | null>(null);

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
        description="Управление доступом и ссылками подписок"
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

      {users.isPending ? <Loading /> : users.isError ? <Notice>{explain(users.error)}</Notice> : (
        <Panel title={`Пользователи · ${users.data.items.length}`}>
          <div className="table-scroll">
            {users.data.items.length === 0 ? <Empty>Сначала создай VPN-подключение, затем добавь пользователя.</Empty> : (
              <table>
                <thead><tr><th scope="col">Клиент</th><th scope="col">Состояние</th><th scope="col">Дата создания</th><th scope="col">Действия</th></tr></thead>
                <tbody>
                  {users.data.items.map(u => (
                    <tr key={u.id}>
                      <td><strong>{u.name}</strong><div className="muted">ID {u.id}</div></td>
                      <td><Badge variant={u.enabled ? 'green' : 'red'}>{u.enabled ? 'Включён' : 'Отключён'}</Badge></td>
                      <td>{new Date(u.created_at * 1000).toLocaleDateString('ru-RU')}</td>
                      <td><div className="flex wrap">
                        <Button className="small" onClick={() => void showSubscription(u)}>Подписка / QR</Button>
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
