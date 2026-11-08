import {Link} from 'react-router';
import Dialog,{DialogClose} from '../../shared/Dialog';
import {Badge,Button,Properties,SettingsSection} from '../../shared/ui';
import {connectionLabel,type ConnectionItem} from './ConnectionTable';
import {connectionAddress,connectionProtection,connectionService,connectionTransport} from './presentation';

export default function ConnectionDetails({item,state,checking,checkFailed,onCheck,onEdit,onRotate,onPickTarget,onClose}:{item:ConnectionItem;state?:string;checking:boolean;checkFailed:boolean;onCheck:()=>void;onEdit:()=>void;onRotate:()=>void;onPickTarget:()=>void;onClose:()=>void}){
 const reality=item.kind.startsWith('vless');
 const service=connectionService(checkFailed?undefined:state);
 const properties=[...(item.kind==='vless-reality'?[{label:'Flow',value:item.server_options?.flow==='none'?'Без flow':'xtls-rprx-vision'}]:[]),{label:'Публичный адрес',value:<code>{connectionAddress(item.address,item.port)}</code>},{label:'Транспорт',value:connectionTransport(item.kind)},{label:'Защита',value:connectionProtection(item.kind)},...(item.sni?[{label:reality?'Домен SNI':'Имя TLS',value:item.sni}]:[]),...(reality?[{label:'Сайт назначения',value:item.target||'Не указан'},{label:'Fingerprint клиента',value:item.fingerprint||'chrome'}]:[]),...(item.kind==='vless-xhttp'?[{label:'Путь XHTTP',value:item.path||'Не получен'}]:[])];
 return <Dialog title={connectionLabel(item.kind)} onClose={onClose} className="connection-details">
  <div className="dialog-body">
   <div className="connection-status-strip"><div><span>В настройках</span><Badge variant={item.enabled?'green':'neutral'}>{item.enabled?'Включено':'Выключено'}</Badge></div><div><span>Служба на сервере</span><Badge variant={service.variant}>{service.label}</Badge></div><Button disabled={checking} onClick={onCheck}>{checking?'Проверяем…':'Проверить службу'}</Button></div>
   {checkFailed&&<p className="error" role="alert">Не удалось получить состояние службы. Повторите проверку.</p>}
   <Properties items={properties}/>
   {reality&&<Properties items={[{label:'Sniffing для маршрутизации',value:(item.server_options?.sniffing??(item.kind==='vless-reality'))?'Включён':'Выключен'}]}/>}
   {item.kind==='hysteria2'&&<SettingsSection title="Параметры Hysteria2" description="Полоса задаётся для каждого соединения с Brutal; BBR и Reno ею не ограничиваются."><Properties items={[{label:'Маскировка Salamander',value:item.server_options?.salamander?'Включена':'Выключена'},{label:'Получение клиентом',value:item.server_options?.up_mbps?`${item.server_options.up_mbps} Мбит/с`:'Автоматически'},{label:'Отправка клиентом',value:item.server_options?.down_mbps?`${item.server_options.down_mbps} Мбит/с`:'Автоматически'},{label:'UDP без активности, секунд',value:item.server_options?.udp_idle_seconds||60},{label:'QUIC без активности, секунд',value:item.server_options?.quic_idle_seconds||30}]}/></SettingsSection>}
   {item.kind==='vless-xhttp'&&<Properties items={[{label:'Режим клиента XHTTP',value:item.client_options?.xhttp_mode||'auto'}]}/>}
   {item.kind==='wireguard'&&<SettingsSection title="Клиентский файл"><Properties items={[{label:'Публичный ключ сервера',value:<code>{item.public_key||'Не получен от сервера'}</code>},{label:'MTU клиента',value:item.client_options?.mtu||1280},{label:'DNS клиента',value:item.client_options?.dns||'1.1.1.1'},{label:'Keepalive, секунд',value:item.client_options?.keepalive??25}]}/></SettingsSection>}
   {reality&&<SettingsSection title="Параметры REALITY" description="Эти публичные параметры передаются клиенту в подписке."><Properties items={[{label:'Публичный ключ',value:<code>{item.public_key||'Не получен от сервера'}</code>},{label:'Short ID',value:<code>{item.short_id||'Не получен от сервера'}</code>}]}/><Button onClick={onPickTarget}>Подобрать SNI</Button></SettingsSection>}
   <p className="muted">Запущенная служба не подтверждает соединение с вашего устройства. Проверяйте доступ в VPN-приложении.</p>
   <Link className="button inline-action" to="/users">Выдать доступ пользователю</Link>
  </div>
  <footer className="dialog-footer"><DialogClose>Закрыть</DialogClose>{(reality||item.kind==='wireguard')&&<Button onClick={onRotate}>Сменить ключи</Button>}{item.kind!=='amneziawg'&&<Button variant="primary" onClick={onEdit}>Настроить подключение</Button>}</footer>
 </Dialog>;
}
