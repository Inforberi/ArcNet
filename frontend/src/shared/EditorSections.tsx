import {useId, type ReactNode} from 'react';
import {Button} from './ui';

// Keep section contents mounted: changing the visible group never resets a draft.
export function EditorNavigation({items,value,onChange,label}:{items:readonly {id:string;label:string}[];value:string;onChange:(id:string)=>void;label:string}){
 return <nav className="editor-section-nav" aria-label={label}>{items.map(item=><Button type="button" key={item.id} aria-pressed={value===item.id} onClick={()=>onChange(item.id)}>{item.label}</Button>)}</nav>;
}
export function EditorSection({id,active,title,children}:{id:string;active:string;title:string;children:ReactNode}){
 const heading=useId();
 return <section data-editor-section={id} hidden={active!==id} aria-labelledby={heading}><h3 id={heading} className="editor-section-title">{title}</h3>{children}</section>;
}
export function revealInvalidField(event:React.FormEvent<HTMLFormElement>,select:(section:string)=>void){
 const input=event.target as HTMLInputElement;
 const group=input.closest<HTMLElement>('[data-editor-section]');
 if(!group)return;
 event.preventDefault();
 select(group.dataset.editorSection!);
 // Native validation visits every invalid input. Focus only the first one.
 if(event.currentTarget.querySelector('input:invalid,select:invalid,textarea:invalid')!==input)return;
 input.setAttribute('aria-invalid','true');
 requestAnimationFrame(()=>input.focus());
}
