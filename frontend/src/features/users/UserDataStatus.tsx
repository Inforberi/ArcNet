import {Icon} from '../../shared/Icon';

export default function UserDataStatus({failed,incomplete}:{failed:boolean;incomplete:boolean}){
 return <details className="users-data-status"><summary><Icon name="info"/><span>{failed?'Статистика недоступна':incomplete?'Не все подключения учтены':'О статистике'}</span><Icon name="chevron"/></summary><div>
 {failed?<p>Не удалось получить свежие показатели. Проверьте состояние VPN-служб и повторите позже.</p>:<p>Скорость и расход появляются после получения счётчиков от VPN-служб. Прочерк означает, что данных пока нет.</p>}
 {incomplete&&<p>Некоторые подключения ещё не входят в учёт. Показанный расход может быть неполным.</p>}
 <p>«Доступ разрешён» означает, что настройки позволяют подключаться. Это не подтверждение, что пользователь сейчас в сети.</p>
 </div></details>
}
