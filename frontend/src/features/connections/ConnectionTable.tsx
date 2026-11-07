import {Link} from 'react-router';
import {type ProtocolHealth, type ProtocolProfile, type VPNConfig} from '../../shared/api';
import {Badge, Button, Empty} from '../../shared/ui';
import {protocolCatalog} from './ConnectionForms';

export function ConnectionTable({vpn,profiles,health,busy,onToggle,onRemove}:{vpn:VPNConfig;profiles:ProtocolProfile[];health:ProtocolHealth[];busy:boolean;onToggle:(profile:ProtocolProfile)=>void;onRemove:(profile:ProtocolProfile)=>void}) {
  const items=[...(vpn.configured?[{kind:'vless-reality',address:vpn.address,port:vpn.port,sni:vpn.sni,enabled:true,profile:null}]:[]),...profiles.map(profile=>({...profile,profile}))];
  if(!items.length)return <Empty><h2>Добавьте первое подключение</h2><p>Выберите протокол. Адрес, порт и ключи подготовим автоматически.</p></Empty>;
  return <div className="table-wrap connections-table"><table><thead><tr><th>Подключение</th><th>Адрес</th><th>Состояние службы</th><th>Действия</th></tr></thead><tbody>
    {items.map(item=>{
      const state=health.find(h=>h.kind===item.kind)?.state;
      const active=state==='active';
      const label=item.kind==='vless-reality'?'VLESS + Reality TCP':protocolCatalog.find(p=>p.kind===item.kind)?.name??item.kind;
      return <tr key={item.kind}>
        <td data-label="Подключение"><strong>{label}</strong>{item.sni&&<small className="connection-meta">SNI: {item.sni}</small>}</td>
        <td data-label="Адрес"><code>{item.address}:{item.port}</code><small className="connection-meta">{item.kind.startsWith('vless')?'TCP':'UDP'}</small></td>
        <td data-label="Состояние службы"><Badge variant={!item.enabled?'blue':active?'green':state==='failed'?'red':'blue'}>{!item.enabled?'Отключено':active?'Запущена':state==='failed'?'Ошибка запуска':state==='activating'?'Запускается':state==='inactive'?'Не запущена':state==='unverified'?'Нет проверки':state==='unknown'?'Недоступно':'Проверяется'}</Badge></td>
        <td data-label="Действия"><div className="flex wrap">
          {item.enabled&&<Link className="button small" to="/users">Пользователи</Link>}
          {item.profile&&<Button className="small" disabled={busy} onClick={()=>onToggle(item.profile!)}>{item.enabled?'Отключить':'Включить'}</Button>}
          {item.profile&&!item.enabled&&<Button className="small" variant="danger" disabled={busy} onClick={()=>onRemove(item.profile!)}>Удалить</Button>}
          <details className="connection-details"><summary>Подробнее</summary><p>Состояние службы не подтверждает соединение с устройства. QR и подписка доступны в разделе «Пользователи» → «Подключить».</p></details>
        </div></td>
      </tr>;
    })}
  </tbody></table></div>;
}
