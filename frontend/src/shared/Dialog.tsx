import {useOptionalConfirmation} from './Confirmation';
import {createContext,useContext,useEffect,useRef,useState,type ReactNode} from 'react';
import * as Primitive from '@radix-ui/react-dialog';
import {Button} from './ui';
import {Icon} from './Icon';
export {canDismiss} from './dialog-dismiss';
const DismissContext=createContext<(()=>void)|null>(null);

export function DialogClose({children,disabled=false,variant,onClick}:{children:ReactNode;disabled?:boolean;variant?:'default'|'primary'|'danger'|'ghost';onClick?:()=>void}){
 const dismiss=useContext(DismissContext);
 return <Button type="button" disabled={disabled} variant={variant} onClick={()=>{onClick?.();dismiss?.()}}>{children}</Button>;
}
export default function Dialog({title,children,onClose,busy=false,dirty,className=''}:{title:string;children:ReactNode;onClose:()=>void;busy?:boolean;dirty?:boolean;className?:string}){
 const confirm=useOptionalConfirmation();
 const [open,setOpen]=useState(true),[edited,setEdited]=useState(false);
 const closing=useRef(false),timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
 const onClosed=useRef(onClose);onClosed.current=onClose;
 const opener=useRef<Element|null>(document.activeElement);
 const finish=()=>{if(!closing.current)return;closing.current=false;clearTimeout(timer.current);onClosed.current()};
 const dismiss=async()=>{
  if(closing.current||busy)return;
  if(dirty??edited){if(!confirm||!await confirm('Есть несохранённые изменения. Закрыть без сохранения?',{title:'Несохранённые изменения',action:'Закрыть без сохранения'}))return;}
  if(closing.current)return;
  closing.current=true;setOpen(false);
  const reduced=window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  timer.current=setTimeout(finish,reduced?0:220);
 };
 useEffect(()=>()=>clearTimeout(timer.current),[]);
 return <DismissContext.Provider value={dismiss}><Primitive.Root open={open} onOpenChange={next=>{if(!next)dismiss()}}>
  <Primitive.Portal><Primitive.Overlay className="dialog-backdrop"/>
   <Primitive.Content className={`app-dialog radix-dialog ${className}`} aria-describedby={undefined}
    onCloseAutoFocus={event=>{event.preventDefault();const target=opener.current instanceof HTMLElement&&opener.current.isConnected?opener.current:document.querySelector<HTMLElement>('main');target?.focus({preventScroll:true})}}
    onAnimationEnd={event=>{if(event.target===event.currentTarget&&!open)finish()}}
    onInputCapture={event=>{if((event.target as HTMLElement).closest('form'))setEdited(true)}}>
    <header className="dialog-heading"><Primitive.Title asChild><h2>{title}</h2></Primitive.Title><Button type="button" variant="ghost" aria-label={`Закрыть: ${title}`} disabled={busy||!open} onClick={dismiss}><Icon name="close"/></Button></header>
    {children}
   </Primitive.Content>
  </Primitive.Portal>
 </Primitive.Root></DismissContext.Provider>;
}
