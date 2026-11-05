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
