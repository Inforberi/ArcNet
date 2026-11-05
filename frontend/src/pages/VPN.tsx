import { useEffect, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type VPNConfig, type ProtocolProfile, type ProtocolKind, type ProtocolHealth } from '../shared/api';
import TargetPicker from '../features/targets/TargetPicker';
import { Badge, Button, Field, PageHeading, Panel, Loading, Notice } from '../shared/ui';

const catalog: {kind: ProtocolKind; name: string; description: string; port: number}[] = [
  { kind: 'vless-xhttp', name: 'VLESS + XHTTP + Reality', description: 'Дополнительный TLS-подобный транспорт Xray (TCP)', port: 2087 },
  { kind: 'hysteria2', name: 'Hysteria2', description: 'QUIC / UDP. Использует проверяемый TLS-сертификат панели', port: 8444 },
  { kind: 'wireguard', name: 'WireGuard', description: 'Отдельные ключи на устройство, туннель UDP', port: 51820 },
  { kind: 'amneziawg', name: 'AmneziaWG 3.1', description: 'Туннель UDP с защитой заголовков; нужен совместимый клиент', port: 51821 },
];

function explanation(err: unknown): string { return err instanceof Error ? err.message : 'Не удалось выполнить операцию'; }

export default function VPN() {
  const client = useQueryClient();
  const vpn = useQuery({ queryKey: ['vpn'], queryFn: () => api<VPNConfig>('/api/vpn') });
  const protocols = useQuery({ queryKey: ['protocols'], queryFn: () => api<{ items: ProtocolProfile[] }>('/api/protocols') });
  const health = useQuery({queryKey:['protocol-health'],queryFn:()=>api<{items:ProtocolHealth[]}>('/api/protocols/health'),refetchInterval:30000});
  const [address, setAddress] = useState('');
  const [target, setTarget] = useState('');
  const [port, setPort] = useState(443);
  const [kind, setKind] = useState<ProtocolKind>('vless-xhttp');
  const [extraPort, setExtraPort] = useState(2087);
  const [sni, setSni] = useState('');
  const [extraTarget, setExtraTarget] = useState('');
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');

  const configuredKinds = new Set(protocols.data?.items.map(item => item.kind) ?? []);
  const chosen = catalog.find(item => item.kind === kind) ?? catalog[0];

  useEffect(() => { if (vpn.data?.address) setAddress(vpn.data.address); }, [vpn.data?.address]);
  useEffect(() => {
    const firstAvailable = catalog.find(item => !configuredKinds.has(item.kind));
    if (firstAvailable && configuredKinds.has(kind)) {
      setKind(firstAvailable.kind);
      setExtraPort(firstAvailable.port);
    }
  }, [protocols.data]); // Update the selection when a profile is created or deleted.

  const setup = useMutation({
    mutationFn: () => api('/api/vpn', 'POST', { address, target, port }),
    onSuccess: async () => { await Promise.all([client.invalidateQueries({ queryKey: ['vpn'] }), client.invalidateQueries({ queryKey: ['protocols'] })]); },
  });
  const create = useMutation({
    mutationFn: () => api('/api/protocols', 'POST', {
      kind, address: vpn.data?.address, port: extraPort,
      sni: kind === 'vless-xhttp' ? sni : kind === 'hysteria2' ? vpn.data?.address : '',
      target: kind === 'vless-xhttp' ? extraTarget : '',
    }),
    onSuccess: async () => { await Promise.all([client.invalidateQueries({ queryKey: ['protocols'] }), client.invalidateQueries({ queryKey: ['users'] }),client.invalidateQueries({queryKey:['protocol-health']})]); },
  });
  const toggle = useMutation({
    mutationFn: (p: ProtocolProfile) => api(`/api/protocols/${p.kind}`, 'PATCH', { enabled: !p.enabled }),
    onSuccess: async () => { await Promise.all([client.invalidateQueries({ queryKey: ['protocols'] }),client.invalidateQueries({queryKey:['protocol-health']})]); },
  });
  const remove = useMutation({
    mutationFn: (p: ProtocolProfile) => api(`/api/protocols/${p.kind}`, 'DELETE'),
    onSuccess: async () => { await Promise.all([client.invalidateQueries({ queryKey: ['protocols'] }),client.invalidateQueries({queryKey:['protocol-health']})]); },
  });

  async function configureReality(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(''); setMessage('');
    try { await setup.mutateAsync(); setMessage('Конфигурация Reality создана и передана Xray'); }
    catch (err) { setError(explanation(err)); }
  }
  async function addProtocol(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(''); setMessage('');
    try { await create.mutateAsync(); setMessage(`${chosen.name}: конфигурация применена. Проверь соединение из VPN-клиента.`); }
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
      <Panel title="Основной VLESS + Reality TCP">
        <form className="panel-body" onSubmit={e => void configureReality(e)}>
          <div className="field-row">
            <Field label="Публичный IP VPS" required value={address} onChange={e => setAddress(e.target.value)} />
            <Field label="TCP-порт" type="number" required min={1} max={65535} value={port} onChange={e => setPort(Number(e.target.value))} />
          </div>
          <TargetPicker sameName onSelect={candidate => setTarget(candidate.host)} onRecommend={candidate => setTarget(current => current.trim() ? current : candidate.host)}/>
          <Field label="Reality target и SNI" required placeholder="www.cloudflare.com" value={target} onChange={e => setTarget(e.target.value)} />
          <Notice>Подбор заполняет пустое поле лучшим прошедшим проверку кандидатом. Ручной ввод сохраняется. Проверка target не подтверждает доступность VPN из сети пользователя.</Notice>
          <div className="form-actions"><Button type="submit" variant="primary" disabled={setup.isPending}>Создать и активировать</Button></div>
        </form>
      </Panel>
    ) : <>
      <Panel title="Основной VLESS + Reality TCP">
        <div className="panel-body">
          {([['Сервер', vpn.data.address], ['TCP-порт', vpn.data.port], ['Reality SNI', vpn.data.sni], ['Публичный ключ', vpn.data.public_key], ['Short ID', vpn.data.short_id]] as const).map(([label, value]) =>
            <div className="kv" key={label}><span>{label}</span><span>{value}</span></div>)}
          <p className="muted">Ключ Reality привязан к подпискам. Изменение требует контролируемой миграции.</p>
        </div>
      </Panel>
      <Panel title="Дополнительные протоколы">
        <div className="panel-body">
          <Notice>Активная конфигурация не гарантирует доступность из сети пользователя. Для WireGuard и AmneziaWG нужны настройки маршрутизации/NAT, а для Hysteria2 — действующий сертификат и установленный движок.</Notice>
          {protocols.isPending ? <Loading /> : protocols.isError ? <Notice>{explanation(protocols.error)}</Notice> : protocols.data.items.length === 0 ? (
            <p className="muted">Дополнительные протоколы пока не созданы.</p>
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
        <form className="panel-body" onSubmit={e => void addProtocol(e)}>
          <label className="field"><span>Тип подключения</span>
            <select value={kind} onChange={event => {
              const next = catalog.find(item => item.kind === event.target.value);
              if (!next) return;
              setKind(next.kind); setExtraPort(next.port); setSni(''); setExtraTarget('');
            }}>
              {catalog.filter(item => !configuredKinds.has(item.kind)).map(item => <option value={item.kind} key={item.kind}>{item.name}</option>)}
            </select>
          </label>
          <p className="muted">{chosen.description}</p>
          <div className="field-row">
            <Field label="Публичный IP" value={vpn.data.address ?? ''} readOnly />
            <Field label={kind === 'vless-xhttp' ? 'TCP-порт' : 'UDP-порт'} type="number" required value={extraPort} min={1} max={65535} onChange={e => setExtraPort(Number(e.target.value))} />
          </div>
          {kind === 'vless-xhttp' && <>
            <TargetPicker onSelect={candidate => {setExtraTarget(candidate.host); setSni(candidate.sni);}}/>
            <Field label="Reality target (если пусто — как у основного VLESS)" value={extraTarget} placeholder={vpn.data.target ?? 'example.com'} onChange={e => setExtraTarget(e.target.value)} />
            <Field label="SNI" value={sni} placeholder="Если пусто, используем target" onChange={e => setSni(e.target.value)} />
          </>}
          {kind === 'hysteria2' && <Notice>Hysteria2 использует сертификат на IP {vpn.data.address}. SNI фиксирован по IP: произвольный домен не поддерживается без отдельного сертификата.</Notice>}
          <div className="form-actions"><Button type="submit" variant="primary" disabled={create.isPending}>Создать и применить</Button></div>
        </form>
      </Panel>}
    </>}
  </>;
}
