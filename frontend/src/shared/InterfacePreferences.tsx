import {createContext,useContext,useEffect,useState,type ReactNode} from 'react';
import {interfaceModeKey,parseInterfaceMode,readInterfaceMode,saveInterfaceMode,type InterfaceMode} from './interface-mode';

const InterfaceContext=createContext({mode:'simple' as InterfaceMode,persisted:true,setMode:(_mode:InterfaceMode)=>{}});
export const useInterfacePreferences=()=>useContext(InterfaceContext);
export function InterfacePreferences({children}:{children:ReactNode}){
 const [mode,setCurrent]=useState<InterfaceMode>(()=>{try{return readInterfaceMode(window.localStorage)}catch{return 'simple'}});
 const [persisted,setPersisted]=useState(true);
 function setMode(next:InterfaceMode){
  setCurrent(next);
  try{setPersisted(saveInterfaceMode(window.localStorage,next))}catch{setPersisted(false)}
 }
 useEffect(()=>{
  function sync(event:StorageEvent){
   if(event.storageArea!==window.localStorage)return;
   if(event.key===interfaceModeKey||event.key===null){setCurrent(parseInterfaceMode(event.newValue));setPersisted(true)}
  }
  window.addEventListener('storage',sync);
  return ()=>window.removeEventListener('storage',sync);
 },[]);
 return <InterfaceContext value={{mode,persisted,setMode}}>{children}</InterfaceContext>;
}
