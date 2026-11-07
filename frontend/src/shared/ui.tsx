import type {ButtonHTMLAttributes, HTMLAttributes, InputHTMLAttributes, ReactNode} from 'react';
import {Icon} from './Icon';
export function Button({variant='default',className='',...props}:ButtonHTMLAttributes<HTMLButtonElement>&{variant?:'default'|'primary'|'danger'|'ghost'}){
  return <button {...props} className={`button ${variant==='default'?'':variant} ${className}`} />;
}
export function Panel({title,actions,children,...props}:{title:string;actions?:ReactNode;children:ReactNode}&HTMLAttributes<HTMLElement>){
  return <section className="panel" {...props}><div className="panel-title"><span>{title}</span>{actions}</div>{children}</section>;
}
export function Field({label,description,...props}:{label:string;description?:string}&InputHTMLAttributes<HTMLInputElement>){
  return <label className="field"><span>{label}</span><input {...props}/>{description&&<small>{description}</small>}</label>;
}
export function Badge({children,variant='blue'}:{children:ReactNode;variant?:'blue'|'red'|'green'}){return <span className={`badge ${variant}`}>{children}</span>}
export function PageHeading({title,description,actions}:{title:string;description:string;actions?:ReactNode}){return <header className="top"><div><h1>{title}</h1><p>{description}</p></div>{actions&&<div className="flex">{actions}</div>}</header>}
export function Empty({children}:{children:ReactNode}){return <div className="empty">{children}</div>}
export function Notice({children}:{children:ReactNode}){return <div className="notice">{children}</div>}
export function Loading(){return <div className="empty">Загрузка…</div>}
export function Advanced({title='Расширенные настройки',children}:{title?:string;children:ReactNode}){
  return <details className="profile-advanced" onInvalidCapture={event => {event.currentTarget.open = true;}}><summary><span>{title}</span><Icon name="chevron" className="advanced-chevron"/></summary><div className="profile-settings">{children}</div></details>;
}
