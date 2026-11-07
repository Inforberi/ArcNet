import {useState,type FormEvent} from 'react';
import Dialog, {canDismiss} from '../../shared/Dialog';
import {Advanced,Button,Field} from '../../shared/ui';
import {connectionLabel,type ConnectionItem} from './ConnectionTable';
import TargetPicker from '../targets/TargetPicker';
import FingerprintField from './FingerprintField';

export type ConnectionSettings=Pick<ConnectionItem,'port'|'sni'|'target'|'enabled'|'fingerprint'|'path'>;
export default function ConnectionEditor({item,busy,error,onClose,onSubmit}:{item:ConnectionItem;busy:boolean;error:string;onClose:()=>void;onSubmit:(settings:ConnectionSettings)=>Promise<void>}){
 const [draft,setDraft]=useState<ConnectionSettings>({path:item.path??'',fingerprint:item.fingerprint??(item.kind.startsWith('vless')?'chrome':''),port:item.port,sni:item.sni,target:item.target,enabled:item.enabled});
 const [pickingTarget,setPickingTarget]=useState(false);
 const reality=item.kind.startsWith('vless');
 const changes=[draft.path!==(item.path??'')?`Путь XHTTP: ${item.path} → ${draft.path}`:'',draft.fingerprint!==(item.fingerprint??(reality?'chrome':''))?`TLS fingerprint: ${item.fingerprint||'chrome'} → ${draft.fingerprint}`:'',draft.port!==item.port?`Порт: ${item.port} → ${draft.port}`:'',draft.sni!==item.sni?`SNI: ${item.sni} → ${draft.sni}`:'',draft.target!==item.target?`Target: ${item.target} → ${draft.target}`:''].filter(Boolean);
 function submit(event:FormEvent){event.preventDefault();void onSubmit(draft);}
 return <Dialog title={`Настройки · ${connectionLabel(item.kind)}`} onClose={onClose} busy={busy} dirty={changes.length>0}><form onSubmit={submit}>
  <div className="dialog-body [&_.field]:mb-0">
   <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_160px]">
   <Field label="Адрес подключения" value={item.address} readOnly description="Клиент подключается к этому адресу. Это не SNI; адрес общий для протоколов этого сервера."/>
   <Field label={reality?'TCP-порт':'UDP-порт'} type="number" required min={1} max={65535} disabled={busy} value={draft.port} onChange={e=>setDraft(d=>({...d,port:Number(e.target.value)}))}/>
   </div>
   {reality?<><Field label="Домен SNI" required maxLength={253} disabled={busy} value={draft.sni} onChange={e=>{const sni=e.target.value;setDraft(d=>({...d,sni,target:d.target===d.sni?sni:d.target}));}}/>
    <Advanced>
     <FingerprintField value={draft.fingerprint} disabled={busy} onChange={fingerprint=>setDraft(d=>({...d,fingerprint}))}/>
     {item.kind==='vless-xhttp'&&<Field label="Путь XHTTP" required maxLength={128} disabled={busy} value={draft.path} onChange={event=>setDraft(d=>({...d,path:event.target.value}))} description="Путь HTTP-запросов. Создаётся автоматически; после изменения обновите подписку клиента."/>}
     <Field label="Сайт назначения (target)" required maxLength={253} disabled={busy} value={draft.target} onChange={e=>setDraft(d=>({...d,target:e.target.value}))}/>
     <Button type="button" disabled={busy} onClick={()=>setPickingTarget(value=>!value)}>{pickingTarget?'Закрыть подбор':'Проверить и подобрать SNI'}</Button>
     {pickingTarget&&<TargetPicker onSelect={candidate=>{setDraft(d=>({...d,target:candidate.host,sni:candidate.sni}));setPickingTarget(false);}}/>}
     <Field label="Публичный ключ REALITY" value={item.public_key??''} readOnly description="Создан автоматически вместе с приватным ключом. Обычное сохранение не перевыпускает ключи."/>
     <Field label="Short ID" value={item.short_id??''} readOnly description="Идентификатор REALITY создан автоматически; клиент получает его в подписке."/>
    </Advanced>
   </>:item.kind==='hysteria2'&&<p className="muted">Имя TLS: {item.sni}. Оно должно соответствовать сертификату сервера.</p>}
   {changes.length>0&&<section aria-label="Изменения" className="rounded-lg border border-[var(--stroke)] p-4"><strong>Будут изменены</strong><ul className="mt-2 grid gap-1 list-disc pl-5">{changes.map(change=><li key={change} className="break-words">{change}</li>)}</ul><p className="muted mt-3">{changes.length===1&&draft.fingerprint!==(item.fingerprint??'chrome')?'Обновите подписку клиента, чтобы применить новый TLS-профиль. Ключи и пользователи сохранятся.':'Применение может кратко прервать соединения. После изменения обновите подписку или импортируйте конфигурацию заново. Ключи и пользователи сохранятся.'}</p></section>}
   {error&&<p className="error" role="alert">{error}</p>}
  </div>
  <footer className="dialog-footer"><Button type="button" disabled={busy} onClick={()=>{if(canDismiss(changes.length>0,busy))onClose();}}>Отмена</Button><Button type="submit" variant="primary" disabled={busy||!changes.length}>{busy?'Применяем…':'Применить изменения'}</Button></footer>
 </form></Dialog>;
}
