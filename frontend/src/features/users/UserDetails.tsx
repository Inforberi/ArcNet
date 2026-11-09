import {useState} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {type VPNUser,ApiError} from '../../shared/api';
import Dialog from '../../shared/Dialog';
import ActionMenu from '../../shared/ActionMenu';
import {useConfirmation} from '../../shared/Confirmation';
import {Button,Properties,SettingsSection} from '../../shared/ui';
import UserEditor from './UserEditor';
import UserTrafficPanel from './UserTrafficPanel';
import {UsageSummary} from './PackageDialog';
import DevicesDialog from './DevicesDialog';
import {userAccess,type UsageReply,type UserPackage} from './usage';
import type {TrafficDetail} from './traffic';

export default function UserDetails({trafficNow,traffic,trafficPending,trafficFailed,user,usage,account,onClose,onConnect,onDelete}:{initialTab?:'overview'|'edit';trafficNow:number;traffic?:TrafficDetail;trafficPending:boolean;trafficFailed:boolean;user:VPNUser;usage?:UsageReply;account?:UserPackage;onClose:()=>void;onConnect:()=>void;onToggle:()=>Promise<unknown>;onDelete:()=>Promise<unknown>}){
 const confirm=useConfirmation(),cache=useQueryClient();
 const [section,setSection]=useState('profile'),[editorState,setEditorState]=useState({busy:false,dirty:false});
 const [devicesOpen,setDevicesOpen]=useState(false),[busy,setBusy]=useState(false),[error,setError]=useState(''),[unknown,setUnknown]=useState(false);
 const locked=busy||editorState.busy;
 function close(){for(const key of ['users','user-editor','user-preferences','user-usage','user-devices'])void cache.invalidateQueries({queryKey:[key]});onClose()}
 async function connect(){if(locked)return;if(user.devices_managed){setDevicesOpen(true);return}if(editorState.dirty&&!await confirm('Закрыть редактор без сохранения и открыть подключение?',{action:'Продолжить без сохранения'}))return;onConnect()}
 async function remove(){if(locked||unknown)return;if(!await confirm(`Удалить пользователя «${user.name}» и отозвать его доступ?`,{title:'Удалить пользователя?',action:'Удалить пользователя'}))return;setBusy(true);setError('');try{await onDelete();close()}catch(e){if(e instanceof ApiError&&e.outcomeUnknown)setUnknown(true);setError(e instanceof Error?e.message:'Не удалось удалить пользователя')}finally{setBusy(false)}}
 const access=userAccess(user,account);
 const activity=<><SettingsSection title="Трафик и доступ"><Properties items={[{label:'Доступ',value:access.label},{label:'Срок',value:account?account.expires_at?new Date(account.expires_at*1000).toLocaleString('ru-RU'):'Без срока':'Нет данных'}]}/><UsageSummary item={account} complete={usage?.complete_coverage??false}/></SettingsSection><UserTrafficPanel now={trafficNow} detail={traffic} pending={trafficPending} failed={trafficFailed}/></>;
 return <><Dialog title={`Пользователь · ${user.name}`} className="user-editor user-workspace sectioned-editor" onClose={close} busy={locked} dirty={editorState.dirty}>
  <div className="user-workspace-toolbar"><Button disabled={locked} variant="primary" onClick={()=>void connect()}>Подключить</Button><ActionMenu label={`Действия пользователя · ${user.name}`} disabled={locked||unknown} items={[{label:'Удалить пользователя',danger:true,onClick:()=>void remove()}]}/></div>
  <nav className="editor-section-nav" aria-label="Разделы пользователя">{[['profile','Основное'],['connections','Доступ'],['package','Лимиты и срок'],['activity','Активность']].map(([id,label])=><Button type="button" key={id} disabled={locked} aria-pressed={section===id} onClick={()=>setSection(id)}>{label}</Button>)}</nav>
  {error&&<p className="error editor-operation-error" role="alert">{error}</p>}
  {unknown&&<p className="notice">Результат удаления неизвестен. Повтор заблокирован; закройте карточку и сверьте список.</p>}
  <UserEditor user={user} section={section} onSectionChange={setSection} activity={activity} onStateChange={setEditorState} onClose={close} navigation={section==='connections'?<div className="dialog-body"><Button disabled={locked} onClick={()=>setDevicesOpen(true)}>Выданные доступы и устройства</Button><p className="muted">Выданный ключ не определяет физическое устройство.</p></div>:undefined}/>
 </Dialog>{devicesOpen&&<DevicesDialog id={user.id} name={user.name} onClose={()=>setDevicesOpen(false)}/>}</>;
}
