import {useState} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import Dialog from '../../shared/Dialog';
import {api} from '../../shared/api';
import {Badge, Button, Field, Loading, Notice, Panel} from '../../shared/ui';
import {nodeTitle, type ManagedNode, type RoutePolicy} from './model';

type Binding = {endpoint_id: string; node_id: string; inbound_tag: string; scope: 'all_tcp_users'};
type Proof = {result: string; exit_ip: string; dns_mode: string; checked_at: number; expires_at: number; current: boolean; published: boolean};
type Deployment = {proof: Proof | null; probe_grant_expires_at: number; id: string; route_id: string; route_version: number; entry_node: string; exit_node: string; state: string; version: number; binding: Binding | null; steps: {step: string; node_id: string; state: string; revision: number}[]};
type Plan = {route_id: string; route_version: number; preview_token: string; binding: Binding; entry_address: string; entry_port: number; exit_address: string; channel_port: number; sni: string; user_count: number; entry_revision: number; exit_revision: number; publication_enabled: false};
const pending = (state: string) => ['exit_queued', 'entry_queued', 'rollback_entry', 'rollback_exit'].includes(state);
const states: Record<string, string> = {prepared: 'Подготовлен', exit_queued: 'Настройка выхода B', entry_queued: 'Настройка входа A', awaiting_probe: 'Применено · VPN не проверен', rollback_entry: 'Восстановление входа A', rollback_exit: 'Восстановление выхода B', rolled_back: 'Прежние настройки восстановлены', attention: 'Нужно восстановление', probe_failed: 'Проверка пути не пройдена', verified: 'Выход подтверждён', published: 'Опубликован'};
const steps: Record<string, string> = {apply_exit: 'Настроить выход B', apply_entry: 'Подключить вход A', restore_entry: 'Восстановить вход A', restore_exit: 'Восстановить выход B'};
const stepStates: Record<string, string> = {not_queued: 'Не начато', queued: 'В очереди', running: 'Ожидание подтверждения', applied: 'Подтверждено узлом', failed: 'Ошибка или нет подтверждения', cancelled: 'Отменено'};
const supported = (p: RoutePolicy) => p.mode === 'cascade' && p.entry_node !== 'local' && p.primary_exit !== 'local' && p.failure_mode === 'closed' && !p.backup_exits.length;
const message = (e: unknown) => e instanceof Error ? e.message : 'Операция не выполнена';

export default function CascadeDeployment({policies, nodes}: {policies: RoutePolicy[]; nodes: ManagedNode[]}) {
  const cache = useQueryClient();
  const journals = useQuery({queryKey: ['cascade-deployments'], queryFn: () => api<{items: Deployment[]}>('/api/cascade-deployments'), refetchInterval: query => query.state.data?.items.some(item => pending(item.state)) ? 2000 : query.state.data?.items.some(item => item.probe_grant_expires_at * 1000 > Date.now()) ? 10000 : false, refetchIntervalInBackground: false});
  const [selected, setSelected] = useState<RoutePolicy | null>(null);
  const [port, setPort] = useState('15443');
  const [preview, setPreview] = useState<{key: string; plan: Plan} | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [error, setError] = useState('');
  const items = journals.data?.items ?? [];
  const latest = policies.find(item => item.id === selected?.id);
  const stale = !!selected && latest?.version !== selected.version;
  const key = selected ? `${selected.id}:${selected.version}:${port}` : '';
  const plan = preview?.key === key && !stale ? preview.plan : null;
  const validPort = /^\d+$/.test(port) && Number(port) >= 1024 && Number(port) <= 65535;
  const refresh = async () => {await Promise.all([cache.invalidateQueries({queryKey: ['cascade-deployments']}), cache.invalidateQueries({queryKey: ['nodes']}), cache.invalidateQueries({queryKey: ['route-policies']})]);};
  const check = useMutation({mutationFn: async ({policy, port, key}: {policy: RoutePolicy; port: number; key: string}) => ({key, plan: await api<Plan>(`/api/routes/${encodeURIComponent(policy.id)}/deployment-preview`, 'POST', {expected_version: policy.version, channel_port: port})}), onSuccess: value => {setPreview(value); setConfirmed(false); setError('');}, onError: e => {setPreview(null); setError(message(e));}});
  const apply = useMutation({mutationFn: (value: Plan) => api(`/api/routes/${encodeURIComponent(value.route_id)}/deployments`, 'POST', {expected_version: value.route_version, channel_port: value.channel_port, preview_token: value.preview_token, confirm: 'APPLY_CASCADE'}), onSuccess: () => {setSelected(null); setPreview(null); setError('');}, onError: e => {setPreview(null); setConfirmed(false); setError(message(e));}, onSettled: refresh});
  const action = useMutation({mutationFn: ({item, verb}: {item: Deployment; verb: 'start' | 'rollback' | 'delete' | 'publish'}) => api(`/api/cascade-deployments/${encodeURIComponent(item.id)}${verb === 'delete' ? '' : '/' + verb}`, verb === 'delete' ? 'DELETE' : 'POST', {expected_version: item.version, confirm: verb === 'start' ? 'APPLY_CASCADE' : verb === 'rollback' ? 'ROLLBACK_CASCADE' : verb === 'publish' ? 'PUBLISH_CASCADE' : 'DELETE_CASCADE_JOURNAL'}), onSuccess: () => setError(''), onError: e => setError(message(e)), onSettled: refresh});
  const [probeInfo, setProbeInfo] = useState('');
  const probe = useMutation({mutationFn: async (item: Deployment) => {
    const bundle = await api<{controller: string}>(`/api/cascade-deployments/${encodeURIComponent(item.id)}/probe-grant`, 'POST', {expected_version: item.version, confirm: 'CREATE_EXTERNAL_PROBE'});
    const blob = new Blob([JSON.stringify(bundle)], {type: 'application/json'});
    const url = URL.createObjectURL(blob); const link = document.createElement('a'); link.href = url; link.download = 'arcvpn-cascade-probe.json'; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
    setProbeInfo(`На независимом Linux-компьютере с доверенным Xray: chmod 600 arcvpn-cascade-probe.json; arcvpn cascade-probe --controller ${bundle.controller} --grant-file arcvpn-cascade-probe.json. Файл содержит доступ к VPN, действует 5 минут; не отправляйте его в чат. После проверки удалите файл.`);
  }, onError: e => setError(message(e)), onSettled: refresh});
  const busy = apply.isPending || action.isPending || probe.isPending;
  const open = (policy: RoutePolicy) => {setSelected(policy); setPreview(null); setConfirmed(false); setError('');};
  const title = (id: string) => nodeTitle(id, nodes);

  return <>
    <Panel title="Применение каскадов" actions={<Button type="button" disabled={busy || journals.isFetching} onClick={() => void journals.refetch()}>Обновить состояние</Button>}>
      <div className="panel-body route-list">
        <Notice>Схема сохраняется отдельно. Каскад меняет выход трафика на серверах. Адрес подключения, порт, ключи и профиль клиента сохраняются — перенастраивать клиент не нужно.</Notice>
        {error && !selected && <p role="alert" className="error">{error}</p>}
        {probeInfo && <Notice>{probeInfo}</Notice>}
        {journals.isPending ? <Loading/> : journals.isError ? <p role="alert" className="error">{journals.error.message}</p> : <>
          {!policies.some(supported) && <p className="muted">Сохраните каскад между двумя удалёнными серверами без резервов и с блокировкой при отказе.</p>}
          {policies.filter(supported).filter(policy => !items.some(item => item.route_id === policy.id)).map(policy => <article className="route-policy-card" key={policy.id}><h3>{policy.name}</h3><p>{title(policy.entry_node)} → {title(policy.primary_exit ?? '')}</p><Button type="button" disabled={busy} onClick={() => open(policy)}>Применить каскад</Button></article>)}
          {!items.length && <p className="muted">Применений пока нет.</p>}
          {items.map(item => <article className="route-policy-card" key={item.id} aria-label={`Применение ${policies.find(p => p.id === item.route_id)?.name ?? item.route_id}`}>
            <div className="route-card-heading"><h3>{policies.find(p => p.id === item.route_id)?.name ?? 'Каскад'}</h3><Badge variant={item.state === 'attention' ? 'red' : 'blue'}>{item.state === 'published' && !item.proof?.current ? 'Проверка устарела · выдача приостановлена' : (states[item.state] ?? item.state)}</Badge></div>
            <p>{title(item.entry_node)} → {title(item.exit_node)}</p><p className="muted">TCP/REALITY · клиентские профили не меняются</p>
            {!item.binding && <Notice>Операция создана старой версией без привязки подключения. Доступен откат.</Notice>}
            {item.state === 'awaiting_probe' && <Notice>Оба узла подтвердили настройки. Клиенты продолжают использовать прежний профиль. Фактический выходной IP и путь DNS ещё нужно проверить; применение можно откатить.</Notice>}
            {item.proof && <p className="muted">Выход: {item.proof.exit_ip || 'не подтверждён'} · DNS: {item.proof.dns_mode === 'doh_via_cascade' ? 'DoH через каскад' : 'не подтверждён'}. Это не проверка DNS-утечек клиентского устройства.</p>}
            {item.state === 'attention' && <Notice>Результат одного из шагов неизвестен или произошла ошибка. Не удаляйте данные нод. Откат сначала восстановит вход A, затем выход B.</Notice>}
            <ol className="cascade-steps">{item.steps.filter(step => step.state !== 'not_queued').map(step => <li key={step.step}>{steps[step.step] ?? step.step}: {stepStates[step.state] ?? step.state}{step.revision > 0 ? ` · r${step.revision}` : ''}</li>)}</ol>
            <div className="flex route-actions">
              {['awaiting_probe', 'probe_failed', 'verified', 'published'].includes(item.state) && <Button disabled={busy} onClick={() => {if (window.confirm('Скачать одноразовый доступ для независимой проверки? Предыдущий отчёт и публикация будут сняты до новой проверки.')) probe.mutate(item);}}>Внешняя проверка</Button>}
              {item.state === 'verified' && item.proof?.current && <Button variant="primary" disabled={busy} onClick={() => {if (window.confirm('Опубликовать подтверждённый вход в подписках? Клиентские адрес, порт и ключи сохранятся.')) action.mutate({item, verb: 'publish'});}}>Опубликовать маршрут</Button>}
              {item.state === 'prepared' && item.binding && <Button disabled={busy} onClick={() => {if (window.confirm('Применить подготовленный каскад для всех пользователей TCP/REALITY входной ноды?')) action.mutate({item, verb: 'start'});}}>Продолжить применение</Button>}
              {!['prepared', 'rolled_back', 'rollback_entry', 'rollback_exit'].includes(item.state) && <Button variant="danger" disabled={busy} onClick={() => {if (window.confirm('Восстановить прежние настройки: сначала вход A, затем выход B? Текущие соединения могут прерваться.')) action.mutate({item, verb: 'rollback'});}}>Откатить каскад</Button>}
              {['prepared', 'rolled_back'].includes(item.state) && <Button disabled={busy} onClick={() => {if (window.confirm(item.state === 'prepared' ? 'Отменить подготовку и освободить ноды?' : 'Удалить завершённый журнал и разрешить редактирование маршрута?')) action.mutate({item, verb: 'delete'});}}>{item.state === 'prepared' ? 'Отменить подготовку' : 'Удалить завершённый журнал'}</Button>}
            </div>
          </article>)}
        </>}
      </div>
    </Panel>
    {selected && <Dialog title={`Применение: ${selected.name}`} busy={apply.isPending} onClose={() => {setSelected(null); setPreview(null); setError('');}}>
      <div className="dialog-body">
        <p>{title(selected.entry_node)} → {title(selected.primary_exit ?? '')}</p>
        <Field label="TCP-порт межсерверного канала на B" type="number" min="1024" max="65535" value={port} disabled={apply.isPending} onChange={e => {setPort(e.target.value); setPreview(null); setConfirmed(false);}} description="Этот порт должен быть доступен с A на B. Firewall этим действием не меняется."/>
        {stale && <Notice>Черновик изменился. {latest && <Button onClick={() => {setSelected(latest); setPreview(null); setConfirmed(false);}}>Загрузить новую версию</Button>}</Notice>}
        {error && <p role="alert" className="error">{error}</p>}
        <Button disabled={!validPort || stale || check.isPending || apply.isPending} onClick={() => check.mutate({policy: selected, port: Number(port), key})}>{check.isPending ? 'Проверка…' : 'Предпросмотр изменений'}</Button>
        {plan && <div className="route-list">
          <h3>Что изменится</h3>
          <p>Подключение клиента остаётся {plan.entry_address}:{plan.entry_port}, TCP/REALITY. Все {plan.user_count} текущих доступов и их ключи сохраняются.</p>
          <p>На B добавится отдельный защищённый канал TCP/{plan.channel_port}, SNI {plan.sni}. Существующее подключение B сохраняется.</p>
          <p>Трафик A пойдёт через B ({plan.exit_address}); при отказе B прямого обхода через A не будет. Сначала B, затем A; возможен разрыв текущих соединений.</p>
          <Notice>DNS-настройки клиента не меняются; фактический путь DNS ещё нужно проверить. После подтверждений узлов статус будет «Применено · VPN не проверен».</Notice>
          <label className="flex"><input type="checkbox" checked={confirmed} disabled={apply.isPending} onChange={e => setConfirmed(e.target.checked)}/><span>Подтверждаю смену выхода на B для этого подключения</span></label>
          <Button variant="primary" disabled={!confirmed || busy} onClick={() => apply.mutate(plan)}>{apply.isPending ? 'Запуск…' : 'Применить настройки'}</Button>
        </div>}
      </div>
    </Dialog>}
  </>;
}
