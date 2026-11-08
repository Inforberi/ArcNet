import type {VPNUser} from '../../shared/api';
import type {UserPackage} from './usage';

export type UserSort = 'id'|'name'|'created'|'traffic'|'expires';
export function sortUsers(users:VPNUser[],packages:Map<number,UserPackage>,sort:UserSort,descending=false) {
 const value=(u:VPNUser):number|string|null=>{
  const p=packages.get(u.id);
  switch(sort){case 'name':return u.name;case 'created':return u.created_at;case 'traffic':return p?.observed_at?p.upload_bytes+p.download_bytes:null;case 'expires':return p?(p.expires_at||Infinity):null;default:return u.id}
 };
 return [...users].sort((a,b)=>{
  const av=value(a),bv=value(b);
  if(av===null||bv===null)return av===bv?a.id-b.id:av===null?1:-1;
  const order=typeof av==='string'&&typeof bv==='string'?av.localeCompare(bv,'ru',{numeric:true}):av===bv?0:av<bv?-1:1;
  return order?(descending?-order:order):a.id-b.id;
 });
}
function csvCell(value:string|number){
 const text=String(value);
 const safe=/^[\s]*[=+\-@]/.test(text)||/^[\t\r\n]/.test(text)?`'${text}`:text;
 return `"${safe.replaceAll('"','""')}"`;
}
export function usersCSV(users:VPNUser[],packages:Map<number,UserPackage>){
 const rows:(string|number)[][]=[['ID','Имя','Группа','Доступ включён','Подключения','Получено, байт','Отправлено, байт','Пакет, байт (0 = без лимита)','Срок (UTC)','Учёт неполный']];
 for(const u of users){const p=packages.get(u.id);rows.push([u.id,u.name,u.preferences?.group??'',u.enabled?'Да':'Нет',u.preferences?.allowed_kinds==null?'Все':u.preferences.allowed_kinds.join('; '),p?.observed_at?p.download_bytes:'',p?.observed_at?p.upload_bytes:'',p?.quota_bytes??'',p?.expires_at?new Date(p.expires_at*1000).toISOString():'',p?.incomplete?'Да':'Нет'])}
 return '\uFEFF'+rows.map(row=>row.map(csvCell).join(',')).join('\r\n')+'\r\n';
}
