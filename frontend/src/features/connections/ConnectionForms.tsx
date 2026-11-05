import {useState, type FormEvent} from 'react';
import {useQuery} from '@tanstack/react-query';
import {api, type ProtocolKind, type VPNConfig} from '../../shared/api';
import {Advanced, Button, Field, Notice} from '../../shared/ui';
import TargetPicker from '../targets/TargetPicker';
import type {RecommendationState} from '../targets/model';

export const protocolCatalog: {kind: ProtocolKind; name: string; description: string; port: number}[] = [
 {kind: 'vless-xhttp', name: 'VLESS + XHTTP + Reality', description: 'Транспорт Xray через TCP', port: 2087},
 {kind: 'hysteria2', name: 'Hysteria2', description: 'QUIC / UDP с сертификатом сервера', port: 8444},
 {kind: 'wireguard', name: 'WireGuard', description: 'Туннель UDP, отдельные ключи подключения', port: 51820},
 {kind: 'amneziawg', name: 'AmneziaWG 3.1', description: 'Туннель UDP с защитой заголовков', port: 51821},
];
type Defaults = {address: string; address_source: string; ports: Record<string, number>};
export type RealityInput = {address: string; target: string; port: number};
export type ProtocolInput = RealityInput & {kind: ProtocolKind; sni: string};
const useDefaults = () => useQuery({queryKey: ['connection-defaults'], queryFn: () => api<Defaults>('/api/vpn/defaults'), staleTime: 0});

export function RealitySetup({busy, onSubmit}: {busy: boolean; onSubmit: (input: RealityInput) => Promise<void>}) {
 const defaults = useDefaults();
 const [manualAddress, setAddress] = useState<string>();
 const [manualPort, setPort] = useState<number>();
 const [target, setTarget] = useState<string>();
 const [recommended, setRecommended] = useState('');
 const [recommendationState, setRecommendationState] = useState<RecommendationState>('checking');
 const address = manualAddress ?? defaults.data?.address ?? '';
 const port = manualPort ?? defaults.data?.ports['vless-reality'] ?? 443;
 const chosenTarget = target ?? recommended;
 const submit = (event: FormEvent<HTMLFormElement>) => {event.preventDefault(); void onSubmit({address, port, target: chosenTarget});};
 return <form className="panel-body" onSubmit={submit}>
  <p>Подберём SNI по проверке с этого сервера. Ключи будут созданы при применении.</p>
  {defaults.isError && <Notice>Не удалось получить параметры сервера: {defaults.error.message}. Их можно указать вручную.</Notice>}
  {!defaults.data?.address && <Field label="Публичный IP VPS" required value={address} onChange={event => setAddress(event.target.value)}/>}
  <div className="kv"><span>Подключение</span><span>{address || 'Нужен публичный IP'}:{port} · TCP</span></div>
  <div className="kv"><span>Reality SNI</span><span>{chosenTarget || (recommendationState === 'checking' ? 'Подбираем по TCP/TLS-проверке…' : 'Автоподбор не нашёл подходящий SNI')}</span></div>
  <Advanced>
   {!!defaults.data?.address && <Field label="Публичный IP VPS" required value={address} onChange={event => setAddress(event.target.value)}/>}
   <Field label="TCP-порт" type="number" required min={1} max={65535} value={port} onChange={event => setPort(Number(event.target.value))}/>
   <TargetPicker automatic sameName onStatus={setRecommendationState} onSelect={candidate => setTarget(candidate.host)} onRecommend={candidate => setRecommended(candidate.host)}/>
   <Field label="Reality target и SNI" required value={chosenTarget} onChange={event => setTarget(event.target.value)}/>
   <Button type="button" onClick={() => {setAddress(undefined); setPort(undefined); setTarget(undefined);}}>Вернуть автоматические параметры</Button>
  </Advanced>
  {!chosenTarget && <p className="muted">Если подбор не найдёт подходящий SNI, откройте расширенные настройки для повторной проверки или ручного ввода.</p>}
  <p className="muted">Порт предложен по конфигурации панели; окончательная проверка выполняется при применении. Проверка SNI не подтверждает VPN-подключение.</p>
  <div className="form-actions"><Button type="submit" variant="primary" disabled={busy || defaults.isPending || !address || !chosenTarget}>Создать и активировать</Button></div>
 </form>;
}

type Draft = {port?: number; target?: string; sni?: string; targetManual?: boolean};
export function ProtocolSetup({vpn, available, busy, onSubmit}: {vpn: VPNConfig; available: ProtocolKind[]; busy: boolean; onSubmit: (input: ProtocolInput) => Promise<void>}) {
 const defaults = useDefaults();
 const [selection, setSelection] = useState<ProtocolKind>('vless-xhttp');
 const kind = available.includes(selection) ? selection : available[0];
 const [drafts, setDrafts] = useState<Partial<Record<ProtocolKind, Draft>>>({});
 const chosen = protocolCatalog.find(item => item.kind === kind)!;
 const draft = drafts[kind] ?? {};
 const port = draft.port ?? defaults.data?.ports[kind] ?? chosen.port;
 const target = kind === 'vless-xhttp' ? draft.target ?? vpn.target ?? '' : '';
 const sni = kind === 'vless-xhttp' ? draft.sni ?? vpn.sni ?? target : kind === 'hysteria2' ? vpn.address ?? '' : '';
 const update = (values: Draft) => setDrafts(all => ({...all, [kind]: {...all[kind], ...values}}));
 const submit = (event: FormEvent<HTMLFormElement>) => {event.preventDefault(); void onSubmit({kind, address: vpn.address ?? '', port, target, sni});};
 return <form className="panel-body" onSubmit={submit}>
  <label className="field"><span>Тип подключения</span><select aria-label="Тип подключения" value={kind} onChange={event => setSelection(event.target.value as ProtocolKind)}>{protocolCatalog.filter(item => available.includes(item.kind)).map(item => <option value={item.kind} key={item.kind}>{item.name}</option>)}</select></label>
  <p className="muted">{chosen.description}</p>
  <div className="kv"><span>Сервер и порт</span><span>{vpn.address}:{port} · {kind === 'vless-xhttp' ? 'TCP' : 'UDP'}</span></div>
  {sni && <div className="kv"><span>TLS / SNI</span><span>{sni}</span></div>}
  {kind === 'hysteria2' && <Notice>Сертификат должен покрывать IP {vpn.address}. Посторонний SNI не подставляется.</Notice>}
  <Advanced>
   <Field label="Публичный IP" value={vpn.address ?? ''} readOnly/>
   <Field label={kind === 'vless-xhttp' ? 'TCP-порт' : 'UDP-порт'} type="number" required min={1} max={65535} value={port} onChange={event => update({port: Number(event.target.value)})}/>
   {kind === 'vless-xhttp' && <>
    <TargetPicker automatic onSelect={candidate => update({target: candidate.host, sni: candidate.sni, targetManual: true})} onRecommend={candidate => setDrafts(all => all[kind]?.targetManual ? all : {...all, [kind]: {...all[kind], target: candidate.host, sni: candidate.sni}})}/>
    <Field label="Reality target (если пусто — как у основного VLESS)" value={target} onChange={event => update({target: event.target.value, targetManual: true})}/>
    <Field label="SNI" value={sni} onChange={event => update({sni: event.target.value, targetManual: true})}/>
   </>}
   <Button type="button" onClick={() => setDrafts(all => ({...all, [kind]: {}}))}>Вернуть автоматические параметры</Button>
  </Advanced>
  <p className="muted">Ключи и служебные параметры создаются автоматически. Доступность порта и движка проверяется при применении.</p>
  <div className="form-actions"><Button type="submit" variant="primary" disabled={busy || defaults.isPending}>Создать и применить</Button></div>
 </form>;
}
