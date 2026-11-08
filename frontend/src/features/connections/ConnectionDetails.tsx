import {Link} from 'react-router';
import Dialog,{DialogClose} from '../../shared/Dialog';
import {Badge,Button,Properties} from '../../shared/ui';
import {Icon} from '../../shared/Icon';
import {connectionLabel,type ConnectionItem} from './ConnectionTable';
import {connectionAddress,connectionProtection,connectionService,connectionTransport} from './presentation';

export default function ConnectionDetails({item,state,checking,checkFailed,onCheck,onEdit,onRotate,onPickTarget,onClose}:{item:ConnectionItem;state?:string;checking:boolean;checkFailed:boolean;onCheck:()=>void;onEdit:()=>void;onRotate:()=>void;onPickTarget:()=>void;onClose:()=>void}){
 const reality=item.kind.startsWith('vless'),service=connectionService(checkFailed?undefined:state);
 const technical=[
  ...(reality?[{label:'Сайт назначения',value:item.target||'Не указан'},{label:'Fingerprint клиента',value:item.fingerprint||'chrome'},{label:'Sniffing',value:(item.server_options?.sniffing??(item.kind==='vless-reality'))?'Включён':'Выключен'}]:[]),
  ...(item.kind==='vless-reality'?[{label:'Flow',value:item.server_options?.flow==='none'?'Без flow':'xtls-rprx-vision'}]:[]),
  ...(item.kind==='vless-xhttp'?[{label:'Путь XHTTP',value:item.path||'Не получен'},{label:'Host HTTP',value:item.server_options?.xhttp_host||'Автоматически'},{label:'Режим клиента XHTTP',value:item.client_options?.xhttp_mode||'auto'}]:[]),
  ...(item.kind==='hysteria2'?[{label:'Salamander',value:item.server_options?.salamander?'Включена':'Выключена'},{label:'Получение клиентом (Brutal)',value:item.server_options?.up_mbps?`${item.server_options.up_mbps} Мбит/с`:'Автоматически'},{label:'Отправка клиентом (Brutal)',value:item.server_options?.down_mbps?`${item.server_options.down_mbps} Мбит/с`:'Автоматически'},{label:'UDP idle, секунд',value:item.server_options?.udp_idle_seconds||60},{label:'QUIC idle, секунд',value:item.server_options?.quic_idle_seconds||30}]:[]),
  ...(item.kind==='wireguard'?[{label:'MTU клиента',value:item.client_options?.mtu||1280},{label:'DNS клиента',value:item.client_options?.dns||'1.1.1.1'},{label:'Keepalive, секунд',value:item.client_options?.keepalive??25}]:[])
 ];
 return <Dialog title={connectionLabel(item.kind)} onClose={onClose} className="connection-details compact-details">
  <div className="dialog-body">
   <div className="connection-status-strip"><div><span>В настройках</span><Badge variant={item.enabled?'green':'neutral'}>{item.enabled?'Включено':'Выключено'}</Badge></div><div><span>Служба</span><Badge variant={service.variant}>{service.label}</Badge></div><Button disabled={checking} onClick={onCheck}>{checking?'Проверяем…':'Проверить службу'}</Button></div>
   {checkFailed&&<p className="error" role="alert">Состояние службы недоступно. Повторите проверку.</p>}
   <div className="compact-properties"><Properties items={[{label:'Адрес подключения',value:<code>{connectionAddress(item.address,item.port)}</code>},{label:'Транспорт / защита',value:`${connectionTransport(item.kind)} · ${connectionProtection(item.kind)}`},...(item.sni?[{label:reality?'Домен SNI':'Имя TLS',value:item.sni}]:[])]}/></div>
   {technical.length>0&&<details className="profile-advanced"><summary>Технические параметры<Icon name="chevron" className="advanced-chevron"/></summary><div className="profile-settings"><Properties items={technical}/>{reality&&<Button onClick={onPickTarget}>Подобрать SNI</Button>}</div></details>}
   {(reality||item.kind==='wireguard')&&<details className="profile-advanced"><summary>Ключи и идентификаторы<Icon name="chevron" className="advanced-chevron"/></summary><div className="profile-settings"><Properties items={[{label:'Публичный ключ',value:<code>{item.public_key||'Не получен от сервера'}</code>},...(reality?[{label:'Short ID',value:<code>{item.short_id||'Не получен от сервера'}</code>}]:[])]}/><Button onClick={onRotate}>Сменить ключи</Button></div></details>}
   <p className="muted text-xs">Служба и подключение с устройства проверяются отдельно. <Link to="/users">Выдать доступ пользователю</Link></p>
  </div>
  <footer className="dialog-footer"><DialogClose>Закрыть</DialogClose>{item.kind!=='amneziawg'&&<Button variant="primary" onClick={onEdit}>Настроить подключение</Button>}</footer>
 </Dialog>;
}
