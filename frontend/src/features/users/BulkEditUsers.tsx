import {useState,type FormEvent} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {api,type VPNUser,type VPNConfig,type ProtocolProfile} from '../../shared/api';
import Dialog,{DialogClose} from '../../shared/Dialog';
import {Button,Field,Notice,Loading} from '../../shared/ui';
import {quotaBytes,type UserPackage} from './usage';

const labels:Record<string,string>={'vless-reality':'VLESS · REALITY TCP','vless-xhttp':'VLESS · REALITY XHTTP',hysteria2:'Hysteria2',wireguard:'WireGuard'};
export default function BulkEditUsers({users,packages,onClose,onDone}:{users:VPNUser[];packages:Map<number,UserPackage>;onClose:()=>void;onDone:()=>void}){
 const cache=useQueryClient();
 const [quotaMode,setQuotaMode]=useState('keep'),[quota,setQuota]=useState('10'),[expiryMode,setExpiryMode]=useState('keep'),[expiry,setExpiry]=useState(''),[connections,setConnections]=useState('keep'),[kinds,setKinds]=useState<string[]>([]),[changeGroup,setChangeGroup]=useState(false),[group,setGroup]=useState(''),[busy,setBusy]=useState(false),[saved,setSaved]=useState(false),[error,setError]=useState('');
 const vpn=useQuery({queryKey:['vpn'],queryFn:()=>api<VPNConfig>('/api/vpn')});
 const profiles=useQuery({queryKey:['protocols'],queryFn:()=>api<{items:ProtocolProfile[]}>('/api/protocols')});
 const options=[...(vpn.data?.configured?['vless-reality']:[]),...(profiles.data?.items.map(p=>p.kind).filter(k=>k!=='amneziawg')??[])];
 const dirty=quotaMode!=='keep'||expiryMode!=='keep'||connections!=='keep'||changeGroup;
 async function save(e:FormEvent){
  e.preventDefault();setError('');
  const changes:Record<string,unknown>={};
  if(quotaMode!=='keep'){const bytes=quotaMode==='unlimited'?0:quotaBytes(quota);if(bytes===undefined){setError('Проверьте размер пакета');return}changes.quota_bytes=bytes}
  if(expiryMode!=='keep'){const timestamp=expiryMode==='none'?0:Math.floor(new Date(expiry).getTime()/1000);if(!Number.isSafeInteger(timestamp)||timestamp<0){setError('Проверьте срок действия');return}changes.expires_at=timestamp}
  if(connections!=='keep')changes.connections={allowed_kinds:connections==='all'?null:kinds};
  if(changeGroup)changes.group=group;
  if((quotaMode!=='keep'||expiryMode!=='keep')&&users.some(u=>!packages.has(u.id))){setError('Нет данных пакетов. Обновите список перед изменением.');return}
  if(!window.confirm(`Применить выбранные изменения к ${users.length} пользователям?`))return;
  setBusy(true);
  try{
   const result=await api<{runtime_state:string}>('/api/users/bulk-edit','PUT',{changes,items:users.map(u=>({id:u.id,expected_version:u.preferences?.version??0,expected_enabled:u.enabled,package_version:packages.get(u.id)?.version??0}))});
   setSaved(true);
   await Promise.all(['users','user-usage','user-editor','user-preferences'].map(key=>cache.invalidateQueries({queryKey:[key]})));
   if(result.runtime_state==='applied'){onDone();onClose()}else setError('Все изменения сохранены. Применение к VPN-службам ещё не подтверждено; панель повторит попытку.');
  }catch(e){setError(e instanceof Error?e.message:'Не удалось изменить пользователей')}finally{setBusy(false)}
 }
 function close(){if(saved)onDone();onClose()}
 return <Dialog title={`Изменить пользователей · ${users.length}`} onClose={close} busy={busy} dirty={dirty&&!saved} className="user-editor">
 <form onSubmit={save}><div className="dialog-body"><p className="muted">Изменятся только отмеченные параметры. Ключи, подписки и накопленный расход сохраняются.</p><fieldset disabled={busy||saved} className="user-editor-fields batch-fields">
 <div className="field-row"><label className="field"><span>Пакет трафика</span><select value={quotaMode} onChange={e=>setQuotaMode(e.target.value)}><option value="keep">Не менять</option><option value="unlimited">Без лимита</option><option value="set">Задать пакет</option></select></label>{quotaMode==='set'&&<Field label="Размер пакета, ГБ" inputMode="decimal" value={quota} required onChange={e=>setQuota(e.target.value)}/>}</div>
 <div className="field-row"><label className="field"><span>Срок действия</span><select value={expiryMode} onChange={e=>setExpiryMode(e.target.value)}><option value="keep">Не менять</option><option value="none">Без срока</option><option value="set">Задать дату</option></select></label>{expiryMode==='set'&&<Field label="Действует до" type="datetime-local" required value={expiry} onChange={e=>setExpiry(e.target.value)} description="Время вашего браузера."/>}</div>
 <label className="field"><span>Разрешённые подключения</span><select value={connections} onChange={e=>setConnections(e.target.value)}><option value="keep">Не менять</option><option value="all">Все, включая новые</option><option value="choose">Выбрать состав</option></select></label>
 {connections==='choose'&&(vpn.isError||profiles.isError?<Notice>Не удалось загрузить подключения. Обновите страницу.</Notice>:vpn.isPending||profiles.isPending?<Loading/>:<div className="connection-choices">{options.map(kind=><label key={kind} className={`connection-choice ${kinds.includes(kind)?'selected':''}`}><input type="checkbox" checked={kinds.includes(kind)} onChange={e=>setKinds(v=>e.target.checked?[...v,kind]:v.filter(k=>k!==kind))}/>{labels[kind]??kind}</label>)}</div>)}
 {connections==='choose'&&!kinds.length&&<Notice>Ни одного подключения не выбрано: доступ ко всем протоколам будет отозван.</Notice>}
 <label className="connection-choice"><input type="checkbox" checked={changeGroup} onChange={e=>setChangeGroup(e.target.checked)}/><span>Изменить группу</span></label>{changeGroup&&<Field label="Группа" value={group} maxLength={50} onChange={e=>setGroup(e.target.value)} description="Пустое поле удалит пользователей из группы."/>}
 {dirty&&<section className="notice"><strong>План: {users.length} пользователей</strong>{quotaMode!=='keep'&&<div>Пакет: {quotaMode==='unlimited'?'без лимита':`${quota} ГБ`}; расход не сбрасывается.</div>}{expiryMode!=='keep'&&<div>Срок: {expiryMode==='none'?'без срока':expiry||'укажите дату'}.</div>}{connections!=='keep'&&<div>Подключения: {connections==='all'?'все текущие и будущие':kinds.map(k=>labels[k]).join(', ')||'ни одного'}.</div>}{changeGroup&&<div>Группа: {group||'без группы'}.</div>}</section>}
 </fieldset>{error&&<Notice>{error}</Notice>}</div><footer className="dialog-footer"><DialogClose disabled={busy}>{saved?'Закрыть':'Отмена'}</DialogClose><Button variant="primary" type="submit" disabled={busy||saved||!dirty||users.length>100||connections==='choose'&&(vpn.isPending||profiles.isPending||vpn.isError||profiles.isError)}>{busy?'Применяем…':'Применить изменения'}</Button></footer></form></Dialog>
}
