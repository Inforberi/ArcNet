import {useConfirmation} from '../shared/Confirmation';
import {useState,type FormEvent,type ChangeEvent} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {api} from '../shared/api';
import {Button,Field,Notice,PageHeading,Panel,Properties,SettingsSection} from '../shared/ui';

type ExportReply={filename:string;content:string};
type Preview={devices_included:boolean;usage_included:boolean;version:number;created_at:number;users:number;current_users:number;protocols:number;current_protocols:number;nodes:number;current_nodes:number;routes:number;current_routes:number;routes_replaced:boolean;routing_profiles:number;current_routing_profiles:number;routing_profiles_replaced:boolean;log_budget_mb?:number;vpn_configured:boolean;vpn_address?:string;vpn_port?:number};
const maxArchiveSize=64*1024*1024;

function download(content:string,filename:string):void{
 const url=URL.createObjectURL(new Blob([content],{type:'application/json;charset=utf-8'}));
 const a=document.createElement('a');a.href=url;a.download=filename;document.body.append(a);a.click();a.remove();
 window.setTimeout(()=>URL.revokeObjectURL(url),5000);
}
export default function Backup(){
 const confirm=useConfirmation();
 const queryClient=useQueryClient();
 const[exportPassword,setExportPassword]=useState('');const[exportConfirmation,setExportConfirmation]=useState('');const[exportBusy,setExportBusy]=useState(false);
 const[newAddress,setNewAddress]=useState('');const[importPassword,setImportPassword]=useState('');const[archive,setArchive]=useState('');const[preview,setPreview]=useState<Preview|null>(null);
 const[importBusy,setImportBusy]=useState(false);const[message,setMessage]=useState('');const[error,setError]=useState('');
 async function exportState(e:FormEvent<HTMLFormElement>){e.preventDefault();setError('');setMessage('');
  if(exportPassword.length<12 || exportPassword!==exportConfirmation){setError('Пароли должны совпадать и содержать минимум 12 символов');return}
  setExportBusy(true);try{const v=await api<ExportReply>('/api/backup/export','POST',{password:exportPassword});download(v.content,v.filename);setMessage('Зашифрованный архив сохранён');setExportPassword('');setExportConfirmation('')}
  catch(e){setError(e instanceof Error?e.message:'Ошибка экспорта')}finally{setExportBusy(false)}
 }
 async function readFile(e:ChangeEvent<HTMLInputElement>){setArchive('');setPreview(null);setError('');const f=e.target.files?.[0];if(!f)return;
  if(f.size>maxArchiveSize){setError('Файл слишком большой (лимит 64 МБ)');return}
  try{setArchive(await f.text())}catch{setError('Файл не удалось прочитать')}
 }
 async function previewImport(){setPreview(null);setError('');setMessage('');if(!archive){setError('Выбери архив');return}
  setImportBusy(true);try{const v=await api<Preview>('/api/backup/preview','POST',{archive,password:importPassword,new_address:newAddress.trim()});setPreview(v)}
  catch(e){setError(e instanceof Error?e.message:'Ошибка проверки архива')}finally{setImportBusy(false)}
 }
 async function restore(){if(!preview || !preview.vpn_configured)return;
  if(!(await confirm(`Заменить ${preview.current_protocols} протоколов и ${preview.current_users} пользователей данными из архива (${preview.protocols} протоколов, ${preview.users} пользователей)? ${preview.routes_replaced ? `Черновики маршрутов: ${preview.current_routes} → ${preview.routes}. Профили правил: ${preview.routing_profiles_replaced ? `${preview.current_routing_profiles} → ${preview.routing_profiles}` : "сохраняются"}.` : "Существующие черновики маршрутов сохраняются; несовместимые ссылки на ноды заблокируют импорт."}`)))return;
  setImportBusy(true);setError('');try{await api('/api/backup/import','POST',{archive,password:importPassword,new_address:newAddress.trim(),confirm:'REPLACE_ARCVPN_STATE'});await queryClient.invalidateQueries();setMessage('Восстановление завершено. Данные всех разделов обновлены.');setArchive('');setImportPassword('');setPreview(null)}
  catch(e){setError(e instanceof Error?e.message:'Ошибка восстановления')}finally{setImportBusy(false)}
 }
 return <>
  <PageHeading title="Резервные копии" description="Перенос пользователей и настроек между экземплярами ArcNet"/>
  <Notice>Архив содержит приватные ключи VPN, токены подписок и хеши ключей нод. Храни его отдельно от VPS. При переносе на другой IP нужно обновить адрес подписки у клиентов: старая IP-ссылка не переносится автоматически. Агентам нод при смене адреса контроллера потребуется обновить endpoint.</Notice>
  {error&&<p className="error" role="alert">{error}</p>}{message&&<div className="notice" role="status">{message}</div>}
  <Panel title="Экспорт настроек и пользователей"><form className="panel-body" onSubmit={exportState}>
   <p className="muted">Сохраняются пользователи, все протоколы и их ключи, подписки, зарегистрированные ноды (с хешами их ключей), черновики маршрутов, лимит логов панели. Маршруты остаются неактивными. Учётная запись администратора и активные сессии не переносятся.</p>
   <div className="field-row"><Field label="Пароль архива" type="password" autoComplete="new-password" minLength={12} maxLength={1024} value={exportPassword} onChange={e=>setExportPassword(e.target.value)} required/>
   <Field label="Подтверждение пароля" type="password" autoComplete="new-password" minLength={12} value={exportConfirmation} onChange={e=>setExportConfirmation(e.target.value)} required/></div>
   <Button variant="primary" type="submit" disabled={exportBusy}>{exportBusy?'Шифрование…':'Скачать зашифрованный архив'}</Button>
  </form></Panel>
  <Panel title="Импорт и восстановление"><div className="panel-body">
   <p className="muted">Восстановление заменит текущую VPN-конфигурацию и пользователей. Перед применением нужно проверить архив; резервная копия текущего состояния обязательна.</p>
   <label className="field"><span>JSON-архив ArcNet</span><input type="file" accept="application/json,.json" onChange={e=>void readFile(e)}/></label>
   <Field label="Публичный IPv4 нового VPS (необязательно)" placeholder="Если IP изменился" value={newAddress} onChange={e=>{setNewAddress(e.target.value);setPreview(null)}}/>
   <Field label="Пароль архива" type="password" autoComplete="off" maxLength={1024} value={importPassword} onChange={e=>{setImportPassword(e.target.value);setPreview(null)}}/>
   <div className="flex wrap"><Button onClick={()=>void previewImport()} disabled={importBusy||!archive||!importPassword}>Проверить архив</Button>
    <Button variant="danger" onClick={()=>void restore()} disabled={importBusy||!preview?.vpn_configured}>Заменить VPN и пользователей</Button></div>
   {preview&&<SettingsSection title="План восстановления" description="Архив проверен. Изменения ещё не применены."><Properties items={[
    {label:'Пользователи',value:`${preview.current_users} → ${preview.users}`},
    {label:'Дополнительные подключения',value:`${preview.current_protocols} → ${preview.protocols}`},
    {label:'Ноды',value:preview.version<=2?'Текущие сохраняются':`${preview.current_nodes??0} → ${preview.nodes??0}`},
    {label:'VPN-адрес',value:preview.vpn_address??'Не указан'},
    {label:'Черновики маршрутов',value:preview.routes_replaced?`${preview.current_routes} → ${preview.routes}`:'Текущие сохраняются'},
    {label:'Профили правил',value:preview.routing_profiles_replaced?`${preview.current_routing_profiles} → ${preview.routing_profiles}`:'Текущие сохраняются'},
    {label:'Пакеты и расход',value:preview.usage_included?'Переносятся; известный расход не уменьшается':'Сохраняются для пользователей с тем же UUID'},
    {label:'Выданные доступы',value:preview.devices_included?'Переносятся; местные отзывы сохраняются':'Сохраняются для пользователей с тем же UUID'}
   ]}/><Notice>Текущее состояние будет заменено. Несовместимые ссылки на отсутствующие ноды блокируют импорт. После восстановления нужно проверить VPN-подключение с устройства.</Notice></SettingsSection>}
  </div></Panel>
 </>;
}
