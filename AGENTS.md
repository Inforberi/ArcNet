> **Обязательное ограничение scope (2026-11-05):** не начинать этап 6 и не добавлять AnyTLS, Trojan, TUIC, Shadowsocks 2022 или экспертные overrides без нового прямого решения пользователя. Достаточный текущий набор: VLESS/REALITY TCP, VLESS/REALITY XHTTP, Hysteria2, WireGuard, AmneziaWG 3.1. Не выдавать исходную реализацию за подтверждённый VPN-трафик. См. `docs/planning/protocol-scope-freeze-2026-11-05.md`.
# АКТУАЛЬНЫЙ ИСТОЧНИК: ArcVPN 0.7.0-stage5-foundation · 2026-11-05

**Продолжение 2026-11-05:** интерактивный draft UI, зависимости нод и backup v4 — `docs/release-0.7.0-stage5-routing-ui.md`; фактические Go/Linux, frontend build и routing UI/API проверки — `docs/testing/local-results/stage5-2026-11-05-routing-ui.md`. Старый frontend-blocked статус ниже исторический; этапы 4–5 по-прежнему не приняты на VPS. Читайте дополнение после foundation. Пользователь разрешил отправку исходников в `https://github.com/Inforberi/ArcNet.git`.

**Последний код:** этап 5 начат по явному решению пользователя, пока этап 4 `EXTERNAL_PENDING`. Stage5 — **только draft-only control plane** (`internal/routes`, `internal/store/routes.go`, `internal/httpapi/routes.go`, `frontend/src/pages/Routing.tsx`). Запрещено утверждать, что реальный A→B, B→A, auto-failover и публикация подписок работают.

Обязательный маршрут чтения: `docs/ai/current-task.md` → `docs/release-0.7.0-stage5-foundation.md` → `docs/planning/stage5-checklist.md` → `docs/testing/stage5-codex-handoff.md`; затем domain-тесты. Никаких произвольных команд через агент, никаких route publish до ACK всех нод и достоверной независимой egress-пробы. Чистая симуляция не доказывает сетевую работоспособность. При работе в Codex закрывать оставшееся из обоих этапов, не проставлять PASS без испытаний.

Все документы ниже относятся к более ранним этапам и сохранены как история.

---

# ИСТОЧНИК ИСТИНЫ: ArcVPN 0.6.5-stage4, 2026-11-04

**Текущий код:** `SOURCE_CHECKS_PASS / FRONTEND_BLOCKED / VPS_PENDING`. Этап 4 всё ещё **не принят**. Сначала прочесть `docs/ai/current-task.md`, `docs/release-0.6.5-stage4-source-audit.md`, `docs/planning/stage4-traceability-0.6.5.md` и `docs/testing/local-results/stage4-2026-11-04-0.6.5.md`. Для внешнего стенда — `docs/testing/stage4-codex-handoff.md`. Не объявлять кодовую проверку завершённым этапом или успешным VLESS handshake.

При любых изменениях: обновлять тесты и документацию; архив передавать целиком. До React typecheck/build/E2E не ставить `LOCAL_PASS`, до двух VPS и независимого VPN-клиента не ставить `STAGE4_ACCEPTED`. **Все нижеследующие заголовки 0.6.0–0.6.4 исторические.**

---

# Источник истины — ArcVPN 0.6.4-stage4

Сначала прочитать `docs/ai/current-task.md`, `docs/release-0.6.4-stage4-security-and-acceptance.md`, `docs/testing/local-results/stage4-2026-11-04-0.6.4.md`. Программные security fixes покрыты Go unit и targeted race; React npm-зависимости недоступны, настоящий build/E2E/VPS/Xray не выполнены. Не объявлять этап 4 завершённым и не устанавливать исходники на production. История 0.6.3 ниже сохранена как контекст, не как актуальный статус.

## Историческая справка о состоянии 0.6.3 (не текущий релиз)

Сначала прочитать `docs/ai/current-task.md` и `docs/release-0.6.3-stage4-continuation.md`. Новое в 0.6.3: `internal/nodeprobe/`, `cmd/arcvpn/node_probe.go`, `internal/store/node_probe.go`, `internal/httpapi/node_probe.go`, UI для one-time grant; права secrets 0600, токен отчёта живёт 5 минут, статус устаревает после 30 минут и сбрасывается при изменении ревизии/дренировании/обновлении/удалении. Статус `pass` относится к **отчёту доверенного независимого runner**; токен сам по себе НЕ доказывает реального handshake или места запуска. Никакой автопубликации после PASS. Бинарник ArcVPN и Xray доверенные, нет arbitrary shell, fallback direct, TLS bypass. Go-проверки прошли, HTTP API race TIMEOUT, npm / React build BLOCKED, реальные VPS/Xray/handshake NOT RUN. Нельзя заявлять `LOCAL_PASS`/`ACCEPTED`.

> **Текущая политика:** реализовать здесь всё, что доступно в коде; реальные VPS-проверки не подменять фиктивными тестами. Каждый этап имеет отдельно `LOCAL_PASS` и `ACCEPTED`; не объявлять production до внешней приёмки. Смотреть `docs/planning/execution-boundaries.md`, `docs/testing/codex-handoff.md` и `docs/planning/stage1-checklist.md`.

# Контроль фактической готовности

Историческая копия исходников: **0.6.3-stage4**. Статус `STAGE4_SOURCE_IN_PROGRESS / GO_TEST_PASS / FRONTEND_BLOCKED / EXTERNAL_PENDING`. Это НЕ production. Фактический handoff: `docs/release-0.6.3-stage4-continuation.md`, запуск и риски: `docs/ops/stage4-nodes.md`. Сначала прочитать `docs/ai/current-task.md`, `docs/planning/roadmap.md` и `docs/planning/execution-boundaries.md`. Реальная установка и многонодовые VPN-протоколы пока не подтверждены. Не утверждать обратного. Любые UI-состояния, где действие не подключено к backend, должны быть явно обозначены как недоступные. Документация обновляется вместе с кодом и тестами.

# Инструкция для Codex и других ИИ-агентов

## 0. Текущий статус

На 2026-11-04 исходники включают базовый stage4 control-plane/SSH-wizard/remote Xray apply, но **полный этап 4 не завершён**: `npm` сборка/E2E, external VPS и handshake не проверены; CLI bootstrap/uninstall/rebind/подписанный upgrade реализованы в исходниках, но **не проверены на VM/VPS**. Этапы 1–3 также `EXTERNAL_PENDING`; SSH hardening из stage3 scope не завершён. Новый legacy web/node-install.sh отключён из-за утечки токена через argv. Фактический source status: `docs/release-0.6.3-stage4-continuation.md`; точный backlog: `docs/planning/stage4-checklist.md`. Исторические документы 0.3.x–0.5.0 не являются текущим статусом.

## 1. Обязательный маршрут чтения

Прежде чем править код: `docs/ai/current-task.md` → `docs/ai/index.md` → только соответствующий feature-документ → соответствующий ADR → тесты затронутого модуля. Не загружать целиком весь контекст референсного Auth-проекта без необходимости.

## 2. Технологические ограничения

- Go + SQLite + React/TypeScript/Vite; React Router Data Mode, TanStack Query; Tailwind и выборочные shadcn/ui.
- Одно приложение для локальной панели; отдельный Xray; на удалённом VPS — агент и Xray.
- Никаких обязательных Redis, PostgreSQL, Docker, Node.js SSR, RabbitMQ, Kafka, Prometheus и фоновых тяжёлых сервисов.
- Не вводить глобальные mutable singleton, universal repository/UoW/framework, generic `utils` для бизнес-логики или пустые пакеты ради схемы.
- В UI переиспользовать токены и компоненты дизайн-системы. Предпочитать композицию, избегать «гигантских» компонентов с десятками флагов.
- Все решения по оптимизации подтверждать профилированием/замерами, а не верой в конкретный фреймворк.

## 3. Границы кода

- Фича владеет предметными правилами (`domain`), сценариями (`service`), адаптерами хранения (`repository`), HTTP (`transport/http`) — **только теми слоями, которые реально нужны**.
- Поток зависимостей: transport → service → domain. Service зависит от узких портов; repository реализует порт, а сборка — в `internal/app`.
- Межфичевые зависимости — через явно обоснованные контракты. Транзакция SQLite имеет одного владельца.
- `cmd` — args/env/signals/exit; `internal/app` — wiring и жизненный цикл; `internal/server/http` — HTTP server, timeouts, shutdown.
- Ноды/подписки/пользователи/маршруты — самостоятельные доменные сущности. Нода и логическая точка подключения — не одно и то же.
- Не копировать код 3x-ui, Remnawave и сторонний UI без анализа лицензий. Идеи/UX допустимы как требования; реализация независимая.

## 4. Безопасность

- Сначала модель угроз, потом SSH-автоматизация, TLS, обновление конфигураций и API привилегий.
- Никогда не отключать SSH и не применять потенциально блокирующий UFW без проверенного rollback.
- Не выключать проверку TLS и SSH host key «для удобства».
- Browser auth: серверные сессии, проверка прав на Go, CSRF для изменения состояния, безопасные cookies, 2FA для администратора.
- Не выводить токены, UUID/ключи доступа, ссылки подписок и приватные ключи в публичные логи/скриншоты.
- Запрещён произвольный shell по команде панели через агент; привилегированные операции — allowlist и контролируемый помощник.
- Публикация конфигурации: проверить → подготовить → применить → подтвердить здоровье → зафиксировать; при ошибке восстановить предыдущую версию.

## 5. Процесс любой задачи

1. Установить фактический статус в `current-task.md`, сформулировать измеримый результат.
2. Уточнить владельца кода и прочесть только нужные docs/feature/ADR.
3. Описать изменения в API/DB/доверенных границах; для опасных изменений — rollback.
4. Реализовать минимальный полный вертикальный сценарий, без заглушек и псевдокода.
5. Написать тесты (unit/HTTP/integration/security) согласно `docs/testing/strategy.md`.
6. Запустить релевантные проверки; фиксировать **фактические команды/результат**, не выдумывать пройденные тесты.
7. В том же PR/рабочем блоке обновить feature-документ, реальную карту путей, ADR (если принято решение), `current-task.md` и журнал изменений.
8. В конце перечислить реализовано / не реализовано / ограничения / доказательства.

## 6. Правила документации

- `docs/product/requirements.md` — **что** должна делать система, без утверждений о готовности.
- `docs/architecture/*.md` — **как** организуем систему и её инварианты.
- `docs/features/*.md` — контракт конкретной возможности, UX и крайние случаи.
- `docs/design/*.md` — дизайн-токены, компоненты, страницы и референсы.
- `docs/adr/*.md` — принятые архитектурные решения и альтернативы.
- `docs/ai/index.md` — текущая карта реальных файлов, не проектных фантазий.
- `docs/ai/current-task.md` — состояние работы; обновлять после каждого блока.
- `docs/planning/*` и `docs/testing/*` — этапы и проверяемые критерии.

Не создавать README в каждой папке/файле. Не превращать docs в свалку повторяющегося текста. Ссылаться на первичный документ.

## 7. Важнейшие требования при дальнейшем программировании

- Система **автоматизирует по умолчанию**, но сохраняет редактируемые и прослеживаемые expert-override. Перед опасным изменением нужен **план с diff**, а не просто сохранение формы.
- Любой SSH installer/SSH change/UFW/network/ACME update разрабатывается **только после** актуальной [модели угроз](docs/security/threat-model.md) и с rollback tests в изолированной VM.
- Прежде чем проектировать диаграмму нод, прочесть [spec схемы и таблицы](docs/design/topology-and-table.md). Прямой сервер, каскад и административный канал — разные сущности. Таблица/граф — две проекции одного состояния.
- Для серверных ограничений не подставлять недоказанные `minimum RAM`, `minimum disk`, `max users`. Источник — [benchmark/profile](docs/ops/server-requirements.md).
- Узкие решения о безопасности и совместимости сначала подтвердить официальным API/документацией и конкретной тестовой версией, потом записать в feature/ADR.

## Требования этапа 2 — обновление 2026-11-04

См. `docs/ai/current-task.md`, `docs/ops/protocol-runtime.md`, `docs/testing/stage2-codex-handoff.md`. В новых исходниках четыре дополнительных профиля и backup v2. **Не путать Go-мок тесты с реальным Xray/WG/QUIC**. При любом изменении профилей сохранять user IDs и секреты, не терять токены подписок. Не исполнять `wg-quick` из untrusted `arcvpn`-writable configs; root `tunnel` CLI с whitelist — обязательная граница прав. Не писать в публичный API приватные ключи. AWG 3.1 не считать установленным, пока не проверены инструменты и kernel/userspace. Изначальный статус: `MY_PART_INCOMPLETE`, `CODEX_EXTERNAL_PENDING`. Документацию обновлять вместе с кодом и реальными тестами.

## Изменение 0.4.2: сборка и runtime AmneziaWG 3.1

Сначала прочитать `docs/release-0.4.2-stage2-source.md`, `docs/ops/protocol-runtime.md`. Зафиксированные исходники upstream собираются **на release host**, не на VPS. Системный `awg-quick` из неизвестного PPA не использовать. AWG runtime — direct pinned userspace. Не включать `DisableCookies=on` по умолчанию без оценки угроз. Не выдавать локальные Go-тесты за проверку handshake. Внешний Codex должен собрать upstream на Go >= 1.25, выпустить проверенный React и провести handshake на тестовых VPS.

## Дополнение 0.4.4: локальное завершение Go-интеграции

Главный актуальный файл: `docs/release-0.4.4-stage2-code-handoff.md`. При изменении состояний нескольких движков обязательны компенсация и восстановление Xray; валидаторы конфигураций не должны выводить приватные ключи. При импорте v2 проверять UUID XHTTP и соответствие адреса ноды. Локальный проверочный скрипт: `scripts/verify-stage2-local.sh`. Статус *Go local PASS* не означает React build, живое подключение или production.

## Дополнение 0.5.0: этап 3

Прочитать `docs/release-0.5.0-stage3-source.md`, `docs/ops/local-development.md`. TOTP проходит через `internal/mfa`, `internal/store/mfa.go`, `internal/httpapi/mfa.go`; не раскрывать секреты ни в логах, ни в backup. CLI backup — `internal/backupfile` и `cmd/arcvpn`; не возвращать незашифрованную SQLite-копию в меню. Root helper читает журналы строго по allowlist и управляет только `journald@arcvpn` namespace; проверка реального systemd обязательна. Для тестов Codex: `docs/testing/stage3-codex-handoff.md`. Stage3 не считается законченным без UFW/updates/full restore и внешнего тестирования.

## Актуализация этапа 3: 0.5.1 Code Handoff

Текущая матрица: `docs/implementation-status.md`, handoff `docs/release-0.5.1-stage3-code-handoff.md`, инструкции внешних испытаний `docs/testing/stage3-codex-handoff.md`. В Go исходниках есть строго ограниченный UFW-helper, зашифрованный перенос v3 и Ed25519-signed updater. Запрещено выдавать code-level тест за проверку systemd/SSH/VPN. Updater использует `RuntimeDirectory=arcvpn-update` и блокировку в `/run/arcvpn-update/lock`; сохранять эти пути синхронно. Изменения БД могут быть необратимыми и откат только бинарника не гарантирует совместимость.

## Новое обязательное дополнение: переход к этапу 4 (аудит 2026-11-04)

- Сначала читать `docs/ai/current-task.md`, `docs/audits/2026-11-04-pre-stage4.md`, `docs/planning/stage4-checklist.md`; старые этапы не переписывать и не называть принятыми без `docs/testing/external-results/`.
- Stage4 начинается с `S4-00..04`, а не с кнопок. Не использовать существующий `curl | sudo bash --token ...` как безопасный production bootstrap: секрет раскрывается через argv. Не делать произвольный remote shell RPC.
- Каждый новый managed node должен иметь typed contract, desired/applied/last-good revision, rollbacks, controlled root helper и раздельные connectivity/service/VPN probe статусы. Heartbeat не доказывает клиентский VPN handshake.
- После каждого блока сохранять весь проект с исходниками, тестами, обновлёнными статусами и точным списком нерешённых задач. Внешние Codex сценарии: `docs/testing/stage4-codex-handoff.md`.

## Актуализация 0.6.0 — этап 4 (частично реализован)

Сначала читать `docs/ai/current-task.md`, `docs/release-0.6.0-stage4-source.md`, `docs/ops/stage4-nodes.md`. Не возвращаться к старой фразе «этап 4 не начат». SSH wizard и агент существуют в исходниках, но `ACCEPTED` отсутствует. Тесты Go проходят; React npm registry недоступен в этой среде. Root-helper receipt не переносить из `/var/lib/arcvpn-node-root` в каталог агента; Xray должен работать с `CAP_NET_BIND_SERVICE`, иначе порт 443 не откроется. Повтор Xray deployment должен использовать прежний operation ID, чтобы потерянный ACK не нарушал идемпотентность. Не отправлять в аргументах SSH/процессов enrollment и private key. Не включать remote endpoints в подписки до внешнего VPN handshake. В конце новой итерации обновить docs/status и передать полный исходный архив.

## Актуальное дополнение 0.6.2-stage4

Перед следующими правками изучить `docs/ai/current-task.md` и `docs/release-0.6.2-stage4-continuation.md`. SSH user-owned `/tmp` **не является trust boundary** для уже проверенного signed release: только SHA-256 root-owned кандидата до исполнения предотвращает подмену после scp. Не менять логику проверки signer без независимого ключа. После потери SSH ACK контроллер обязан сверять корневой marker, локальную версию, SHA и systemd/managed resources; неопределённое состояние — FAIL, а не успех. `drain` сейчас блокирует только выдачу новых **диагностических** VLESS-ссылок, не разрывает текущий VPN-трафик. Не выставлять `vpn_probe=pass` без внешнего клиента и не публиковать remote endpoint автоматически.

Go/Bash/TSX syntax не заменяют `npm ci`, TypeScript/Vite production build, Playwright и настоящие VM/VPS испытания. Последние не выполнены. Внешний handoff `docs/testing/stage4-codex-handoff.md`, локальные тесты `docs/testing/local-results/stage4-2026-11-04-0.6.2.md`. Релизная версия и manifest signed node bundle должны быть `0.6.2-stage4` при сборке именно этих исходников.


## Передача 0.6.3

Версия билда подписанного пакета/CLI для этих исходников — `0.6.3-stage4`, включая `scripts/build-release.sh`. Документ фактического тестирования: `docs/testing/local-results/stage4-2026-11-04-0.6.3.md`; handoff: `docs/testing/stage4-codex-handoff.md`; контракт runner: `docs/ops/stage4-nodes.md`. История 0.6.0–0.6.2 ниже не означает текущий код. Проверить `npm ci`/TypeScript/Vite/Playwright на сетевом CI и реальную Xray-совместимость на отдельной VM. Запретить приёмку, если эти шаги не подтверждены.
