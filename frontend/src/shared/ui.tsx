import type {ButtonHTMLAttributes, HTMLAttributes, InputHTMLAttributes, ReactNode} from 'react';
import {Icon,type IconName} from './Icon';
import {useEffect,useId,useState} from 'react';
import {useInterfacePreferences} from './InterfacePreferences';
export function SettingsSection({title,description,actions,children}:{title:string;description?:string;actions?:ReactNode;children:ReactNode}){
 const heading=useId();
 return <section className="settings-section" aria-labelledby={heading}><header className="settings-section-heading"><div><h3 id={heading}>{title}</h3>{description&&<p>{description}</p>}</div>{actions}</header><div className="settings-section-content">{children}</div></section>;
}
export function Properties({items}:{items:{label:string;value:ReactNode}[]}){
 return <dl className="properties-list">{items.map(item=><div key={item.label}><dt>{item.label}</dt><dd>{item.value}</dd></div>)}</dl>;
}
export function Button({variant='default',className='',...props}:ButtonHTMLAttributes<HTMLButtonElement>&{variant?:'default'|'primary'|'danger'|'ghost'}){
  return <button {...props} className={`button ${variant==='default'?'':variant} ${className}`} />;
}
export function IconButton({icon,label,className='',...props}:Omit<Parameters<typeof Button>[0],'children'>&{icon:IconName;label:string}){
 return <Button type="button" {...props} className={`icon-button ${className}`} aria-label={label} title={label}><Icon name={icon}/></Button>;
}
export function SwitchButton({checked,label,...props}:Omit<ButtonHTMLAttributes<HTMLButtonElement>,'children'|'role'|'aria-checked'>&{checked:boolean;label:string}){
 return <button type="button" {...props} role="switch" aria-checked={checked} aria-label={label} title={label} className="switch-control"><span/></button>;
}
export function StatStrip({items,label}:{label:string;items:{label:string;value:ReactNode;tone?:'green'|'amber'|'neutral'}[]}){
 return <section className="stat-strip" aria-label={label}>{items.map(item=><div key={item.label}><span>{item.label}</span><strong data-tone={item.tone}>{item.value}</strong></div>)}</section>;
}
export function SettingsLayout({navigation,children}:{navigation:ReactNode;children:ReactNode}){
 return <div className="settings-layout"><aside className="settings-menu">{navigation}</aside><div className="settings-content">{children}</div></div>;
}
export function SettingRow({title,description,children}:{title:string;description?:string;children:ReactNode}){
 const id=useId();return <div className="settings-row" role="group" aria-labelledby={id}><div><h3 id={id}>{title}</h3>{description&&<p>{description}</p>}</div><div className="settings-control">{children}</div></div>;
}
export function Panel({title,actions,children,...props}:{title:string;actions?:ReactNode;children:ReactNode}&HTMLAttributes<HTMLElement>){
  return <section className="panel" {...props}><div className="panel-title"><span>{title}</span>{actions}</div>{children}</section>;
}
export function Field({label,description,error,id:providedID,...props}:{label:string;description?:string;error?:string}&InputHTMLAttributes<HTMLInputElement>){
 const generated=useId(),id=providedID??generated;
 const described=[props['aria-describedby'],description?`${id}-description`:null,error?`${id}-error`:null].filter(Boolean).join(' ')||undefined;
 return <label className="field" htmlFor={id}><span>{label}</span><input {...props} id={id} aria-describedby={described} aria-invalid={error?true:props['aria-invalid']}/>{description&&<small id={`${id}-description`}>{description}</small>}{error&&<small id={`${id}-error`} className="error" role="alert">{error}</small>}</label>;
}
export function Badge({children,variant='blue',...props}:HTMLAttributes<HTMLSpanElement>&{variant?:'blue'|'red'|'green'|'neutral'}){return <span {...props} className={`badge ${variant} ${props.className??''}`}>{children}</span>}
export function PageHeading({title,description,actions}:{title:string;description:string;actions?:ReactNode}){return <header className="top"><div><h1>{title}</h1><p>{description}</p></div>{actions&&<div className="flex">{actions}</div>}</header>}
export function Empty({children}:{children:ReactNode}){return <div className="empty">{children}</div>}
export function Notice({children}:{children:ReactNode}){return <div className="notice">{children}</div>}
export function Loading(){return <div className="empty">Загрузка…</div>}
export function Advanced({title='Расширенные настройки',children,open}:{title?:string;children:ReactNode;open?:boolean}){
  const {mode}=useInterfacePreferences();
  const [expanded,setExpanded]=useState(open||mode==='advanced');
  useEffect(()=>{setExpanded(mode==='advanced')},[mode]);
  useEffect(()=>{if(open)setExpanded(true)},[open]);
  return <details open={expanded} className="profile-advanced" onInvalidCapture={event => {event.currentTarget.open=true;setExpanded(true)}}><summary onClick={event=>{event.preventDefault();const next=!expanded;setExpanded(next)}}><span>{title}</span><Icon name="chevron" className="advanced-chevron"/></summary><div className="profile-settings">{children}</div></details>;
}
