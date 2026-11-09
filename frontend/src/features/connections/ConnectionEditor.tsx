import {useRef,useState,type FormEvent} from 'react';
import Dialog,{DialogClose} from '../../shared/Dialog';
import {Button,Field,SettingsSection,Properties} from '../../shared/ui';
import {EditorNavigation,EditorSection,revealInvalidField} from '../../shared/EditorSections';
import {connectionLabel,type ConnectionItem} from './ConnectionTable';
import TargetPicker from '../targets/TargetPicker';
import FingerprintField from './FingerprintField';
import XHTTPFields from './XHTTPFields';
import {serverOptionChanges,clientOptionChanges} from './presentation';

export type ConnectionSettings=Pick<ConnectionItem,'port'|'sni'|'target'|'enabled'|'fingerprint'|'path'|'client_options'|'server_options'>;
export default function ConnectionEditor({item,busy,error,onClose,onSubmit,initialPickingTarget=false,unknown=false}:{item:ConnectionItem;busy:boolean;error:string;onClose:()=>void;onSubmit:(settings:ConnectionSettings)=>Promise<void>;initialPickingTarget?:boolean;unknown?:boolean}){
 const reality=item.kind.startsWith('vless');
 const [draft,setDraft]=useState<ConnectionSettings>({server_options:item.server_options??{},client_options:item.client_options??{},path:item.path??'',fingerprint:item.fingerprint??(reality?'chrome':''),port:item.port,sni:item.sni,target:item.target,enabled:item.enabled});
 const [section,setSection]=useState(initialPickingTarget?'security':'main');
 const [pickingTarget,setPickingTarget]=useState(initialPickingTarget);
 const submitting=useRef(false);
 const clientChanges=clientOptionChanges(item.kind,item.client_options,draft.client_options);
 const serverChanges=serverOptionChanges(item.kind,item.server_options,draft.server_options);
 const serverChanged=serverChanges.length>0||draft.path!==(item.path??'')||draft.port!==item.port||draft.sni!==item.sni||draft.target!==item.target;
 const changes=[draft.path!==(item.path??'')?`Путь XHTTP: ${item.path} → ${draft.path}`:'',draft.fingerprint!==(item.fingerprint??(reality?'chrome':''))?`TLS fingerprint: ${item.fingerprint||'chrome'} → ${draft.fingerprint}`:'',draft.port!==item.port?`Порт: ${item.port} → ${draft.port}`:'',draft.sni!==item.sni?`SNI: ${item.sni} → ${draft.sni}`:'',draft.target!==item.target?`Target: ${item.target} → ${draft.target}`:'',...clientChanges,...serverChanges].filter(Boolean);
 const sections=[{id:'main',label:'Основное'},{id:'transport',label:item.kind==='wireguard'?'Клиентский файл':'Транспорт'},...(reality?[{id:'security',label:'Защита и SNI'}]:[]),{id:'keys',label:reality?'Ключи':'Сведения'}];
 async function submit(event:FormEvent){event.preventDefault();if(busy||unknown||submitting.current)return;submitting.current=true;try{await onSubmit(draft)}finally{submitting.current=false}}
 return <Dialog title={`Изменить подключение · ${connectionLabel(item.kind)}`} className="connection-editor sectioned-editor" onClose={onClose} busy={busy} dirty={changes.length>0}>
  <form onSubmit={submit} onInvalidCapture={event=>revealInvalidField(event,setSection)}>
   <EditorNavigation items={sections} value={section} onChange={setSection} label="Разделы подключения"/>
   <div className="editor-scroll dialog-body">
    <EditorSection id="main" active={section} title="Основное">
     <Properties items={[{label:'Адрес сервера',value:<code>{item.address}</code>},{label:'Подключение',value:connectionLabel(item.kind)}]}/>
     <Field label={reality?'TCP-порт':'UDP-порт'} type="number" required min={1} max={65535} disabled={busy} value={draft.port} onChange={e=>setDraft(d=>({...d,port:Number(e.target.value)}))}/>
    </EditorSection>
    <EditorSection id="transport" active={section} title={item.kind==='wireguard'?'Клиентский файл':'Транспорт'}>
     {reality?<>
      {item.kind==='vless-xhttp'?<XHTTPFields value={{path:draft.path??'',host:draft.server_options?.xhttp_host??'',mode:draft.client_options?.xhttp_mode||'auto'}} disabled={busy} onChange={patch=>setDraft(d=>({...d,path:patch.path??d.path,server_options:{...d.server_options,...(patch.host!==undefined?{xhttp_host:patch.host}:{})},client_options:{...d.client_options,...(patch.mode!==undefined?{xhttp_mode:patch.mode}:{})}}))}/>:<label className="field"><span>Flow VLESS TCP</span><select disabled={busy} value={draft.server_options?.flow==='none'?'none':''} onChange={e=>setDraft(d=>({...d,server_options:{...d.server_options,flow:e.target.value}}))}><option value="">XTLS Vision — по умолчанию</option><option value="none">Без XTLS flow</option></select><small>Изменится на сервере и в подписках назначенных пользователей. REALITY сохраняется.</small></label>}
      <label className="connection-choice"><input type="checkbox" disabled={busy} checked={draft.server_options?.sniffing??(item.kind==='vless-reality')} onChange={e=>setDraft(d=>({...d,server_options:{...d.server_options,sniffing:e.target.checked}}))}/><span>Определять домен для маршрутизации<small>Sniffing HTTP/TLS. Адрес назначения сохраняется.</small></span></label>
     </>:item.kind==='hysteria2'?<>
      <SettingsSection title="Маскировка и скорость">
       <label className="connection-choice"><input type="checkbox" disabled={busy} checked={draft.server_options?.salamander??false} onChange={e=>setDraft(d=>({...d,server_options:{...d.server_options,salamander:e.target.checked}}))}/><span>Маскировка Salamander<small>Секрет создаётся автоматически. После применения обновите подписку.</small></span></label>
       <div className="field-row"><Field label="Brutal: клиент получает, Мбит/с" type="number" min={0} max={100000} step={1} value={draft.server_options?.up_mbps??0} disabled={busy} onChange={e=>setDraft(d=>({...d,server_options:{...d.server_options,up_mbps:Number(e.target.value)}}))}/><Field label="Brutal: клиент отправляет, Мбит/с" type="number" min={0} max={100000} step={1} value={draft.server_options?.down_mbps??0} disabled={busy} onChange={e=>setDraft(d=>({...d,server_options:{...d.server_options,down_mbps:Number(e.target.value)}}))}/></div>
       <small className="muted">0 — не задавать. Ограничение соединения при согласованном Brutal; не ограничивает BBR/Reno и не заменяет пакет трафика.</small>
      </SettingsSection>
      <SettingsSection title="Таймауты сервера"><div className="field-row"><Field label="UDP без активности, секунд" type="number" required min={2} max={600} step={1} disabled={busy} value={draft.server_options?.udp_idle_seconds??60} onChange={e=>setDraft(d=>({...d,server_options:{...d.server_options,udp_idle_seconds:Number(e.target.value)}}))}/><Field label="QUIC без активности, секунд" type="number" required min={4} max={120} step={1} disabled={busy} value={draft.server_options?.quic_idle_seconds??30} onChange={e=>setDraft(d=>({...d,server_options:{...d.server_options,quic_idle_seconds:Number(e.target.value)}}))}/></div><Button type="button" disabled={busy} onClick={()=>setDraft(d=>{const options={...d.server_options};delete options.udp_idle_seconds;delete options.quic_idle_seconds;return {...d,server_options:options}})}>Стандартные таймауты</Button></SettingsSection>
     </>:<SettingsSection title="WireGuard" description="Изменится скачиваемый .conf. После сохранения импортируйте файл заново.">
      <div className="field-row"><Field label="MTU клиента" type="number" min={1280} max={1500} step={1} disabled={busy} value={draft.client_options?.mtu??1280} onChange={e=>setDraft(d=>({...d,client_options:{...d.client_options,mtu:Number(e.target.value)}}))}/><Field label="Keepalive, секунд" type="number" min={0} max={65535} step={1} disabled={busy} value={draft.client_options?.keepalive??25} onChange={e=>setDraft(d=>({...d,client_options:{...d.client_options,keepalive:Number(e.target.value)}}))} description="0 — выключить; по умолчанию 25."/></div>
      <Field label="DNS клиента" maxLength={200} disabled={busy} value={draft.client_options?.dns??'1.1.1.1'} onChange={e=>setDraft(d=>({...d,client_options:{...d.client_options,dns:e.target.value}}))} description="До четырёх IPv4-адресов через запятую. По умолчанию 1.1.1.1."/>
      <Button type="button" disabled={busy} onClick={()=>setDraft(d=>({...d,client_options:{}}))}>Восстановить значения по умолчанию</Button>
     </SettingsSection>}
    </EditorSection>
    {reality&&<EditorSection id="security" active={section} title="Защита и SNI">
     <Field label="Домен SNI" required maxLength={253} disabled={busy} value={draft.sni} onChange={e=>{const sni=e.target.value;setDraft(d=>({...d,sni,target:d.target===d.sni?sni:d.target}))}}/>
     <Field label="Сайт назначения (target)" required maxLength={253} disabled={busy} value={draft.target} onChange={e=>setDraft(d=>({...d,target:e.target.value}))}/>
     <FingerprintField value={draft.fingerprint} disabled={busy} onChange={fingerprint=>setDraft(d=>({...d,fingerprint}))}/>
     <Button type="button" disabled={busy} aria-expanded={pickingTarget} onClick={()=>setPickingTarget(value=>!value)}>{pickingTarget?'Скрыть подбор SNI':'Проверить и подобрать SNI'}</Button>
     {pickingTarget&&<SettingsSection title="Подбор SNI" description="Выбор изменит SNI и target в черновике. Ключи, порт и fingerprint сохранятся."><TargetPicker onSelect={candidate=>{setDraft(d=>({...d,target:candidate.host,sni:candidate.sni}));setPickingTarget(false)}}/></SettingsSection>}
    </EditorSection>}
    <EditorSection id="keys" active={section} title={reality?'Ключи и идентификаторы':'Сведения о защите'}>
     {reality?<><Properties items={[{label:'Публичный ключ REALITY',value:<code>{item.public_key||'Не получен'}</code>},{label:'Short ID',value:<code>{item.short_id||'Не получен'}</code>},{label:'ALPN',value:'Определяется REALITY и fingerprint клиента'}]}/><p className="muted">Сохранение настроек не перевыпускает ключи.</p></>:item.kind==='hysteria2'?<><Properties items={[{label:'Имя сертификата',value:item.sni},{label:'Проверка TLS',value:'Обязательна'},{label:'Транспорт',value:'QUIC / UDP'}]}/><p className="muted">Сертификат управляется сервером. Для смены имени нужен сертификат на новый адрес.</p></>:<p className="muted">Ключи туннеля и интерфейс сервера сохраняются. Меняются только параметры клиентского файла.</p>}
    </EditorSection>
    {changes.length>0&&<section className="editor-changes" aria-label="Изменения"><strong>Будут изменены</strong><ul>{changes.map(change=><li key={change}>{change}</li>)}</ul><p className="muted">{serverChanged?'Применение может прервать соединения. Обновите подписку или импортируйте конфигурацию заново.':'Изменится клиентский профиль без перезапуска службы. Обновите подписку или импортируйте файл заново.'} Ключи и пользователи сохранятся.</p></section>}
   </div>
   {error&&<div className="editor-operation-error"><p className="error" role="alert">{error}</p>{unknown&&<p className="muted">Закройте редактор и обновите список, чтобы сверить результат. Повторное применение из этого окна заблокировано.</p>}</div>}
   <footer className="dialog-footer"><DialogClose disabled={busy}>Отмена</DialogClose><Button type="submit" variant="primary" pending={busy} disabled={unknown||!changes.length}>Применить изменения</Button></footer>
  </form>
 </Dialog>;
}
