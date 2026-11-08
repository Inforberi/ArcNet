import {Button} from '../../shared/ui';
import {connectionTitle} from '../connections/presentation';
import type {ProtocolProfile} from '../../shared/api';
export const connectionChoiceLabels=(profiles:ProtocolProfile[]=[])=>Object.fromEntries(profiles.map(p=>[p.id||p.kind,`${connectionTitle(p.kind)} · ${p.port}`]));

export default function ConnectionChoiceList({options,value,onChange,disabled=false,labels={}}:{options:string[];value:string[]|null;onChange:(value:string[])=>void;disabled?:boolean;labels?:Record<string,string>}){
 const selected=value??options;
 const count=options.filter(kind=>selected.includes(kind)).length;
 return <div className="connection-choice-list">
  <div className="section-heading"><small className="muted">Выбрано: {count} из {options.length}</small><div className="flex wrap"><Button type="button" className="small" disabled={disabled||!options.length} onClick={()=>onChange([...options])}>Выбрать текущие</Button><Button type="button" className="small" disabled={disabled||!selected.length} onClick={()=>onChange([])}>Снять выбор</Button></div></div>
  <div className="connection-choices">{options.map(kind=><label key={kind} className={`connection-choice ${selected.includes(kind)?'selected':''}`}><input type="checkbox" disabled={disabled} checked={selected.includes(kind)} onChange={e=>onChange(e.target.checked?[...selected,kind]:selected.filter(value=>value!==kind))}/><span>{labels[kind]||connectionTitle(kind)}</span></label>)}</div>
  {!options.length&&<p className="muted">Сначала добавьте подключение на сервере.</p>}
 </div>;
}
