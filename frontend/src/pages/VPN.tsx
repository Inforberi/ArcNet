import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type VPNConfig, type ProtocolProfile, type ProtocolHealth } from '../shared/api';
import {RealitySetup, ProtocolSetup, protocolCatalog as catalog, type RealityInput, type ProtocolInput} from '../features/connections/ConnectionForms';
import { Badge, Button, PageHeading, Panel, Loading, Notice } from '../shared/ui';

function explanation(err: unknown): string { return err instanceof Error ? err.message : 'Не удалось выполнить операцию'; }

export default function VPN() {
  const client = useQueryClient();
  const vpn = useQuery({ queryKey: ['vpn'], queryFn: () => api<VPNConfig>('/api/vpn') });
  const protocols = useQuery({ queryKey: ['protocols'], queryFn: () => api<{ items: ProtocolProfile[] }>('/api/protocols') });
  const health = useQuery({queryKey:['protocol-health'],queryFn:()=>api<{items:ProtocolHealth[]}>('/api/protocols/health')});
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [addingReality, setAddingReality] = useState(false);

  const configuredKinds = new Set(protocols.data?.items.map(item => item.kind) ?? []);

  const setup = useMutation({
    mutationFn: (input: RealityInput) => api('/api/vpn', 'POST', input),
    onSuccess: async () => { await Promise.all([client.invalidateQueries({ queryKey: ['vpn'] }), client.invalidateQueries({ queryKey: ['protocols'] }), client.invalidateQueries({queryKey:['connection-defaults']})]); },
  });
  const create = useMutation({
    mutationFn: (input: ProtocolInput) => api('/api/protocols', 'POST', input),
    onSuccess: async () => { await Promise.all([client.invalidateQueries({ queryKey: ['vpn'] }), client.invalidateQueries({ queryKey: ['protocols'] }), client.invalidateQueries({queryKey:['connection-defaults']}), client.invalidateQueries({ queryKey: ['users'] }),client.invalidateQueries({queryKey:['protocol-health']})]); },
  });
  const toggle = useMutation({
    mutationFn: (p: ProtocolProfile) => api(`/api/protocols/${p.kind}`, 'PATCH', { enabled: !p.enabled }),
    onSuccess: async () => { await Promise.all([client.invalidateQueries({ queryKey: ['protocols'] }), client.invalidateQueries({queryKey:['connection-defaults']}),client.invalidateQueries({queryKey:['protocol-health']})]); },
  });
  const remove = useMutation({
    mutationFn: (p: ProtocolProfile) => api(`/api/protocols/${p.kind}`, 'DELETE'),
    onSuccess: async () => { await Promise.all([client.invalidateQueries({ queryKey: ['vpn'] }), client.invalidateQueries({ queryKey: ['protocols'] }), client.invalidateQueries({queryKey:['connection-defaults']}),client.invalidateQueries({queryKey:['protocol-health']})]); },
  });

  async function configureReality(input: RealityInput) { setError(''); setMessage('');
    try { await setup.mutateAsync(input); setMessage('Конфигурация Reality создана и передана Xray'); }
    catch (err) { setError(explanation(err)); }
  }
  async function addProtocol(input: ProtocolInput) { setError(''); setMessage('');
    try { await create.mutateAsync(input); setMessage(`${catalog.find(item => item.kind === input.kind)?.name}: конфигурация применена. Проверь соединение из VPN-клиента.`); }
    catch (err) { setError(explanation(err)); }
  }
  async function switchProtocol(p: ProtocolProfile) {
    setError(''); setMessage('');
    try { await toggle.mutateAsync(p); setMessage(`${p.kind}: настройки обновлены`); }
    catch (err) { setError(explanation(err)); }
  }
  async function deleteProtocol(p: ProtocolProfile) {
    if (!window.confirm(`Удалить протокол ${p.kind} вместе с пользовательскими ключами?`)) return;
    setError(''); setMessage('');
    try { await remove.mutateAsync(p); setMessage('Протокол и его ключи удалены'); }
    catch (err) { setError(explanation(err)); }
  }

  return <>
    <PageHeading title="Подключения" description="Независимые серверные протоколы, их ключи и порты" />
    {error && <p className="error" role="alert">{error}</p>}
    {message && <p className="notice" role="status">{message}</p>}
    {vpn.isPending ? <Loading /> : vpn.isError ? <Notice>{explanation(vpn.error)}</Notice> : !vpn.data.configured ? (
      <Panel title="VLESS + Reality TCP">
        {addingReality ? <RealitySetup busy={setup.isPending} onSubmit={configureReality}/> : <div className="panel-body">
          <p className="muted">Необязательное подключение. XHTTP, Hysteria2 и WireGuard можно создать независимо ниже.</p>
          <Button onClick={() => setAddingReality(true)}>Добавить VLESS + Reality TCP</Button>
        </div>}
      </Panel>
    ) : (
      <Panel title="VLESS + Reality TCP">
        <div className="panel-body">
          {([['Сервер', vpn.data.address], ['TCP-порт', vpn.data.port], ['Reality SNI', vpn.data.sni], ['Публичный ключ', vpn.data.public_key], ['Short ID', vpn.data.short_id]] as const).map(([label, value]) =>
            <div className="kv" key={label}><span>{label}</span><span>{value}</span></div>)}
          <p className="muted">Ключ Reality привязан к подпискам. Изменение требует контролируемой миграции.</p>
        </div>
      </Panel>
    )}
    {!vpn.isPending && !vpn.isError && <>
      <Panel title="Протоколы подключения">
        <div className="panel-body">
          <Notice>Активная конфигурация не гарантирует доступность из сети пользователя. Для WireGuard и AmneziaWG нужны настройки маршрутизации/NAT, а для Hysteria2 — действующий сертификат и установленный движок.</Notice>
          {protocols.isPending ? <Loading /> : protocols.isError ? <Notice>{explanation(protocols.error)}</Notice> : protocols.data.items.length === 0 ? (
            <p className="muted">Выберите любой протокол ниже. Предварительная настройка VLESS TCP не требуется.</p>
          ) : <div className="protocol-grid">
            {protocols.data.items.map(profile => <article className="protocol-card" key={profile.kind}>
              <div className="flex wrap protocol-header"><strong>{catalog.find(v => v.kind === profile.kind)?.name ?? profile.kind}</strong>
                <Badge variant={profile.enabled ? 'green' : 'red'}>{profile.enabled ? 'Включён' : 'Отключён'}</Badge>
              </div>
              <p className="muted">{profile.address}:{profile.port} · {profile.kind === 'vless-xhttp' ? 'TCP' : 'UDP'}</p>
              {profile.sni && <p className="muted">TLS / SNI: {profile.sni}</p>}
              <p className="muted">Локальная служба: {health.data?.items.find(h=>h.kind===profile.kind)?.state ?? 'проверяется'} (не проверка со стороны клиента)</p>
              <div className="flex wrap">
                <Button disabled={toggle.isPending || remove.isPending} onClick={() => void switchProtocol(profile)}>
                  {profile.enabled ? 'Отключить' : 'Включить'}
                </Button>
                {!profile.enabled && <Button variant="danger" disabled={remove.isPending} onClick={() => void deleteProtocol(profile)}>Удалить</Button>}
              </div>
            </article>)}
          </div>}
        </div>
      </Panel>
      {catalog.some(item => !configuredKinds.has(item.kind)) && <Panel title="Добавить протокол">
        <ProtocolSetup vpn={vpn.data} available={catalog.filter(item => !configuredKinds.has(item.kind)).map(item => item.kind)} busy={create.isPending} onSubmit={addProtocol}/>
      </Panel>}
    </>}
  </>;
}
