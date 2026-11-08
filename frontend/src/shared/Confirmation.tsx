import {createContext,useCallback,useContext,useEffect,useRef,useState,type ReactNode} from 'react';
import {useLocation} from 'react-router';
import Dialog,{DialogClose} from './Dialog';
import {Field} from './ui';

type Options={title?:string;action?:string;phrase?:string};
type Request={message:string;options:Options};
type Confirm=(message:string,options?:Options)=>Promise<boolean>;
const Context=createContext<Confirm|null>(null);
export function useOptionalConfirmation(){return useContext(Context)}
export function useConfirmation(){const confirm=useContext(Context);if(!confirm)throw new Error('ConfirmationProvider missing');return confirm;}
export function ConfirmationProvider({children}:{children:ReactNode}){
 const [request,setRequest]=useState<Request|null>(null),[phrase,setPhrase]=useState('');
 const accepted=useRef(false);
 const resolver=useRef<((accepted:boolean)=>void)|null>(null);
 const {pathname}=useLocation();
 const finish=useCallback((accepted:boolean)=>{const resolve=resolver.current;resolver.current=null;setRequest(null);setPhrase('');resolve?.(accepted)},[]);
 const confirm=useCallback<Confirm>((message,options={})=>{
  if(resolver.current)return Promise.resolve(false);
  return new Promise(resolve=>{resolver.current=resolve;accepted.current=false;setPhrase('');setRequest({message,options})});
 },[]);
 useEffect(()=>{finish(false)},[pathname,finish]);
 useEffect(()=>()=>{resolver.current?.(false);resolver.current=null},[]);
 return <Context value={confirm}>{children}{request&&<Dialog title={request.options.title??'Подтвердите действие'} className="confirmation-dialog" onClose={()=>finish(accepted.current)} dirty={false}>
  <div className="dialog-body"><p className="confirmation-message">{request.message}</p>{request.options.phrase&&<Field label={`Введите: ${request.options.phrase}`} value={phrase} onChange={event=>setPhrase(event.target.value)} autoComplete="off" spellCheck={false}/>}</div>
  <footer className="dialog-footer"><DialogClose>Отмена</DialogClose><DialogClose variant="danger" disabled={!!request.options.phrase&&phrase!==request.options.phrase} onClick={()=>{accepted.current=true}}>{request.options.action??'Подтвердить'}</DialogClose></footer>
 </Dialog>}</Context>;
}
