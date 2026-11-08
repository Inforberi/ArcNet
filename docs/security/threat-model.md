## Онлайн-обновления — 2026-11-07

Доверие — pinned Ed25519 signer; signed catalog с отдельным префиксом, fixed repository URLs и publicnet SSRF guard. Браузер не передаёт URL/команды/ключ. Root повторяет подпись ZIP/каталога, platform/min-version/hash/size и запрет downgrade. Подготовка и установка сериализованы; проверка фактической версии/monotonic start не позволяет выдать предыдущий результат за успех. Ограничены размер, concurrency и время загрузки. Publisher key compromise остаётся риском; полный rollback БД/units не заявляется. [Контракт](../features/online-updates.md).

## Редактирование локальных протоколов — 2026-11-07

Новые PUT/DELETE требуют действующую сессию/CSRF и актуальную opaque revision под application mutation lock; приватные ключи/TLS paths/произвольные команды не принимаются из формы. Секреты сохраняются, revision вычисляется по публичной проекции профиля. Validated edit сохраняет тип/address identity и проверяет port/transport/certificate constraints, paused runtime проверяется до записи и перед apply. Ошибка применения компенсируется восстановлением сохранённых параметров/runtime; ошибку компенсации нельзя скрывать. Применение может кратко прервать соединения общим apply pipeline. Удаление только disabled, с сохранением последней local identity пользователей и legacy dependencies; удаление ключей явно подтверждается. Live DB/runtime могут кратко различаться во время existing apply pipeline — атомарная публикация всем читателям отдельной операцией здесь не заявлена. Обязательна отдельная проверка реального runtime/rollback на изолированной VM перед приёмкой. SSH/UFW/units и схема БД этим блоком не меняются.

## Первый владелец — дополнение 2026-11-07

Публичная панель без администратора не даёт открытую регистрацию: случайный код 24 байта создаётся локальным CLI, действует 15 минут, хранится хешем и атомарно гасится с созданием единственного администратора. Повторная инициализация существующего владельца запрещена. Origin, bounded body/input, throttle и общий лимит password KDF сохраняются. Код не переносится portable backup; оператор не публикует терминал/журнал с кодом. Первый вход после claim отдельный. MFA challenge не создаёт сессию; смена credentials требует действующую CSRF-сессию/текущий пароль, сохраняет активную MFA и отзывает сессии. Это не заменяет TLS, firewall и независимую VPN-приёмку.

# Модель угроз: первая итерация

**Статус:** проектная модель угроз. Требует пересмотра перед реализацией SSH-установщика, инфраструктурного API, публикации подписок и перед релизом. Не является независимым аудитом.

## Защищаемые активы

- SSH-credentials и возможность изменения хостовой ОС.
- Административные сессии, TOTP, recovery codes, API tokens.
- Приватные ключи Reality, TLS, сертификаты mTLS узлов, master key SQLite.
- Учётные данные VPN-пользователей и приватные URL подписок.
- Целостность Xray-конфигураций, firewall и маршрутов.
- База пользователей, ограничения трафика, история и аудит.
- Доступность работающего туннеля и административного SSH после обновлений.

## Границы доверия

Дополнение A2: установочный tar.gz содержит privileged scripts, поэтому подписывается весь архив через отдельный `arcvpn-installer-v1` manifest (Ed25519, platform/version/name/size/SHA256/source-manifest hash). Signature material имеет prefix `ArcVPN installer manifest v1\n`, отличающий его от core-only updater. Ключ подписи остаётся вне Git/build context/VPS, verify использует независимо доверенный публичный ключ и bounded/no-follow reads. Соседний public key из недоверенной загрузки не устанавливает доверие. Signature подтверждает происхождение байтов, а не работоспособность ОС/VPN или безопасность произвольного будущего in-place migration. A3 bootstrap и safe extraction ещё не реализованы; сетевой root payload не исполняется этим блоком.

Дополнение A1 (2026-11-06): root CLI управляет только фиксированным набором локальных units. Перед lifecycle/update требуется общий root-owned lock `/run/arcvpn-lifecycle/lock`; файл не удаляется вместе с unit RuntimeDirectory. Root marker `paused` блокирует повторный apply/restart из worker/helper/ACME hook. План включённых движков читается без secrets отдельным процессом пользователя arcvpn, не root-подключением к изменяемой SQLite; принимаются только типизированные флаги. Остановка панели с проверкой завершения предшествует final counters и остановке helpers/движков. Ошибка/неизвестное systemd-состояние не считается успешной остановкой. Ошибка start оставляет marker/остановленный runtime, конфигурации/учёт не сбрасываются; частичное восстановление явно отражается в ошибке. Настоящие systemd/kill/rollback на изолированном тестовом VPS ещё должны быть проверены; fixtures не являются приёмкой. SSH/UFW, удалённые ноды и другие units вне allowlist не меняются.

1. Браузер ↔ Go API: HTTPS, Cookie + CSRF, server-side permissions; браузер не является trusted authority.
2. Панель ↔ новый VPS по SSH: host key verification, временный доступ, строго проверенный bootstrap.
3. Панель ↔ уже зарегистрированная нода: **реализовано в исходниках 0.6.1** исходящее HTTPS с проверкой server TLS + индивидуальным bearer token ноды, scoped typed commands, ротация/отзыв, защита ACK/revision. **mTLS пока не реализован**; он остаётся вариантом усиления. Потеря token при компрометации ноды допускает имитацию её состояния до отзыва — это остаточный риск.
4. Локальный Go ↔ Xray API: только локальный endpoint, без публичного доступа.
5. Go ↔ privileged helper: allowlist/typed commands, audited permissions, no arbitrary shell.
6. Подписка ↔ клиент: публично доступный сервер, приватный токен, форматы без административных секретов.
7. Сервер ↔ интернет/ACME/updates: потенциально недоверенные сети; TLS/checksums/signatures.

## Наиболее критичные злоупотребления

Дополнение локальной итерации 2026-11-05: [ADR-005](../adr/005-traffic-profiles-and-target-diagnostics.md). Target-пробы и HTTPS feed — новая граница исходящего трафика. Контроли: сессия/CSRF, ограниченные задания без root/shell, DNS всех адресов и dial конкретного публичного IP, TLS verification, запрет redirect/proxy, лимиты времени/размера/очереди; независимый pinned signer, expiry и anti-replay feed. Узел может лгать о результатах: ни его отчёт, ни TLS target не повышают `vpn_probe` и не разрешают publish. Профиль правил — неисполняемое намерение; его импорт не активирует direct или Geo/runtime. Клиентский failover требует отдельной проверки сети устройства и не исполняется браузером панели.

| Угроза | Обязательный контроль |
|---|---|
| Угон админ-сессии | TLS, MFA, Secure/HttpOnly cookies, session revoke, CSRF, reauth для опасных действий |
| SSRF через SSH wizard | Endpoint/IP scope, DNS re-resolution checks, блокировка metadata/local без policy, аудит |
| MITM при первом SSH | Host-key fingerprint из независимого канала; не использовать автоматическое silent trust |
| Привилегированное исполнение на ноде | Нет shell RPC, allowlisted helper, systemd isolation, scoped mTLS identity |
| Компрометация одной ноды | Отдельные node keys, ротация, отзыв, ограничение cross-node доступа |
| Config injection/JSON error | Строгие DTO, config validation, Xray test, last-good, no raw shell |
| Firewall lockout | Предварительно открыть альтернативу, staged policy, rollback-timer на VPS, probe SSH |
| Кража подписки | Длинный непредсказуемый токен, revoke/rotate, no token logs, limit |
| Supply chain | Pinned versions, подписи/хеши, CI attestations, controlled update |
| Отключение панели | Локальные last-good конфиги, независимая работа Xray; внешние health monitors, backup |
| Ошибочная статистика | Пометки stale/unknown, monotonic counters с epoch/restart, лог reconciliation |
| Утечка клиентского трафика | Явная fail-policy, end-to-end проверки egress; отсутствие обещаний за пределами server control |

## Не решает продукт автоматически

- Атаку на гипервизор/VPS-провайдера и хостовый root.
- Кражу данных с уже скомпрометированного клиента.
- Блокировку всех доступных входных IP и точек подписки.
- Абсолютно надёжное ограничение физических устройств с помощью IP/HWID.
- Бесшовное сохранение TCP сессий при гибели хоста.

## Необходимые security gates до публичного выпуска

1. Threat modeling review на границах SSH, agent RPC, credentials и подписок.
2. Интеграционные негативные тесты из `docs/testing/strategy.md`.
3. Ручная проверка redaction и authorization для всех административных операций.
4. Отдельный тест восстановления VPS после частичного rollback.
5. Проверка внешнего управления firewall и условия ACME IP.
6. Независимая проверка безопасности (в идеале внешний аудит) перед заявлениями о превосходстве над 3x-ui.


## Дополнение по SSH lifecycle 0.6.1 — угрозы и границы реализации

Это **кодовые ограничения, не аудит**. Удаление/upgrade VPS — опасные операции даже для подписанного бинарника. Они доступны только администратору с CSRF, требуют pinned SSH Ed25519 host key и независимого ручного сравнения его fingerprint, публичного IP, оригинального идентификатора установки и root-owned marker на VPS. Политика `detach|purge` не передаёт произвольные пути и не выполняется через постоянного агента. Операции в рамках одного node_id сериализованы в persistent SQLite journal; restart контроллера завершает незаконченный job как `failed`, а не `success`.

**Остаточные угрозы**: компрометация панели/админ-сессии/SSH ключа; скачанный неподходящий но подписанный Xray; потеря питания/SSH между заменой файлов и commit journal; зависимость от SSH signer trust и OOB fingerprint; скомпрометированный root на VPS; TCB systemd, apt и произвольный shell установщика, выполняемый **только во время явно подтверждённого SSH bootstrap**; утечка ключа из RAM/crash dump; backend использует TLS+bearer, не mTLS. `sh -n`, Go unit и проверка криптографической подписи не являются доказательством безопасности всей последовательности на реальном Linux.

**Обязательная проверка**: с отдельно подготовленным snapshot/console recovery на VM изолированно смоделировать потерю SSH/контроллера при `install`, `enroll`, `upgrade`, `detach` и `purge`; проверить права `/etc/arcvpn-node`, системные группы и собственность каталогов, отсутствие утечек токена и отказ при чужих файлах. Публикацию VPN endpoint нельзя разрешать на основании heartbeat/systemd без независимого клиентского handshake и подтверждения сетевых правил. Сверяться с `docs/testing/stage4-codex-handoff.md` и `docs/release-0.6.1-stage4-continuation.md`.

### Дополнение 0.6.2: подмена SSH stage и ложные подтверждения

Модель угроз включает root-owned конечные файлы, полученные из SSH user-writable staging: проверка подписи только на контроллере не предотвращает TOCTOU-подмену staged binary на VPS перед запуском. Install/upgrade теперь сверяют SHA-256 **в root-owned конечной локации до исполнения**; в качестве ожидаемых хешей используются байты уже верифицированного Ed25519 release. При неоднозначной потере SSH-ответа запрещён оптимистический `success`: применяется read-only state reconciliation по marker, digest, systemd и managed paths, иначе состояние требует расследования. Нельзя считать HTTPS `/healthz` с VPS внешним VPN-probe. Административный drain не отзывает уже сохранённые VPN credentials.

## 2026-11-05: локальное recovery и resource telemetry

[ADR-006](../adr/006-bounded-dashboard-and-local-recovery.md): recovery только через локальную service identity, root-wrapper drops privileges до DB; активная MFA сохраняется по умолчанию, pending enrollment отменяется, отдельный reset-2fa явный. Пароль/отзыв сессий/MFA/audit атомарны. Настройки проецируются без hashes/keys/tokens; process argv не исполняется и не печатается целиком. Wrapper не принимает произвольные systemd units. Password KDF одновременно выполняется максимум один на Server, без уменьшения стоимости хеша; это ограничитель, не полная защита от DoS.

Telemetry — bounded bearer v1 от конкретной ноды, clock/range/rate checks, ограниченная retention. Authenticated node может лгать о ресурсах: эти данные не VPN proof/ACK и не влияют на публикацию. DB identity restore/отзыв убирает старые наблюдения. Нет сетевых target-проб на каждый dashboard GET, GET не пишет observations, private API no-store; worker один и ограничен lifecycle. Полная VM/VPS приёмка остаётся внешним gate.


## Локальный учёт Xray — 2026-11-05

[ADR-008](../adr/008-local-xray-traffic-packages.md): StatsService на private Unix socket, directory 0700 / socket owner effective UID / запрет group-other permissions. Нет TCP API, HandlerService или HTTP ingest клиентского расхода. CLI fixed argv, bounded stdout/time, reset=false; identity/config/epoch сверяются до и после чтения. Ошибки и неполный учёт видны; unexpected restart/stale source закрывают конечные пакеты через reconcile. Компрометированный UID arcvpn/root может менять SQLite/config/counters и остаётся вне гарантии аттестации. При недоступном helper старый VPN runtime может продолжаться — apply_failed не называется блокировкой. Пакеты других движков/нод запрещены до полноценного источника и исполнения.
## Device credentials — 2026-11-05

Device ID — отдельный отзываемый credential, не аппаратная аттестация. Скопированные ключи не различаются. Переход отключает общий ключ; mutations session+CSRF+transaction CAS, token export отдельно от списочного API. Публичные `/device/{token}` ответы (включая ошибки) no-store; panic log содержит route pattern/type, не сырой URL/секрет. Audit не содержит ключей. Старый backup не отменяет местные отзывы; unmanaged UDP/remote paths блокируют режим. При неудачном apply прежний трафик может продолжаться; требуется VPS disconnect-проверка. [ADR-009](../adr/009-device-credentials.md).
## Hysteria statistics — 2026-11-05

Private loopback API `127.0.0.1:19823` требует случайный машинный secret. До HTTP Authorization проверяются systemd MainPID/InvocationID/start, UID и принадлежность LISTEN socket inode этому процессу; повтор после connect, затем сверка process/config после ответа. Proxy, redirect и clear counters запрещены. Private files, timeout/body bounds, per-source freshness и existing counter/restore safeguards сохраняются. Никаких новых root-команд или firewall изменений. [ADR-010](../adr/010-hysteria-traffic-and-devices.md).
## WireGuard helper statistics

Только фиксированный read-only RPC `stats wireguard`, никакого интерфейса/argv/path от браузера. Root helper читает только public-key/transfer, never dump/private-key. Root-owned epoch receipt привязан к boot/interface/config SHA; peer set/server key/revision и эпоха проверяются. При ошибке receipt после up выполняется bounded down. Socket parent root-owned, без group/other write; private peer keys не возвращаются stats API и не попадают в audit. Новый путь требует отдельного isolated VM rollback/systemd/kernel теста; source checks не являются его приёмкой. [ADR-011](../adr/011-wireguard-traffic-and-devices.md).


## Каскадный канал/helper — 2026-11-06

[ADR-015](../adr/015-cascade-channel-and-node-operation.md): отдельные machine credentials и listener; новая fixed-shape операция без shell/paths/API, no direct fallback на A, default block, duplicate-key rejection. Скомпрометированные controller/node UID/root остаются доверенной границей, receipt/active не аттестуют egress. Политика AsIs/CIDR блокирует буквальные private destinations, но не гарантирует запрет domain→private или отсутствие DNS leaks: это открытый gate перед публикацией. Новая операция не ставится в controller queue; старые агенты не считаются совместимыми по одному protocol1. Firewall/OS не меняются. Нужны persistent encrypted credentials/rotation, LogicalEndpoint bindings, CAS многоузловых revisions, rollback/reconcile, независимая egress/DNS/UDP приёмка и отдельный publish.


## Постоянный журнал каскада — 2026-11-06

[ADR-016](../adr/016-persistent-cascade-deployment.md): AES-GCM/AAD per deployment/step/digest, monotonic compensation revisions, transaction ACK→next-stage, node leases и guards на users/routes/endpoint/deploy/SSH lifecycle/restore. ACK loss не доказывает отсутствие apply; attention сохраняет резервации и last-good snapshots. Восстановление A предшествует удалению канала B. Capability read-only/2s deadline и freshness180s не являются root attestation: агент/root доверены, публикация остаётся запрещена. Runtime journal отсутствует в portable backup; нужен согласованный SQLite+node-control.key/OS snapshot. Admin UI/start API ещё нет, internal method не должен становиться обычным Save. Внешние process/systemd/VPS crash/egress/DNS gates открыты.


## Cascade proof/publication 2026-11-06

[ADR-017](../adr/017-cascade-external-proof-and-publication.md): self-report trusted runner, hash-only one-time grants, оба node revision/digest, expected exit IP, fixed HTTPS/DoH tunnel without local DNS/dial fallback, bounded file/process/body. Отчёт не root/location attestation, DoH не OS leak-test. Отдельный CAS publication, чтение проверяет current evidence/expiry/states; старые выданные credentials не исчезают при metadata TTL. Root fixed finalRules блокируют private IP после resolution, произвольные overrides не добавлены. Новые gates требуют node release и внешней приёмки.

## Прямой remote учёт — 2026-11-06

[ADR-019](../adr/019-direct-node-traffic-accounting.md): отдельная capability и typed apply, строго StatsService на фиксированном Unix socket владельца arcvpn-xray/0700/UMask0077. Агент не получает config/private keys через statistics RPC. Report привязан к индивидуальному bearer, реально applied operation/revision/digest, roster/owner из защищённых server records; чужие IDs не назначают расходы. Mutation lock плюс одна SQLite transaction защищают restore/delete/reset и атомарные суммы. Counter.Source и portable v12 препятствуют столкновению локальных/удалённых checkpoints и потере high-water. Root/Xray/агент доверены; это не attestation. Restart-tail помечается incomplete/review. Finite quota gates сохраняются до автономных budgets/подтверждённого stop при offline. Предоставленный VPS содержит 3x-ui: сохранять чужие units/порты/данные; source tests не доказывают совместное развёртывание или rollback ОС.

## Клиентские exports — 2026-11-08
Typed client_options не содержит shell/произвольных путей. Kind allowlist, bounded integers, DNS IPv4-only и запрет CR/LF защищают .conf от инъекции. Изменение exports не меняет server networking; active IPv6/утечки на клиенте требуют отдельной приёмки. Settings mutation атомарна с профилем и CAS, combined server apply компенсирует metadata при отказе. Поля передаются только через authenticated CSRF API; subscription выдаётся существующим bearer token. Backup16 явно несовместим со старым reader. Это source-level изменение, не сетевой PASS.

## Серверные options — 2026-11-08
API принимает только nullable sniffing и ограниченные целые таймауты, с allowlist по kind. Нет shell, пути сертификата/приватного ключа, raw JSON движка или произвольного proxy URL. Existing CSRF/session/revision/paused/port/dependency gates сохранены; engine validation и rollback возвращают metadata/runtime. Sniffing использует только HTTP/TLS и routeOnly, не добавляет access logs. Root helper contract/сетевые правила не менялись. Истинная VM/runtime rollback приёмка остаётся обязательной, source fixtures её не заменяют. Backup17 требует нового reader; downgrade только бинарника не обещает сохранение overrides.

## Смена ключей и публикация — 2026-11-08

Отдельные authenticated/CSRF endpoints разрешают только REALITY TCP/XHTTP/WireGuard; preview не выдаёт credentials. CAS digest включает ревизию и полный отсортированный состав назначенных пользователей. Генерация использует доверенный Xray либо Go crypto, не вводимые браузером private keys. Salamander32-byte secret сохраняется в приватном ObfsKey; публичный JSON его скрывает, bearer URI и encrypted backup18 содержат для работы клиента. Bounded flow/полоса проверяются по kind; произвольный masquerade URL не разрешён.

App mutation lock сериализует edit/rotation; atomic publication epoch и буфер до4МиБ не дают выдать промежуточный профиль, если эти операции пересеклись с чтением. Ответ отбрасывается с503/Retry-After1; streaming не оборачивается. Это не универсальный guard всех старых mutations и не ОС-транзакция. Apply failure запускает существующий rollback; отказ самого rollback остаётся ошибкой восстановления. VM/runtime gates открыты. Тесты публикации выполнены с race detector.
## XHTTP Host — 2026-11-08

Typed server_options.xhttp_host разрешён только XHTTP. Bounded ASCII domain labels запрещают CR/LF, URL/path/port и произвольные заголовки. Поле используется только как ожидаемый/передаваемый HTTP Host; не меняет адрес исходящего target, resolver, listen, firewall или root-helper. Existing session/CSRF/revision/apply/rollback/publication gates сохранены. Backup20 защищает от потери поля при импорте под старой версией. Реальная совместимость клиента/Xray пока не проверена; source change не доказывает handshake.
## Идентификаторы дополнительных VLESS — 2026-11-08

Тип и ключ профиля разделены в DTO, legacy aliases сохранены без переписывания существующих ключей. Новый ID — только allowlisted VLESS type и UUIDv4, создаётся сервером. Settings/FK/credentials/revision/назначения/export работают по одному ключу; создание ограничено32 protocol records. Произвольный process/unit name из ID не строится: все VLESS используют прежнюю фиксированную службу Xray, health нормализует тип через allowlist.

Назначения проверяют существование ID, runtime фильтрует клиентов каждого inbound; удаление disabled extra profile очищает его назначения/direct preference одной транзакцией. Backup21 валидирует ID/type/ссылки/порт, старые IDs версий запрещены. Общие UUID устройств не являются аттестацией и не дают доступ к inbound без записи клиента. Создание/enable/delete защищают finite exports publication epoch; existing CSRF/apply/compensation сохраняются. Root helper, firewall и SSH этим source-блоком не менялись. Реальные VPN/OS/VM gates остаются открытыми.
