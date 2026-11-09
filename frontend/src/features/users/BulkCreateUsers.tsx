import {useState,type FormEvent} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {api,ApiError} from '../../shared/api';
import Dialog,{DialogClose} from '../../shared/Dialog';
import {Button,Field,Notice} from '../../shared/ui';

export default function BulkCreateUsers({onClose}:{onClose:()=>void}){
 const cache=useQueryClient();
 const [text,setText]=useState(''),[group,setGroup]=useState(''),[busy,setBusy]=useState(false),[saved,setSaved]=useState(false),[message,setMessage]=useState(''),[error,setError]=useState(''),[unknown,setUnknown]=useState(false);
 const names=text.split(/\r?\n/).map(v=>v.trim()).filter(Boolean);
 const duplicate=new Set(names).size!==names.length;
 async function refresh(){await Promise.all(['users','user-usage','user-editor'].map(key=>cache.invalidateQueries({queryKey:[key]})))}
 async function save(e:FormEvent){e.preventDefault();if(busy||saved||unknown)return;setBusy(true);setError('');setMessage('');try{
  const result=await api<{runtime_state:string;count:number}>('/api/users/bulk-create','POST',{names,group});
  setSaved(true);await refresh();
  if(result.runtime_state==='applied')onClose();else setMessage(`Создано: ${result.count}. Применение к VPN-службам ещё не подтверждено; панель повторит попытку.`);
 }catch(e){setError(e instanceof Error?e.message:'Не удалось создать пользователей');if(e instanceof ApiError&&e.outcomeUnknown)setUnknown(true)}finally{setBusy(false)}}
 return <Dialog title="Добавить пользователей списком" onClose={()=>{if(unknown)void refresh();onClose()}} busy={busy} dirty={!saved&&!!(text||group)}>
  <form onSubmit={save}><div className="dialog-body"><fieldset disabled={busy||saved} className="user-editor-fields batch-fields"><label className="field"><span>Имена пользователей</span><textarea rows={7} maxLength={6500} required value={text} onChange={e=>setText(e.target.value)} placeholder={'Алексей\nМарина\nТестовый доступ'}/><small>Одно имя на строку, до 100 пользователей. Повторы и занятые имена не создаются.</small></label><Field label="Группа" maxLength={50} value={group} onChange={e=>setGroup(e.target.value)}/><p className="muted">Создадим {names.length} пользователей без срока и лимита трафика. Доступны все подключения; состав и пакеты можно изменить сразу после создания массовым действием.</p>{duplicate&&<p className="error">Есть повторяющиеся имена.</p>}</fieldset>{message&&<Notice>{message}</Notice>}{error&&<p className="error" role="alert">{error}</p>}</div><footer className="dialog-footer"><DialogClose disabled={busy}>{saved?'Закрыть':'Отмена'}</DialogClose><Button variant="primary" type="submit" pending={busy} disabled={saved||unknown||duplicate||names.length<1||names.length>100}>Создать ({names.length})</Button></footer></form>
 </Dialog>
}
