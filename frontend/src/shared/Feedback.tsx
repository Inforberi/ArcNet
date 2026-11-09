import {createContext,useCallback,useContext,useEffect,useId,useRef,useState,type ReactNode} from 'react';
import {Button} from './ui';

const Context=createContext<(message:string)=>void>(()=>{});
export const useFeedback=()=>useContext(Context);
type Viewport={message:string;last:string|undefined;register:(id:string)=>()=>void;dismiss:()=>void;pause:(value:boolean)=>void};
const ViewportContext=createContext<Viewport|null>(null);
function Message({value,modal=false}:{value:Viewport;modal?:boolean}){
 return <div className={`feedback-region ${modal?'modal-feedback-region':''}`} aria-live="polite" aria-atomic="true">{value.message&&<div className="feedback" onPointerEnter={()=>value.pause(true)} onPointerLeave={()=>value.pause(false)} onFocus={()=>value.pause(true)} onBlur={e=>{if(!e.currentTarget.contains(e.relatedTarget))value.pause(false)}}><span>{value.message}</span><Button type="button" variant="ghost" aria-label="Закрыть уведомление" onClick={value.dismiss}>×</Button></div>}</div>;
}
export function ModalFeedback(){
 const value=useContext(ViewportContext),id=useId(),register=value?.register;
 useEffect(()=>register?.(id),[register,id]);
 return value?.last===id?<Message value={value} modal/>:null;
}

// Transient, noncritical results only. Errors belong to the affected form.
export function FeedbackProvider({children}:{children:ReactNode}){
 const [message,setMessage]=useState(''),[paused,setPaused]=useState(false);
 const [viewports,setViewports]=useState<string[]>([]);
 const register=useCallback((id:string)=>{setViewports(ids=>[...ids,id]);return ()=>setViewports(ids=>ids.filter(value=>value!==id))},[]);
 const timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
 const show=useCallback((text:string)=>{clearTimeout(timer.current);setMessage(text)},[]);
 useEffect(()=>{if(!message||paused)return;timer.current=setTimeout(()=>setMessage(''),4500);return ()=>clearTimeout(timer.current)},[message,paused]);
 const value={message,last:viewports.at(-1),register,dismiss:()=>{setMessage('');setPaused(false)},pause:setPaused};
 return <Context value={show}><ViewportContext value={value}>{children}{!viewports.length&&<Message value={value}/>}</ViewportContext></Context>;
}
