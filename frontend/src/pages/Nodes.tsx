import {useConfirmation} from '../shared/Confirmation';
import Dialog,{DialogClose} from '../shared/Dialog';
import TargetPicker from '../features/targets/TargetPicker';
import {useEffect,useState} from 'react';
import {useMutation,useQuery,useQueryClient} from '@tanstack/react-query';
import {PageHeading,Panel,Notice,Button,Field,Loading,Badge,ErrorState} from '../shared/ui';
import {api} from '../shared/api';

type NodeControl={
  node_id:string;address:string;port:number;desired_revision:number;applied_revision:number;
  last_good_revision:number;deployment_id:string;deployment_status:string;
  revision_status:string;updated_at:number;desired_digest:string;probe_checked_at:number;probe_revision:number;vpn_probe:'unknown'|'pass'|'fail'|'stale';
  published:boolean;draining:boolean;last_error?:string;
};
type Node={id:string;name:string;online:boolean;xray_state:string;version:string;agent_protocol:number;last_seen:number;memory_total:number;memory_available:number;load_1:number;uptime_seconds:number;control:NodeControl};
type NodeList={items:Node[];node_mode:string;controller:string};
type NodeOperation={id:string;node_id:string;revision:number;state:string;kind:string;created_at:number;updated_at:number;attempts:number;error?:string};
type Bootstrap={id:string;name:string;address:string;status:string;step:string;operation_type:string;removal_mode?:string;target_version?:string;error?:string;updated_at:number};
type RemovalPlan={installation_id:string;state:string;mode:'purge'|'detach';changes:string[]};
type UpgradePlan={installation_id:string;current_version:string;target_version:string;changes:string[]};
type Fingerprint={host:string;resolved_ip:string;port:number;type:string;fingerprint:string};
type Preflight={host_key:Fingerprint;items:Record<string,string>;warnings:string[];blockers:string[];plan:string[]};
type Credentials={host:string;user:string;ssh_port:number;vpn_port:number;fingerprint:string;private_key:string};
type Enrollment={token:string;expires_seconds:number;controller:string};
type ProbeGrant={token:string;node_id:string;revision:number;digest:string;expires_at:number};
type DirectLinks={node_id:string;links:{user_id:number;name:string;link:string}[];vpn_probe:string;warning:string};

// Shell snippets are copy-only instructions, never executed by the panel.
// Quote every value originating from server responses, including the origin.
function shellQuote(value:string):string { return "'"+value.replace(/'/g, "'\\''")+"'"; }
function readableTime(stamp:number){return stamp?new Date(stamp*1000).toLocaleString('ru-RU'):'Нет данных';}
function probeLabel(node:Node){
  const {vpn_probe,probe_checked_at}=node.control;
  if(vpn_probe==='pass')return `PASS · ${readableTime(probe_checked_at)}`;
  if(vpn_probe==='fail')return `FAIL · ${readableTime(probe_checked_at)}`;
  if(vpn_probe==='stale')return 'Устарела, требуется повторная проверка';
  return 'Нет подтверждённой внешней проверки';
}
function revState(node:Node){
  if(!node.online)return 'Связь потеряна';
  if(node.control.revision_status==='applied')return `Подтверждена r${node.control.applied_revision}`;
  if(node.control.revision_status==='failed')return 'Ошибка применения';
  if(node.control.revision_status==='none')return 'Конфигурация не отправлялась';
  return `Ожидается r${node.control.desired_revision}`;
}
function errorMessage(e:unknown){return e instanceof Error?e.message:'Операция завершилась с ошибкой';}

function NodeUpgradeWizard({node,onClose}:{node:Node;onClose:()=>void}) {
 const confirm=useConfirmation();
  const cache=useQueryClient();
  const [sshPort,setSSHPort]=useState('22');
  const [user,setUser]=useState('root');
  const [privateKey,setPrivateKey]=useState('');
  const [fingerprint,setFingerprint]=useState<Fingerprint|null>(null);
  const [verified,setVerified]=useState(false);
  const [plan,setPlan]=useState<UpgradePlan|null>(null);
  const [error,setError]=useState('');
  const [queued,setQueued]=useState('');
  const target=node.control.address;
  const credentials=():Credentials=>({host:target,user,ssh_port:Number(sshPort),vpn_port:node.control.port,fingerprint:fingerprint?.fingerprint??'',private_key:privateKey});
  const reset=()=>{setFingerprint(null);setVerified(false);setPlan(null);};
  const inspect=useMutation({
    mutationFn:()=>api<Fingerprint>('/api/nodes/ssh/inspect','POST',{host:target,port:Number(sshPort)}),
    onSuccess:key=>{setFingerprint(key);setVerified(false);setPlan(null);setError('');},onError:e=>setError(errorMessage(e))
  });
  const check=useMutation({
    mutationFn:()=>api<UpgradePlan>(`/api/nodes/${encodeURIComponent(node.id)}/ssh/upgrade-plan`,'POST',{credentials:credentials()}),
    onSuccess:result=>{setPlan(result);setError('');},onError:e=>{setPlan(null);setError(errorMessage(e));}
  });
  const upgrade=useMutation({
    mutationFn:()=>api<Bootstrap>(`/api/nodes/${encodeURIComponent(node.id)}/ssh/upgrade`,'POST',{
      credentials:credentials(),installation_id:plan?.installation_id,expected_current:plan?.current_version,approve:'CONFIRM_UPGRADE'
    }),
    onSuccess:result=>{setQueued(result.id);setError('');setPrivateKey('');setPlan(null);void cache.invalidateQueries({queryKey:['node-bootstraps']});},
    onError:e=>setError(errorMessage(e))
  });
  return <Panel title={`Обновление ноды: ${node.name}`}><div className="panel-body node-wizard">
    <Notice>Обновление подписанного агента и Xray может прервать текущие VPN-соединения. Сначала создайте snapshot VPS и проверьте SSH-доступ через консоль провайдера. При ошибке будет предпринята попытка отката бинарников.</Notice>
    <p className="muted">Управляемая нода: {target}:{node.control.port}. Для обновления нужен новый совместимый подписанный пакет на контроллере. Конфигурация Xray, SSH и правила firewall не изменяются.</p>
    {error&&<p className="error" role="alert">{error}</p>}
    <div className="node-form-grid">
      <Field label="SSH-порт" type="number" min={1} max={65535} value={sshPort} onChange={event=>{setSSHPort(event.target.value);reset();}} />
      <Field label="SSH-пользователь" value={user} onChange={event=>{setUser(event.target.value);setPlan(null);}} />
    </div>
    <Button disabled={inspect.isPending||!target} onClick={()=>inspect.mutate()}>1. Получить SSH fingerprint</Button>
    {fingerprint&&<div className="node-fingerprint"><code>{fingerprint.fingerprint}</code><label className="node-check"><input type="checkbox" checked={verified} onChange={event=>{setVerified(event.target.checked);setPlan(null);}}/> Я сверил fingerprint независимо через консоль провайдера.</label></div>}
    {verified&&<><label className="field"><span>Приватный SSH-ключ (не сохраняется)</span><textarea className="node-key" rows={5} value={privateKey} onChange={event=>{setPrivateKey(event.target.value);setPlan(null);}} autoComplete="off" spellCheck={false}/></label>
      <Button disabled={check.isPending||privateKey.length<100} onClick={()=>check.mutate()}>2. Проверить обновление</Button></>}
    {plan&&<div className="node-preflight"><h3>План обновления</h3><p><strong>{plan.current_version} → {plan.target_version}</strong></p><p>ID исходной установки: <code>{plan.installation_id}</code></p><ol>{plan.changes.map(line=><li key={line}>{line}</li>)}</ol>
      <Button variant="danger" disabled={upgrade.isPending||privateKey.length<100} onClick={async ()=>{if((await confirm('Введите точно: ОБНОВИТЬ НОДУ', {phrase:'ОБНОВИТЬ НОДУ',action:'Подтвердить операцию'})))upgrade.mutate();}}>{upgrade.isPending?'Запуск…':'3. Подтвердить обновление'}</Button>
    </div>}
    {queued&&<p role="status">Обновление поставлено в очередь. Операция: <code>{queued}</code>. Статус проверяйте в журнале SSH.</p>}
    <Button onClick={()=>{setPrivateKey('');onClose();}}>Закрыть</Button>
  </div></Panel>;
}

export default function Nodes(){
 const confirm=useConfirmation();
  const [installOpen,setInstallOpen]=useState(false);
  const cache=useQueryClient();
  const nodes=useQuery({queryKey:['nodes'],queryFn:()=>api<NodeList>('/api/nodes')});
  const operations=useQuery({queryKey:['node-bootstraps'],queryFn:()=>api<{items:Bootstrap[]}>('/api/nodes/ssh/operations')});
  const [sniOpen,setSniOpen]=useState(false);
  const [sniNode,setSniNode]=useState('local');
  const [view,setView]=useState<'table'|'diagram'>('table');
  const [error,setError]=useState('');
  const [name,setName]=useState('');
  const [host,setHost]=useState('');
  const [sshPort,setSSHPort]=useState('22');
  const [vpnPort,setVPNPort]=useState('443');
  const [user,setUser]=useState('root');
  const [privateKey,setPrivateKey]=useState('');
  const [fingerprint,setFingerprint]=useState<Fingerprint|null>(null);
  const [hostVerified,setHostVerified]=useState(false);
  const [preflight,setPreflight]=useState<Preflight|null>(null);
  const [latestOperation,setLatestOperation]=useState('');
  const [manual,setManual]=useState(false);
  const [linkNode,setLinkNode]=useState('');
  const [probeGrant,setProbeGrant]=useState<ProbeGrant|null>(null);
  useEffect(()=>{
    if(!probeGrant)return;
    const remaining=Math.max(0,probeGrant.expires_at*1000-Date.now());
    const id=window.setTimeout(()=>setProbeGrant(current=>current?.token===probeGrant.token?null:current),remaining);
    return ()=>window.clearTimeout(id);
  },[probeGrant]);
  const [filter,setFilter]=useState('');
  const [onlineFilter,setOnlineFilter]=useState<'all'|'online'|'offline'>('all');
  const [removalNode,setRemovalNode]=useState('');
  const [upgradeNode,setUpgradeNode]=useState('');
  const [removalMode,setRemovalMode]=useState<'purge'|'detach'>('purge');
  const [removalSSHPort,setRemovalSSHPort]=useState('22');
  const [removalUser,setRemovalUser]=useState('root');
  const [removalPrivateKey,setRemovalPrivateKey]=useState('');
  const [removalFingerprint,setRemovalFingerprint]=useState<Fingerprint|null>(null);
  const [removalVerified,setRemovalVerified]=useState(false);
  const [removalPlan,setRemovalPlan]=useState<RemovalPlan|null>(null);
  const directLinks=useQuery({queryKey:['node-links',linkNode],queryFn:()=>api<DirectLinks>(`/api/nodes/${encodeURIComponent(linkNode)}/links`),enabled:Boolean(linkNode)});
  const [enrollment,setEnrollment]=useState<Enrollment|null>(null);
  const requestProbe=useMutation({
    mutationFn:()=>api<ProbeGrant>(`/api/nodes/${encodeURIComponent(linkNode)}/probe/grant`,'POST',{}),
    onSuccess:grant=>{setProbeGrant(grant);setError('');},
    onError:e=>{setProbeGrant(null);setError(errorMessage(e));}
  });

  const credentials=():Credentials=>({host,user,ssh_port:Number(sshPort),vpn_port:Number(vpnPort),fingerprint:fingerprint?.fingerprint??'',private_key:privateKey});
  const resetHost=()=>{setFingerprint(null);setHostVerified(false);setPreflight(null);};
  const inspect=useMutation({mutationFn:()=>api<Fingerprint>('/api/nodes/ssh/inspect','POST',{host,port:Number(sshPort)}),onSuccess:key=>{setError('');setFingerprint(key);setPreflight(null);setHostVerified(false);},onError:e=>setError(errorMessage(e))});
  const check=useMutation({mutationFn:()=>api<Preflight>('/api/nodes/ssh/preflight','POST',credentials()),onSuccess:result=>{setError('');setPreflight(result);},onError:e=>{setError(errorMessage(e));setPreflight(null);}});
  const install=useMutation({mutationFn:()=>api<Bootstrap>('/api/nodes/ssh/install','POST',{name,credentials:credentials(),approve:'CONFIRM_INSTALL'}),onSuccess:result=>{setError('');setLatestOperation(result.id);setPrivateKey('');setPreflight(null);void cache.invalidateQueries({queryKey:['node-bootstraps']});},onError:e=>setError(errorMessage(e))});
  const enroll=useMutation({mutationFn:()=>api<Enrollment>('/api/nodes/enroll','POST',{name,address:host,port:Number(vpnPort)}),onSuccess:result=>{setError('');setEnrollment(result);},onError:e=>setError(errorMessage(e))});
  const deploy=useMutation({mutationFn:(id:string)=>api<NodeOperation>(`/api/nodes/${encodeURIComponent(id)}/deploy`,'POST'),onSuccess:()=>{setError('');void cache.invalidateQueries({queryKey:['nodes']});},onError:e=>setError(errorMessage(e))});
  const retry=useMutation({mutationFn:(id:string)=>api(`/api/nodes/${encodeURIComponent(id)}/retry`,'POST'),onSuccess:()=>{setError('');void cache.invalidateQueries({queryKey:['nodes']});},onError:e=>setError(errorMessage(e))});
  const cancel=useMutation({mutationFn:(id:string)=>api(`/api/nodes/${encodeURIComponent(id)}/cancel`,'POST'),onSuccess:()=>{setError('');void cache.invalidateQueries({queryKey:['nodes']});},onError:e=>setError(errorMessage(e))});
  const revoke=useMutation({mutationFn:(id:string)=>api(`/api/nodes/${encodeURIComponent(id)}`,'DELETE'),onSuccess:()=>{setError('');void cache.invalidateQueries({queryKey:['nodes']});},onError:e=>setError(errorMessage(e))});
  const drain=useMutation({mutationFn:({id,draining}:{id:string;draining:boolean})=>api(`/api/nodes/${encodeURIComponent(id)}/drain`,'POST',{draining}),onSuccess:()=>{setError('');setLinkNode('');setProbeGrant(null);void cache.invalidateQueries({queryKey:['nodes']});void cache.invalidateQueries({queryKey:['node-links']});},onError:e=>setError(errorMessage(e))});
  const selectedRemovalNode=(nodes.data?.items??[]).find(item=>item.id===removalNode);
  const selectedUpgradeNode=(nodes.data?.items??[]).find(item=>item.id===upgradeNode);
  const filteredNodes=(nodes.data?.items??[]).filter(item=>{
    if(onlineFilter==='online'&&!item.online)return false;
    if(onlineFilter==='offline'&&item.online)return false;
    const query=filter.trim().toLocaleLowerCase('ru-RU');
    return !query||`${item.name} ${item.control.address} ${item.id}`.toLocaleLowerCase('ru-RU').includes(query);
  });
  const removalCredentials=():Credentials=>({
    host:selectedRemovalNode?.control.address??'',user:removalUser,ssh_port:Number(removalSSHPort),
    vpn_port:selectedRemovalNode?.control.port??443,fingerprint:removalFingerprint?.fingerprint??'',private_key:removalPrivateKey
  });
  const resetRemoval=()=>{setRemovalFingerprint(null);setRemovalVerified(false);setRemovalPlan(null);};
  const inspectRemoval=useMutation({
    mutationFn:()=>api<Fingerprint>('/api/nodes/ssh/inspect','POST',{host:selectedRemovalNode?.control.address,port:Number(removalSSHPort)}),
    onSuccess:key=>{setRemovalFingerprint(key);setRemovalVerified(false);setRemovalPlan(null);setError('');},onError:e=>setError(errorMessage(e))
  });
  const checkRemoval=useMutation({
    mutationFn:()=>api<RemovalPlan>(`/api/nodes/${encodeURIComponent(removalNode)}/ssh/remove-plan`,'POST',{credentials:removalCredentials(),mode:removalMode}),
    onSuccess:result=>{setRemovalPlan(result);setError('');},onError:e=>{setRemovalPlan(null);setError(errorMessage(e));}
  });
  const removeNode=useMutation({
    mutationFn:()=>api<Bootstrap>(`/api/nodes/${encodeURIComponent(removalNode)}/ssh/remove`,'POST',{
      credentials:removalCredentials(),mode:removalMode,installation_id:removalPlan?.installation_id,
      approve:removalMode==='purge'?'CONFIRM_PURGE':'CONFIRM_DETACH'
    }),
    onSuccess:()=>{setError('');setRemovalPrivateKey('');setRemovalPlan(null);setRemovalNode('');void cache.invalidateQueries({queryKey:['node-bootstraps']});},
    onError:e=>setError(errorMessage(e))
  });
  const busy=deploy.isPending||retry.isPending||cancel.isPending||revoke.isPending||removeNode.isPending||drain.isPending;

  function actionButtons(node:Node){
    const state=node.control.deployment_status;
    return <div className="node-actions">
      <Button disabled={busy||!node.control.address||state==='queued'||state==='running'} onClick={()=>deploy.mutate(node.id)}>Применить VPN</Button>
      {state==='failed'&&<Button disabled={busy} onClick={()=>retry.mutate(node.id)}>Повторить</Button>}
      {state==='queued'&&<Button disabled={busy} onClick={()=>cancel.mutate(node.id)}>Отменить</Button>}
      {state==='applied'&&!node.control.draining&&<Button onClick={()=>{setLinkNode(node.id);setProbeGrant(null);}}>Тестовые ссылки</Button>}
      <Button disabled={busy||(!node.control.draining&&state==='queued')} onClick={async ()=>{const next=!node.control.draining;if((await confirm(next?'Прекратить выдачу новых диагностических ссылок? Уже сохранённые VPN-ссылки и соединения останутся действительными.':'Возобновить выдачу диагностических ссылок? Это не публикует ноду в подписках и не доказывает VPN-доступность.')))drain.mutate({id:node.id,draining:next});}}>{node.control.draining?'Возобновить выдачу':'Drain: скрыть ссылки'}</Button>
      <Button disabled={busy} onClick={()=>{setUpgradeNode(node.id);setRemovalNode('');}}>Обновить Xray/агент</Button>
      <Button disabled={busy} onClick={()=>{setRemovalNode(node.id);setUpgradeNode('');setRemovalSSHPort('22');setRemovalUser('root');setRemovalPrivateKey('');resetRemoval();}}>Удалить с VPS</Button>
      <Button variant="danger" disabled={busy} onClick={async ()=>{if((await confirm(`Отозвать идентификатор ${node.name}? Работу VPN на VPS это не остановит.`)))revoke.mutate(node.id);}}>Только отозвать</Button>
    </div>;
  }
  function summary(node:Node){
    const control=node.control;
    return <>
      <div className="node-statuses">
        <span><strong>Агент:</strong> <Badge variant={node.online?'green':'red'}>{node.online?'На связи':'Нет связи'}</Badge></span>
        <span><strong>Протокол управления:</strong> {node.agent_protocol===1?'v1':'Несовместим / неизвестен'}</span>
        <span><strong>Xray:</strong> {node.online?node.xray_state:'Неизвестно'}</span>
        <span><strong>Ревизия:</strong> {revState(node)}</span>
        <span><strong>Выдача новых ссылок:</strong> {control.draining?'Остановлена (drain)':'Разрешена для диагностики'}</span>
        <span><strong>VPN probe:</strong> {probeLabel(node)}</span>
      </div>
      {control.last_error&&<p className="error" role="status">{control.last_error}</p>}
    </>;
  }

  return <>
    <PageHeading title="Серверы" description="Подключение и управление удалёнными Xray-нодами" actions={<><Button variant="primary" onClick={()=>setInstallOpen(true)}>Добавить сервер</Button><Button onClick={()=>{void cache.invalidateQueries({queryKey:['nodes']});void cache.invalidateQueries({queryKey:['node-bootstraps']});}}>Обновить</Button></>}/>
    {error&&<p className="error" role="alert">{error}</p>}
    <Panel title="Удалённые ноды" actions={<div className="node-view-switch"><Button variant={view==='table'?'primary':'default'} onClick={()=>setView('table')}>Таблица</Button><Button variant={view==='diagram'?'primary':'default'} onClick={()=>setView('diagram')}>Схема</Button></div>}>
      <div className="panel-body node-filter-bar">
        <Field label="Поиск ноды" value={filter} onChange={event=>setFilter(event.target.value)} placeholder="Имя, IP или ID"/>
        <label className="field"><span>Связь агента</span><select value={onlineFilter} onChange={event=>setOnlineFilter(event.target.value as 'all'|'online'|'offline')}><option value="all">Все</option><option value="online">На связи</option><option value="offline">Без связи</option></select></label>
      </div>
      {nodes.isPending?<Loading/>:nodes.isError?<ErrorState error={nodes.error} onRetry={()=>void nodes.refetch()} retrying={nodes.isFetching}/>:nodes.data?.items.length===0?<div className="empty">Ноды ещё не зарегистрированы.</div>:filteredNodes.length===0?<div className="empty">По заданным фильтрам ноды не найдены.</div>:view==='table'?<div className="table-scroll"><table><thead><tr><th>Нода</th><th>Управление</th><th>Ревизия</th><th>Проверка VPN</th><th>Последний сигнал</th><th>Действия</th></tr></thead><tbody>
        {filteredNodes.map(node=><tr key={node.id}><td><strong>{node.name}</strong><small className="muted node-secondary">{node.control.address||'Адрес не назначен'} · {node.version||'Версия неизвестна'} · протокол {node.agent_protocol===1?'v1':'неизвестен'}</small></td><td><Badge variant={node.online?'green':'red'}>{node.online?'Агент на связи':'Нет связи'}</Badge><small className="node-secondary">Xray: {node.online?node.xray_state:'неизвестно'}</small></td><td>{revState(node)}{node.control.draining&&<small className="node-secondary">Drain: новые ссылки скрыты</small>}{node.control.last_error&&<small className="error node-secondary">{node.control.last_error}</small>}</td><td>{probeLabel(node)}</td><td>{readableTime(node.last_seen)}</td><td>{actionButtons(node)}</td></tr>)}
      </tbody></table></div>:<div className="node-diagram" aria-label="Схема управления удалёнными серверами">
        <div className="node-controller"><strong>Контроллер ArcNet</strong><small>{nodes.data?.controller||'HTTPS URL не задан'}</small></div>
        <div className="node-diagram-grid">{filteredNodes.map(node=><article className="node-diagram-card" key={node.id}><h3>{node.name}</h3>{summary(node)}<small className="muted">{node.control.address}:{node.control.port}</small>{actionButtons(node)}</article>)}</div>
      </div>}
    </Panel>
    <Button className="node-sni-action" onClick={()=>setSniOpen(current=>!current)} aria-expanded={sniOpen}>{sniOpen?'Закрыть проверку SNI':'Проверить SNI с сервера'}</Button>
    {sniOpen&&<Panel title="Диагностика Reality target/SNI"><div className="panel-body"><label className="field"><span>Сервер для проверки SNI</span><select value={sniNode} onChange={event=>setSniNode(event.target.value)}><option value="local">Сервер панели</option>{nodes.data?.items.map(node=><option key={node.id} value={node.id}>{node.name}</option>)}</select></label><TargetPicker key={sniNode} nodeID={sniNode}/><p className="muted">Диагностика не изменяет SNI действующей ноды и не подтверждает VPN-подключение.</p></div></Panel>}
    {linkNode&&<Panel title="VLESS-ссылки для внешнего тестирования"><div className="panel-body">
      <p className="muted">Административные тестовые ссылки. Наличие подтверждённой ревизии не доказывает доступность VPN извне. Не публикуются в подписках автоматически.</p>
      <Button onClick={()=>{setLinkNode('');setProbeGrant(null);}}>Закрыть ссылки</Button>
      {directLinks.isPending?<Loading/>:directLinks.isError?<Notice>Невозможно получить ссылки. Убедись, что применение конфигурации подтверждено агентом.</Notice>:
        (directLinks.data?.links??[]).map(item=><div className="node-link-item" key={item.user_id}><strong>{item.name}</strong><textarea readOnly rows={3} aria-label={`VLESS ссылка пользователя ${item.name}`} value={item.link}/><Button onClick={()=>{void navigator.clipboard.writeText(item.link).catch(()=>setError('Не удалось скопировать ссылку'));}}>Скопировать</Button></div>)}
      <div className="node-preflight">
        <h3>Независимая проверка VPN</h3>
        <Notice>Проверка запускается на отдельной машине или в другой сети, не на контроллере и не на VPN-ноде. Требуются проверенный Xray и CLI ArcNet той же версии. Только успешный HTTPS-запрос через VLESS/REALITY считается PASS. Запрос на один конкретный адрес не подтверждает доступность всех сайтов или сетей.</Notice>
        <p>Текущее состояние: <strong>{nodes.data?.items.find(node=>node.id===linkNode)?probeLabel(nodes.data.items.find(node=>node.id===linkNode)!):'Нет данных'}</strong></p>
        <Button disabled={requestProbe.isPending||directLinks.isError||directLinks.isPending||(directLinks.data?.links.length??0)===0} onClick={()=>requestProbe.mutate()}>Создать одноразовый допуск (5 минут)</Button>
        {probeGrant&&<>
          <p className="muted">Выберите одну действующую VLESS-ссылку выше и сохраните на внешней машине в файл <code>vless.txt</code> с правами 0600. Одноразовый допуск заменяет предыдущий и действует только для ревизии r{probeGrant.revision}. Не размещайте токен и VLESS-ссылку в shell-аргументах или истории.</p>
          <label className="field"><span>Одноразовый токен отчёта (секрет)</span><textarea readOnly rows={2} value={probeGrant.token}/></label>
          <p className="muted">Срок действия: {readableTime(probeGrant.expires_at)}</p>
          <pre className="code" tabIndex={0}>{`chmod 600 ./vless.txt
arcvpn node-probe --link-file ./vless.txt --xray /usr/local/bin/xray --controller ${shellQuote(nodes.data?.controller??'')} --node-id ${shellQuote(probeGrant.node_id)} --revision ${probeGrant.revision} --digest ${shellQuote(probeGrant.digest)} --report-token-stdin`}</pre>
          <p className="muted">Вставьте токен через stdin после запуска команды. Результат появится в панели после обновления. Допуск лишь авторизует отчёт: он не является криптографическим доказательством места запуска клиента.</p>
          <Button onClick={()=>{setProbeGrant(null);}}>Скрыть токен</Button>
        </>}
      </div>
    </div></Panel>}
    {selectedUpgradeNode&&<NodeUpgradeWizard key={selectedUpgradeNode.id} node={selectedUpgradeNode} onClose={()=>setUpgradeNode('')}/>}
    {selectedRemovalNode&&<Panel title={`Удаление управляемой ноды: ${selectedRemovalNode.name}`}><div className="panel-body node-wizard">
      <Notice>Действие необратимо. При полном удалении Xray остановится, действующие VPN-подключения будут разорваны. При отключении управления Xray продолжит работу, но панель больше не сможет применять конфигурации. Чужие службы, SSH и firewall не меняются.</Notice>
      <p className="muted">Цель: {selectedRemovalNode.control.address}:{selectedRemovalNode.control.port}. Разрешено удаление только ресурсов исходной установки ArcNet с подтверждённым маркером. Для нод, установленных вручную, используйте локальную команду из инструкции.</p>
      <label className="field"><span>Режим</span><select value={removalMode} onChange={event=>{setRemovalMode(event.target.value as 'purge'|'detach');setRemovalPlan(null);}}><option value="purge">Полностью удалить управляемые компоненты и остановить Xray</option><option value="detach">Отключить управление, оставить Xray запущенным</option></select></label>
      <div className="node-form-grid">
        <Field label="SSH-порт" type="number" min={1} max={65535} value={removalSSHPort} onChange={event=>{setRemovalSSHPort(event.target.value);resetRemoval();}} />
        <Field label="SSH-пользователь" value={removalUser} onChange={event=>{setRemovalUser(event.target.value);setRemovalPlan(null);}} />
      </div>
      <Button disabled={inspectRemoval.isPending} onClick={()=>inspectRemoval.mutate()}>1. Получить SSH fingerprint</Button>
      {removalFingerprint&&<div className="node-fingerprint"><strong>Ed25519 SSH fingerprint</strong><code>{removalFingerprint.fingerprint}</code><label className="node-check"><input type="checkbox" checked={removalVerified} onChange={event=>{setRemovalVerified(event.target.checked);setRemovalPlan(null);}}/> Я сверил ключ через независимую консоль VPS-провайдера.</label></div>}
      {removalVerified&&<><label className="field"><span>Приватный SSH-ключ (не сохраняется)</span><textarea className="node-key" rows={5} value={removalPrivateKey} onChange={event=>{setRemovalPrivateKey(event.target.value);setRemovalPlan(null);}} autoComplete="off" spellCheck={false}/></label>
        <Button disabled={checkRemoval.isPending||removalPrivateKey.length<100} onClick={()=>checkRemoval.mutate()}>2. Проверить план удаления</Button></>}
      {removalPlan&&<div className="node-preflight"><h3>План удаления</h3><p>ID исходной установки: <code>{removalPlan.installation_id}</code></p><ol>{removalPlan.changes.map(step=><li key={step}>{step}</li>)}</ol>
        <Button variant="danger" disabled={removeNode.isPending||removalPrivateKey.length<100} onClick={async ()=>{const confirmation=removalMode==='purge'?'ПОДТВЕРЖДАЮ УДАЛЕНИЕ':'ПОДТВЕРЖДАЮ ОТКЛЮЧЕНИЕ';if((await confirm(`Введите точно: ${confirmation}`, {phrase:confirmation,action:'Подтвердить операцию'})))removeNode.mutate();}}>{removeNode.isPending?'Выполнение…':'3. Подтвердить операцию на VPS'}</Button>
      </div>}
      <Button onClick={()=>{setRemovalNode('');setRemovalPrivateKey('');resetRemoval();}}>Закрыть без изменений</Button>
    </div></Panel>}
    {installOpen&&<Dialog title="Добавить сервер" onClose={()=>{setInstallOpen(false);setPrivateKey('');setPreflight(null);}} busy={inspect.isPending||check.isPending||install.isPending} dirty={!!privateKey}><div className="dialog-body node-wizard">
      {error&&<Notice>{error}</Notice>}
      <p className="muted">Только публичный VPS с Debian/Ubuntu, Linux amd64, systemd и SSH-ключом. Для установки нужен заранее подписанный пакет агента и Xray на контроллере. SSH-пароль не используется; приватный ключ не сохраняется в базе.</p>
      <div className="node-form-grid">
        <Field label="Название" value={name} onChange={event=>{setName(event.target.value);setPreflight(null);}} maxLength={64} placeholder="nl-1" required />
        <Field label="IP-адрес или DNS VPS" value={host} onChange={event=>{setHost(event.target.value);resetHost();}} placeholder="vpn.example.com" required />
        <Field label="SSH-порт" type="number" min={1} max={65535} value={sshPort} onChange={event=>{setSSHPort(event.target.value);resetHost();}} />
        <Field label="Порт VPN" type="number" min={1} max={65535} value={vpnPort} onChange={event=>{setVPNPort(event.target.value);setPreflight(null);}} />
        <Field label="SSH-пользователь" value={user} onChange={event=>{setUser(event.target.value);setPreflight(null);}} />
      </div>
      <Button disabled={inspect.isPending||!host||!sshPort} onClick={()=>inspect.mutate()}>{inspect.isPending?'Проверка…':'1. Получить SSH fingerprint'}</Button>
      {fingerprint&&<div className="node-fingerprint"><strong>SSH-ключ сервера (Ed25519)</strong><code>{fingerprint.fingerprint}</code><small>{fingerprint.resolved_ip}:{fingerprint.port}</small><label className="node-check"><input type="checkbox" checked={hostVerified} onChange={event=>{setHostVerified(event.target.checked);setPreflight(null);}}/> Я сверил fingerprint через независимый доверенный канал (например, консоль провайдера VPS).</label></div>}
      {hostVerified&&<>
        <label className="field"><span>Приватный SSH-ключ (только в памяти страницы)</span><textarea className="node-key" value={privateKey} onChange={event=>{setPrivateKey(event.target.value);setPreflight(null);}} rows={5} placeholder="Вставьте приватный SSH-ключ. Он будет удалён из формы после запуска установки." autoComplete="off" spellCheck={false}/></label>
        <Button disabled={check.isPending||privateKey.length<100} onClick={()=>check.mutate()}>{check.isPending?'Проверка…':'2. Проверить VPS без изменений'}</Button>
      </>}
      {preflight&&<div className="node-preflight"><h3>Результат preflight</h3>
        <div className="node-facts">{Object.entries(preflight.items).map(([key,value])=><div key={key}><span>{key}</span><strong>{value}</strong></div>)}</div>
        {preflight.blockers.map(blocker=><p role="alert" className="error" key={blocker}>{blocker}</p>)}
        {preflight.warnings.map(warning=><p className="muted" key={warning}>{warning}</p>)}
        <h3>План изменений</h3><ol>{preflight.plan.map(line=><li key={line}>{line}</li>)}</ol>
        <Button variant="primary" disabled={install.isPending||preflight.blockers.length>0||!name.trim()} onClick={async ()=>{if((await confirm('Подтвердить установку подписанных бинарников, systemd-служб и регистрацию агента на этом VPS?')))install.mutate();}}>{install.isPending?'Запуск…':'3. Подтвердить и установить'}</Button>
      </div>}
      {latestOperation&&<p role="status">Установка запущена. ID операции: <code>{latestOperation}</code>. Закрытие вкладки не отменяет серверную операцию.</p>}
    </div><div className="dialog-footer"><DialogClose disabled={inspect.isPending||check.isPending||install.isPending}>Закрыть</DialogClose></div></Dialog>}
    <Panel title="Операции SSH-установки"><div className="panel-body">
      {operations.isPending?<Loading/>:operations.isError?<ErrorState error={operations.error} onRetry={()=>void operations.refetch()} retrying={operations.isFetching}/>:<div className="table-scroll"><table><thead><tr><th>ID / нода</th><th>Шаг</th><th>Статус</th><th>Последнее обновление</th></tr></thead><tbody>
        {(operations.data?.items??[]).map(op=><tr key={op.id}><td>{op.name}<small className="node-secondary">{op.operation_type==='remove'?`Удаление (${op.removal_mode})`:op.operation_type==='upgrade'?`Обновление → ${op.target_version??'?'}`:'Установка'} · <code>{op.id}</code></small></td><td>{op.step}</td><td>{op.status}{op.error&&<small className="error node-secondary">{op.error}</small>}</td><td>{readableTime(op.updated_at)}</td></tr>)}
      </tbody></table>{operations.data?.items.length===0&&<div className="empty">Операций пока нет.</div>}</div>}
    </div></Panel>
    <Panel title="Ручная регистрация подготовленного агента"><div className="panel-body">
      <p className="muted">Для чистого VPS можно выполнить проверенную локальную установку одной командой после независимого получения подписанного ZIP, публичного ключа и доверенного бинарника ArcNet. Токен вводится через stdin, а не в аргументы или историю команд. Для уже установленного агента используйте ручную регистрацию.</p>
      <Button onClick={()=>{setManual(value=>!value);setEnrollment(null);}}>{manual?'Скрыть ручную регистрацию':'Показать ручную регистрацию'}</Button>
      {manual&&<><Button disabled={enroll.isPending||!name.trim()||!host||!vpnPort} onClick={()=>enroll.mutate()}>Выдать одноразовый токен (10 минут)</Button>
        {enrollment&&<><p>Контроллер: <code>{enrollment.controller}</code></p><label className="field"><span>Токен регистрации (никому не передавать)</span><textarea readOnly rows={3} value={enrollment.token}/></label>
          <p className="muted">Чистая нода, после копирования проверенного CLI, signed-node.zip и node-release-public.hex в /root:</p>
          <pre className="code" tabIndex={0}>{`sudo /root/arcvpn node-install --signed-bundle /root/signed-node.zip --public-key /root/node-release-public.hex --controller ${shellQuote(enrollment.controller)} --vpn-port ${vpnPort} --token-stdin`}</pre>
          <p className="muted">Уже установленный агент:</p>
          <pre className="code" tabIndex={0}>{`sudo -u arcvpn-node /opt/arcvpn-node/bin/agent node-register --controller ${shellQuote(enrollment.controller)} --token-stdin --config /var/lib/arcvpn-node/node.json`}</pre>
          <p className="muted">Вставьте токен в стандартный ввод и нажмите Enter. После локальной установки агент зарегистрируется, но применение VPN и внешний handshake проверяются отдельно. Никогда не скачивайте установщик через незаверенный curl | bash.</p>
          <Button onClick={()=>{setEnrollment(null);}}>Скрыть токен</Button>
        </>}
      </>}
    </div></Panel>
    <Notice>«Агент на связи» и «Xray active» не подтверждают VPN handshake. Внешний VPN-probe требует отдельной проверяющей машины и одноразового допуска; даже PASS не приводит к автоматической публикации endpoint в подписках.</Notice>
  </>;
}
