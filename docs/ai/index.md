## 2026-11-06: UI применения каскадов (browser pending)

`internal/store/cascade_ui.go` — stable endpoint/binding/preflight/baseline; `internal/app/cascade_ui.go`, `cascade_clients.go` — preview token/apply/preserve listeners; `internal/httpapi/cascade_ui{,_test}.go` — authenticated API и opt-in fixture; `frontend/src/features/routing/CascadeDeployment.tsx`, `RoutePaths.tsx` — UI; `tests/cascade_ui_e2e.mjs` — browser scenario. [Точка продолжения](../testing/local-results/stage5-2026-11-06-cascade-ui.md).

## 2026-11-06: постоянный журнал каскада

`internal/store/cascade{,_test}.go` — encrypted snapshots/leases/CAS/stages; `node_control.go` — typed queue/poll/ACK transitions; `nodes.go`, `internal/nodeagent/agent.go`, `internal/noderuntime/runtime.go` — helper capability/heartbeat. `internal/app/cascade{,_test}.go` — internal preparation. Guards: store/routes/devices/usage/node_profile/node_bootstrap/store; `internal/httpapi/{httpapi,cascade_test}.go` —409. [ADR-016](../adr/016-persistent-cascade-deployment.md), [проверки](../testing/local-results/stage5-2026-11-06-cascade-journal.md). Admin UI/start API не подключены.

## 2026-11-06: каскадный канал

`internal/xray/cascade{,_test,_integration_test}.go` — machine identity/renderer/real Xray test; `internal/noderuntime/cascade{,_test}.go`, `runtime.go` — отдельная allowlisted apply operation с receipt/rollback. Controller/UI queue не подключены. [ADR-015](../adr/015-cascade-channel-and-node-operation.md), [результаты](../testing/local-results/stage5-2026-11-06-cascade-channel.md).

## 2026-11-06: A3 bootstrap

`deploy/bootstrap-install.sh` — pinned installer с OpenSSL/Python safe extraction; `scripts/testing/bootstrap-fixture.py` — offline signed-hostile archive fixture; [команда/приёмка](../ops/vps-preview-acceptance.md), [проверки](../testing/local-results/stage5-2026-11-06-bootstrap.md). Публичный prerelease A2; VPS остаётся пользовательской проверкой. Старые статусы ниже исторические.

## 2026-11-06: подготовка к самостоятельной VPS-приёмке

A2: `deploy/release.Dockerfile`, `scripts/build-installer-docker.sh`, `scripts/{build-release,package-release}.sh` — Ubuntu22.04 linux/amd64 build, metadata/source snapshot/installer. `internal/releasebundle`, `cmd/arcvpn-release` — offline full-archive signer/verifier/keygen, отдельный от updater протокол. [Guide](../ops/installer-release-and-migration.md), [ADR-014](../adr/014-signed-installer-and-build-baseline.md), [результаты](../testing/local-results/stage5-2026-11-06-installer-bundle.md). Далее A3 bootstrap; keys вне Git, публичного Release нет.

[План A1–A4/B](../planning/pre-vps-install-and-cli-2026-11-06.md): A1 реализован локально, A2 имеет local signed installer; далее A3 установка одной командой. Испытания проводит пользователь.

- `internal/lifecycle/{manager,guard}.go` — одноразовый root coordinator и pause/lock; `internal/store/runtime.go` — safe enabled flags.
- `cmd/arcvpn/lifecycle.go`, `deploy/arcvpn-menu.sh`, `internal/app/runtime.go` — CLI/меню, service-user prepare/reconcile, safe status.
- `cmd/arcvpn/main.go`, `internal/updater/apply.go`, `install.sh` — guards для helper/worker/update/cert/units, boot tmpfiles. Нет новых постоянных сервисов.
- Тесты рядом с модулями, `cmd/arcvpn/{menu,lifecycle,lifecycle_install}_test.go`; [контракт](../ops/local-service-control.md), [ADR-013](../adr/013-local-runtime-lifecycle.md), [результаты](../testing/local-results/stage5-2026-11-06-local-lifecycle.md).

## 2026-11-06: первый протокол без обязательного TCP REALITY

- `internal/store/local_server.go` — самостоятельная read-only модель local host address, `OptionalReality`, guard последнего профиля.
- `internal/app/{app,protocols,devices,usage,backup}.go`, `internal/xray/{xray,statistics}.go` — отсутствие скрытого listener, users/учёт/экспорт/restore auxiliary-only.
- `internal/backup/backup.go` — v10, legacy v1–v9 и independent remap; API в `internal/httpapi/{httpapi,protocols,portable,connection_defaults,dashboard}.go`.
- `frontend/src/features/connections/ConnectionForms.tsx`, `frontend/src/pages/{VPN,Users,Backup}.tsx` — доступное первое создание, общие URI/QR подписи и `.conf`.
- Новые `first_protocol_test.go`, `local_server_test.go`, `backup_v10_test.go`, `tests/first_protocol_e2e.mjs`; [ADR-012](../adr/012-independent-first-local-protocol.md), [результаты](../testing/local-results/stage5-2026-11-06-first-protocol.md).


## 2026-11-05: локальные пакеты трафика Xray

- `internal/usage/{usage,state}.go` — counters/epoch, пакет, причины ограничения, portable accounting.
- `internal/store/usage{,_backup}.go` — транзакционные суммы/checkpoints/debits, point access lookup, CAS, restore high-water и компенсация удаления.
- `internal/xray/statistics.go`, `internal/app/usage.go`, `cmd/arcvpn/main.go` — private StatsService, bounded CLI, один collector, reconcile с apply/rollback.
- `internal/httpapi/usage.go`, `frontend/src/features/users/{usage.ts,PackageDialog.tsx}`, `frontend/src/pages/Users.tsx` — защищённые API, расход/остаток, период/срок, immutable editor draft.
- `internal/backup/backup.go`, `internal/app/backup.go` — backup v6; тесты рядом с модулями, `tests/user_usage_e2e.mjs`, `tests/run-user-usage-browser.sh`.
- Контракт: [ADR-008](../adr/008-local-xray-traffic-packages.md), [объём](../release-0.7.0-stage5-user-packages.md), [доказательства](../testing/local-results/stage5-2026-11-05-user-packages.md). Remote/UDP accounting, устройства и каскадный runtime не завершены.

## 2026-11-05: подписка мониторинга и простые формы

- `internal/store/monitoring.go`, `internal/httpapi/monitoring.go`, `internal/nodeagent/telemetry.go` — сохраняемый cadence5, SSE hub, отдельный agent lifecycle; тесты рядом.
- `internal/httpapi/connection_defaults.go`, `frontend/src/features/connections/ConnectionForms.tsx`, `frontend/src/shared/ui.tsx` — trusted defaults, composition/Advanced, ручные drafts.
- `frontend/src/features/dashboard/Monitoring.tsx`, `frontend/src/app/Layout.tsx` — одна подписка, fallback/offline/online/watchdog.
- `tests/monitoring_e2e.mjs`, `tests/run-monitoring-browser.sh` — UI/API fixtures, не VPN.
- [ADR-007](../adr/007-live-monitoring-and-simple-connection-forms.md), [результаты](../testing/local-results/stage5-2026-11-05-live-monitoring.md), [пакеты/устройства/каскады: остаток](../planning/simple-operation-and-user-packages-2026-11-05.md).

## 2026-11-05: дашборд и локальный recovery

[Релиз](../release-0.7.0-stage5-dashboard-recovery.md), [результаты](../testing/local-results/stage5-2026-11-05-dashboard-recovery.md), [ADR-006](../adr/006-bounded-dashboard-and-local-recovery.md).

- `internal/metrics/sampler.go`, `internal/store/{telemetry,user_counts}.go`, `internal/httpapi/dashboard.go`, `internal/nodeagent/telemetry.go` — bounded resources и отдельные состояния, не VPN proof.
- `frontend/src/pages/Overview.tsx`, `frontend/src/features/dashboard/{model.ts,TrafficChart.tsx}` — адаптивный дашборд, история/поиск/fleet.
- `cmd/arcvpn/recovery.go`, `internal/store/recovery.go`, `deploy/arcvpn-menu.sh` — safe settings/password/session CLI; [инструкция](../ops/local-admin-recovery.md).
- Тесты рядом с пакетами, `frontend/tests/dashboard.test.ts`, `tests/dashboard_e2e.mjs`; настоящий локальный API, синтетические reports, не реальные VPS.

## 2026-11-05: профили трафика, backup v5 и диагностика SNI

Актуальное продолжение: [объём](../release-0.7.0-stage5-profiles-and-targets.md), [результаты](../testing/local-results/stage5-2026-11-05-profiles-targets.md), [ADR-005](../adr/005-traffic-profiles-and-target-diagnostics.md).

- `internal/routingprofile/profile.go`, `internal/store/routing_profiles.go`, `internal/app/routing_profiles.go`, `internal/httpapi/routing_profiles.go` — отдельные traffic profiles, CAS, pure model, импорт/экспорт без runtime.
- `frontend/src/pages/Routing.tsx`, `frontend/src/features/routing-profiles/{model.ts,RoutingProfiles.tsx}`, `frontend/src/shared/Dialog.tsx` — базовый список/мобильный редактор; `frontend/src/features/routing/RoutePaths.tsx` — прежний граф/форма физических путей, отдельно lazy.
- `internal/publicnet/publicnet.go`, `internal/targetcheck/{check.go,catalog.go}`, `internal/store/target_checks.go`, `internal/httpapi/target_checks.go`, `internal/nodeagent/target_checks.go` — SSRF boundary, typed ограниченные TLS/HTTP пробы, signed feed/expiry/anti-replay; это не VPN proof.
- `frontend/src/features/targets/{model.ts,TargetPicker.tsx}`, `frontend/src/pages/{VPN,Nodes}.tsx` — подбор для нового подключения и per-node диагностика; старый SNI не меняется автоматически.
- `internal/backup/backup.go`, `internal/app/backup.go`, `internal/httpapi/portable.go`, `frontend/src/pages/Backup.tsx` — backup v5 с профилями, legacy preservation и preview.
- `tests/traffic_profiles_e2e.mjs`, дополненные `tests/routing_e2e.mjs`, `frontend/tests/traffic-profiles.test.ts` и тесты рядом с Go пакетами — реальные локальные проверки.
- [Клиентский failover](../features/client-auto-switching.md) пока только контракт: не путать с существующей симуляцией `internal/routes`.

## План дальнейшего UX и автоматизации — 2026-11-05

Уточнение установки: [одна команда, домен по желанию, IP TLS 160 часов и журналы](../planning/ux-and-safe-automation-2026-11-05.md#уточнение-установка-одной-командой-необязательный-домен-и-журналы); требования `SYS-04 / SEC-04 / OPS-04`. Реальные существующие точки: `install.sh` (Certbot/timer/deploy-hook/namespace), `cmd/arcvpn/journal_helper.go`, `internal/logging/rotate.go`, `internal/store/settings.go`. Требования шире текущей реализации и не закрыты внешней приёмкой.

[Единый план](../planning/ux-and-safe-automation-2026-11-05.md): простые мастера, расширенные формы текущих функций, SNI/target каталог и per-node TCP/TLS проверка, полный lifecycle нод из UI, Geo-импорт, мобильный UX и измеримое сравнение с 3x-ui. Это требования, не новые программные пакеты или результаты VPN-проверок.

## Продолжение stage5: редактор и backup v4 (2026-11-05)

- `frontend/src/features/routing/{model.ts,RouteCanvas.tsx}`, `frontend/src/pages/Routing.tsx` — единый черновик, интерактивные стрелки, форма, таблица; не runtime.
- `internal/app/routes.go`, `internal/store/routes_integrity_test.go` — сериализация с restore, зависимые ноды, гонки удаления/сохранения и CAS.
- `internal/backup/backup_v4_test.go`, `internal/app/stage2_test.go` — шифрованный v4 и восстановление/rollback маршрутов.
- `frontend/package-lock.json`, `frontend/tests/routing.test.ts`, `tests/{routing_e2e.mjs,run-routing-browser.sh}` — воспроизводимая сборка и проверки.
- [Точный объём](../release-0.7.0-stage5-routing-ui.md), [ADR-004](../adr/004-draft-routing-editor-and-backup.md), [фактические результаты](../testing/local-results/stage5-2026-11-05-routing-ui.md).

> **Прочитать перед расширением протоколов:** `docs/planning/protocol-scope-freeze-2026-11-05.md`. Этап 6 отложен по решению пользователя; текущих пяти профилей достаточно.

# Текущая карта исходников — 0.7.0-stage5-foundation

Читать `current-task.md`, затем `docs/release-0.7.0-stage5-foundation.md`, `docs/planning/stage5-checklist.md`, `docs/testing/stage5-codex-handoff.md`. Новые реальные файлы: `internal/routes/{policy,diff,simulate}.go`, `internal/store/routes.go`, `internal/httpapi/routes.go`, `frontend/src/pages/Routing.tsx`, `scripts/verify-stage5-source.sh`. Данные draft-only; не путать с работающим Xray каскадом. Статус этапа 4 остаётся внешне неподтверждённым.

---

# Текущая карта исходников: ArcVPN 0.6.5-stage4

Сначала [current-task](current-task.md) → [аудит 0.6.5](../release-0.6.5-stage4-source-audit.md) → [матрица требований](../planning/stage4-traceability-0.6.5.md) → [фактические тесты](../testing/local-results/stage4-2026-11-04-0.6.5.md) → [VPS handoff](../testing/stage4-codex-handoff.md). Статус: `SOURCE_CHECKS_PASS`, не `LOCAL_PASS`/`ACCEPTED`. Старые названия версий в документе ниже — история.

---

## Актуализация 0.6.4-stage4

Первичная точка чтения: `docs/ai/current-task.md` → `docs/release-0.6.4-stage4-security-and-acceptance.md` → `docs/testing/local-results/stage4-2026-11-04-0.6.4.md` → `docs/planning/stage4-checklist.md`. Новые файлы: `internal/nodeprotocol/{origin.go,origin_test.go}`, `internal/nodeagent/credentials_security_test.go`, `internal/nodebootstrap/archive_security_test.go`; изменены `runtime.go` и `runtime_test.go`, `install.go`, `updater.go`, `Nodes.tsx`. Предыдущие status links 0.6.3 ниже исторические.

# Карта проекта для ИИ

## Начинай отсюда

1. `docs/ai/current-task.md` — текущий статус 0.6.4 (этап 4, аудит безопасности пройден, фронтенд и VPS не проверены) и конкретный следующий блок.
2. `docs/audits/2026-11-04-pre-stage4.md` — сверка архива и исходников перед этапом 4.
3. `docs/release-0.6.3-stage4-continuation.md`, `docs/ops/stage4-nodes.md` — фактический code handoff и безопасная эксплуатация; `docs/planning/stage4-checklist.md` — остаток задач; `docs/planning/roadmap.md` и `docs/planning/execution-boundaries.md` — этапы и границы.
4. `docs/implementation-status.md` — что действительно подтверждено, а что остаётся внешним gate.
5. `docs/product/requirements.md` — полный целевой scope.
6. `docs/architecture/overview.md` — целевая архитектура.
7. `docs/design/` — согласованная дизайн-система, примеры 3x-ui.
8. `docs/security/threat-model.md` — модель угроз и обязательные ограничения.

## Текущий код (не путать с планируемыми пакетами)

- Backend: `cmd/arcvpn`, `internal/{db,store,auth,xray,app,httpapi,metrics,backup,nodeagent,logging,qrcode}`.
- `web/` — только legacy UI для локальной диагностики; не считать production UI.
- Единственный целевой React UI: `frontend/src/{app,shared,pages}`; временные build assets `web/react-dist/` компилируются в бинарник по tag `arcvpn_embed_react` и затем удаляются сборочным скриптом. Настоящий `frontend/dist` пока не получен.
- Установщик: `install.sh`, `deploy/arcvpn-menu.sh`; сборка `scripts/build-release.sh`, production assets `web/react_embedded.go` и `web/react_unavailable.go`.
- Старый бинарник `dist/arcvpn-linux-amd64` в исходной копии — только developer artifact; новый production binary появится **только после** `scripts/build-release.sh`. Browser E2E: `tests/react_e2e.py`, CI workflow: `.github/workflows/ci.yml`.

Изменения документации вносить в текущем рабочем блоке с кодом. Указывай запущенные тесты и не утверждай, что неподтверждённый протокол или установка реально работают.

## Передача работы и проверка

- `docs/testing/codex-handoff.md` — пошаговая внешняя приёмка.
- `docs/testing/external-results/` — только фактические протоколы проверки, никакого «PASS» без запуска.
- `docs/features/backup-and-migration.md` — ArcVPN → ArcVPN, без импорта из 3x-ui.
- `docs/planning/acceptance.md` — единые IDs сценариев.

При появлении нового пакета обновлять эту карту путей.

## Модули, добавленные для этапа 2

- `internal/protocols/protocols.go`, `render.go` — генераторы протоколов, ключей, URI, client `.conf`, ограничения.
- `internal/store/protocols.go`, `internal/store/store.go` — SQL миграции профилей и отдельных пользователей.
- `internal/app/protocols.go`, `protocol_health.go` — use-cases, жизненный цикл, локальный health.
- `internal/engines/manager.go`, `quick.go` — конфиги демонов, whitelisted helper, защищённый root запуск WG/AWG.
- `internal/httpapi/protocols.go` — REST, экспорт/QR, auth.
- `internal/backup/backup.go` — backup v2, обратная совместимость v1.
- `frontend/src/pages/VPN.tsx`, `Users.tsx`, `Backup.tsx` — React UI реальных API.
- `deploy/arcvpn-tunnel-net.sh` — фиксированные root NAT/FORWARD команды, `install.sh` — systemd конфигурация.
- `docs/ops/protocol-runtime.md` и `docs/testing/stage2-codex-handoff.md` — эксплуатация и проверка.

### Обновление 0.4.2
- `scripts/build-amneziawg.sh` — сборка pinned upstream AWG3.1 + исходники и notice.
- `scripts/build-release.sh`, `install.sh` — релизные checksums/предустановка AWG binaries.
- `internal/engines/amnezia_userspace.go` — прямой запуск AWG userspace под root, без `wg-quick up` и arbitrary hooks.
- `docs/release-0.4.2-stage2-source.md` — фактические проверки и ограничения.

### Дополнение 0.4.4

- `docs/release-0.4.4-stage2-code-handoff.md` — первичный статус завершения локальной Go-интеграции и передачи в Codex.
- `internal/app/protocol_rollback_test.go` — регрессия отказа вспомогательного движка после обновления Xray.
- `scripts/verify-stage2-local.sh` — один повторяемый источник результатов локальных Go/Bash проверок.

### Дополнение 0.5.1
- `internal/firewall/`, `internal/httpapi/firewall.go`, `internal/ops/firewall.go` — управление собственными правилами UFW через root helper.
- `internal/updater/`, `internal/httpapi/update.go`, `internal/ops/update.go` — подписанные обновления и rollback.
- `internal/backup/backup.go`, `internal/store/portable_nodes.go`, `internal/app/backup.go` — portable encrypted snapshot v3.
- `docs/ops/signed-upgrades.md`, `docs/release-0.5.1-stage3-code-handoff.md`, `docs/testing/stage3-codex-handoff.md` — актуальное руководство и приемка.

## Этап 4 — текущие исходники 0.6.3, локальная Go-проверка

- `internal/store/node_{control,profile,bootstrap}.go`, `internal/store/nodes.go` — SQL revisions/operations/enrollment/reality/credentials/bootstrap.
- `internal/httpapi/node_{control,bootstrap,links,removal,upgrade}.go`, `internal/httpapi/nodes.go` — authenticated controller API, privileged SSH bootstrap, agent poll/ACK и админские direct links.
- `internal/nodebootstrap/{ssh,install,local,lifecycle,upgrade,version}.go` — host pinning, SSRF policy, preflight, signed release и ограниченный SSH installer.
- `internal/nodeagent/agent.go` — outbound HTTPS, one-time register stdin, polling, credential rotation.
- `internal/noderuntime/runtime.go`, `cmd/arcvpn/node_helper.go` — allowlisted Xray root-helper, atomic apply/rollback/receipt.
- `frontend/src/pages/Nodes.tsx` — SSH wizard, ноды в таблице/схеме, операции и тестовые ссылки. **React build не подтверждён**.
- `scripts/build-stage4-node-bundle.sh`, `docs/ops/stage4-nodes.md`, `docs/release-0.6.0-stage4-source.md`, `docs/testing/stage4-codex-handoff.md` — подписанный релиз и воспроизводимый handoff.

- `internal/store/node_drain_test.go`, `internal/httpapi/node_drain_test.go`, `internal/nodebootstrap/stage4_security_test.go` — миграция drain/CSRF/подписанные root SHA/скрипты readback; `tests/react_e2e.py` — исправленный сценарий UI, **не запускавшийся в браузере**.
- `docs/release-0.6.2-stage4-continuation.md`, `docs/testing/local-results/stage4-2026-11-04-0.6.2.md` — новая точка передачи; 0.6.1 — история.

**Не выдавать код за production:** npm lockfile/build, E2E, VPS и VPN handshake не выполнены; CLI bootstrap/safe uninstall/rebind/upgrade реализованы на уровне исходников, но не проверены на VM/VPS, stage4 частично закончен. Сверяться с `docs/release-0.6.2-stage4-continuation.md`, `docs/planning/stage4-checklist.md` и `docs/testing/local-results/stage4-2026-11-04-0.6.2.md`.


### Продолжение 0.6.3 — внешний data-plane probe

`internal/nodeprobe/` — узкий Xray VLESS/REALITY SOCKS5/HTTPS 204 клиент без direct fallback; `cmd/arcvpn/node_probe.go` — независимый CLI с приватными файлами и одноразовым grant; `internal/store/node_probe.go` — SQLite grant/revision/digest TTL/migration; `internal/httpapi/node_probe.go` — admin+CSRF grant и bearer отчёт; `frontend/src/pages/Nodes.tsx` — UI probe и раздельные статусы. Источник истины: [release 0.6.3](../release-0.6.3-stage4-continuation.md), [тесты](../testing/local-results/stage4-2026-11-04-0.6.3.md), [текущая задача](current-task.md). CLI без реального Xray/network test не доказывает доступность VPN.
## 2026-11-05: отдельные доступы устройств

- `internal/devices/devices.go`, `internal/store/devices.go` — credential identity, лимиты/CAS, private storage/token index, сохранение отзывов при restore.
- `internal/app/devices.go`, `internal/xray/{xray,statistics}.go` — apply/retry, отключение общего ключа, агрегация aliases в пользовательский расход.
- `internal/httpapi/devices.go`, `frontend/src/features/users/DevicesDialog.tsx` — session/CSRF API, индивидуальная подписка, UI-композиция и advanced limit.
- `internal/backup/backup_v7_test.go`, тесты рядом с модулями, `tests/devices_e2e.mjs` — encryption/restore/API/browser checks. Stand запускается opt-in тестом `TestDevicesBrowserFixture`.
- Контракт: [ADR-009](../adr/009-device-credentials.md). Это локальные Xray credentials, не физический HWID и не VPS-приёмка.
## 2026-11-05: Hysteria2 accounting и устройства

- `internal/protocols/hysteria.go` — единый namespace auth IDs, generated JSON и URI устройств/legacy.
- `internal/engines/hysteria_stats.go` — private key/config, systemd process epoch, проверка владельца loopback listener, bounded non-resetting HTTP collector.
- `internal/store/{usage,usage_backup,devices}.go`, `internal/app/{usage,protocols,devices}.go` — общий пакет/независимая freshness/coverage/apply/retry.
- `internal/backup/backup_v8_test.go`, новые Hysteria tests в engines/protocols/store/httpapi, opt-in настоящий QUIC fixture `TestRealHysteriaAccountingAndRevokedDevice`.
- [ADR-010](../adr/010-hysteria-traffic-and-devices.md); WG/AWG/remote accounting не закрыты.
## WireGuard: отдельные peers и общий пакет

- `internal/devices/wireguard.go`, `internal/store/wireguard_devices.go` — keypairs, pool validation/allocation/migration и restore preservation.
- `internal/protocols/wireguard.go`, `internal/app/devices.go`, `internal/httpapi/devices.go` — allowlist, device `.conf` export, UI `DevicesDialog.tsx`.
- `internal/engines/wireguard_stats.go`, `quick.go`, `cmd/arcvpn/main.go` — root receipt, fixed stats RPC, bounded public transfer collector.
- `internal/store/usage.go`, `internal/app/usage.go`, `internal/backup/backup.go` — per-source freshness, общий пакет, backup v9; тесты рядом.
- [ADR-011](../adr/011-wireguard-traffic-and-devices.md). AmneziaWG исключён пользователем из следующего объёма; runtime VPS acceptance остаётся открытым.
## Публичный вход и упаковка

`README.md`, `LICENSE`, `docs/ops/install-and-update.md` — актуальный пользовательский вход, MIT, clean install/signed update и явные ограничения. `docs/planning/short-work-blocks.md` — обязательный процесс коротких задач/достаточных проверок. `scripts/package-release.sh` — явный список artifacts и установочный tar.gz; `tests/release_packaging.sh` — быстрый макет целостности/исключения секретов, не настоящий build/установка. `build-release.sh` по умолчанию 0.7 без обязательной сборки AmneziaWG, installer определяет legacy extras по manifest.
