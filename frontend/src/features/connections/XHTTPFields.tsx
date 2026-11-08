import {Button,Field,SettingsSection} from '../../shared/ui';

type XHTTPDraft={path:string;host:string;mode:string};
export default function XHTTPFields({value,onChange,disabled}:{value:XHTTPDraft;onChange:(patch:Partial<XHTTPDraft>)=>void;disabled:boolean}){
 return <SettingsSection title="Транспорт XHTTP" description="Путь и Host должны совпадать на сервере и в профиле клиента. SNI настраивается отдельно.">
  <div className="field-row">
   <Field label="Путь XHTTP" required maxLength={128} disabled={disabled} value={value.path} onChange={e=>onChange({path:e.target.value})} description="Начинается с /. После изменения обновите подписку клиента."/>
   <Field label="Host HTTP — необязательно" maxLength={253} autoCapitalize="none" spellCheck={false} disabled={disabled} value={value.host} placeholder="Автоматически" onChange={e=>onChange({host:e.target.value})} description="Домен или IPv4 без https://, порта и пути. Пусто — сервер не ограничивает Host; клиент выбирает его автоматически."/>
  </div>
  <label className="field"><span>Режим XHTTP клиента</span><select disabled={disabled} value={value.mode} onChange={e=>onChange({mode:e.target.value})}><option value="auto">Автоматически — по умолчанию</option><option value="packet-up">packet-up</option><option value="stream-up">stream-up</option><option value="stream-one">stream-one</option></select><small>Передаётся в подписке. Сервер принимает режим автоматически; поддержка зависит от версии VPN-приложения.</small></label>
  <Button type="button" disabled={disabled} onClick={()=>onChange({host:'',mode:'auto'})}>Автоматические Host и режим</Button>
  <small className="muted">Сброс сохраняет путь, ключи, порт и SNI. Это не подбор параметров из сети пользователя.</small>
 </SettingsSection>;
}
