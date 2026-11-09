import {useState} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {type VPNUser} from '../../shared/api';
import Dialog,{DialogClose} from '../../shared/Dialog';
import {useConfirmation} from '../../shared/Confirmation';
import {Advanced,Button,Properties,SettingsSection} from '../../shared/ui';
import UserEditor from './UserEditor';
import UserTrafficPanel from './UserTrafficPanel';
import {UsageSummary} from './PackageDialog';
import DevicesDialog from './DevicesDialog';
import {userAccess,type UsageReply,type UserPackage} from './usage';
import type {TrafficDetail} from './traffic';

export default function UserDetails({initialTab='overview',trafficNow,traffic,trafficPending,trafficFailed,user,usage,account,onClose,onConnect,onToggle,onDelete}:{initialTab?:'overview'|'edit';trafficNow:number;traffic?:TrafficDetail;trafficPending:boolean;trafficFailed:boolean;user:VPNUser;usage?:UsageReply;account?:UserPackage;onClose:()=>void;onConnect:()=>void;onToggle:()=>Promise<unknown>;onDelete:()=>Promise<unknown>}){
 const confirm=useConfirmation(),cache=useQueryClient();
 const [tab,setTab]=useState<'edit'|'activity'>(initialTab==='edit'?'edit':'activity');
 const [editorState,setEditorState]=useState({busy:false,dirty:false});
 const [devicesOpen,setDevicesOpen]=useState(false),[busy,setBusy]=useState(false),[error,setError]=useState('');
 const locked=busy||editorState.busy;
 function close(){for(const key of ['users','user-editor','user-preferences','user-usage','user-devices'])void cache.invalidateQueries({queryKey:[key]});onClose()}
 async function connect(){if(locked)return;if(user.devices_managed){setDevicesOpen(true);return}if(editorState.dirty&&!await confirm('Закрыть редактор без сохранения и открыть подключение?',{action:'Продолжить без сохранения'}))return;onConnect()}
 async function change(remove=false){
  if(!await confirm(remove?`Удалить пользователя «${user.name}» и отозвать его доступ?`:`${user.enabled?'Отключить':'Включить'} доступ пользователя «${user.name}»?`,{action:remove?'Удалить пользователя':user.enabled?'Отключить доступ':'Включить доступ'}))return;
  setBusy(true);setError('');try{if(remove){await onDelete();close()}else await onToggle()}catch(e){setError(e instanceof Error?e.message:'Не удалось изменить доступ')}finally{setBusy(false)}
 }
 const access=userAccess(user,account);
 return <><Dialog title={`Пользователь · ${user.name}`} className="user-editor user-workspace sectioned-editor" onClose={close} busy={locked} dirty={editorState.dirty}>
  <div className="user-workspace-toolbar"><nav className="segmented-control" aria-label="Разделы пользователя"><Button aria-pressed={tab==='edit'} disabled={locked} onClick={()=>setTab('edit')}>Параметры</Button><Button aria-pressed={tab==='activity'} disabled={locked} onClick={()=>setTab('activity')}>Активность</Button></nav><Button disabled={locked} variant="primary" onClick={()=>void connect()}>Подключить</Button></div>
  <div className="user-parameters-pane" hidden={tab!=='edit'}><UserEditor user={user} onStateChange={setEditorState} onClose={close}/></div>
  <div hidden={tab!=='activity'}><div className="dialog-body">
   <SettingsSection title="Трафик и доступ"><Properties items={[{label:'Доступ',value:access.label},{label:'Срок',value:account?account.expires_at?new Date(account.expires_at*1000).toLocaleString('ru-RU'):'Без срока':'Нет данных'}]}/><UsageSummary item={account} complete={usage?.complete_coverage??false}/></SettingsSection>
   <UserTrafficPanel now={trafficNow} detail={traffic} pending={trafficPending} failed={trafficFailed}/>
   <Advanced title="Доступы и дополнительные действия"><p className="muted">Выданные ключи не определяют количество физических устройств. Наблюдения активных IP пока недоступны.</p><div className="flex wrap"><Button disabled={locked} onClick={()=>setDevicesOpen(true)}>Ранее выданные доступы</Button><Button disabled={locked||editorState.dirty} title={editorState.dirty?'Сначала сохраните или отмените изменения параметров':undefined} onClick={()=>void change()}>{user.enabled?'Отключить доступ':'Включить доступ'}</Button><Button variant="danger" disabled={locked} onClick={()=>void change(true)}>Удалить пользователя</Button></div></Advanced>
   {error&&<p className="error" role="alert">{error}</p>}
  </div><footer className="dialog-footer"><DialogClose disabled={locked}>Закрыть</DialogClose></footer></div>
 </Dialog>{devicesOpen&&<DevicesDialog id={user.id} name={user.name} onClose={()=>setDevicesOpen(false)}/>}</>;
}
