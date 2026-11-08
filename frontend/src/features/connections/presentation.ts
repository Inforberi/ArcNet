export const connectionTransport=(kind:string)=>kind==='vless-reality'?'TCP / RAW':kind==='vless-xhttp'?'XHTTP':kind==='hysteria2'?'QUIC / UDP':'UDP';
export const connectionProtection=(kind:string)=>kind.startsWith('vless')?'REALITY':kind==='hysteria2'?'TLS':'Ключи туннеля';
export const connectionAddress=(address:string,port:number)=>`${address.includes(':')?`[${address}]`:address}:${port}`;
export function connectionService(state:string|undefined){
 switch(state){
  case 'active':return {label:'Запущена',variant:'green' as const};
  case 'failed':return {label:'Ошибка запуска',variant:'red' as const};
  case 'activating':return {label:'Запускается',variant:'neutral' as const};
  case 'inactive':return {label:'Не запущена',variant:'neutral' as const};
  default:return {label:'Нет свежих данных',variant:'neutral' as const};
 }
}
const titles:Record<string,string>={'vless-reality':'VLESS · REALITY · TCP','vless-xhttp':'VLESS · REALITY · XHTTP',hysteria2:'Hysteria2',wireguard:'WireGuard'};
export const connectionTitle=(kind:string)=>titles[kind]??kind;
import type {ServerOptions,ClientOptions} from '../../shared/api';
export function clientOptionChanges(kind:string,before:ClientOptions={},after:ClientOptions={}){
 const fields:[string,string|number,string|number][]=kind==='wireguard'?
  [['MTU клиента',before.mtu||1280,after.mtu||1280],['DNS клиента',before.dns||'1.1.1.1',after.dns||'1.1.1.1'],['Keepalive',before.keepalive??25,after.keepalive??25]]:
  kind==='vless-xhttp'?[['Режим XHTTP',before.xhttp_mode||'auto',after.xhttp_mode||'auto']]:[];
 return fields.filter(([,old,next])=>old!==next).map(([label,old,next])=>`${label}: ${old} → ${next}`);
}

export function serverOptionChanges(kind:string,before:ServerOptions={},after:ServerOptions={}){
 const fields:[string,string|number|boolean,string|number|boolean][]=kind.startsWith('vless')?[['Sniffing',before.sniffing??(kind==='vless-reality'),after.sniffing??(kind==='vless-reality')]]:kind==='hysteria2'?[['UDP timeout',before.udp_idle_seconds||60,after.udp_idle_seconds||60],['QUIC timeout',before.quic_idle_seconds||30,after.quic_idle_seconds||30]]:[];
 if(kind==='vless-reality')fields.push(['Flow',before.flow==='none'?'без flow':'Vision',after.flow==='none'?'без flow':'Vision']);
 if(kind==='hysteria2')fields.push(['Salamander',before.salamander??false,after.salamander??false],['От сервера клиенту, Мбит/с',before.up_mbps??0,after.up_mbps??0],['От клиента серверу, Мбит/с',before.down_mbps??0,after.down_mbps??0]);
 return fields.filter(([,old,next])=>old!==next).map(([label,old,next])=>`${label}: ${typeof old==='boolean'?(old?'включён':'выключен'):old} → ${typeof next==='boolean'?(next?'включён':'выключен'):next}`);
}
