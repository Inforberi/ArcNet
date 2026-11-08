import {type ServerOptions,type ClientOptions,type ProtocolHealth, type ProtocolProfile, type VPNConfig} from '../../shared/api';
import {Badge, Button, Empty} from '../../shared/ui';
import {protocolCatalog} from './ConnectionForms';
import {connectionAddress,connectionService,connectionTransport} from './presentation';

export type ConnectionItem = {server_options?:ServerOptions;client_options?:ClientOptions;fingerprint?:string;public_key?:string;short_id?:string;path?:string;kind:string;address:string;port:number;sni:string;target:string;enabled:boolean;revision:string};
export const connectionLabel=(kind:string)=>kind==='vless-reality'?'VLESS + Reality TCP':protocolCatalog.find(p=>p.kind===kind)?.name??kind;
export function connectionItems(vpn:VPNConfig,profiles:ProtocolProfile[]):ConnectionItem[]{
 return [...(vpn.configured?[{kind:'vless-reality',server_options:vpn.server_options,fingerprint:vpn.fingerprint,public_key:vpn.public_key,short_id:vpn.short_id,address:vpn.address!,port:vpn.port!,sni:vpn.sni??'',target:vpn.target??'',enabled:vpn.enabled??true,revision:vpn.revision??''}]:[]),...profiles.map(p=>({...p,sni:p.sni??'',target:p.target??'',revision:p.revision??''}))];
}
export function ConnectionTable({items,health,busy,onDetails,onEdit,onToggle,onRemove}:{items:ConnectionItem[];health:ProtocolHealth[];busy:boolean;onDetails:(item:ConnectionItem)=>void;onEdit:(item:ConnectionItem)=>void;onToggle:(item:ConnectionItem)=>void;onRemove:(item:ConnectionItem)=>void}) {
 if(!items.length)return <Empty><h2>Добавьте первый протокол</h2><p>Выберите нужный протокол. Адрес, порт и ключи подготовим автоматически.</p></Empty>;
 return <div className="table-wrap connections-table"><table><thead><tr><th>Протокол</th><th>Транспорт</th><th>Адрес</th><th>Состояние службы</th><th>Управление</th></tr></thead><tbody>
  {items.map(item=>{
   const state=health.find(h=>h.kind===item.kind)?.state;
   const service=connectionService(state),managed=item.kind!=='amneziawg';
   return <tr key={item.kind}>
    <td data-label="Протокол"><Button variant="ghost" className="connection-name" onClick={()=>onDetails(item)}>{connectionLabel(item.kind)}</Button><small className="connection-meta">{item.enabled?'Включено в настройках':'Выключено в настройках'}</small></td>
    <td data-label="Транспорт">{connectionTransport(item.kind)}</td>
    <td data-label="Адрес"><code>{connectionAddress(item.address,item.port)}</code></td>
    <td data-label="Состояние службы"><Badge variant={service.variant}>{service.label}</Badge></td>
    <td data-label="Управление"><div className="flex flex-wrap items-center gap-2">
     {managed?<><Button className="small" disabled={busy} onClick={()=>onEdit(item)}>Изменить</Button><Button className="small" disabled={busy} onClick={()=>onToggle(item)}>{item.enabled?'Отключить':'Включить'}</Button>{!item.enabled&&<Button className="small" variant="danger" disabled={busy} onClick={()=>onRemove(item)}>Удалить</Button>}</>:<span className="muted">Сохранённый протокол</span>}
    </div></td>
   </tr>;
  })}
 </tbody></table></div>;
}
