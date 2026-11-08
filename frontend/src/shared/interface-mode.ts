export type InterfaceMode='simple'|'advanced';
export const interfaceModeKey='arcnet-interface-mode';
export function parseInterfaceMode(value:string|null):InterfaceMode{return value==='advanced'?'advanced':'simple'}
export function readInterfaceMode(storage:Pick<Storage,'getItem'>):InterfaceMode{
 try{return parseInterfaceMode(storage.getItem(interfaceModeKey))}catch{return 'simple'}
}
export function saveInterfaceMode(storage:Pick<Storage,'setItem'>,mode:InterfaceMode):boolean{
 try{storage.setItem(interfaceModeKey,mode);return true}catch{return false}
}
