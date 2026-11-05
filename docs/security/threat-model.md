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
