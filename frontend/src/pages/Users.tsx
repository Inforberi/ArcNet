import UserToolbar from '../features/users/UserToolbar';
import UserDataStatus from '../features/users/UserDataStatus';
import {connectionTitle} from '../features/connections/presentation';
import {Icon} from '../shared/Icon';
import {activityText,trafficFresh,trafficNow,type TrafficReply} from '../features/users/traffic';
import {rateText} from '../shared/traffic-chart';
import BulkCreateUsers from '../features/users/BulkCreateUsers';
import BulkEditUsers from '../features/users/BulkEditUsers';
import {sortUsers,usersCSV,matchesUserSearch,type UserSort} from '../features/users/user-table';
import BulkUserAccess from '../features/users/BulkUserAccess';
import { useEffect,useRef, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api,ApiError, type VPNUser, type Subscription, type ConnectionProfile } from '../shared/api';
import {UsageSummary} from '../features/users/PackageDialog';
import DevicesDialog from '../features/users/DevicesDialog';
import DirectAccess from '../features/users/DirectAccess';
import UserDetails from '../features/users/UserDetails';
import Dialog,{DialogClose} from '../shared/Dialog';
import {useFeedback} from '../shared/Feedback';
import {userAccess, type UsageReply,type UserPackage} from '../features/users/usage';
import { Badge, Button, Field, Loading, PageHeading, Panel, Notice, Empty,StatStrip,IconButton,SwitchButton,ErrorState } from '../shared/ui';



export default function Users() {
  const feedback=useFeedback();
  const accessRequest=useRef(0);
  const cache = useQueryClient();
  const users = useQuery({ queryKey: ['users'], queryFn: () => api<{ items: VPNUser[] }>('/api/users') });
  const usage = useQuery({queryKey:['user-usage'],queryFn:()=>api<UsageReply>('/api/users/usage')});
  const [deviceUser,setDeviceUser] = useState<VPNUser | null>(null);
  const [detailID,setDetailID] = useState<number|null>(null);
  const [detailTab,setDetailTab] = useState<'overview'|'edit'>('overview');
  const [search,setSearch] = useState('');
  const [filter,setFilter] = useState('all');
  const [groupFilter,setGroupFilter]=useState('all');
  const [sort,setSort]=useState<UserSort>('created'),[descending,setDescending]=useState(true);
  const [bulkCreating,setBulkCreating]=useState(false);
  const [bulkEditing,setBulkEditing]=useState<{users:VPNUser[];packages:Map<number,UserPackage>}|null>(null);
  const [selected,setSelected]=useState<number[]>([]);
  const [page,setPage]=useState(0);
  const [pageSize,setPageSize]=useState(50);
  const packages = new Map(usage.data?.items.map(item=>[item.user_id,item]));
  const groups=[...new Set((users.data?.items??[]).map(u=>u.preferences?.group??''))].sort((a,b)=>a.localeCompare(b,'ru'));
  const visibleUsers=sortUsers(users.data?.items??[],packages,sort,descending).filter(user=>matchesUserSearch(user,search)&& (groupFilter==='all'||groupFilter===`g:${user.preferences?.group??''}`)&& (filter==='all'||filter==='enabled'&&userAccess(user,packages.get(user.id)).allowed||filter==='disabled'&&!user.enabled||filter==='blocked'&&user.enabled&&(!!packages.get(user.id)?.block_reason||user.preferences?.allowed_kinds?.length===0)));
  const pageIndex=Math.min(page,Math.max(0,Math.ceil(visibleUsers.length/pageSize)-1));
  const displayedUsers=visibleUsers.slice(pageIndex*pageSize,(pageIndex+1)*pageSize);
  const detailUser=users.data?.items.find(user=>user.id===detailID);
  const historyID=detailUser?.id??null;
  const trafficIDs=[...new Set([...displayedUsers.map(u=>u.id),...(historyID?[historyID]:[])])].sort((a,b)=>a-b).join(',');
  const traffic=useQuery({queryKey:['user-traffic',trafficIDs,historyID],enabled:!!trafficIDs,queryFn:({signal})=>api<TrafficReply>(`/api/users/traffic?ids=${trafficIDs}${historyID?`&history_id=${historyID}`:``}`,'GET',undefined,signal),staleTime:4000,retry:false,refetchOnReconnect:false});
  const trafficTime=trafficNow(traffic.data?.sampled_at??0,traffic.dataUpdatedAt);
  const trafficItems=new Map(traffic.data?.items.map(item=>[item.user_id,item]));
  const selectedUsers=(users.data?.items??[]).filter(u=>selected.includes(u.id));
  const allSelected=displayedUsers.length>0&&displayedUsers.every(u=>selected.includes(u.id));
  function selectVisible(checked:boolean){setSelected(ids=>checked?[...new Set([...ids,...displayedUsers.map(u=>u.id)])]:ids.filter(id=>!displayedUsers.some(u=>u.id===id)))}
  function exportCSV(){
    const rows=selectedUsers.length?sortUsers(selectedUsers,packages,sort,descending):visibleUsers;
    const url=URL.createObjectURL(new Blob([usersCSV(rows,packages)],{type:'text/csv;charset=utf-8'}));
    const link=document.createElement('a');link.href=url;link.download=`arcnet-users-${new Date().toISOString().slice(0,10)}.csv`;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
  }

  const [creating, setCreating] = useState(false);
  const [creationUnknown,setCreationUnknown]=useState(false);
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [subscription, setSubscription] = useState<Subscription | null>(null);
  const [subscriptionUser, setSubscriptionUser] = useState<VPNUser | null>(null);
  const [connections, setConnections] = useState<ConnectionProfile[]>([]);
  const [selectedQR, setSelectedQR] = useState<string | null>(null);
  const [accessError,setAccessError]=useState(''),[accessPending,setAccessPending]=useState(false);
  function closeAccess(){accessRequest.current++;setSubscription(null);setSubscriptionUser(null);setConnections([]);setSelectedQR(null);setAccessPending(false);setAccessError('')}
  useEffect(()=>()=>{accessRequest.current++},[]);

  useEffect(() => {
    if (subscriptionUser && (packages.get(subscriptionUser.id)?.block_reason || users.data?.items.find(item=>item.id===subscriptionUser.id)?.enabled === false)) {
      closeAccess();
    }
  }, [usage.data, users.data, subscriptionUser]);

  const create = useMutation({
    mutationFn: () => api<VPNUser>('/api/users', 'POST', { name: name.trim() }),
    onSuccess: async (created) => {
      setDetailTab('edit');setDetailID(created.id);
      setName('');
      setCreating(false);
      feedback('Пользователь создан');
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
    const request=++accessRequest.current;
    setAccessError('');setAccessPending(true);setSubscriptionUser(user);
    setSubscription(null);
    setConnections([]);setSelectedQR(null);
    try {
      const [result, linked] = await Promise.all([
        api<Subscription>(`/api/users/${user.id}/subscription`),
        api<{items: ConnectionProfile[]}>(`/api/users/${user.id}/connections`),
      ]);
      if(request!==accessRequest.current)return;
      setConnections(linked.items);
      setSelectedQR(null);
      setSubscription(result);
      setSubscriptionUser(user);
    } catch (err) {
      if(request===accessRequest.current)setAccessError(explain(err));
    }finally{if(request===accessRequest.current)setAccessPending(false)}
  }

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if(create.isPending||creationUnknown)return;
    setError('');
    setMessage('');
    if (!name.trim()) { setError('Укажи имя пользователя'); return; }
    try { await create.mutateAsync(); }
    catch (err) { if(err instanceof ApiError&&err.outcomeUnknown)setCreationUnknown(true);setError(explain(err)); }
  }

  async function copy(value: string) {
    setError('');
    try { await navigator.clipboard.writeText(value); feedback('Ссылка скопирована'); }
    catch { setAccessError('Не удалось скопировать. Откройте «Показать ссылку» и скопируйте её вручную.'); }
  }

  return (
    <>
      <PageHeading
        title="Пользователи"
        description="Выдавайте доступ, следите за трафиком и управляйте ограничениями."
        actions={<><Button onClick={()=>setBulkCreating(true)}><Icon name="plus"/>Массовое добавление</Button><Button variant="primary" onClick={() => {setError('');setCreationUnknown(false);setCreating(true)}}><Icon name="plus"/>Добавить пользователя</Button></>}
      />
      {users.data&&<StatStrip label="Сводка пользователей" items={[{label:'Всего пользователей',value:users.data.items.length},{label:'Доступ разрешён',value:usage.isSuccess?users.data.items.filter(user=>userAccess(user,packages.get(user.id)).allowed).length:'—'},{label:'Ограничены',value:usage.isSuccess?users.data.items.filter(user=>!user.enabled||!!packages.get(user.id)?.block_reason||user.preferences?.allowed_kinds?.length===0).length:'—',tone:'amber'}]}/>}
      {error && !creating && <p className="error" role="alert">{error}</p>}
      {message && <p className="notice" role="status">{message}</p>}

      {bulkCreating&&<BulkCreateUsers onClose={()=>setBulkCreating(false)}/>}
      {bulkEditing&&<BulkEditUsers users={bulkEditing.users} packages={bulkEditing.packages} onDone={()=>setSelected([])} onClose={()=>setBulkEditing(null)}/>}
      {creating && (
        <Dialog title="Добавить пользователя" onClose={()=>{setCreating(false);setName('');setError('');void users.refetch()}} busy={create.isPending} dirty={!!name.trim()}>
          <form className="panel-body" onSubmit={e => void submit(e)}>
            <Field
              label="Имя пользователя" value={name} maxLength={60} required
              onChange={e => setName(e.target.value)}
              autoComplete="off"
            />
            {error&&<p className="error" role="alert">{error}</p>}
            <div className="form-actions">
              <DialogClose disabled={create.isPending}>Отмена</DialogClose>
              <Button type="submit" variant="primary" pending={create.isPending} disabled={creationUnknown}>Создать и настроить</Button>
            </div>
          </form>
        </Dialog>
      )}

      {usage.isError && <Notice>{usage.error.message}</Notice>}
      {usage.data?.items.some(p=>p.quota_bytes>0||p.expires_at>0)&&!usage.data.source_fresh&&<p className="muted">Учёт пакетов временно недоступен. Сохранённый расход показан в таблице.</p>}
      {usage.data?.items.some(p=>p.quota_bytes>0||p.expires_at>0)&&['apply_failed','source_unavailable','unverified'].includes(usage.data.runtime_state)&&<Notice>Изменение ограничений ещё не подтверждено VPN-службой. Проверьте состояние сервера.</Notice>}
      {users.isPending ? <Loading /> : users.isError ? <ErrorState error={users.error} onRetry={()=>void users.refetch()} retrying={users.isFetching}/> : (
        <Panel title="Список пользователей" actions={<Button className="small inline-action" disabled={!selectedUsers.length&&!visibleUsers.length} onClick={exportCSV} title="Скачать CSV без ключей и ссылок подписок"><Icon name="download"/>{selectedUsers.length?`Скачать выбранных (${selectedUsers.length})`:'Скачать список'}</Button>}>
          <UserToolbar value={{search,status:filter,group:groupFilter,sort,descending}} groups={groups} onChange={next=>{if(next.search!==search||next.status!==filter||next.group!==groupFilter)setSelected([]);setSearch(next.search);setFilter(next.status);setGroupFilter(next.group);setSort(next.sort);setDescending(next.descending);setPage(0)}}/>
          <div className="users-list-meta"><span className="muted">{visibleUsers.length===users.data.items.length?`Всего: ${visibleUsers.length}`:`Найдено: ${visibleUsers.length} из ${users.data.items.length}`}</span><UserDataStatus failed={traffic.isError} incomplete={!!traffic.data&&!traffic.data.complete_coverage}/></div>
          {selectedUsers.length>0&&<BulkUserAccess onEdit={()=>setBulkEditing({users:[...selectedUsers],packages:new Map(packages)})} users={selectedUsers} onDone={()=>setSelected([])} onCancel={()=>setSelected([])}/>}
          <div className="table-scroll users-table responsive-data">
            {users.data.items.length === 0 ? <Empty>Добавьте пользователя, затем нажмите «Подключить», чтобы получить QR и подписку.</Empty> : (
              <table>
                <thead><tr><th scope="col"><label className="user-select"><input type="checkbox" aria-label="Выбрать пользователей на странице" checked={allSelected} ref={el=>{if(el)el.indeterminate=displayedUsers.some(u=>selected.includes(u.id))&&!allSelected}} onChange={e=>selectVisible(e.target.checked)}/>Пользователь</label></th><th scope="col">Доступ</th><th scope="col">Подключения</th><th scope="col">Трафик</th><th scope="col">Скорость</th><th scope="col">Активность</th><th scope="col">Срок действия</th><th scope="col">Действия</th></tr></thead>
                <tbody>
                  {!visibleUsers.length&&<tr><td colSpan={8}><Empty>Ничего не найдено. Измените поиск или фильтр.</Empty></td></tr>}
                  {displayedUsers.map(u => (
                    <tr key={u.id}>
                      <td data-label="Пользователь"><div className="user-identity"><input type="checkbox" aria-label={`Выбрать ${u.name}`} checked={selected.includes(u.id)} onChange={e=>setSelected(ids=>e.target.checked?[...ids,u.id]:ids.filter(id=>id!==u.id))}/><Button className="user-name" variant="ghost" onClick={()=>{setDetailTab('overview');setDetailID(u.id)}}>{u.name}</Button><small className="muted">ID {u.id}{u.preferences?.group?` · ${u.preferences.group}`:''}</small></div></td>
                      <td data-label="Доступ"><Badge variant={userAccess(u,packages.get(u.id)).tone} title={userAccess(u,packages.get(u.id)).description}>{userAccess(u,packages.get(u.id)).label}</Badge></td>
                      <td data-label="Подключения"><div className="user-connection-tags">{u.preferences?.allowed_kinds==null?<Badge variant="neutral">Все подключения</Badge>:u.preferences.allowed_kinds.length?u.preferences.allowed_kinds.map(kind=><Badge variant="neutral" key={kind}>{connectionTitle(kind)}</Badge>):<span className="muted">Нет разрешённых</span>}</div><small className="muted">{u.preferences?.access_mode==='direct'?'Прямая ссылка':'Подписка'}</small></td>
                      <td data-label="Трафик"><UsageSummary item={packages.get(u.id)} complete={usage.data?.complete_coverage ?? false}/></td>
                      <td data-label="Скорость">{!traffic.isError&&trafficFresh(trafficItems.get(u.id),trafficTime)?<div className="user-rate"><span>↓ {rateText(trafficItems.get(u.id)?.download_bps)}</span><span>↑ {rateText(trafficItems.get(u.id)?.upload_bps)}</span></div>:<span className="muted" aria-label="Скорость неизвестна" title="Панель ещё не получила свежие счётчики">—</span>}</td>
                      <td data-label="Активность"><span>{activityText(trafficItems.get(u.id),traffic.isError,trafficTime)}</span>{trafficItems.get(u.id)?.last_activity_at?<small className="muted user-activity-time">Трафик замечен {new Date(trafficItems.get(u.id)!.last_activity_at*1000).toLocaleString('ru-RU')}</small>:null}</td>
                      <td data-label="Срок действия">{packages.has(u.id)?packages.get(u.id)!.expires_at?new Date(packages.get(u.id)!.expires_at*1000).toLocaleDateString('ru-RU'):'Без срока':'Нет данных'}</td>
                      <td data-label="Действия"><div className="row-actions">
                        <IconButton icon="qr" label={`Подключить · ${u.name}`} onClick={()=>{if(u.devices_managed){setSubscription(null);setSubscriptionUser(null);setDeviceUser(u);}else void showSubscription(u);}}/>
                        <IconButton icon="edit" label={`Настроить · ${u.name}`} onClick={()=>{setDetailTab('edit');setDetailID(u.id)}}/>
                        <SwitchButton checked={u.enabled} label={`${u.enabled?'Отключить':'Включить'} доступ · ${u.name}`} disabled={toggle.isPending} onClick={()=>{setError('');void toggle.mutateAsync(u).catch(error=>setError(explain(error)))}}/>
                      </div></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
          {visibleUsers.length>25&&<nav className="users-pagination" aria-label="Страницы пользователей"><span className="muted">{visibleUsers.length?`Показано ${pageIndex*pageSize+1}–${Math.min((pageIndex+1)*pageSize,visibleUsers.length)} из ${visibleUsers.length}`:'Нет совпадений'}</span>{visibleUsers.length>25&&<label className="field users-page-size"><span>Строк на странице</span><select value={pageSize} onChange={e=>{setPageSize(Number(e.target.value));setPage(0)}}>{[25,50,100].map(n=><option key={n} value={n}>{n}</option>)}</select></label>}{visibleUsers.length>pageSize&&<div className="users-page-actions"><Button className="small" disabled={pageIndex===0} onClick={()=>setPage(pageIndex-1)}>Назад</Button><span className="muted">{pageIndex+1} / {Math.ceil(visibleUsers.length/pageSize)}</span><Button className="small" disabled={(pageIndex+1)*pageSize>=visibleUsers.length} onClick={()=>setPage(pageIndex+1)}>Далее</Button></div>}</nav>}
        </Panel>
      )}

      {deviceUser && <DevicesDialog id={deviceUser.id} name={deviceUser.name} onClose={()=>setDeviceUser(null)}/>}
      {detailUser&&<UserDetails trafficNow={trafficTime} traffic={traffic.data?.detail} trafficPending={traffic.isPending} trafficFailed={traffic.isError} initialTab={detailTab} key={`${detailUser.id}:${detailTab}`} user={detailUser} usage={usage.data} account={packages.get(detailUser.id)} onClose={()=>setDetailID(null)} onConnect={()=>{setDetailID(null);void showSubscription(detailUser);}} onToggle={()=>toggle.mutateAsync(detailUser)} onDelete={()=>remove.mutateAsync(detailUser)}/>}

      {subscriptionUser && (
        <Dialog className="access-dialog" title={`Подключить · ${subscriptionUser.name}`} onClose={closeAccess}>
          <div className="dialog-body">
            {accessPending&&<Loading/>}
            {accessError&&<p className="error" role="alert">{accessError}</p>}
            {!subscription&&!accessPending&&<Button onClick={()=>void showSubscription(subscriptionUser)}>Повторить загрузку подключений</Button>}
            {subscription&&<><DirectAccess preferredMode={subscriptionUser.preferences?.access_mode} preferredKind={subscriptionUser.preferences?.direct_kind} links={subscription.links} ids={subscription.link_ids} subscription={{title:subscription.title,url:subscription.url,qrURL:`/api/users/${subscriptionUser.id}/qr?format=subscription`}} qrURL={selection => `/api/users/${subscriptionUser.id}/qr?format=link&selection=${encodeURIComponent(selection)}`} onCopy={value=>navigator.clipboard.writeText(value)} onError={()=>{}}/>
            <details className="profile-advanced"><summary>Дополнительные форматы</summary>
              <div className="profile-settings">
                <div className="flex wrap"><a className="button inline-action" href={`/api/users/${subscriptionUser.id}/export?format=raw`} download>Скачать ссылки</a><a className="button inline-action" href={`/api/users/${subscriptionUser.id}/export?format=base64`} download>Скачать Base64</a><a className="button inline-action" href={`/api/users/${subscriptionUser.id}/export?format=url`} download>Скачать URL</a></div>
              </div>
            </details></>}
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
              <Button onClick={closeAccess}>Закрыть</Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  );
}
