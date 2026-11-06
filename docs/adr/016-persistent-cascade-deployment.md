# ADR-016: постоянный журнал каскадного применения

2026-11-06. Продолжение [канала/helper](015-cascade-channel-and-node-operation.md). **Внутренний диагностический сценарий, не публичная активация маршрута.**

## Хранение и границы

`internal/app/cascade.go` под общим mutation lock получает сохранённый fail-closed маршрут без резервов, два существующих managed TCP/REALITY профиля, снимок пользователей и генерирует отдельный channel. Local node и конечные пакеты/device mode пока запрещены: распределённого исполнения лимитов нет. Конфиги проходят строгий helper validator до сохранения. Адреса профилей должны совпадать с node-control endpoints.

`internal/store/cascade.go` хранит журнал, четыре шага (`apply_exit`, `apply_entry`, `restore_entry`, `restore_exit`), аренду двух нод и capability observations. Channel secret и каждый config/snapshot зашифрованы AES-GCM существующим `node-control.key`; AAD содержит deployment ID, для шага — также имя шага и digest. Public DTO содержит только ID/ревизии/состояние/ноды. Control secrets/runtime journal не экспортируются в portable VPN backup; исходная SQLite+ключ нужны для восстановления журнала. Удаление SQLite/ключа не является rollback нод.

Ограничение —32 журнала; явный `DeleteCascade` допустим только для prepared/rolled_back и с CAS. Активные/неопределённые операции нельзя удалить ради освобождения нод. После rollback channel secret очищается, зашифрованные snapshots остаются до удаления журнала. Нет нового worker/таймера/зависимости; переходы выполняются при start/ACK/существующем stalled check. Это архитектурное ограничение работы, не замер производительности512МиБ.

## Последовательность

1. `PrepareCascade` атомарно проверяет версию черновика, roster digest, отсутствие устройств/конечных пакетов, acknowledged direct baselines и свежую capability обоих helper. Захватывает node leases, сохраняет секрет и обе предыдущие конфигурации. Подготовка не меняет runtime.
2. `StartCascade` с CAS повторяет roster/readiness checks, сбрасывает публикацию/probe обеих нод и ставит B в существующий encrypted node queue с kind `apply_cascade_xray_v1`.
3. Только authenticated ACK B с совпадающими operation ID/revision/digest фиксирует B и ставит A в очередь **в одной SQLite-транзакции**. Повтор ACK не создаёт новую A operation. Poll сохраняет исходный ID при повторной доставке/перезапуске.
4. ACK A переводит в `awaiting_probe`; это **не** pass/активный маршрут/публикация. Для Journal нет endpoint publish.
5. Ошибка B восстанавливает B. Ошибка A сначала восстанавливает A, затем B; compensation получает новые монотонные node revisions. Неудачная компенсация сохраняет leases и требует вмешательства.
6. Потеря ACK ограничена прежними8 доставками. Exhaustion/stalled/withdrawn capability → `attention`, без предположения, что apply не состоялся. Явный `RollbackCascade` с CAS отменяет текущие pending IDs и восстанавливает A, затем B. Поздние ACK старой операции отвергаются. Состояние/снимки переживают перезапуск БД; process/systemd crash ещё не принят.

## Совместимость и конкуренция

Read-only helper kind `capabilities_v1` сообщает cascade protocol1 без чтения/записи config или вызова systemctl. Агент получает ответ через существующий приватный socket с дедлайном2с и включает его в authenticated heartbeat. Отсутствующий/старый helper → capability0, старый protocol1 сам по себе каскад не разрешает. Freshness180с проверяется перед prepare/start/delivery; downgrade во время ожидания переводит в attention. Это self-report доверенного агента, не криптографическая аттестация root/helper и не proof VPN.

Leases блокируют сторонние deploy/retry/cancel/drain, SSH upgrade/removal/revoke, изменение node endpoint, пользовательских credentials/enable/devices/package и замену/удаление занятого черновика. Полный и legacy restore блокируются, пока журнал не удалён; после rollback его надо явно удалить. Ошибка в обычном HTTP mutation возвращает409. Это временная строгая сериализация; live распределённый reconcile пользователей/лимитов не реализован.

## Что остаётся

Нет admin HTTP prepare/start/rollback/list и UI, LogicalEndpoint/user binding, ротации/повторного использования channel, Geo/DNS, независимой revision-bound egress/UDP проверки и публикации подписок. Ручное внутреннее Start меняет весь TCP listener ноды; **не подключать его к обычной кнопке сохранения черновика**. Не применять в production до staged preview/привязок/внешних gates. Нужен новый согласованный node release/helper, A2 prerelease не обновлён. Реальные systemd/SSH/две VPS/REALITY handshake/отключение трафика/512МиБ NOT RUN. [Проверки](../testing/local-results/stage5-2026-11-06-cascade-journal.md).
