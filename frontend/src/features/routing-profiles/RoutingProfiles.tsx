import {useConfirmation} from '../../shared/Confirmation';
import {useRef, useState} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {api, ApiError} from '../../shared/api';
import {Badge, Button, Field, Loading, Notice} from '../../shared/ui';
import Dialog from '../../shared/Dialog';
import {actionTitles, countEntries, emptyProfile, moveGroup, textEntries, withoutMetadata, type Action, type RoutingProfile} from './model';

type Decision = {action: Action | ''; matched?: string; status: string; reason: string; pending: string[]; network_verified: boolean};
const explainError = (error: unknown) => error instanceof Error ? error.message : 'Не удалось выполнить операцию';
const download = (body: unknown) => {
  const url = URL.createObjectURL(new Blob([JSON.stringify(body, null, 2)], {type: 'application/json'}));
  const anchor = document.createElement('a'); anchor.href = url; anchor.download = 'arcvpn-routing-profile.json'; anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
};

export default function RoutingProfiles() {
 const confirm=useConfirmation();
  const cache = useQueryClient();
  const profiles = useQuery({queryKey: ['routing-profiles'], queryFn: () => api<{items: RoutingProfile[]; server_application_enabled: boolean}>('/api/routing-profiles')});
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<RoutingProfile>(emptyProfile);
  const [initial, setInitial] = useState('');
  const [group, setGroup] = useState<Action>('direct');
  const [texts, setTexts] = useState<Record<Action, string>>({proxy: '', direct: '', block: ''});
  const [filter, setFilter] = useState('');
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [conflict, setConflict] = useState(false);
  const [address, setAddress] = useState('');
  const [decision, setDecision] = useState<Decision | null>(null);
  const [importing, setImporting] = useState(false);
  const file = useRef<HTMLInputElement>(null);
  const listFile = useRef<HTMLInputElement>(null);
  const candidate = (): RoutingProfile => ({...draft, proxy: textEntries(texts.proxy), direct: textEntries(texts.direct), block: textEntries(texts.block)});
  const checkFingerprint = useRef('');
  checkFingerprint.current = JSON.stringify({profile: candidate(), address: address.trim()});
  const dirty = JSON.stringify(candidate()) !== initial && initial !== '';
  const change = (value: Partial<RoutingProfile>) => {setDraft(current => ({...current, ...value})); setDecision(null); setError('');};
  const edit = async (profile: RoutingProfile) => {
    if (dirty && !(await confirm('Заменить несохранённые правки другим профилем?'))) return;
    setDraft(profile); setTexts({proxy: profile.proxy.join('\n'), direct: profile.direct.join('\n'), block: profile.block.join('\n')});
    setInitial(JSON.stringify(profile)); setConflict(false); setDecision(null); setError(''); setMessage(''); setGroup('direct'); setOpen(true);
  };
  const save = useMutation({
    mutationFn: (profile: RoutingProfile) => api<RoutingProfile>(profile.id ? `/api/routing-profiles/${encodeURIComponent(profile.id)}` : '/api/routing-profiles', profile.id ? 'PUT' : 'POST', {profile, expected_version: profile.version}),
    onSuccess: profile => {
      setDraft(profile); setTexts({proxy: profile.proxy.join('\n'), direct: profile.direct.join('\n'), block: profile.block.join('\n')});
      setInitial(JSON.stringify(profile)); setOpen(false); setConflict(false); setError(''); setMessage('Профиль сохранён. Подключения и подписки не изменены.');
      void cache.invalidateQueries({queryKey: ['routing-profiles']});
    },
    onError: error => {setError(explainError(error)); if (error instanceof ApiError && error.status === 409) {setConflict(true); void profiles.refetch();}},
  });
  const remove = useMutation({
    mutationFn: (profile: RoutingProfile) => api(`/api/routing-profiles/${encodeURIComponent(profile.id)}`, 'DELETE', {expected_version: profile.version}),
    onSuccess: () => {setMessage('Профиль удалён из панели. Уже импортированные клиентские профили не отзываются автоматически.'); void profiles.refetch();},
    onError: error => setError(explainError(error)),
  });
  const check = useMutation({mutationFn: (payload: {profile: RoutingProfile; address: string}) => api<Decision>('/api/routing-profiles/check', 'POST', payload), onSuccess: (result, payload) => {if (JSON.stringify(payload) === checkFingerprint.current) setDecision(result);}, onError: error => setError(explainError(error))});
  const busy = save.isPending || importing;
  const importProfile = async (selected?: File) => {
    if (!selected) return;
    setImporting(true); setError('');
    try {
      if (selected.size > 256 * 1024) throw new Error('Размер профиля — не более 256 КБ');
      const result = await api<{profile: RoutingProfile}>('/api/routing-profiles/import', 'POST', {archive: await selected.text()});
      edit(result.profile);
    } catch (error) {setError(explainError(error));} finally {setImporting(false); if (file.current) file.current.value = '';}
  };
  const importList = async (selected?: File) => {
    if (!selected) return;
    setImporting(true); setError('');
    try {
      if (selected.size > 256 * 1024) throw new Error('Размер списка — не более 256 КБ');
      const result = await api<{entries: string[]; duplicates: number}>('/api/routing-profiles/list-preview', 'POST', {text: await selected.text()});
      const baseline = await api<{entries: string[]}>('/api/routing-profiles/list-preview', 'POST', {text: texts[group]});
      const existing = baseline.entries;
      const combined = [...new Set([...existing, ...result.entries])];
      setTexts(current => ({...current, [group]: combined.join('\n')})); setDecision(null);
      setMessage(`В форму добавлено ${combined.length - existing.length} записей. Дубликаты пропущены; сохраните профиль отдельно.`);
    } catch (error) {setError(explainError(error));} finally {setImporting(false); if (listFile.current) listFile.current.value = '';}
  };
  const latest = profiles.data?.items.find(profile => profile.id === draft.id);
  const visible = profiles.data?.items.filter(profile => profile.name.toLocaleLowerCase('ru').includes(filter.toLocaleLowerCase('ru'))) ?? [];
  return <>
    <div className="routing-intro"><div><h2>Правила трафика</h2><p className="muted">Свой профиль: какие сайты направлять через VPN, напрямую или блокировать.</p></div><div className="flex wrap">
      <Button variant="primary" disabled={busy} onClick={() => edit(emptyProfile())}>Создать профиль</Button>
      <Button disabled={busy} onClick={() => file.current?.click()}>Импортировать</Button>
      {!open && dirty && <Button onClick={() => setOpen(true)}>Продолжить правки</Button>}
    </div></div>
    <input ref={file} type="file" hidden accept="application/json,.json" aria-label="Импорт профиля маршрутизации" onChange={event => void importProfile(event.target.files?.[0])}/>
    <p className="inline-status">Сейчас это редактор профилей. Сохранение не включает правила на сервере и не обновляет клиент автоматически.</p>
    {!open && error && <p className="error" role="alert">{error}</p>}
    {!open && message && <p role="status">{message}</p>}
    {profiles.isPending ? <Loading/> : profiles.isError ? <Notice>{profiles.error.message}</Notice> : <>
      {(profiles.data?.items.length ?? 0) > 3 && <Field label="Найти профиль" value={filter} onChange={event => setFilter(event.target.value)}/>}
      {visible.length === 0 ? <div className="routing-empty"><h3>{filter ? 'Профиль не найден' : 'Настройте маршрутизацию под себя'}</h3><p className="muted">Создайте профиль и добавьте нужные сайты или загрузите свои списки.</p></div> : <div className="routing-profile-grid">{visible.map(profile => <article className="routing-profile-summary" key={profile.id}>
        <div className="route-card-heading"><h3>{profile.name}</h3><Badge>{profile.enabled ? 'Правила включены в профиле' : 'Правила выключены'}</Badge></div>
        <p className="muted">{countEntries(profile)} записей · для остального: {actionTitles[profile.default_action].toLocaleLowerCase('ru')} · r{profile.version}</p>
        <Button onClick={() => edit(profile)}>Настроить</Button>
        <details className="profile-actions"><summary>Другие действия</summary><div className="flex wrap">
          <Button onClick={() => edit({...withoutMetadata(profile), name: `${profile.name.slice(0, 65)} — копия`})}>Создать копию</Button>
          <Button onClick={async () => {try {download(await api(`/api/routing-profiles/${encodeURIComponent(profile.id)}/export`));} catch (error) {setError(explainError(error));}}}>Скачать профиль ArcNet</Button>
          <Button variant="danger" disabled={remove.isPending} onClick={async () => {if ((await confirm(`Удалить профиль «${profile.name}» из панели?`))) remove.mutate(profile);}}>Удалить</Button>
        </div></details>
      </article>)}</div>}
    </>}
    {open && <Dialog title={draft.id ? 'Настройка маршрутизации' : 'Новый профиль маршрутизации'} busy={busy} onClose={() => setOpen(false)}>
      <form onSubmit={event => {event.preventDefault(); save.mutate(candidate());}}>
        <fieldset className="dialog-body route-fieldset" disabled={busy}>
          <Field label="Название профиля" value={draft.name} maxLength={80} required onChange={event => change({name: event.target.value})}/>
          <label className="routing-enable"><div><strong>Использовать правила маршрутизации</strong><small>Выключение сохраняет списки. Базовый VPN-маршрут остаётся.</small></div><input type="checkbox" checked={draft.enabled} onChange={event => change({enabled: event.target.checked})}/></label>
          {draft.enabled ? <>
            <div className="routing-group-tabs" role="group" aria-label="Списки маршрутизации">{(['proxy', 'direct', 'block'] as Action[]).map(action => <Button type="button" key={action} aria-pressed={group === action} variant={group === action ? 'primary' : 'default'} onClick={() => setGroup(action)}>{actionTitles[action]} <small>{textEntries(texts[action]).length}</small></Button>)}</div>
            {group === 'direct' && <p className="muted">Клиентский прямой выход использует IP устройства. На сервере прямой выход имеет другой смысл; здесь задаётся намерение профиля.</p>}
            <label className="field"><span>{actionTitles[group]} — домены, IP и Geo-категории</span><textarea aria-label={`${actionTitles[group]} — домены, IP и Geo-категории`} className="routing-list-input" rows={6} maxLength={256 * 1024} value={texts[group]} placeholder={'example.com\n203.0.113.0/24\ngeosite:ru'} onChange={event => {setTexts(current => ({...current, [group]: event.target.value})); setDecision(null); setError('');}}/><small>По одной записи в строке. Домен включает поддомены; full:example.com — только точное имя. Категории Geo проверяются отдельно в клиенте.</small></label>
            <input ref={listFile} hidden type="file" aria-label="Импорт списка адресов" accept="text/plain,.txt,.list" onChange={event => void importList(event.target.files?.[0])}/>
            <Button type="button" onClick={() => listFile.current?.click()}>Добавить список из файла</Button>
            {message && <p role="status">{message}</p>}
          </> : <p className="muted">Ваши {countEntries(candidate())} записей сохранены в форме. Включите правила, чтобы редактировать их.</p>}
          <details className="profile-advanced"><summary>Дополнительно: порядок, DNS и остальные адреса</summary><div className="profile-settings">
            <label className="field"><span>Если ни одно правило не подошло</span><select aria-label="Действие для остального трафика" value={draft.default_action} onChange={event => change({default_action: event.target.value as Action})}>{(['proxy', 'direct', 'block'] as Action[]).map(action => <option key={action} value={action}>{actionTitles[action]}</option>)}</select></label>
            <p className="muted">Группы проверяются сверху вниз. Первое совпадение определяет действие.</p>
            <ol className="routing-order">{draft.order.map((action, index) => <li key={action}><span>{actionTitles[action]}</span><div className="flex"><Button type="button" aria-label={`Поднять группу ${actionTitles[action]}`} disabled={index === 0} onClick={() => change(moveGroup(draft, index, -1))}>↑</Button><Button type="button" aria-label={`Опустить группу ${actionTitles[action]}`} disabled={index === 2} onClick={() => change(moveGroup(draft, index, 1))}>↓</Button></div></li>)}</ol>
            <label className="field"><span>DNS-стратегия правил</span><select aria-label="DNS-стратегия правил" value={draft.domain_strategy} onChange={event => change({domain_strategy: event.target.value as RoutingProfile['domain_strategy']})}><option value="AsIs">Использовать имеющийся адрес (AsIs)</option><option value="IPIfNonMatch">Запрашивать IP, если домен не подошёл</option><option value="IPOnDemand">Запрашивать IP для IP-правил</option></select></label>
            <Field label="DNS через VPN (HTTPS)" type="url" required value={draft.remote_dns} onChange={event => change({remote_dns: event.target.value})}/>
            <Field label="DNS для прямого выхода (HTTPS)" type="url" required value={draft.direct_dns} onChange={event => change({direct_dns: event.target.value})}/>
          </div></details>
          <details className="profile-advanced"><summary>Проверить адрес</summary><div className="profile-settings"><Field label="Домен или IP для проверки" value={address} onChange={event => {setAddress(event.target.value); setDecision(null);}}/><Button type="button" disabled={check.isPending || !address.trim()} onClick={() => check.mutate({profile: candidate(), address: address.trim()})}>Проверить правило</Button>{decision && <div role="status"><strong>{decision.action ? actionTitles[decision.action] : 'Нужны DNS или геоданные'}</strong><p>{decision.reason}{decision.matched ? `: ${decision.matched}` : ''}</p>{decision.pending.length > 0 && <p className="muted">Не проверено: {decision.pending.join(', ')}</p>}<small>Модель правил, без реального VPN-запроса.</small></div>}</div></details>
          {error && <p className="error" role="alert">{error}</p>}
          {conflict && <p className="notice" role="alert">Ваши правки остаются в форме. {latest ? <Button type="button" onClick={() => {setInitial(''); setDraft(latest); setTexts({proxy: latest.proxy.join('\n'), direct: latest.direct.join('\n'), block: latest.block.join('\n')}); setInitial(JSON.stringify(latest)); setConflict(false); setError(''); setDecision(null);}}>Загрузить серверную версию</Button> : <Button type="button" onClick={() => {change(withoutMetadata(candidate())); setConflict(false);}}>Сохранить новым профилем</Button>}</p>}
        </fieldset>
        <footer className="dialog-footer"><Button type="button" disabled={busy} onClick={() => setOpen(false)}>Закрыть</Button><Button type="submit" variant="primary" disabled={busy || conflict}>{save.isPending ? 'Сохранение…' : 'Сохранить профиль'}</Button></footer>
      </form>
    </Dialog>}
  </>;
}
