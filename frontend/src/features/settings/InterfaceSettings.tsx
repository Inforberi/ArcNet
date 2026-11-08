import {useInterfacePreferences} from '../../shared/InterfacePreferences';
import {Notice,Panel,SettingRow} from '../../shared/ui';

export default function InterfaceSettings(){
 const {mode,setMode,persisted}=useInterfacePreferences();
 return <Panel title="Интерфейс">
  <SettingRow title="Режим интерфейса" description="В простом режиме подробности свёрнуты. Расширенный режим раскрывает дополнительные параметры; подключения и права не меняются.">
   <label className="field"><span className="sr-only">Режим интерфейса</span><select value={mode} onChange={e=>setMode(e.target.value==='advanced'?'advanced':'simple')}><option value="simple">Простой</option><option value="advanced">Расширенный</option></select></label>
  </SettingRow>
  <SettingRow title="Тема" description="Цвета всех экранов управляются общей дизайн-системой."><span>Тёмная · ArcNet v7</span></SettingRow>
  <SettingRow title="Анимации" description="Учитывают системную настройку уменьшения движения."><span>По настройкам устройства</span></SettingRow>
  {!persisted&&<div className="panel-body"><Notice>Браузер не разрешил сохранить выбор. Режим действует до перезагрузки страницы.</Notice></div>}
 </Panel>;
}
