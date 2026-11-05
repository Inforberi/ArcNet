# Предварительная модель данных

**Это логическая модель, а не готовая SQL-схема.** Хранить и изменять независимо:

| Entity | Идентичность / инварианты |
|---|---|
| `Administrator` | отдельная административная identity, не VPN User |
| `Session` | серверная сессия с revocation и MFA state |
| `VPNUser` | стабильный UUID, статус, квота, срок, политика; не равен email |
| `DeviceCredential` | отдельные ключи/credential; физическое устройство достоверно не доказано |
| `UserGroup` | явные назначения профилей и политик |
| `Node` | физическая машина, address, agent identity, состояние, версия |
| `LogicalEndpoint` | «вход RU», «выход NL», роль может переехать на новый VPS |
| `ConnectionProfile` | логическая настройка протокола и транспорта |
| `InboundDeployment` | применение профиля на конкретной Node, порт и конфигурация |
| `OutboundDeployment` | связь между нодами или обычный egress |
| `RoutePolicy` | прямой маршрут/каскад/резерв, fail-open/closed |
| `Subscription` | неизменный ID пользователя, токен с ротацией, формат/состав |
| `SubscriptionPublication` | опубликованная ревизия, URL, last-good snapshot |
| `ConfigurationRevision` | desired/applying/active/failed, checksum, actor |
| `DeploymentOperation` | idempotency key, state, step, retries, error |
| `TrafficAggregate` | период, пользователь, нода, направления, последовательность |
| `HealthObservation` | источник проверки, временная метка, результат |
| `AuditEvent` | actor, action, resource, old/new non-secret summary |

## Связи

- `VPNUser` ↔ `ConnectionProfile` через явные назначения; один пользователь может быть доступен на многих нодах.
- `Node` ↔ `LogicalEndpoint`: логическая точка переносима, физический IP — нет.
- `RoutePolicy` ссылается на LogicalEndpoint, а не на неподвижный IP.
- `Subscription` ссылается на разрешённые LogicalEndpoint и опубликованные рабочие конфигурации.
- `ConfigurationRevision` не удаляется при создании следующей, пока нужна для отката и аудита.

## Жизненный цикл пользователя

`active` / `disabled` / `expired` / `quota_exhausted`; автоматическое продление и обнуление квоты — отдельные события с понятными временными границами. В UI показывать, почему клиент недоступен. Не путать административное выключение с истечением срока.

## Учет трафика

Нода собирает ограниченные локальные счётчики, передаёт инкрементальные отчёты с sequence/idempotency. Центральная система дедуплицирует полученные данные. При потере связи отчёты могут запаздывать; предел накопления, потеря данных и политика локального quota enforcement должны быть определены до релиза.
