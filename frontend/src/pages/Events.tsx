import{useQuery}from'@tanstack/react-query';
import{api,type AuditEvent}from'../shared/api';
import{PageHeading,Panel,Loading,Notice}from'../shared/ui';
export default function Events(){const query=useQuery({queryKey:['events'],queryFn:()=>api<{items:AuditEvent[]}>('/api/events'),refetchInterval:30_000});return <><PageHeading title="Журнал" description="Последние административные операции"/>{query.isPending?<Loading/>:query.isError?<Notice>{query.error.message}</Notice>:<Panel title="События"><div className="table-scroll"><table><thead><tr><th>Дата</th><th>Операция</th><th>Описание</th></tr></thead><tbody>{query.data.items.map(x=><tr key={x.id}><td>{new Date(Number(x.time)*1000).toLocaleString('ru-RU')}</td><td>{x.action}</td><td>{x.details}</td></tr>)}</tbody></table></div></Panel>}</>}
