import type {ButtonHTMLAttributes, HTMLAttributes, InputHTMLAttributes, ReactNode} from 'react';
import {Icon} from './Icon';
import {useId} from 'react';
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
export function Panel({title,actions,children,...props}:{title:string;actions?:ReactNode;children:ReactNode}&HTMLAttributes<HTMLElement>){
  return <section className="panel" {...props}><div className="panel-title"><span>{title}</span>{actions}</div>{children}</section>;
}
export function Field({label,description,...props}:{label:string;description?:string}&InputHTMLAttributes<HTMLInputElement>){
  return <label className="field"><span>{label}</span><input {...props}/>{description&&<small>{description}</small>}</label>;
}
export function Badge({children,variant='blue',...props}:HTMLAttributes<HTMLSpanElement>&{variant?:'blue'|'red'|'green'|'neutral'}){return <span {...props} className={`badge ${variant} ${props.className??''}`}>{children}</span>}
export function PageHeading({title,description,actions}:{title:string;description:string;actions?:ReactNode}){return <header className="top"><div><h1>{title}</h1><p>{description}</p></div>{actions&&<div className="flex">{actions}</div>}</header>}
export function Empty({children}:{children:ReactNode}){return <div className="empty">{children}</div>}
export function Notice({children}:{children:ReactNode}){return <div className="notice">{children}</div>}
export function Loading(){return <div className="empty">Загрузка…</div>}
export function Advanced({title='Расширенные настройки',children,open}:{title?:string;children:ReactNode;open?:boolean}){
  return <details open={open} className="profile-advanced" onInvalidCapture={event => {event.currentTarget.open = true;}}><summary><span>{title}</span><Icon name="chevron" className="advanced-chevron"/></summary><div className="profile-settings">{children}</div></details>;
}
