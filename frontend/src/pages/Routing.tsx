import {lazy, Suspense, useCallback, useState} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {api, ApiError, type VPNConfig} from '../shared/api';
import {PageHeading, Panel, Notice, Loading, Button, Field, Badge} from '../shared/ui';
import {changeDraft, draftError, moveBackup, newDraft, nodeTitle, type ManagedNode, type RouteList, type RoutePolicy, type RoutePreview} from '../features/routing/model';

const RouteCanvas = lazy(() => import('../features/routing/RouteCanvas'));
type Decision = {at: number; active_exit?: string; effective_path: string; reason: string; switched: boolean};
type Simulation = {simulated: boolean; verified: boolean; published: boolean; decisions: Decision[]};
const errorMessage = (e: unknown) => e instanceof Error ? e.message : 'Не удалось выполнить операцию';
const fieldTitles: Record<string, string> = {policy: 'Маршрут', name: 'Название', mode: 'Режим', entry_node: 'Вход', primary_exit: 'Основной выход', backup_exits: 'Резервные выходы', failure_mode: 'Политика отказа', failures_to_switch: 'Порог ошибок', successes_to_recover: 'Порог восстановления', cooldown_seconds: 'Время ожидания'};
const pathTitles: Record<string, string> = {direct: 'Прямой выход', cascade: 'Каскад', blocked: 'Заблокирован', fail_open: 'Прямой обход при отказе'};

export default function Routing() {
  const cache = useQueryClient();
  const vpn = useQuery({queryKey: ['vpn'], queryFn: () => api<VPNConfig>('/api/vpn')});
  const routes = useQuery({queryKey: ['route-policies'], queryFn: () => api<RouteList>('/api/routes')});
  const nodes = useQuery({queryKey: ['nodes'], queryFn: () => api<{items: ManagedNode[]}>('/api/nodes')});
  const available = nodes.data?.items ?? [];
  const policies = routes.data?.items ?? [];
  const nodeIDs = ['local', ...available.map(node => node.id)];
  const [draft, setDraft] = useState<RoutePolicy>(newDraft);
  const [view, setView] = useState<'diagram' | 'table'>(() => window.matchMedia('(max-width: 680px)').matches ? 'table' : 'diagram');
  const [preview, setPreview] = useState<{key: string; data: RoutePreview} | null>(null);
  const [simulation, setSimulation] = useState<{id: string; version: number; data: Simulation} | null>(null);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [conflict, setConflict] = useState(false);
  const candidate = {...draft, name: draft.name.trim()};
  const key = JSON.stringify(candidate);
  const validation = draftError(candidate, nodeIDs, policies);
  const editing = !!draft.id;
  const visiblePreview = preview?.key === key ? preview.data : null;
  const update = useCallback((next: RoutePolicy) => {
    setDraft(current => changeDraft(current, next));
    setPreview(null); setError(''); setMessage('');
  }, []);
  const patch = (value: Partial<RoutePolicy>) => update(changeDraft(draft, value));
  const report = (e: unknown) => {
    setError(errorMessage(e));
    if (e instanceof ApiError && e.status === 409) {
      setConflict(true);
      void cache.invalidateQueries({queryKey: ['route-policies']});
    }
  };
  const reset = () => {setDraft(newDraft()); setPreview(null); setError(''); setConflict(false);};
  const check = useMutation({
    mutationFn: async (policy: RoutePolicy) => ({key: JSON.stringify(policy), data: await api<RoutePreview>('/api/routes/preview', 'POST', policy)}),
    onSuccess: result => {setPreview(result); setError('');}, onError: report,
  });
  const save = useMutation({
    mutationFn: (policy: RoutePolicy) => policy.id
      ? api<RoutePolicy>(`/api/routes/${encodeURIComponent(policy.id)}`, 'PUT', {policy, expected_version: policy.version})
      : api<RoutePolicy>('/api/routes', 'POST', policy),
    onSuccess: () => {reset(); setMessage('Черновик сохранён. VPN и подписки не изменены.'); void cache.invalidateQueries({queryKey: ['route-policies']});},
    onError: report,
  });
  const remove = useMutation({
    mutationFn: (policy: RoutePolicy) => api(`/api/routes/${encodeURIComponent(policy.id)}`, 'DELETE', {expected_version: policy.version}),
    onSuccess: (_, policy) => {
      if (draft.id === policy.id) reset();
      setSimulation(null); setMessage('Черновик удалён.');
      void cache.invalidateQueries({queryKey: ['route-policies']});
    }, onError: report,
  });
  const startEdit = (policy: RoutePolicy) => {
    setDraft({...policy, backup_exits: [...policy.backup_exits]}); setPreview(null); setError(''); setMessage(''); setConflict(false);
    document.getElementById('route-editor')?.scrollIntoView({block: 'start', behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'});
  };
  const simulate = useMutation({
    mutationFn: async (policy: RoutePolicy) => {
      const first = policy.mode === 'direct' ? policy.entry_node : (policy.primary_exit ?? '');
      const health = (primary: string) => ({[policy.entry_node]: 'healthy', [first]: primary, ...Object.fromEntries(policy.backup_exits.map(id => [id, 'healthy']))});
      const base = 1_700_000_000;
      const samples = policy.mode === 'direct' ? [{at: base, health: health('healthy')}, {at: base + 1, health: health('unhealthy')}] : [
        {at: base, health: health('healthy')},
        ...Array.from({length: policy.failures_to_switch}, (_, i) => ({at: base + 1 + i, health: health('unhealthy')})),
        ...Array.from({length: policy.successes_to_recover}, (_, i) => ({at: base + policy.cooldown_seconds + policy.failures_to_switch + 1 + i, health: health('healthy')})),
      ];
      return {id: policy.id, version: policy.version, data: await api<Simulation>(`/api/routes/${encodeURIComponent(policy.id)}/simulate`, 'POST', {samples})};
    }, onSuccess: result => {setSimulation(result); setError('');}, onError: report,
  });
  const busy = save.isPending || remove.isPending;
  const unavailable = nodes.isPending || nodes.isError || routes.isPending || routes.isError;
  const latest = policies.find(policy => policy.id === draft.id);
  const title = (id: string) => nodeTitle(id, available);
  const describeChange = (field: string, value: string) => {
    if (field === 'entry_node' || field === 'primary_exit') return value ? title(value) : '—';
    if (field === 'backup_exits') return value ? value.split(',').map(title).join(' → ') : 'Нет';
    if (field === 'policy') return value === 'not present' ? 'Отсутствует' : 'Новый черновик';
    if (field === 'failure_mode') return value === 'closed' ? 'Блокировать при отказе' : value === 'open' ? 'Прямой обход' : '—';
    if (field === 'mode') return value === 'direct' ? 'Прямой' : value === 'cascade' ? 'Каскад' : value;
    return value || '—';
  };
  const actions = (policy: RoutePolicy) => <div className="flex route-actions">
    <Button type="button" disabled={busy} onClick={() => startEdit(policy)}>Редактировать</Button>
    <Button type="button" disabled={busy || simulate.isPending} onClick={() => simulate.mutate(policy)}>Симулировать отказ</Button>
    <Button type="button" variant="danger" disabled={busy} onClick={() => {if (window.confirm(`Удалить черновик «${policy.name}»?`)) remove.mutate(policy);}}>Удалить черновик</Button>
  </div>;

  return <>
    <PageHeading title="Маршруты" description="Конструктор прямых маршрутов, каскадов и резервных выходов"/>
    <Notice>Только черновики: схема НЕ применяется к Xray и НЕ меняет подписки. Работоспособность VPN и автоматическое переключение ещё не подтверждены.</Notice>
    {vpn.isError && <Notice>{vpn.error.message}</Notice>}
    {vpn.data && !vpn.data.configured && <Notice>Локальный VPN ещё не настроен. Сохранение маршрутов его не активирует.</Notice>}
    {nodes.isError && <Notice>Не удалось загрузить ноды. Обновите данные перед сохранением.</Notice>}
    {error && <p role="alert" className="error">{error}</p>}
    {message && <p role="status" className="notice">{message}</p>}
    {conflict && <div className="notice" role="alert">Версия на сервере изменилась. Ваши правки сохранены в форме. {latest ? <Button type="button" disabled={busy} onClick={() => startEdit(latest)}>Загрузить серверную версию (заменить правки)</Button> : <Button type="button" onClick={() => {setDraft({...draft, id: '', version: 0}); setConflict(false);}}>Сохранить правки новым черновиком</Button>}</div>}
    <Panel id="route-editor" title={editing ? `Редактирование: ${draft.name || 'маршрут'} · r${draft.version}` : 'Новый маршрут'} actions={<div className="node-view-switch" role="group" aria-label="Представление маршрутов">
      <Button type="button" aria-pressed={view === 'diagram'} variant={view === 'diagram' ? 'primary' : 'default'} onClick={() => setView('diagram')}>Схема</Button>
      <Button type="button" aria-pressed={view === 'table'} variant={view === 'table' ? 'primary' : 'default'} onClick={() => setView('table')}>Таблица</Button>
    </div>}>
      {view === 'diagram' && !unavailable && <Suspense fallback={<Loading/>}><RouteCanvas draft={draft} nodes={available} policies={policies} disabled={busy} onChange={update}/></Suspense>}
      {view === 'diagram' && unavailable && <div className="panel-body"><p className="muted">Для схемы нужны актуальные ноды и маршруты.</p><Button type="button" onClick={() => {void nodes.refetch(); void routes.refetch();}}>Обновить данные</Button></div>}
      <form className="panel-body route-form" onSubmit={event => {event.preventDefault(); if (validation) {setError(validation); return;} save.mutate(candidate);}}>
        <fieldset disabled={busy} className="route-fieldset">
          <Field label="Название маршрута" value={draft.name} maxLength={80} required onChange={event => patch({name: event.target.value})}/>
          <div className="node-form-grid">
            <label className="field"><span>Режим</span><select aria-label="Режим" value={draft.mode} onChange={event => patch({mode: event.target.value as RoutePolicy['mode']})}><option value="direct">Прямой: вход → интернет</option><option value="cascade">Каскад: вход → выход → интернет</option></select></label>
            <label className="field"><span>Входная нода</span><select aria-label="Входная нода" value={draft.entry_node} onChange={event => patch({entry_node: event.target.value})}>{!nodeIDs.includes(draft.entry_node) && <option value={draft.entry_node}>{title(draft.entry_node)}</option>}{nodeIDs.map(id => <option key={id} value={id}>{title(id)}</option>)}</select></label>
          </div>
          {draft.mode === 'cascade' && <>
            <label className="field"><span>Основной выход</span><select aria-label="Основной выход" required value={draft.primary_exit ?? ''} onChange={event => patch({primary_exit: event.target.value})}><option value="">Выберите выход</option>{draft.primary_exit && !nodeIDs.includes(draft.primary_exit) && <option value={draft.primary_exit}>{title(draft.primary_exit)}</option>}{nodeIDs.filter(id => id !== draft.entry_node).map(id => <option key={id} value={id}>{title(id)}</option>)}</select></label>
            <fieldset className="route-backups"><legend>Резервные выходы · {draft.backup_exits.length}/4</legend>
              <p className="muted">Резерв выбирается по порядку, когда основной выход недоступен. Это не параллельная балансировка.</p>
              <div className="route-backup-options">{nodeIDs.filter(id => id !== draft.entry_node && id !== draft.primary_exit).map(id => <label key={id}><input type="checkbox" checked={draft.backup_exits.includes(id)} disabled={draft.backup_exits.length >= 4 && !draft.backup_exits.includes(id)} onChange={event => patch({backup_exits: event.target.checked ? [...draft.backup_exits, id] : draft.backup_exits.filter(value => value !== id)})}/>{title(id)}</label>)}</div>
              {draft.backup_exits.length > 0 && <ol className="route-backup-order">{draft.backup_exits.map((id, index) => <li key={id}><span>{title(id)}</span><div className="flex"><Button type="button" aria-label={`Поднять резерв ${title(id)}`} disabled={index === 0} onClick={() => update(moveBackup(draft, index, -1))}>↑</Button><Button type="button" aria-label={`Опустить резерв ${title(id)}`} disabled={index === draft.backup_exits.length - 1} onClick={() => update(moveBackup(draft, index, 1))}>↓</Button><Button type="button" aria-label={`Убрать резерв ${title(id)}`} onClick={() => patch({backup_exits: draft.backup_exits.filter(value => value !== id)})}>Убрать</Button></div></li>)}</ol>}
            </fieldset>
            <details className="route-settings"><summary>Параметры симуляции отказа</summary><div className="node-form-grid">
              <Field label="Ошибок до переключения" type="number" required min={1} max={20} step={1} value={draft.failures_to_switch} onChange={event => patch({failures_to_switch: Number(event.target.value)})}/>
              <Field label="Успехов до восстановления" type="number" required min={1} max={20} step={1} value={draft.successes_to_recover} onChange={event => patch({successes_to_recover: Number(event.target.value)})}/>
              <Field label="Ожидание до возврата, секунд" type="number" required min={0} max={86400} step={1} value={draft.cooldown_seconds} onChange={event => patch({cooldown_seconds: Number(event.target.value)})}/>
            </div></details>
          </>}
          <p className="route-path" aria-label="Путь черновика">Клиент → {title(draft.entry_node)}{draft.mode === 'cascade' ? ` → ${draft.primary_exit ? title(draft.primary_exit) : 'выберите выход'}` : ''} → Интернет</p>
          <p className="muted">{draft.failure_mode === 'closed' ? 'При отказе: блокировать, без прямого обхода (fail-closed).' : 'Сохранённая политика допускает прямой обход при отказе (fail-open).'} Все изменения сохраняются только в черновике.</p>
          {draft.failure_mode === 'open' && <Button type="button" onClick={() => patch({failure_mode: 'closed'})}>Использовать безопасный fail-closed</Button>}
          {validation && draft.name.trim() && <p className="error" role="status">{validation}</p>}
          <div className="flex route-actions"><Button type="button" disabled={check.isPending || !!validation || unavailable} onClick={() => check.mutate(candidate)}>{check.isPending ? 'Проверка…' : 'Проверить граф и изменения'}</Button><Button type="submit" variant="primary" disabled={!!validation || unavailable || conflict}>{save.isPending ? 'Сохранение…' : editing ? 'Сохранить изменения' : 'Сохранить черновик'}</Button><Button type="button" onClick={() => {reset(); setMessage('');}}>Сбросить форму</Button></div>
        </fieldset>
        {visiblePreview && <div className="route-preview" role="status"><strong>Граф проверен сервером · {visiblePreview.edges.length} межнодовых связей</strong><p className="muted">Конфигурация нод не изменена. План изменений черновика:</p>{visiblePreview.changes.length === 0 ? <p>Изменений нет.</p> : <div className="table-scroll"><table><thead><tr><th>Поле</th><th>Было</th><th>Станет</th></tr></thead><tbody>{visiblePreview.changes.map(change => <tr key={change.field}><td>{fieldTitles[change.field] ?? change.field}</td><td>{describeChange(change.field, change.before)}</td><td>{describeChange(change.field, change.after)}</td></tr>)}</tbody></table></div>}</div>}
      </form>
    </Panel>
    <Panel title={`Сохранённые черновики · ${policies.length}`}>
      {routes.isPending ? <Loading/> : routes.isError ? <div className="panel-body"><Notice>{routes.error.message}</Notice><Button type="button" onClick={() => void routes.refetch()}>Повторить загрузку</Button></div> : policies.length === 0 ? <div className="empty">Маршрутов ещё нет. Выберите вход и выход на схеме или в форме.</div> : view === 'table' ? <div className="table-scroll route-policy-table"><table><thead><tr><th>Маршрут</th><th>Вход → выход</th><th>Резерв по порядку</th><th>Состояние</th><th>Действия</th></tr></thead><tbody>{policies.map(policy => <tr key={policy.id}><td data-label="Маршрут"><strong>{policy.name}</strong><small className="node-secondary">r{policy.version}</small></td><td data-label="Вход → выход">{title(policy.entry_node)} → {policy.mode === 'direct' ? 'Интернет' : title(policy.primary_exit ?? '')}</td><td data-label="Резерв по порядку">{policy.backup_exits.map(title).join(' → ') || 'Нет'}</td><td data-label="Состояние"><Badge>Черновик</Badge></td><td data-label="Действия">{actions(policy)}</td></tr>)}</tbody></table></div> : <div className="panel-body route-list">{policies.map(policy => <article key={policy.id} className="route-policy-card"><div className="route-card-heading"><h3>{policy.name} <small>r{policy.version}</small></h3><Badge>Черновик</Badge></div><p className="route-path">Клиент → {title(policy.entry_node)}{policy.mode === 'cascade' ? ` → ${title(policy.primary_exit ?? '')}` : ''} → Интернет</p><p className="muted">{policy.backup_exits.length ? `Резерв: ${policy.backup_exits.map(title).join(' → ')}` : 'Без резервных выходов'}</p>{actions(policy)}</article>)}</div>}
    </Panel>
    {simulation && policies.some(policy => policy.id === simulation.id && policy.version === simulation.version) && <Panel title="Симуляция отказа: основной выход недоступен, затем восстановлен"><div className="panel-body"><Notice>Только модель с заданными наблюдениями. Реальная сеть и подписки не проверяются и не изменяются.</Notice><ol>{simulation.data.decisions.map((decision, index) => <li key={index}>Шаг {index + 1}: {pathTitles[decision.effective_path] ?? decision.effective_path}{decision.active_exit ? ` · ${title(decision.active_exit)}` : ''}{decision.switched ? ' · переключение' : ''}</li>)}</ol><Button type="button" onClick={() => setSimulation(null)}>Скрыть симуляцию</Button></div></Panel>}
  </>;
}
