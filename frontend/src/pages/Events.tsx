import {useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {api,type AuditEvent} from '../shared/api';
import {Button,Empty,Field,PageHeading,Panel,Loading,Notice} from '../shared/ui';
export default function Events(){
 const [search,setSearch]=useState('');
 const query=useQuery({queryKey:['events'],queryFn:()=>api<{items:AuditEvent[]}>('/api/events')});
 const items=(query.data?.items??[]).filter(item=>`${item.action} ${item.details}`.toLocaleLowerCase('ru').includes(search.trim().toLocaleLowerCase('ru')));
 return <><PageHeading title="Журнал" description="Последние административные операции" actions={<Button disabled={query.isFetching} onClick={()=>void query.refetch()}>Обновить</Button>}/>
 {query.isPending?<Loading/>:query.isError?<Notice>{query.error.message} <Button onClick={()=>void query.refetch()}>Повторить</Button></Notice>:<Panel title="События"><div className="panel-body"><Field type="search" label="Поиск в загруженных событиях" value={search} onChange={event=>setSearch(event.target.value)} placeholder="Операция или описание"/></div>{!items.length?<Empty>{search?'Событий по этому запросу нет. Измените поиск.':'Административных событий пока нет.'}</Empty>:<div className="table-scroll"><table><thead><tr><th>Дата</th><th>Операция</th><th>Описание</th></tr></thead><tbody>{items.map(item=><tr key={item.id}><td>{new Date(Number(item.time)*1000).toLocaleString('ru-RU')}</td><td>{item.action}</td><td>{item.details}</td></tr>)}</tbody></table></div>}</Panel>}</>;
}
