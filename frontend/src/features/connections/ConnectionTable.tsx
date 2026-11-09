import {type ServerOptions,type ClientOptions,type ProtocolHealth, type ProtocolProfile, type VPNConfig} from '../../shared/api';
import {Badge, Button, Empty,IconButton,SwitchButton} from '../../shared/ui';
import {Icon} from '../../shared/Icon';
import {protocolCatalog} from './ConnectionForms';
import {connectionAddress,connectionService,connectionTransport,connectionProtection} from './presentation';

export const connectionKey=(item:{id?:string;kind:string})=>item.id||item.kind;
export type ConnectionItem = {id?:string;server_options?:ServerOptions;client_options?:ClientOptions;fingerprint?:string;public_key?:string;short_id?:string;path?:string;kind:string;address:string;port:number;sni:string;target:string;enabled:boolean;revision:string};
export const connectionLabel=(kind:string)=>kind==='vless-reality'?'VLESS · REALITY · TCP':protocolCatalog.find(p=>p.kind===kind)?.name??kind;
export function connectionItems(vpn:VPNConfig,profiles:ProtocolProfile[]):ConnectionItem[]{
 return [...(vpn.configured?[{kind:'vless-reality',server_options:vpn.server_options,fingerprint:vpn.fingerprint,public_key:vpn.public_key,short_id:vpn.short_id,address:vpn.address!,port:vpn.port!,sni:vpn.sni??'',target:vpn.target??'',enabled:vpn.enabled??true,revision:vpn.revision??''}]:[]),...profiles.map(p=>({...p,sni:p.sni??'',target:p.target??'',revision:p.revision??''}))];
}
export function ConnectionTable({items,health,assignments,busy,onDetails,onEdit,onRefresh,onToggle,onRemove}:{items:ConnectionItem[];health:ProtocolHealth[];assignments?:ReadonlyMap<string,number>;busy:boolean;onDetails:(item:ConnectionItem)=>void;onEdit:(item:ConnectionItem)=>void;onRefresh:(item:ConnectionItem)=>void;onToggle:(item:ConnectionItem)=>void;onRemove:(item:ConnectionItem)=>void}) {
 if(!items.length)return <Empty><h2>Добавьте первое подключение</h2><p>Выберите протокол. Адрес, порт и ключи подготовим автоматически.</p></Empty>;
 return <div className="table-wrap connections-table"><table><thead><tr><th>Протокол</th><th>Сервер / порт</th><th>TLS и маскировка</th><th>Назначения</th><th>Состояние</th><th>Действия</th></tr></thead><tbody>
  {items.map(item=>{
   const key=connectionKey(item),state=health.find(h=>h.kind===key)?.state;
   const service=connectionService(state),managed=item.kind!=='amneziawg';
   return <tr key={key} onClick={()=>onDetails(item)}>
    <td data-label="Протокол"><div className="connection-identity"><span className="connection-symbol"><Icon name={item.kind==='hysteria2'?'wave':item.kind==='wireguard'?'shield':'protocols'}/></span><div><Button variant="ghost" className="connection-name" onClick={event=>{event.stopPropagation();onDetails(item)}}>{connectionLabel(item.kind)}</Button><small>{connectionTransport(item.kind)}</small></div></div></td>
    <td data-label="Сервер / порт"><code>{connectionAddress(item.address,item.port)}</code></td>
    <td data-label="TLS и маскировка"><strong>{connectionProtection(item.kind)}</strong>{item.sni&&<small>SNI: {item.sni}</small>}</td>
    <td data-label="Назначения"><strong>{assignments?`${assignments.get(key)??0} пользователей`:'Нет данных'}</strong></td>
    <td data-label="Состояние"><Badge variant={service.variant}>{state==='unverified'||!state?'Не проверено':service.label}</Badge><small>{item.enabled?'Включено в настройках':'Отключено'}</small></td>
    <td data-label="Действия" onClick={event=>event.stopPropagation()}><div className="row-actions">
     {managed&&item.kind.startsWith('vless')&&<IconButton icon="refresh" label={`Подобрать SNI · ${item.port}`} disabled={busy} onClick={()=>onRefresh(item)}/>}
     {managed&&<IconButton icon="edit" label={`Настроить подключение · ${item.port}`} disabled={busy} onClick={()=>onEdit(item)}/>}
     {managed&&<SwitchButton checked={item.enabled} label={`${item.enabled?'Отключить':'Включить'} подключение · ${item.port}`} disabled={busy} onClick={()=>onToggle(item)}/>}
     {managed&&!item.enabled&&<IconButton variant="danger" icon="trash" label={`Удалить подключение · ${item.port}`} disabled={busy} onClick={()=>onRemove(item)}/>}
    </div></td>
   </tr>;
  })}
 </tbody></table></div>;
}
