# Дашборд сервера панели и подключённых нод

Актуальное продолжение8 ноября: [пакетный polling и пользовательская телеметрия](user-traffic.md). История решения: [подписка и интервал5 секунд](../adr/007-live-monitoring-and-simple-connection-forms.md).

Дата: 2026-11-05. Реализовано в исходниках; [ADR-006](../adr/006-bounded-dashboard-and-local-recovery.md), [локальные результаты](../testing/local-results/stage5-2026-11-05-dashboard-recovery.md). Приёмка реальных VPS/VPN отсутствует.

## Что показывает UI

- Выбор сервера панели или зарегистрированной ноды. CPU, RAM, swap, диск, load average, uptime, TCP/UDP sockets и текущая скорость сети ОС; временные метки и null вместо выдуманного нуля.
- Сводка серверов с поиском, фильтром требующих внимания и раскрытием по 20 карточек. Карточки на мобильных, без обязательного горизонтального скролла. Включённые пользователи — число конфигураций, не online VPN-клиенты.
- Раздельные состояния: агент/панель, Xray service, последняя внешняя VPN-проба, маршрут/egress. Heartbeat и `active` не доказывают VPN. Offline нода не получает текущий service/CPU статус из старого отчёта. Последняя VPN-проба показывается с возрастом; route пока всегда «Не проверен».
- История сети до двух часов, последний снимок каждой минуты. SVG с клавиатурным выбором и разрывами при неизвестных значениях, смене epoch и длительном пропуске. Данные не генерируются для заполнения графика.

## Источники и ограничения

`internal/metrics/sampler.go`: bounded `/proc/stat`, meminfo, uptime/loadavg, net/dev, маршруты по умолчанию, sockstat/6 и Statfs файловой системы DB/конфигурации агента. CPU — non-idle по двум снимкам первых восьми counters, guest не учитывается повторно, iowait включён в idle. Первый процент/дельта отсутствуют. Сеть учитывает выбранные default-route интерфейсы, исключая loopback/типовые tunnel/veth/bridge; смена состава и reset counters не создают spike. На сложной policy routing/нестандартных интерфейсах возможен unknown, не обещается accounting всех физических каналов.

Это host/namespace metrics: контейнерный `/proc` может отражать VM, а не memory/CPU budget контейнера. Не показаны выдуманные active VPN users, per-user traffic, quota/expiry или независимо проверенная subscription availability. Показатели с ноды являются утверждениями аутентифицированного агента, не remote attestation.

## API и границы

- GET `/api/overview` — совместимые общие поля плюс `servers` и `sampled_at`; количество пользователей через SQL aggregate без чтения credentials. `system` теперь содержит nullable реальные показатели, а не placeholder CPU/disk.
- GET `/api/servers/{id}/telemetry?history=0|1` — выбранный сервер, snapshot, состояния и опциональная история. `local` — панель. По умолчанию история включена; текущий UI исключает её из частого snapshot polling. Требуется админ-сессия; `Cache-Control: no-store`. GET не сохраняет наблюдения в DB.
- POST `/api/nodes/telemetry` — optional wire v1 `{version,node_id,snapshot}` с bearer. Проверяются identity, размер/метаданные/диапазоны/часы ±300 секунд и rate limit не чаще 5 секунд; `local` запрещён. Optional header `X-ArcVPN-Telemetry: 2` возвращает JSON с `interval_seconds`; без header сохраняется ответ 204. К status/probe/ACK/publish доступа нет. Контроллер пересчитывает remote rate из counters/epoch/interfaces, первый отчёт не создаёт скорость.
- GET/PUT `/api/settings/monitoring` — общая сохраняемая частота, PUT требует CSRF; GET `/api/monitoring/events` — same-origin SSE с сессией, refresh/keepalive/unauthorized, без credentials в URL.
- Старый heartbeat wire не меняется. Новый агент терпит 404 старого контроллера; старый агент остаётся с частичными memory/load/uptime данными, CPU/disk/network не заполняются нулями. Ошибка systemctl даёт unknown, а не доказанный stopped.

## Экономные настройки

Контрольный профиль пользователя — **512 МиБ / 1 vCPU**. Это не заявленный minimum полного VPN.

- Один lifecycle worker на контроллер: по сохраняемой настройке 5/10/15/30/60 секунд (по умолчанию 5), cache 5 секунд, без collector на вкладку; systemctl-state кешируется на 5 секунд. Resource report нового агента — отдельный цикл по настройке контроллера, не блокируется apply/target checks. Heartbeat отдельно с прежней базой 20 секунд и jitter/backoff. Старые агенты не получают новый cadence автоматически без обновления бинарника.
- Resource freshness 180 секунд, heartbeat freshness 75 секунд. CPU/network требуют двух подходящих снимков. Никаких target/VPN-проб на каждое открытие дашборда.
- Один record/minute/node, 120 минут, глобальный cap 32768 строк; rate-limited writes, purge при новом bucket. История не копируется в VPN-state backup и удаляется при замене/отзыве identity. В metrics audit/log не пишется каждое измерение.
- UI snapshot/fleet — общий polling без SSE по решению8 ноября (по умолчанию5с, без наложения циклов); история раз в минуту. Скрытая вкладка не опрашивает данные. Только выбранный сервер получает history; до 20 карточек за одно раскрытие. Без новой chart library, Redis, Prometheus или Node.js на VPS.
- Ошибка записи истории не превращает известные текущие показатели в нули; флаг доступности истории выводится отдельно. Worker завершается до закрытия DB.

Полная долговременная VPN-нагрузка, performance per-user accounting, cgroup-aware dashboard, реальная fleet и bare-VPS systemd/network acceptance ещё не подтверждены. Не считать локальный UI/container benchmark выполнением этих gates.

Первичные источники, проверены 2026-11-05: [Linux procfs](https://docs.kernel.org/filesystems/proc.html), [Statfs/Bavail](https://man7.org/linux/man-pages/man2/statfs.2.html).
