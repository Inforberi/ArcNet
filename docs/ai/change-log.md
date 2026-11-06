## 2026-11-06 — план установки и самостоятельной VPS-приёмки

Продолжение A2: один настоящий Ubuntu22.04 linux/amd64 release build со встроенным UI, provenance и source snapshot; полный tar.gz подписан новым offline installer signer, actual archive verify PASS. Новые releasebundle security tests вошли в этот единственный release pipeline; повторных race/browser matrix нет. Docker опционален на build host, не VPS. Добавлен безопасный маршрут clean migration; in-place updater/публичный Release/bootstrap не заявлены готовыми. [Guide](../ops/installer-release-and-migration.md), [результаты](../testing/local-results/stage5-2026-11-06-installer-bundle.md).

Продолжение A1: реализованы общие root-coordinated start/stop/restart/status/logs, safe flags plan/service-user DB, pause/lock и worker/helper/units/cert/update guards. Targeted Go tests/vet/race, shell fixtures и diagnostic core build PASS. Никаких actual systemd/VPS/handshake/512МиБ PASS; полный release/миграция и bootstrap остаются A2/A3. [Контракт](../ops/local-service-control.md), [ADR-013](../adr/013-local-runtime-lifecycle.md), [результаты](../testing/local-results/stage5-2026-11-06-local-lifecycle.md).

По запросу пользователя закреплены подготовка установочного пакета/проверенного bootstrap одной командой и общий start/stop/restart/status локальной установки. Пользователь сам выполняет серверные испытания. План отделяет первый smoke test от будущего каскадного runtime; A1 CLI — следующая кодовая задача. Это docs-only: проверены ссылки/команды по wrapper и diff, сборки/Go/frontend/серверные проверки не запускались. Никаких новых runtime PASS. [План](../planning/pre-vps-install-and-cli-2026-11-06.md).

## 2026-11-06 — независимый первый локальный протокол

Убрана обязательная настройка TCP REALITY перед XHTTP/Hysteria/WG: отдельная identity-проекция адреса, optional TCP, согласованные users/devices/подписки/учёт и backup v10. Никакого скрытого VLESS или новых dependencies/RPC/schema. Сохранены credentials и legacy backup compatibility; добавлены first-create/restore failure/port/address/freshness/export regression. Targeted Linux tests/vet/race и TypeScript/build PASS; отдельный Chromium UI/API сценарий с simulated engines. Реальный VPS/VPN NOT RUN, remote/Geo/cascade/failover остаются открытыми. [ADR-012](../adr/012-independent-first-local-protocol.md), [результаты](../testing/local-results/stage5-2026-11-06-first-protocol.md).


## 2026-11-05 — локальные пакеты Xray, accounting и backup v6

Подтверждённые пользовательские counters через private Unix StatsService, транзакционные суммы/минутные списания/checkpoints, пакет/срок/явный новый период, CAS и reconcile. UI использует Dialog/Advanced и показывает план изменения, unknown/pending/разрыв учёта. При failed delete восстанавливаются credentials и accounting; v6 переносит состояние без тихого уменьшения уже известного расхода. Point access lookup измерен на 10000 fixture users. [Объём](../release-0.7.0-stage5-user-packages.md), [результаты и ограничения](../testing/local-results/stage5-2026-11-05-user-packages.md). Только локальный Xray; устройства и distributed/UDP исполнение пока отсутствуют, VPS acceptance нет.

## 2026-11-05 — live monitoring и простые формы подключения

Сохраняемый default5, SSE авторизованного layout, fallback/offline/online/watchdog, bounded hub и agent resource lifecycle отдельно от apply; совместимость 204/404. Trusted defaults API и композиция `ConnectionForms`/`Advanced`, автоматические target-пробы, сохранение ручных drafts. Go test/vet/race, frontend build/tests, browser checks — [фактические результаты](../testing/local-results/stage5-2026-11-05-live-monitoring.md). Уточнены требования пользовательского пакета ГБ и устройств, runtime не добавлен. Каскады/лимиты/VPS остаются незавершёнными.

## 2026-11-05 — дашборд серверов и локальное восстановление доступа

Добавлены лёгкие ресурсы/история панели и нод, раздельные health-состояния, мобильный UI и безопасные CLI settings/password/session команды. SQL aggregate заменил полное чтение пользователей в overview; ограничен одновременный login KDF. Активная MFA при reset пароля сохраняется по умолчанию. Linux tests/vet/race, frontend10 tests/build, embedded React и Chromium dashboard/profile/routing regression PASS. UI backend испытан под 512 МиБ/1vCPU; это не VPN minimum. Geo/cascade/client failover и внешняя приёмка остаются открыты. [Точные результаты](../testing/local-results/stage5-2026-11-05-dashboard-recovery.md).

## 2026-11-05 — проверенные локальные профили/SNI и контракт клиентского failover

- После отправки `c25da9d` добавлены SNI picker нового VLESS/XHTTP и диагностический UI для выбранной ноды; автоматический подбор не перезаписывает ручное поле и не мигрирует действующие конфигурации.
- Signed feed: независимый ключ, подпись, expiry и watermark предотвращают replay даже после ручной замены каталога. Агент проверяет wire v1, identity, expiry и digest; нет root/shell или ложного VPN proof.
- Исправлены stale check response формы и найденная настоящим Chromium дедупликация `example.com` / `domain:example.com`; собственный import/export профилей, CAS, выключение без потери списков и backup v5 покрыты тестами.
- Linux full Go tests/vet/targeted race, frontend tsc/Vite/7 tests, embedded frontend-check, новый traffic-profile E2E и прежний routing regression E2E — PASS. [Точные команды/ошибки/результаты](../testing/local-results/stage5-2026-11-05-profiles-targets.md).
- Уточнена требуемая клиентская иерархия failover с параллельными диагностическими раундами: протокол текущего сервера, затем другой сервер; runtime/совместимость Happ ещё не реализованы/не проверены. Сохранены stage4 VPS и stage5 runtime gates, никакого VPN PASS.
- [Feature](../features/traffic-profiles-and-targets.md), [ADR-005](../adr/005-traffic-profiles-and-target-diagnostics.md), карта путей и current-task обновлены; полный source archive сопровождается SHA-256 manifest.

## 2026-11-05 — установка без домена и политика журналов; реализация UX в работе

- Уточнён согласованный план: проверяемая установка одной командой, IP TLS `shortlived` (160 часов), необязательный домен и безопасное позднее переключение, проверка renewal/last-good, отдельные размер/retention журналов без истории посещений и секретов. Добавлены `SYS-04`, `SEC-04`, `OPS-04`; установщик этим блоком не изменён.
- Фактически в `install.sh` уже есть Certbot IP/shortlived, timer каждые 8 часов и namespace 200 МиБ; это исходники, не подтверждение установки/продления на VPS. Первичные источники Let's Encrypt проверены 2026-11-05, ссылки в плане.
- Пользователь сообщил об отсутствии тестового стенда и разрешил продолжать локальную реализацию. `current-task.md` явно отмечает незавершённые профили/UI/backup v5 и SNI-пробы рабочего дерева; прежний PASS не распространяется на них.
- В этом блоке: `bash -n install.sh` — exit 0; `GOCACHE=/private/tmp/arcvpn-audit-go-cache go test ./internal/store ./internal/targetcheck ./internal/routingprofile ./internal/app` — exit 0 (в двух новых доменных пакетах пока нет тестов). Новых VM/VPS/ACME/VPN испытаний нет.

## 2026-11-05 — уточнение UX маршрутизации по аналогии с Happ (docs-only)

- По уточнению пользователя меню региональных шаблонов заменено включаемыми правилами маршрутизации: свои профили, VPN/прямой выход/блокировка, домены/IP/Geo, импорт/экспорт, DNS и расширенный порядок правил.
- Выключение дополнительных правил сохраняет базовый VPN-маршрут и профиль; не означает отключения VPN или fail-open. Разделены клиентский прямой выход, серверный выход и физический каскад нод.
- Проверена официальная документация Happ: совместимость экспорта зависит от формата/версии/ОС, есть restricted JSON mode и reconnect-семантика. Реализация интеграции не заявлена.
- Обновлён [план](../planning/ux-and-safe-automation-2026-11-05.md); код/API/БД не изменены. Runtime-тесты не запускались: это корректировка документации.

## 2026-11-05 — план UX, SNI и безопасной автоматизации (docs-only)

- Дополнено по новому запросу: различие угроз оператора доступа и хостера, проверяемый автоматический hardening с VM/rollback gate, альтернативный вход при блокировке IP, RU-пресеты с явной семантикой server/client routing и без прямого обхода по умолчанию.
- По запросу пользователя зафиксирован [план](../planning/ux-and-safe-automation-2026-11-05.md): простой базовый сценарий с расширенными формами, каталог SNI и per-node выбор по пригодности/стабильности/задержке, lifecycle нод из UI, Geo-импорт и мобильные сценарии.
- Уточнены проверка TLS вместо одного ICMP ping, происхождение списков, защита загрузчиков/проб от SSRF, сохранение ручного выбора и отдельная миграция действующих подключений.
- Без обещания неотслеживаемости или превосходства над 3x-ui без измерений; набор протоколов заморожен. Код/API/БД/агент не изменены; новые runtime/Go/frontend/VPS тесты не запускались. Проверяются только документация и целостность исходного архива.

## 2026-11-05 — редактор черновиков маршрутов и backup v4

- React Flow: основные/резервные стрелки, общая форма и таблица, мобильные карточки, явные draft-only состояния; исправлены скрытые выходы, сброс формы, устаревший preview и обработка CAS-конфликтов.
- Защита зависимых нод при revoke/SSH purge/detach и гонках с сохранением маршрутов. Backup v4 переносит черновики атомарно с нодами, проверяет граф и ссылки, сохраняет чтение v1/v2/v3 и восстанавливает маршруты при ошибке применения движков.
- Настоящий npm lockfile; исправлена несуществующая react-query 5.90.0, обновлены Router/Vite по audit. Linux Go tests/vet/selected race/build, frontend test/typecheck/build и новый настоящий Chromium UI/API E2E (8 страниц × 5 размеров × 2 темы) PASS. npm audit: 0 известных уязвимостей на момент проверки.
- Реальные каскады, VPN failover, публикация, SNI/Geo и VPS приёмка не добавлены и не подтверждены. Этап 6 не начат.
- Результат: [release](../release-0.7.0-stage5-routing-ui.md); доказательства: [протокол](../testing/local-results/stage5-2026-11-05-routing-ui.md).

## 2026-11-05 — фиксация набора VPN-протоколов (docs-only)

- По решению пользователя текущих пяти VPN-вариантов достаточно: VLESS/REALITY TCP, VLESS/REALITY XHTTP, Hysteria2, WireGuard, AmneziaWG 3.1.
- Этап 6 отложен: AnyTLS, Trojan, TUIC, Shadowsocks 2022, экспертный UI/overrides не входят в текущую разработку.
- Обновлены roadmap, decision register, README, AGENTS и текущая задача; код/API/БД не изменялись. Новых результатов Go/React/VPS тестирования не заявлено.
- Источник: `docs/planning/protocol-scope-freeze-2026-11-05.md`.


## 2026-11-05 — 0.7.0-stage5-foundation (draft-only)

- Добавлены typed RoutePolicy, graph validator, deterministic failover simulator, diff, SQLite CAS versioning, session/CSRF HTTP API и экран React маршрутов.
- Новые unit/store/http тесты; live Xray не изменялся. Этап 4 не принят, этап 5 не завершён.
- Документы: `docs/release-0.7.0-stage5-foundation.md`, `docs/planning/stage5-checklist.md`, `docs/testing/stage5-codex-handoff.md`.

# 2026-11-04 — ArcVPN 0.6.5, единая работа над этапом 4

- Source: `node-control.key` no-follow bounded private inode; migration-neutral pruning control and SSH lifecycle operation histories without deleting active origin; IPv6 safe SSH/SCP; agent retry backoff/jitter; CI fail-closed on absent reviewed npm lockfile. Версия agent/build = 0.6.5-stage4.
- Added Go regressions for key symlink/FIFO/mode, history retention/origin preservation, IPv6 target + SCP destination, contact backoff.
- Фактический `verify-stage4-local.sh --source` exit=0; TS parse 17 files PASS. `npm` EAI_AGAIN; frontend build/E2E NOT RUN. VPS/Xray/handshake NOT RUN. Не LOCAL_PASS, не ACCEPTED.
- Документы: `release-0.6.5-stage4-source-audit.md`, `planning/stage4-traceability-0.6.5.md`, `testing/local-results/stage4-2026-11-04-0.6.5.md`.

---

## 2026-11-04 — 0.6.4-stage4 (source hardening, НЕ приёмка)

- Однодескрипторное bounded no-follow чтение приватных credentials, подписанных архивов, ключей и root receipt; тесты symlink/FIFO/size/permissions.
- Fail-closed replay root-helper при повреждённом Xray конфиге, отказе службы или испорченной receipt; rollback удаления только собственных системных пользователей.
- Strict HTTPS origin и shell quoting UI команд. Введён `scripts/verify-stage4-local.sh` с раздельными `--source`/`--full`.
- Отдельные Go test/vet/race/CLI сборка и синтаксис скриптов — PASS. Комбинированный source-verifier — TIMEOUT на повторном HTTP API race. npm production React/VPS/VLESS E2E не проверены. Ряд исходных пунктов S4-xx не закрыт доказательствами. Stage 4 НЕ принят.

## 2026-11-04 — исходники 0.6.4-stage4, закрытие обнаруженных security defects

Исправлено атомарное ограниченное чтение приватных credentials, подписанного архива и signing keys; удаление чужих системных пользователей при rollback в гонке регистрации; обманчивый повторный `applied` по квитанции без сверки конфигурации и systemd; небезопасный/malformed controller HTTPS origin в агенте/SSH preflight и shell snippet UI. Добавлены тесты на symlinks, FIFO, oversize, key permissions, corrupt receipt, stale ACK. Full Go test/vet/build + выбранные race tests PASS; React build/E2E и реальные VPN/VPS — NOT RUN/BLOCKED. Этап не принят. Подробнее: `docs/release-0.6.4-stage4-security-and-acceptance.md`.

## 2026-11-04 — 0.6.3-stage4: независимая data-plane проба

- Добавлены `internal/nodeprobe/`, новый CLI `node-probe`, верифицируемая HTTPS 204 проба через Xray VLESS/REALITY, локальный SOCKS5 и запрет direct fallback.
- Добавлены миграция SQLite, одноразовые 5-минутные разрешения с hash, привязка к ревизии/digest, TTL 30 минут для отчётов, запрет replay и автоматической публикации; invalidation при deploy/drain/update/remove.
- Добавлены admin+CSRF API выдачи grant, отдельный bearer API отчёта, UI и отрицательные/конкурентные тесты.
- Локальные Go unit/vet/build/race критических пакетов PASS, TS/Bash/Python syntax PASS; HTTP API race TIMEOUT; npm `EAI_AGAIN`, реальный React/VPS/Xray/VPN NOT RUN. Этап 4 не закрыт; протокол `docs/testing/local-results/stage4-2026-11-04-0.6.3.md`.

## 2026-11-04 — 0.6.2-stage4, продолжение (source-only)

- Сверка root-owned agent/Xray бинарников по SHA-256 signed release после копирования из потенциально изменяемой SSH staging области; отказ до исполнения, при невалидном Xray candidate не рестартовать действующий VPN.
- Read-only SSH receipts для install/upgrade/detach/purge после потери ответа; egress HTTPS `/healthz`, UFW/nft предупреждения, секретобезопасные SSH exit codes.
- SQLite `node_control.draining` + API/React drain и запрет новых диагностических VLESS-ссылок, без ложной публикации, остановки сервиса или подмены внешнего VPN-probe.
- Обновлён `tests/react_e2e.py` под страницу Nodes этапа 4; Go unit/HTTP/security тесты и документация. **React E2E не исполнялся; VPS не проверялся.**

# Журнал изменений проектной документации

## 2026-11-04 — 0.6.1-stage4 (продолжение кода)

Исправлен невалидный POSIX preflight из 0.6.0, добавлено явное версионирование агента/протокола, миграция SQLite, проверка неподдерживаемых agent commands. Реализованы локальный подписанный node-install с токеном только через stdin, безопасный managed-only detach/purge, SSH удаление с root-owned marker и проверкой исходного IP/SSH fingerprint, rebind на новый контроллер, подписанный SSH-upgrade agent+Xray с rollback и серверным журналом/версией, UI wizard и фильтры. Неуспешный enrollment делает bounded managed cleanup, если identity заведомо отсутствует.

`go test -count=1 ./...`, `go vet ./...`, Go dev build, синтаксис Shell и parse 16 TS/TSX PASS; race subset PASS, общий HTTP race TIMEOUT. npm/React production build/VPS/handshake NOT RUN. Этап 4 не принят. Подробности: `docs/release-0.6.1-stage4-continuation.md`.

## 2026-11-03

- Зафиксирован стек: Go, SQLite, React/TypeScript/Vite, React Router Data Mode, TanStack Query, Tailwind, выборочные shadcn/ui, композиция.
- Уточнён режим Standalone: собственный локальный VPS без обязательного Go-агента; Hybrid/Controller/Remote Node — опциональные режимы.
- Зафиксированы отсутствие обязательного домена, HTTPS по публичному IP, подписка независимо от входных VPN-нод, резервные URL.
- Добавлены требования к автоматической установке VPS через SSH, UFW, управлению секретами, каскадам в любом направлении, резервированию.
- Сохранены четыре пользовательских скриншота 3x-ui как визуальные референсы и пример Go-архитектуры как **необязательный** источник архитектурных принципов.
- Реализация проекта не начиналась.

### 2026-11-03 — дополнение по новой постановке

- Добавлены автоматизация по умолчанию и управляемые ручные overrides.
- Добавлены два режима визуализации серверов: связанная схема/таблица, единый state.
- Сформированы подробный preflight VPS и SSH-install wizard с критериями безопасности.
- Развёрнута политика UFW/SSH/TLS/IP certificates/agent privileges, модель угроз и runbooks.
- Расширены requirement IDs и acceptance gates.
- Реализация и реальное тестирование по-прежнему не начинались.

## 2026-11-04 — пересмотр порядка разработки (только документация)

- Переписана дорожная карта в согласованные 7 этапов: UI, локальные протоколы, безопасность VPS, ноды, каскады, дополнительные протоколы, итоговая приёмка.
- Добавлена матрица «делаем здесь / внешние проверки Codex», статусы `LOCAL_PASS`, `EXTERNAL_PENDING`, `ACCEPTED`, runbook и чеклист этапа 1.
- **Импорт из 3x-ui исключён** как требование; отдельно описан полный перенос ArcVPN между VPS через UI.
- Введено правило: отсутствие VPS не тормозит программирование, но не даёт оснований сдавать непроверенный этап/production.
- Не заявлено новых запущенных тестов, новых реализованных протоколов или завершения этапа 1.

## 2026-11-04 — доработка исходников этапа 1 (0.3.3)

- Добавлено встраивание production React в Go-бинарник по build tag `arcvpn_embed_react`; команда `frontend-check`, fail-closed installer.
- Включены проверка настоящих JS/CSS статических ресурсов, отсутствие symlink в web root и no-store для SPA HTML.
- Исправлены обработка 401, обновление сессий, error boundary, кэш после восстановления, статус маршрутов и мобильные темы.
- Созданы CI workflow и полноценный сценарий `tests/react_e2e.py`.
- Go tests/vet/race, shell, TS syntax, synthetic embed — PASS; npm/Vite/React E2E и чистый VPS — **NOT RUN/BLOCKED**, этап не закрыт.

## 2026-11-04 — продолжение этапа 2 (0.4.1)

- Исправлено преждевременное удаление секретов и остановка старых служб при восстановлении конфигурации. Порядок применения теперь валидация → фиксация состояний → установка → systemd → компенсирующий откат.
- Запрос статуса через строго разрешённый root-helper, без доверия системным ошибкам как `inactive`.
- HTTP не показывает подключения отключённых клиентов и запрещает кеширование ссылок подписок.
- Регрессионные тесты на отказ валидатора, отказ запуска службы и восстановление ранее удалённой конфигурации. Go test/vet/race PASS; frontend production build/VPS E2E не запускались.

## 2026-11-04 — 0.4.2 (стадия 2)

Зафиксирована воспроизводимая сборка официальных AWG3.1 компонентов из Git-тегов на CI и поставка вместе с проверенными контрольными суммами; root runtime больше не полагается на старый kernel module, использует защищённый config staging и прямой userspace. Безопасные настройки AWG3.1, тесты регрессий, документация. **Реальный upstream build/React/подключение в этой среде не подтверждены.**

## 2026-11-04 — 0.4.4

Локальная интеграция: откат Xray при отказе вспомогательного движка, восстановление первоначального состояния без прежнего файла, атомарное сохранение rollback, запрет секретов в выводе валидаторов, проверка согласованности переносимых XHTTP/WG профилей, запрет выдачи подключений отключённым пользователям. Регрессионные тесты и `scripts/verify-stage2-local.sh` проходят. Внешняя сборка React/AWG и реальный VPS остаются открыты. Подробно: `docs/release-0.4.4-stage2-code-handoff.md`.


## 2026-11-04 — сверка полного source ZIP и подготовка этапа 4 (только docs)

- Проверен пользовательский архив `arcvpn-stage3-0.5.1-complete-source-handoff.zip`: 181 запись, совпадение SHA-256 с приложенным файлом.
- Повторно выполнены `go test -count=1 ./...`, `go vet ./...`, `bash -n` и локальный dev `go build`: PASS. React/VPS/E2E — NOT RUN. Состояние зафиксировано в `docs/testing/local-results/pre-stage4-2026-11-04.md`.
- Подтверждено: этап 4 ещё не реализован; имеются только telemetry-only агент, реестр и enrollment. Этапы 1–3 по-прежнему EXTERNAL_PENDING, без production gate.
- Выявлен незакрытый SSH hardening (предусмотрен исходным scope этапа 3, но не найден в UI/API). Выявлены security gaps preview installer: enrollment token передаётся через argv, а получение бинарника и SHA-256 с одного HTTPS endpoint не образует независимой подписи релиза. Зафиксировано как блокер перед stage4 rollout.
- Созданы `docs/audits/2026-11-04-pre-stage4.md`, `docs/planning/stage4-checklist.md`, `docs/testing/stage4-codex-handoff.md`. Обновлены `AGENTS.md`, `README.md`, AI index/current-task, implementation status, feature/roadmap.
- Функциональный runtime-код **не менялся**. Этап 4 не объявлен ни LOCAL_PASS, ни ACCEPTED.

## 2026-11-04 — 0.6.0-stage4-source-preview (код частично реализован)

Добавлены контроллер операций/ревизий SQLite с AES-GCM, per-node credential rotation и ревок, SSH Ed25519 fingerprint+SSRF/preflight, подписанный Ed25519 installer агента/Xray, root-helper Xray apply/rollback, agent polling/ACK и UI схемы/таблицы. Устранены дефекты установки: capability для порта 443, root-owned receipt, POSIX quoting SSH-аргументов, installer compensation, bounded retries и operation ID при потерянном ACK, bounded SSH workers, фиксация таймаута даже при отменённом worker context. Legacy `/node-install.sh` отключён. Создан `scripts/build-stage4-node-bundle.sh`. `go test -count=1 ./...`, `go vet ./...`, race subset и Bash syntax — PASS. `npm install --package-lock-only` — BLOCKED `EAI_AGAIN`, React build/VM/VPS/VPN E2E — NOT RUN. Этап 4 пока **не завершён**, production недопустим. Подробности: `docs/release-0.6.0-stage4-source.md`.
## 2026-11-05 — индивидуальные доступы устройств

Локальные TCP/XHTTP credentials с отдельной подпиской, серверным CAS/лимитом, отзывом и освобождением слота. Общий ключ отключается при переходе; общий пакет пользователя сохраняется. Apply failure виден и повторяется. Backup v7 сохраняет device credentials/отзывы; старый архив не отменяет местный отзыв. UI собран из Dialog/Field/Button/Advanced и существующих стилей; без зависимостей и Python. [Контракт](../adr/009-device-credentials.md), [проверки](../testing/local-results/stage5-2026-11-05-devices.md). Remote/UDP devices и VPS enforcement не приняты.
## 2026-11-05 — Hysteria2: расход и отдельные доступы

TrafficStats collector с private loopback/проверкой процесса, независимая freshness Xray/Hysteria, общий пакет/списания, individual device credentials/подписки, backup v8. Использованы существующие UI-компоненты и apply/retry; нет новых dependencies/privileged RPC. Настоящий loopback QUIC/TLS v2.13.0 подтвердил counters и отказ отозванному ключу после restart. Systemd/VPS gate остаётся открытым. [ADR-010](../adr/010-hysteria-traffic-and-devices.md).
## WireGuard — отдельные peer/IP и учёт

Индивидуальные WG keys/IP, резервирование в прежней подсети, device `.conf` download, исключение legacy/revoked peers, public transfer stats через fixed root helper, per-source freshness и общий пакет, backup v9/legacy restore без ротации локальных keys. AmneziaWG не расширяется и исключён из дальнейшего плана по прямому решению пользователя. VM/VPS/kernel приёмка не заявлена. [Контракт](../adr/011-wireguard-traffic-and-devices.md).
## 2026-11-06 — публичный README, MIT и короткие блоки

README переписан как актуальный вход без устаревших lockfile/0.6.5 статусов. Добавлены MIT по прямому решению владельца, инструкция clean install/signed update/recovery, правила коротких рабочих блоков и обновления README в AGENTS. Анонимный GitHub API подтвердил public repository и отсутствие Releases; публикация релиза не выполнялась.

Исправлена упаковка: build version 0.7, AmneziaWG не обязателен по умолчанию, installer принимает базовый manifest без AWG. Установочный tar.gz включает install/deploy/docs/license/version/checksums и optional public key; private keys/DB/unlisted files не включаются. Сохраняется строгая проверка полного legacy bundle, если он явно выбран.

Проверки: `bash -n install.sh scripts/build-release.sh scripts/package-release.sh tests/release_packaging.sh` PASS; local Markdown links PASS; `tests/release_packaging.sh` в существующем Linux образе — PASS (0.48 s): состав, версия 0.7, исключение private/stale AWG, tampered installer rejection, incomplete legacy/unsafe version rejection. Это synthetic packaging fixture; настоящий release build, Go/frontend/race/E2E/VM/VPS не запускались в этом блоке. Signed updater не менялся; автоматическая миграция shell/systemd/ОС не заявлена.
