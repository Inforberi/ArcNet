import {Button,Field} from '../../shared/ui';
import type {UserSort} from './user-table';

export type UserFilters={search:string;status:string;group:string;sort:UserSort;descending:boolean};
const sorting:[UserSort,boolean,string][]=[['created',true,'Сначала новые'],['created',false,'Сначала старые'],['name',false,'Имя: А–Я'],['name',true,'Имя: Я–А'],['traffic',true,'Больше трафика'],['traffic',false,'Меньше трафика'],['expires',false,'Срок: сначала ближайший'],['expires',true,'Срок: сначала дальний'],['id',false,'ID: по возрастанию'],['id',true,'ID: по убыванию']];
export default function UserToolbar({value,groups,onChange}:{value:UserFilters;groups:string[];onChange:(next:UserFilters)=>void}){
 const update=(part:Partial<UserFilters>)=>onChange({...value,...part});
 const filtered=!!value.search.trim()||value.status!=='all'||value.group!=='all';
 return <div className="users-toolbar">
  <Field label="Поиск" type="search" placeholder="Имя, подписка, группа, протокол…" value={value.search} onChange={e=>update({search:e.target.value})}/>
  <label className="field"><span>Доступ</span><select value={value.status} onChange={e=>update({status:e.target.value})}><option value="all">Все пользователи</option><option value="enabled">Доступ разрешён</option><option value="disabled">Отключены вручную</option><option value="blocked">Есть ограничения</option></select></label>
  {(groups.some(Boolean)||value.group!=='all')&&<label className="field"><span>Группа</span><select value={value.group} onChange={e=>update({group:e.target.value})}><option value="all">Все группы</option>{value.group!=='all'&&!groups.includes(value.group.slice(2))&&<option value={value.group} disabled>{value.group.slice(2)} · нет пользователей</option>}{groups.map(g=><option key={g} value={`g:${g}`}>{g||'Без группы'}</option>)}</select></label>}
  <label className="field"><span>Порядок</span><select value={`${value.sort}:${value.descending?'desc':'asc'}`} onChange={e=>{const [sort,direction]=e.target.value.split(':');update({sort:sort as UserSort,descending:direction==='desc'})}}>{sorting.map(([sort,descending,label])=><option key={`${sort}:${descending}`} value={`${sort}:${descending?'desc':'asc'}`}>{label}</option>)}</select></label>
  {filtered&&<Button variant="ghost" className="users-clear-filters" onClick={()=>update({search:'',status:'all',group:'all'})}>Сбросить фильтры</Button>}
 </div>
}
