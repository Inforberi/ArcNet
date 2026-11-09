import {useConfirmation} from '../../shared/Confirmation';
import {useEffect,useState,type ReactNode} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {api} from '../../shared/api';
import Dialog,{canDismiss,DialogClose} from '../../shared/Dialog';
import DirectAccess from './DirectAccess';
import {Advanced,Button,Field,Loading,Notice,ErrorState} from '../../shared/ui';

type Device={id:string;name:string;created_at:number;revoked_at:number;wireguard_address?:string};
type Reply={items:Device[];managed:boolean;limit:number;version:number;active:number;supported:boolean;runtime_state:string;remote_pending:boolean};
type Links={title?:string;url:string;links:string[];link_ids:string[];device_id:string;wireguard_url?:string};

export default function DevicesDialog({id,name,onClose,navigation}:{id:number;name:string;onClose:()=>void;navigation?:ReactNode}) {
 const confirm=useConfirmation();
 const cache=useQueryClient();
 const query=useQuery({queryKey:['user-devices',id],queryFn:()=>api<Reply>(`/api/users/${id}/devices`),refetchInterval:q=>q.state.data?.remote_pending?5000:false,refetchIntervalInBackground:false});
 const [limit,setLimit]=useState('0');const [limitDirty,setLimitDirty]=useState(false);const [busy,setBusy]=useState(false);const [error,setError]=useState('');
 const [message,setMessage]=useState('');const [links,setLinks]=useState<Links|null>(null);
 const data=query.data;
 useEffect(()=>{if(data){if(!limitDirty)setLimit(String(data.limit));setLinks(null)}},[data?.version]);
 async function change(method:'PUT'|'DELETE',body:object,device?:string) {
  if(!data)return;setBusy(true);setError('');setMessage('');setLinks(null);
  try {
   const updated=await api<Reply>(`/api/users/${id}/devices${device?`/${device}`:''}`,method,{...body,expected_version:data.version});
   cache.setQueryData(['user-devices',id],updated);
   if(method==='PUT'){setLimit(String(updated.limit));setLimitDirty(false)}
   await cache.invalidateQueries({queryKey:['users']});
   await cache.invalidateQueries({queryKey:['user-usage']});
   setMessage(updated.runtime_state==='applied'&&!updated.remote_pending?'Изменение применено.':'Изменение сохранено. Применение VPN-службой ещё не подтверждено.');
  }catch(e){setError(e instanceof Error?e.message:'Не удалось сохранить');await query.refetch()}
  finally{setBusy(false)}
 }
 async function showLinks(device:Device) {
  setBusy(true);setError('');setLinks(null);
  try{setLinks(await api<Links>(`/api/users/${id}/devices/${device.id}/links`))}
  catch(e){setError(e instanceof Error?e.message:'Не удалось получить подключения')}
  finally{setBusy(false)}
 }
 return <Dialog title={`Устройства · ${name}`} onClose={onClose} busy={busy} dirty={limitDirty}>
  {navigation&&<div onClickCapture={event=>{if((event.target as HTMLElement).closest('button[aria-pressed="false"]')&&!canDismiss(limitDirty,busy)){event.preventDefault();event.stopPropagation();}}}>{navigation}</div>}
  <div className="dialog-body">
   <section className="device-observation"><h3 className="section">Устройства и активные IP</h3><p className="muted">Наблюдения активных подключений пока недоступны. Обновление подписки не подтверждает, что устройство подключено к VPN.</p><details className="device-explanation"><summary>Как определяются устройства?</summary><p>IP может измениться при переключении сети, а несколько устройств могут использовать один IP. Название и версия приложения тоже не определяют устройство: обновление приложения не означает новое устройство.</p><p>Чтобы узнавать то же устройство после обновления, нужен устойчивый идентификатор от клиента. Сейчас такого источника нет, поэтому панель не показывает предполагаемые устройства как подтверждённые.</p></details></section>
   {error&&<p className="error" role="alert">{error}</p>}{message&&<p className="notice" role="status">{message}</p>}
   {query.isPending?<Loading/>:query.isError?<ErrorState error={query.error} onRetry={()=>void query.refetch()} retrying={query.isFetching}/>:data&&<>
    {!data.supported&&<Notice>Поддерживаются локальные Xray/Hysteria2/WireGuard и обновлённые прямые TCP/REALITY-ноды. Для удалённых нод нужны device-capability и отсутствие конечных пакетов или активного каскада.</Notice>}
    {data.managed&&!message&&data.runtime_state!=='applied'&&<Notice>Применение доступа VPN-службой пока не подтверждено. При ошибке панель повторит применение; старые соединения могут продолжать работать.</Notice>}
    {data.managed?<p>Ранее выданных доступов: {data.active} / {data.limit||'без ограничения'}. Общий ключ отключён.</p>:<p className="muted">Используется общий профиль. Число устройств и IP нельзя определить по одному ключу.</p>}
    {data.remote_pending&&<Notice>Удалённые ноды ещё не подтвердили текущие доступы. Выдача новых ссылок заблокирована; отзыв не считается завершённым до применения узлами.</Notice>}
    {data.items.length>0&&<Advanced title="Ранее выданные доступы"><div className="protocol-grid">{data.items.map(device=><div className="protocol-card" key={device.id}>
     <strong>{device.name}</strong><p className="muted">{device.revoked_at?'Доступ отозван':`Создан ${new Date(device.created_at*1000).toLocaleDateString('ru-RU')}`}</p>
     {device.wireguard_address&&<p className="muted">WireGuard · {device.wireguard_address}</p>}
     {!device.revoked_at&&<div className="flex wrap"><Button disabled={busy} onClick={()=>void showLinks(device)}>Подключить</Button><Button variant="danger" disabled={busy} onClick={async ()=>{if((await confirm(`Отозвать доступ «${device.name}»? Его подписка перестанет работать. Завершение VPN-соединений зависит от применения изменения службой.`)))void change('DELETE',{},device.id)}}>Отозвать</Button></div>}
    </div>)}</div></Advanced>}
    {links&&<div>
      <DirectAccess links={links.links} ids={links.link_ids} subscription={{title:links.title,url:links.url,qrURL:`/api/users/${id}/devices/${encodeURIComponent(links.device_id)}/qr?format=subscription`}} qrURL={selection=>`/api/users/${id}/devices/${encodeURIComponent(links.device_id)}/qr?selection=${encodeURIComponent(selection)}`} onCopy={value=>navigator.clipboard.writeText(value)} onError={()=>{}}/>
      {links.wireguard_url&&<a className="button inline-action" href={links.wireguard_url} download>Скачать WireGuard .conf</a>}
    </div>}
    {!data.managed&&<div className="notice"><strong>Лимит одновременных подключений</strong><p>Исполнение ограничения устройств или IP ещё не подключено. Сохранение числа в интерфейсе само по себе не ограничит доступ.</p></div>}
    {data.managed&&<Advanced title="Лимит ранее выданных доступов"><form onSubmit={async e=>{e.preventDefault();const value=Number(limit);if(!Number.isInteger(value)||value<0||value>100){setError('Укажите целое число от 0 до 100');return}if(!(await confirm(`Лимит доступов: ${data.limit||'без ограничения'} → ${value||'без ограничения'}. Уже выданные доступы автоматически не отзываются.`)))return;void change('PUT',{limit:value})}}>
     <Field label="Лимит активных доступов" type="number" min={0} max={100} step={1} value={limit} onChange={e=>{setLimit(e.target.value);setLimitDirty(true)}}/><p className="muted">0 — без ограничения тарифа. Технический предел — 100 активных доступов на пользователя.</p>
     <Button type="submit" disabled={busy||!data.supported||Number(limit)===data.limit}>Сохранить лимит</Button>
    </form></Advanced>}
   </>}
  </div>
  <footer className="dialog-footer"><DialogClose disabled={busy}>Закрыть</DialogClose></footer>
 </Dialog>
}
